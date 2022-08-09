package statemachine

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/canonical/ubuntu-image/internal/imagedefinition"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/partition"
	"github.com/diskfs/go-diskfs/partition/gpt"
	"github.com/diskfs/go-diskfs/partition/mbr"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/gadget/quantity"
	"github.com/snapcore/snapd/seed"
	"github.com/snapcore/snapd/timings"
)

// validateInput ensures that command line flags for the state machine are valid. These
// flags are applicable to all image types
func (stateMachine *StateMachine) validateInput() error {
	// Validate command line options
	if stateMachine.stateMachineFlags.Thru != "" && stateMachine.stateMachineFlags.Until != "" {
		return fmt.Errorf("cannot specify both --until and --thru")
	}
	if stateMachine.stateMachineFlags.WorkDir == "" && stateMachine.stateMachineFlags.Resume {
		return fmt.Errorf("must specify workdir when using --resume flag")
	}

	logLevelFlags := []bool{stateMachine.commonFlags.Debug,
		stateMachine.commonFlags.Verbose,
		stateMachine.commonFlags.Quiet,
	}

	logLevels := 0
	for _, logLevelFlag := range logLevelFlags {
		if logLevelFlag {
			logLevels++
		}
	}

	if logLevels > 1 {
		return fmt.Errorf("--quiet, --verbose, and --debug flags are mutually exclusive")
	}

	return nil
}

// validateUntilThru validates that the the state passed as --until
// or --thru exists in the state machine's list of states
func (stateMachine *StateMachine) validateUntilThru() error {
	// if --until or --thru was given, make sure the specified state exists
	var searchState string
	var stateFound bool = false
	if stateMachine.stateMachineFlags.Until != "" {
		searchState = stateMachine.stateMachineFlags.Until
	}
	if stateMachine.stateMachineFlags.Thru != "" {
		searchState = stateMachine.stateMachineFlags.Thru
	}

	if searchState != "" {
		for _, state := range stateMachine.states {
			if state.name == searchState {
				stateFound = true
				break
			}
		}
		if !stateFound {
			return fmt.Errorf("state %s is not a valid state name", searchState)
		}
	}

	return nil
}

// cleanup cleans the workdir. For now this is just deleting the temporary directory if necessary
// but will have more functionality added to it later
func (stateMachine *StateMachine) cleanup() error {
	if stateMachine.cleanWorkDir {
		if err := osRemoveAll(stateMachine.stateMachineFlags.WorkDir); err != nil {
			return fmt.Errorf("Error cleaning up workDir: %s", err.Error())
		}
	}
	return nil
}

// runHooks reads through the --hooks-directory flags and calls a helper function to execute the scripts
func (stateMachine *StateMachine) runHooks(hookName, envKey, envVal string) error {
	os.Setenv(envKey, envVal)
	for _, hooksDir := range stateMachine.commonFlags.HooksDirectories {
		hooksDirectoryd := filepath.Join(hooksDir, hookName+".d")
		hookScripts, err := ioutilReadDir(hooksDirectoryd)

		// It's okay for hooks-directory.d to not exist, but if it does exist run all the scripts in it
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("Error reading hooks directory: %s", err.Error())
		}

		for _, hookScript := range hookScripts {
			hookScriptPath := filepath.Join(hooksDirectoryd, hookScript.Name())
			if stateMachine.commonFlags.Debug || stateMachine.commonFlags.Verbose {
				fmt.Printf("Running hook script: %s\n", hookScriptPath)
			}
			if err := helper.RunScript(hookScriptPath); err != nil {
				return fmt.Errorf("Error running hook %s: %s", hookScriptPath, err.Error())
			}
		}

		// if hookName exists in the hook directory, run it
		hookScript := filepath.Join(hooksDir, hookName)
		_, err = os.Stat(hookScript)
		if err == nil {
			if stateMachine.commonFlags.Debug || stateMachine.commonFlags.Verbose {
				fmt.Printf("Running hook script: %s\n", hookScript)
			}
			if err := helper.RunScript(hookScript); err != nil {
				return fmt.Errorf("Error running hook %s: %s", hookScript, err.Error())
			}
		}
	}
	return nil
}

