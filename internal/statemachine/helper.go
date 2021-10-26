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
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/partition"
	"github.com/diskfs/go-diskfs/partition/gpt"
	"github.com/diskfs/go-diskfs/partition/mbr"

	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/gadget/quantity"
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
			if stateMachine.commonFlags.Debug {
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
			if stateMachine.commonFlags.Debug {
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
				fmt.Printf("WARNING: rootfs structure size %s smaller "+
					"than actual rootfs contents %s\n",
					structure.Size.IECString(),
					stateMachine.RootfsSize.IECString())
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
			// use mkfs functions from snapd to create the filesystems
			ddArgs := []string{"if=/dev/zero", "of=" + partImg, "count=0",
				"bs=" + strconv.FormatUint(uint64(blockSize), 10), "seek=1"}
			if err := helperCopyBlob(ddArgs); err != nil {
				return fmt.Errorf("Error zeroing image file %s: %s",
					partImg, err.Error())
			}
		}
		err := mkfsMakeWithContent(structure.Filesystem, partImg, structure.Label,
			contentRoot, structure.Size, quantity.Size(512))
		if err != nil {
			return fmt.Errorf("Error running mkfs: %s", err.Error())
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

// setupLiveBuildCommands creates the live build commands used in classic images
func setupLiveBuildCommands(rootfs, arch string, env []string, enableCrossBuild bool) (lbConfig, lbBuild exec.Cmd, err error) {

	lbConfig = *exec.Command("lb", "config")
	lbBuild = *exec.Command("lb", "build")

	lbConfig.Stdout = os.Stdout
	lbConfig.Stderr = os.Stderr
	lbConfig.Env = append(os.Environ(), env...)
	lbBuild.Stdout = os.Stdout
	lbBuild.Stderr = os.Stderr
	lbBuild.Env = append(os.Environ(), env...)

	autoSrc := os.Getenv("UBUNTU_IMAGE_LIVECD_ROOTFS_AUTO_PATH")
	if autoSrc == "" {
		dpkgArgs := "dpkg -L livecd-rootfs | grep \"auto$\""
		dpkgCommand := execCommand("bash", "-c", dpkgArgs)
		dpkgBytes, err := dpkgCommand.Output()
		if err != nil {
			return lbConfig, lbBuild, err
		}
		autoSrc = strings.TrimSpace(string(dpkgBytes))
	}
	autoDst := rootfs + "/auto"
	if err := osutilCopySpecialFile(autoSrc, autoDst); err != nil {
		return lbConfig, lbBuild, fmt.Errorf("Error copying livecd-rootfs/auto: %s", err.Error())
	}

	if arch != getHostArch() && enableCrossBuild {
		// For cases where we want to cross-build, we need to pass
		// additional options to live-build with the arch to use and path
		// to the qemu static
		var qemuPath string
		qemuPath = os.Getenv("UBUNTU_IMAGE_QEMU_USER_STATIC_PATH")
		if qemuPath == "" {
			static := getQemuStaticForArch(arch)
			qemuPath, err = exec.LookPath(static)
			if err != nil {
				return lbConfig, lbBuild, fmt.Errorf("Use " +
					"UBUNTU_IMAGE_QEMU_USER_STATIC_PATH in case " +
					"of non-standard archs or custom paths")
			}
		}
		lbConfig.Args = append(lbConfig.Args, []string{"--bootstrap-qemu-arch",
			arch, "--bootstrap-qemu-static", qemuPath, "--architectures", arch}...)
	}

	return lbConfig, lbBuild, nil
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
			partitionType := gpt.Type(structureType)
			gptPartition := &gpt.Partition{
				Start: uint64(math.Ceil(float64(*structure.Offset) / float64(sectorSize))),
				Size:  uint64(structure.Size),
				Type:  partitionType,
				Name:  structure.Name,
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