// handleLkBootloader handles the special "lk" bootloader case where some extra
// files need to be added to the bootfs
func (stateMachine *StateMachine) handleLkBootloader(volume *gadget.Volume) error {
	if volume.Bootloader != "lk" {
		return nil
	}
	// For the LK bootloader we need to copy boot.img and snapbootsel.bin to
	// the gadget folder so they can be used as partition content. The first
	// one comes from the kernel snap, while the second one is modified by
	// the prepare_image step to set the right core and kernel for the kernel
	// command line.
	bootDir := filepath.Join(stateMachine.tempDirs.unpack,
		"image", "boot", "lk")
	gadgetDir := filepath.Join(stateMachine.tempDirs.unpack, "gadget")
	if _, err := os.Stat(bootDir); err != nil {
		return fmt.Errorf("got lk bootloader but directory %s does not exist", bootDir)
	}
	err := osMkdir(gadgetDir, 0755)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("Failed to create gadget dir: %s", err.Error())
	}
	files, err := ioutilReadDir(bootDir)
	if err != nil {
		return fmt.Errorf("Error reading lk bootloader dir: %s", err.Error())
	}
	for _, lkFile := range files {
		srcFile := filepath.Join(bootDir, lkFile.Name())
		if err := osutilCopySpecialFile(srcFile, gadgetDir); err != nil {
			return fmt.Errorf("Error copying lk bootloader dir: %s", err.Error())
		}
	}
	return nil
}

// shouldSkipStructure returns whether a structure should be skipped during certain processing
func shouldSkipStructure(structure gadget.VolumeStructure, isSeeded bool) bool {
	if isSeeded &&
		(structure.Role == gadget.SystemBoot ||
			structure.Role == gadget.SystemData ||
			structure.Role == gadget.SystemSave ||
			structure.Label == gadget.SystemBoot) {
		return true
	}
	return false
}

// copyStructureContent handles copying raw blobs or creating formatted filesystems
func (stateMachine *StateMachine) copyStructureContent(volume *gadget.Volume,
	structure gadget.VolumeStructure, structureNumber int,
	contentRoot, partImg string) error {
	if structure.Filesystem == "" {
		// copy the contents to the new location
		// first zero it out. Structures without filesystem specified in the gadget
		// yaml must have the size specified, so the bs= argument below is valid
		ddArgs := []string{"if=/dev/zero", "of=" + partImg, "count=0",
			"bs=" + strconv.FormatUint(uint64(structure.Size), 10),
			"seek=1"}
		if err := helperCopyBlob(ddArgs); err != nil {
			return fmt.Errorf("Error zeroing partition: %s",
				err.Error())
		}
		var runningOffset quantity.Offset = 0
		for _, content := range structure.Content {
			if content.Offset != nil {
				runningOffset = *content.Offset
			}
			// now copy the raw content file specified in gadget.yaml
			inFile := filepath.Join(stateMachine.tempDirs.unpack,
				"gadget", content.Image)
			ddArgs = []string{"if=" + inFile, "of=" + partImg, "bs=" + mockableBlockSize,
				"seek=" + strconv.FormatUint(uint64(runningOffset), 10),
				"conv=sparse,notrunc"}
			if err := helperCopyBlob(ddArgs); err != nil {
				return fmt.Errorf("Error copying image blob: %s",
					err.Error())
			}
			runningOffset += quantity.Offset(content.Size)
		}
	} else {
		var blockSize quantity.Size
		if structure.Role == gadget.SystemData || structure.Role == gadget.SystemSeed {
			// system-data and system-seed structures are not required to have
			// an explicit size set in the yaml file
			if structure.Size < stateMachine.RootfsSize {
				if !stateMachine.commonFlags.Quiet {
					fmt.Printf("WARNING: rootfs structure size %s smaller "+
						"than actual rootfs contents %s\n",
						structure.Size.IECString(),
						stateMachine.RootfsSize.IECString())
				}
				blockSize = stateMachine.RootfsSize
				structure.Size = stateMachine.RootfsSize
				volume.Structure[structureNumber] = structure
			} else {
				blockSize = structure.Size
			}
		} else {
			blockSize = structure.Size
		}
		if structure.Role == gadget.SystemData {
			os.Create(partImg)
			os.Truncate(partImg, int64(stateMachine.RootfsSize))
		} else {
			// zero out the .img file
			ddArgs := []string{"if=/dev/zero", "of=" + partImg, "count=0",
				"bs=" + strconv.FormatUint(uint64(blockSize), 10), "seek=1"}
			if err := helperCopyBlob(ddArgs); err != nil {
				return fmt.Errorf("Error zeroing image file %s: %s",
					partImg, err.Error())
			}
		}
		// use mkfs functions from snapd to create the filesystems
		if structure.Content == nil {
			err := mkfsMake(structure.Filesystem, partImg, structure.Label,
				structure.Size, stateMachine.SectorSize)
			if err != nil {
				return fmt.Errorf("Error running mkfs: %s", err.Error())
			}
		} else {
			err := mkfsMakeWithContent(structure.Filesystem, partImg, structure.Label,
				contentRoot, structure.Size, stateMachine.SectorSize)
			if err != nil {
				return fmt.Errorf("Error running mkfs with content: %s", err.Error())
			}
		}
	}
	return nil
}

// handleSecureBoot handles a special case where files need to be moved from /boot/ to
// /EFI/ubuntu/ so that SecureBoot can still be used
func (stateMachine *StateMachine) handleSecureBoot(volume *gadget.Volume, targetDir string) error {
	var bootDir, ubuntuDir string
	if volume.Bootloader == "u-boot" {
		bootDir = filepath.Join(stateMachine.tempDirs.unpack,
			"image", "boot", "uboot")
		ubuntuDir = targetDir
	} else if volume.Bootloader == "piboot" {
		bootDir = filepath.Join(stateMachine.tempDirs.unpack,
			"image", "boot", "piboot")
		ubuntuDir = targetDir
	} else if volume.Bootloader == "grub" {
		bootDir = filepath.Join(stateMachine.tempDirs.unpack,
			"image", "boot", "grub")
		ubuntuDir = filepath.Join(targetDir, "EFI", "ubuntu")
	}

	if _, err := os.Stat(bootDir); err != nil {
		// this won't always exist, and that's fine
		return nil
	}

	// copy the files from bootDir to ubuntuDir
	if err := osMkdirAll(ubuntuDir, 0755); err != nil {
		return fmt.Errorf("Error creating ubuntu dir: %s", err.Error())
	}

	files, err := ioutilReadDir(bootDir)
	if err != nil {
		return fmt.Errorf("Error reading boot dir: %s", err.Error())
	}
	for _, bootFile := range files {
		srcFile := filepath.Join(bootDir, bootFile.Name())
		dstFile := filepath.Join(ubuntuDir, bootFile.Name())
		if err := osRename(srcFile, dstFile); err != nil {
			return fmt.Errorf("Error copying boot dir: %s", err.Error())
		}
	}

	return nil
}

// WriteSnapManifest generates a snap manifest based on the contents of the selected snapsDir
func WriteSnapManifest(snapsDir string, outputPath string) error {
	files, err := ioutilReadDir(snapsDir)
	if err != nil {
		// As per previous ubuntu-image manifest generation, we skip generating
		// manifests for non-existent/invalid paths
		return nil
	}

	manifest, err := osCreate(outputPath)
	if err != nil {
		return fmt.Errorf("Error creating manifest file: %s", err.Error())
	}
	defer manifest.Close()

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".snap") {
			split := strings.SplitN(file.Name(), "_", 2)
			fmt.Fprintf(manifest, "%s %s\n", split[0], split[1])
		}
	}
	return nil
}

// getHostArch uses dpkg to return the host architecture of the current system
func getHostArch() string {
	cmd := exec.Command("dpkg", "--print-architecture")
	outputBytes, _ := cmd.Output()
	return strings.TrimSpace(string(outputBytes))
}

// getHostSuite checks the release name of the host system to use as a default if --suite is not passed
func getHostSuite() string {
	cmd := exec.Command("lsb_release", "-c", "-s")
	outputBytes, _ := cmd.Output()
	return strings.TrimSpace(string(outputBytes))
}

// getQemuStaticForArch returns the name of the qemu binary for the specified arch
func getQemuStaticForArch(arch string) string {
	archs := map[string]string{
		"armhf":   "qemu-arm-static",
		"arm64":   "qemu-aarch64-static",
		"ppc64el": "qemu-ppc64le-static",
	}
	if static, exists := archs[arch]; exists {
		return static
	}
	return ""
}

// maxOffset returns the maximum of two quantity.Offset types
func maxOffset(offset1, offset2 quantity.Offset) quantity.Offset {
	if offset1 > offset2 {
		return offset1
	}
	return offset2
}

// createPartitionTable creates a disk image file and writes the partition table to it
func createPartitionTable(volumeName string, volume *gadget.Volume, sectorSize uint64, isSeeded bool) *partition.Table {
	var gptPartitions = make([]*gpt.Partition, 0)
	var mbrPartitions = make([]*mbr.Partition, 0)
	var partitionTable partition.Table

	for _, structure := range volume.Structure {
		if structure.Role == "mbr" || structure.Type == "bare" ||
			shouldSkipStructure(structure, isSeeded) {
			continue
		}

		var structureType string
		// Check for hybrid MBR/GPT
		if strings.Contains(structure.Type, ",") {
			types := strings.Split(structure.Type, ",")
			if volume.Schema == "gpt" {
				structureType = types[1]
			} else {
				structureType = types[0]
			}
		} else {
			structureType = structure.Type
		}

		if volume.Schema == "mbr" {
			bootable := false
			if structure.Role == gadget.SystemBoot || structure.Label == gadget.SystemBoot {
				bootable = true
			}
			// mbr.Type is a byte. snapd has already verified that this string
			// is exactly two chars, so we can parse those two chars to a byte
			partitionType, _ := strconv.ParseUint(structureType, 16, 8)
			mbrPartition := &mbr.Partition{
				Start:    uint32(math.Ceil(float64(*structure.Offset) / float64(sectorSize))),
				Size:     uint32(math.Ceil(float64(structure.Size) / float64(sectorSize))),
				Type:     mbr.Type(partitionType),
				Bootable: bootable,
			}
			mbrPartitions = append(mbrPartitions, mbrPartition)
		} else {
			var partitionName string
			if structure.Role == "system-data" && structure.Name == "" {
				partitionName = "writable"
			} else {
				partitionName = structure.Name
			}

			partitionType := gpt.Type(structureType)
			gptPartition := &gpt.Partition{
				Start: uint64(math.Ceil(float64(*structure.Offset) / float64(sectorSize))),
				Size:  uint64(structure.Size),
				Type:  partitionType,
				Name:  partitionName,
			}
			gptPartitions = append(gptPartitions, gptPartition)
		}
	}

	if volume.Schema == "mbr" {
		mbrTable := &mbr.Table{
			Partitions:         mbrPartitions,
			LogicalSectorSize:  int(sectorSize),
			PhysicalSectorSize: int(sectorSize),
		}
		partitionTable = mbrTable
	} else {
		gptTable := &gpt.Table{
			Partitions:         gptPartitions,
			LogicalSectorSize:  int(sectorSize),
			PhysicalSectorSize: int(sectorSize),
			ProtectiveMBR:      true,
		}
		partitionTable = gptTable
	}
	return &partitionTable
}

// calculateImageSize calculates the total sum of all partition sizes in an image
func (stateMachine *StateMachine) calculateImageSize() (quantity.Size, error) {
	if stateMachine.GadgetInfo == nil {
		return 0, fmt.Errorf("Cannot calculate image size before initializing GadgetInfo")
	}
	var imgSize quantity.Size = 0
	for _, volume := range stateMachine.GadgetInfo.Volumes {
		for _, structure := range volume.Structure {
			imgSize += structure.Size
		}
	}
	return imgSize, nil
}

// copyDataToImage runs dd commands to copy the raw data to the final image with appropriate offsets
func (stateMachine *StateMachine) copyDataToImage(volumeName string, volume *gadget.Volume, diskImg *disk.Disk) error {
	for structureNumber, structure := range volume.Structure {
		if shouldSkipStructure(structure, stateMachine.IsSeeded) {
			continue
		}
		sectorSize := diskImg.LogicalBlocksize
		// set up the arguments to dd the structures into an image
		partImg := filepath.Join(stateMachine.tempDirs.volumes, volumeName,
			"part"+strconv.Itoa(structureNumber)+".img")
		seek := strconv.FormatInt(int64(getStructureOffset(structure))/sectorSize, 10)
		count := strconv.FormatFloat(math.Ceil(float64(structure.Size)/float64(sectorSize)), 'f', 0, 64)
		ddArgs := []string{
			"if=" + partImg,
			"of=" + diskImg.File.Name(),
			"bs=" + strconv.FormatInt(sectorSize, 10),
			"seek=" + seek,
			"count=" + count,
			"conv=notrunc",
			"conv=sparse",
		}
		if err := helperCopyBlob(ddArgs); err != nil {
			return fmt.Errorf("Error writing disk image: %s",
				err.Error())
		}
	}
	return nil
}

// writeOffsetValues handles any OffsetWrite values present in the volume structures.
func writeOffsetValues(volume *gadget.Volume, imgName string, sectorSize, imgSize uint64) error {
	imgFile, err := osOpenFile(imgName, os.O_RDWR, 0755)
	if err != nil {
		return fmt.Errorf("Error opening image file to write offsets: %s", err.Error())
	}
	defer imgFile.Close()
	for _, structure := range volume.Structure {
		if structure.OffsetWrite != nil {
			offset := uint64(*structure.Offset) / sectorSize
			if imgSize-4 < offset {
				return fmt.Errorf("write offset beyond end of file")
			}
			offsetBytes := make([]byte, 4)
			binary.LittleEndian.PutUint32(offsetBytes, uint32(offset))
			_, err := imgFile.WriteAt(offsetBytes, int64(structure.OffsetWrite.Offset))
			if err != nil {
				return fmt.Errorf("Failed to write offset to disk at %d: %s",
					structure.OffsetWrite.Offset, err.Error())
			}
		}
	}
	return nil
}

// getStructureOffset returns 0 if structure.Offset is nil, otherwise the value stored there
func getStructureOffset(structure gadget.VolumeStructure) quantity.Offset {
	if structure.Offset == nil {
		return 0
	}
	return *structure.Offset
}

// generateUniqueDiskID returns a random 4-byte long disk ID, unique per the list of existing IDs
func generateUniqueDiskID(existing *[][]byte) ([]byte, error) {
	var retry bool
	randomBytes := make([]byte, 4)
	// we'll try 10 times, not to loop into infinity in case the RNG is broken (no entropy?)
	for i := 0; i < 10; i++ {
		retry = false
		_, err := randRead(randomBytes)
		if err != nil {
			retry = true
			continue
		}
		for _, id := range *existing {
			if bytes.Compare(randomBytes, id) == 0 {
				retry = true
				break
			}
		}

		if !retry {
			break
		}
	}
	if retry {
		// this means for some weird reason we didn't get an unique ID after many retries
		return nil, fmt.Errorf("Failed to generate unique disk ID. Random generator failure?")
	}
	*existing = append(*existing, randomBytes)
	return randomBytes, nil
}

// parseSnapsAndChannels converts the command line arguments to a format that is expected
// by snapd's image.Prepare()
func parseSnapsAndChannels(snaps []string) (snapNames []string, snapChannels map[string]string, err error) {
	snapNames = make([]string, len(snaps))
	snapChannels = make(map[string]string)
	for ii, snap := range snaps {
		if strings.Contains(snap, "=") {
			splitSnap := strings.Split(snap, "=")
			if len(splitSnap) != 2 {
				return snapNames, snapChannels,
					fmt.Errorf("Invalid syntax passed to --snap: %s. "+
						"Argument must be in the form --snap=name or "+
						"--snap=name=channel", snap)
			}
			snapNames[ii] = splitSnap[0]
			snapChannels[splitSnap[0]] = splitSnap[1]
		} else {
			snapNames[ii] = snap
		}
	}
	return snapNames, snapChannels, nil
}

// removePreseeding removes preseeded snaps from an existing rootfs and returns
// a slice of the snaps and their channels that were removed this way
func removePreseeding(rootfs string) (seededSnaps map[string]string, err error) {
	// seededSnaps maps the snap name and channel that was seeded
	seededSnaps = make(map[string]string)

	// open the seed and run LoadAssertions and LoadMeta to get a list of snaps
	snapdDir := filepath.Join(rootfs, "var", "lib", "snapd")
	seedDir := filepath.Join(snapdDir, "seed")
	preseed, err := seedOpen(seedDir, "")
	if err != nil {
		return seededSnaps, err
	}
	measurer := timings.New(nil)
	if err := preseed.LoadAssertions(nil, nil); err != nil {
		return seededSnaps, err
	}
	if err := preseed.LoadMeta(seed.AllModes, nil, measurer); err != nil {
		return seededSnaps, err
	}

	// iterate over the snaps in the seed and add them to the list
	preseed.Iter(func(sn *seed.Snap) error {
		seededSnaps[sn.SnapName()] = sn.Channel
		return nil
	})

	// now delete the preseeded snaps from the rootfs
	if err := osRemoveAll(snapdDir); err != nil {
		return seededSnaps, err
	}
	return seededSnaps, nil
}

// generateGerminateCmd creates the appropriate germinate command for the
// values configured in the image definition yaml file
func generateGerminateCmd(imageDefinition imagedefinition.ImageDefinition) *exec.Cmd {
	// determine the value for the seed-dist in the form of <archive>.<series>
	seedDist := imageDefinition.Rootfs.Flavor
	if imageDefinition.Rootfs.Seed.SeedBranch != "" {
		seedDist = seedDist + "." + imageDefinition.Rootfs.Seed.SeedBranch
	}

	seedSource := strings.Join(imageDefinition.Rootfs.Seed.SeedURLs, ",")

	germinateCmd := execCommand("germinate",
		"--mirror", imageDefinition.Rootfs.Mirror,
		"--arch", imageDefinition.Architecture,
		"--dist", imageDefinition.Series,
		"--seed-source", seedSource,
		"--seed-dist", seedDist,
		"--no-rdepends",
	)

	if imageDefinition.Rootfs.Seed.Vcs {
		germinateCmd.Args = append(germinateCmd.Args, "--vcs=auto")
	}

	if len(imageDefinition.Rootfs.Components) > 0 {
		components := strings.Join(imageDefinition.Rootfs.Components, ",")
		germinateCmd.Args = append(germinateCmd.Args, "--components="+components)
	}

	return germinateCmd
}

// cloneGitRepo takes options from the image definition and clones the git
// repo with the corresponding options
func cloneGitRepo(imageDefinition imagedefinition.ImageDefinition, workDir string) error {
	// clone the repo
	cloneOptions := &git.CloneOptions{
		URL:          imageDefinition.Gadget.GadgetURL,
		SingleBranch: true,
	}
	if imageDefinition.Gadget.GadgetBranch != "" {
		cloneOptions.ReferenceName = plumbing.NewBranchReferenceName(imageDefinition.Gadget.GadgetBranch)
	}

	cloneOptions.Validate()

	_, err := git.PlainClone(workDir, false, cloneOptions)
	return err
}

// generateDebootstrapCmd generates the debootstrap command used to create a chroot
// environment that will eventually become the rootfs of the resulting image
func generateDebootstrapCmd(imageDefinition imagedefinition.ImageDefinition, targetDir string, includeList []string) *exec.Cmd {
	debootstrapCmd := execCommand("debootstrap",
		"--arch", imageDefinition.Architecture,
		"--variant=minbase",
		"--include=ca-certificates", // ca-certificates is needed to use PPAs
	)

	if len(imageDefinition.Rootfs.Components) > 0 {
		components := strings.Join(imageDefinition.Rootfs.Components, ",")
		debootstrapCmd.Args = append(debootstrapCmd.Args, "--components="+components)
	}

	// add the SUITE TARGET and MIRROR arguments
	debootstrapCmd.Args = append(debootstrapCmd.Args, []string{
		imageDefinition.Series,
		targetDir,
		imageDefinition.Rootfs.Mirror,
	}...)

	return debootstrapCmd
}

// generatePocketList returns a slice of strings that need to be added to
// /etc/apt/sources.list in the chroot based on the value of "pocket"
// in the rootfs section of the image definition
func generatePocketList(imageDef imagedefinition.ImageDefinition) []string {
	pocketMap := map[string][]string{
		"release": {},
		"security": {
			fmt.Sprintf("deb http://security.ubuntu.com/ubuntu/ %s-security %s\n",
				imageDef.Series, strings.Join(imageDef.Rootfs.Components, " "),
			),
		},
		"updates": {
			fmt.Sprintf("deb http://archive.ubuntu.com/ubuntu/ %s-updates %s\n",
				imageDef.Series, strings.Join(imageDef.Rootfs.Components, " "),
			),
			fmt.Sprintf("deb http://security.ubuntu.com/ubuntu/ %s-security %s\n",
				imageDef.Series, strings.Join(imageDef.Rootfs.Components, " "),
			),
		},
		"proposed": {
			fmt.Sprintf("deb http://archive.ubuntu.com/ubuntu/ %s-updates %s\n",
				imageDef.Series, strings.Join(imageDef.Rootfs.Components, " "),
			),
			fmt.Sprintf("deb http://security.ubuntu.com/ubuntu/ %s-security %s\n",
				imageDef.Series, strings.Join(imageDef.Rootfs.Components, " "),
			),
			fmt.Sprintf("deb http://archive.ubuntu.com/ubuntu/ %s-proposed %s\n",
				imageDef.Series, strings.Join(imageDef.Rootfs.Components, " "),
			),
		},
	}

	// Schema validation has already confirmed the Pocket is a valid value
	return pocketMap[strings.ToLower(imageDef.Rootfs.Pocket)]
}

// generateAptCmd generates the apt command used to create a chroot
// environment that will eventually become the rootfs of the resulting image
func generateAptCmds(targetDir string, packageList []string) []*exec.Cmd {
	updateCmd := execCommand("chroot", targetDir, "apt", "update")

	installCmd := execCommand("chroot", targetDir, "apt", "install",
		"--assume-yes",
		"--quiet",
		"--option=Dpkg::options::=--force-unsafe-io",
		"--option=Dpkg::Options::=--force-confold",
	)

	for _, aptPackage := range packageList {
		installCmd.Args = append(installCmd.Args, aptPackage)
	}

	// Env is sometimes used for mocking command calls in tests,
	// so only overwrite env if it is nil
	if installCmd.Env == nil {
		installCmd.Env = os.Environ()
	}
	installCmd.Env = append(installCmd.Env, "DEBIAN_FRONTEND=noninteractive")

	return []*exec.Cmd{updateCmd, installCmd}
}

// createPPAInfo generates the name for a PPA sources.list file
// in the convention of add-apt-repository, and the contents
// that define the sources.list in the DEB822 format
func createPPAInfo(ppa *imagedefinition.PPA, series string) (fileName string, fileContents string) {
	splitName := strings.Split(ppa.PPAName, "/")
	user := splitName[0]
	ppaName := splitName[1]

	fileName = fmt.Sprintf("%s-ubuntu-%s-%s.sources", user, ppaName, series)

	var domain string
	if ppa.Auth == "" {
		domain = "https://ppa.launchpadcontent.net"
	} else {
		domain = fmt.Sprintf("https://%s@private-ppa.launchpadcontent.net", ppa.Auth)
	}

	fullDomain := fmt.Sprintf("%s/%s/%s/ubuntu", domain, user, ppaName)
	fileContents = fmt.Sprintf("X-Repolib-Name: %s\nEnabled: yes\nTypes: deb\n"+
		"URIS: %s\nSuites: %s\nComponents: main",
		ppa.PPAName, fullDomain, series)

	return fileName, fileContents
}

// importPPAKeys imports keys for ppas with specified fingerprints.
// The schema parsing has already validated that either Fingerprint is
// specified or the PPA is public. If no fingerprint is provided, this
// function reaches out to the Launchpad API to get the signing key
func importPPAKeys(ppa *imagedefinition.PPA, tmpGPGDir, keyFilePath string, debug bool) error {
	if ppa.Fingerprint == "" {
		// The YAML schema has already validated that if no fingerprint is
		// provided, then this is a public PPA. We will get the fingerprint
		// from the Launchpad API
		type launchpadAPI struct {
			SigningKeyFingerprint string `json:"signing_key_fingerprint"`
			// plus many other fields that aren't needed at the moment
		}
		launchpadInstance := launchpadAPI{}

		splitName := strings.Split(ppa.PPAName, "/")
		launchpadURL := fmt.Sprintf("https://api.launchpad.net/devel/~%s/+archive/ubuntu/%s",
			splitName[0], splitName[1])
		resp, err := httpGet(launchpadURL)
		if err != nil {
			return fmt.Errorf("Error getting signing key for ppa \"%s\": %s",
				ppa.PPAName, err.Error())
		}

		body, err := ioutilReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("Error reading signing key for ppa \"%s\": %s",
				ppa.PPAName, err.Error())
		}

		err = jsonUnmarshal(body, &launchpadInstance)
		if err != nil {
			return fmt.Errorf("Error unmarshalling launchpad API response: %s", err.Error())
		}

		ppa.Fingerprint = launchpadInstance.SigningKeyFingerprint
	}
	commonGPGArgs := []string{
		"--no-default-keyring",
		"--no-options",
		"--homedir",
		tmpGPGDir,
		"--secret-keyring",
		filepath.Join(tmpGPGDir, "tempring.gpg"),
		"--keyserver",
		"hkp://keyserver.ubuntu.com:80",
	}
	recvKeyArgs := append(commonGPGArgs, []string{"--recv-keys", ppa.Fingerprint}...)
	exportKeyArgs := append(commonGPGArgs, []string{"--output", keyFilePath, "--export", ppa.Fingerprint}...)
	gpgCmds := []*exec.Cmd{
		execCommand(
			"gpg",
			recvKeyArgs...,
		),
		execCommand(
			"gpg",
			exportKeyArgs...,
		),
	}

	for _, gpgCmd := range gpgCmds {
		gpgOutput := helper.SetCommandOutput(gpgCmd, debug)
		err := gpgCmd.Run()
		if err != nil {
			return fmt.Errorf("Error running gpg command \"%s\". Error is \"%s\". Full output below:\n%s",
				gpgCmd.String(), err.Error(), gpgOutput.String())
		}
	}

	return nil
}

// mountFromHost mounts mountpoints from the host system in the chroot
// for certain operations that require this
func mountFromHost(targetDir, mountpoint string) (mountCmd, umountCmd *exec.Cmd) {
	mountCmd = execCommand("mount", "--bind", mountpoint, filepath.Join(targetDir, mountpoint))
	umountCmd = execCommand("umount", filepath.Join(targetDir, mountpoint))
	return mountCmd, umountCmd
}
