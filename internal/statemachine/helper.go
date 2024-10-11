package statemachine

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/diskfs/go-diskfs/disk"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/gadget/quantity"
	"github.com/snapcore/snapd/seed"
	"github.com/snapcore/snapd/timings"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/canonical/ubuntu-image/internal/imagedefinition"
)

var runCmd = helper.RunCmd
var blockSize string = "1"

var (
	MKE2FS_CONFIG_ENV  = "MKE2FS_CONFIG"
	MKE2FS_CONFIG_FILE = "mke2fs.conf"
	MKE2FS_BASE_PATH   = "/etc/ubuntu-image/mkfs"
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

func (stateMachine *StateMachine) setConfDefDir(confFileArg string) error {
	path, err := filepath.Abs(filepath.Dir(confFileArg))
	if err != nil {
		return fmt.Errorf("unable to determine the configuration definition directory: %w", err)
	}
	stateMachine.ConfDefPath = path

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
	files, err := osReadDir(bootDir)
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

// copyStructureContent handles copying raw blobs or creating formatted filesystems
func (stateMachine *StateMachine) copyStructureContent(structure *gadget.VolumeStructure, contentRoot, partImg string) error {

	if !structure.HasFilesystem() {
		// binary blobs like eg. raw bootloader images
		err := copyStructureNoFS(stateMachine.tempDirs.unpack, structure, partImg)
		if err != nil {
			return err
		}
	} else {
		err := stateMachine.prepareAndCreateFS(structure, contentRoot, partImg)
		if err != nil {
			return err
		}
	}
	return nil
}

// copyStructureNoFS copies the contents to the new location.
// It first zeros it out. Structures without filesystem specified in the gadget
// yaml must have the size specified, so the bs= argument below is valid
func copyStructureNoFS(unpackDir string, structure *gadget.VolumeStructure, partImg string) error {
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
		inFile := filepath.Join(unpackDir, "gadget", content.Image)
		ddArgs = []string{"if=" + inFile, "of=" + partImg, "bs=" + blockSize,
			"seek=" + strconv.FormatUint(uint64(runningOffset), 10),
			"conv=sparse,notrunc"}
		if err := helperCopyBlob(ddArgs); err != nil {
			return fmt.Errorf("Error copying image blob: %s",
				err.Error())
		}
		runningOffset += quantity.Offset(content.Size)
	}

	return nil
}

// prepareAndCreateFS prepares and creates a filesystem for the given structure
func (stateMachine *StateMachine) prepareAndCreateFS(structure *gadget.VolumeStructure, contentRoot, partImg string) error {
	err := prepareDiskImg(structure, partImg, structure.Size, stateMachine.RootfsSize)
	if err != nil {
		return err
	}

	return makeFS(structure, contentRoot, partImg, stateMachine.SectorSize, stateMachine.series)
}

// prepareDiskImg prepares a raw image
func prepareDiskImg(structure *gadget.VolumeStructure, partImg string, partSize quantity.Size, rootfsSize quantity.Size) error {
	if helper.IsRootfsStructure(structure) {
		_, err := os.Create(partImg)
		if err != nil {
			return fmt.Errorf("unable to create partImg file: %w", err)
		}
		err = os.Truncate(partImg, int64(rootfsSize))
		if err != nil {
			return fmt.Errorf("unable to truncate partImg file: %w", err)
		}
	} else {
		// zero out the .img file
		ddArgs := []string{"if=/dev/zero", "of=" + partImg, "count=0",
			"bs=" + strconv.FormatUint(uint64(partSize), 10), "seek=1"}
		if err := helperCopyBlob(ddArgs); err != nil {
			return fmt.Errorf("Error zeroing image file %s: %s",
				partImg, err.Error())
		}
	}
	return nil
}

// makeFS actually creates the filesystem for the given structure
func makeFS(structure *gadget.VolumeStructure, contentRoot string, partImg string, sectorSize quantity.Size, series string) error {
	hasC, err := hasContent(structure, contentRoot)
	if err != nil {
		return err
	}

	// select the mkfs.ext4 conf to use
	err = setMk2fsConf(series)
	if err != nil {
		return fmt.Errorf("Error preparing env for mkfs: %s", err.Error())
	}

	if hasC {
		err := mkfsMakeWithContent(structure.Filesystem, partImg, structure.Label,
			contentRoot, structure.Size, sectorSize)
		if err != nil {
			return fmt.Errorf("Error running mkfs with content: %s", err.Error())
		}
		return nil
	}
	err = mkfsMake(structure.Filesystem, partImg, structure.Label,
		structure.Size, sectorSize)
	if err != nil {
		return fmt.Errorf("Error running mkfs: %s", err.Error())
	}

	return nil
}

// hasContent checks if the structure or the contentRoot dir contains anything
func hasContent(structure *gadget.VolumeStructure, contentRoot string) (bool, error) {
	contentFiles, err := osReadDir(contentRoot)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("Error listing contents of volume \"%s\": %s",
			contentRoot, err.Error())
	}

	return structure.Content != nil || len(contentFiles) > 0, nil
}

const mbrDiskSignatureAddress = 440

func fixDiskIDOnMBR(imgName string) error {
	var existingDiskIds [][]byte
	randomBytes, err := generateUniqueDiskID(&existingDiskIds)
	if err != nil {
		return fmt.Errorf("Error generating disk ID: %s", err.Error())
	}
	diskFile, err := osOpenFile(imgName, os.O_RDWR, 0755)
	if err != nil {
		return fmt.Errorf("Error opening disk to write MBR disk identifier: %s",
			err.Error())
	}
	defer diskFile.Close()
	_, err = diskFile.WriteAt(randomBytes, mbrDiskSignatureAddress)
	if err != nil {
		return fmt.Errorf("Error writing MBR disk identifier: %s", err.Error())
	}

	return nil
}

// The MKE2FS_BASE_PATH folder is setup to handle codename and release number as a series.
func setMk2fsConf(series string) error {
	mk2fsConfPath := strings.Join([]string{osGetenv("SNAP"), MKE2FS_BASE_PATH, series, MKE2FS_CONFIG_FILE}, "/")

	_, err := os.Stat(mk2fsConfPath)
	if err != nil {
		fmt.Printf("WARNING: No mkfs configuration found for this series: %s. Will fallback on the default one.\n", series)
		return nil
	}

	return osSetenv(MKE2FS_CONFIG_ENV, mk2fsConfPath)
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

	files, err := osReadDir(bootDir)
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
	files, err := osReadDir(snapsDir)
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
			fmt.Fprintf(manifest, "%s %s\n", split[0], strings.TrimSuffix(split[1], ".snap"))
		}
	}
	return nil
}

// getHostArch uses dpkg to return the host architecture of the current system
func getHostArch() string {
	cmd := exec.Command("dpkg", "--print-architecture")
	outputBytes, _ := cmd.Output() // nolint: errcheck
	return strings.TrimSpace(string(outputBytes))
}

// getHostSuite checks the release name of the host system to use as a default if --suite is not passed
func getHostSuite() string {
	cmd := exec.Command("lsb_release", "-c", "-s")
	outputBytes, _ := cmd.Output() // nolint: errcheck
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

// raiseStructureSizes raise both Size and MinSize to at least the given size
// It helps make sure whatever value is used in snapd, it is set to the size we need
func raiseStructureSizes(s *gadget.VolumeStructure, size quantity.Size) {
	if s.MinSize < size {
		s.MinSize = size
	}
	if s.Size < size {
		s.Size = size
	}
}

// copyDataToImage runs dd commands to copy the raw data to the final image with appropriate offsets
func (stateMachine *StateMachine) copyDataToImage(volumeName string, volume *gadget.Volume, diskImg *disk.Disk) error {
	// Resolve gadget information to on disk volume
	onDisk := gadget.OnDiskStructsFromGadget(volume)
	for structureNumber := range volume.Structure {
		structure := &volume.Structure[structureNumber]
		if helper.ShouldSkipStructure(structure, stateMachine.IsSeeded) {
			continue
		}
		sectorSize := diskImg.LogicalBlocksize
		// set up the arguments to dd the structures into an image
		partImg := filepath.Join(stateMachine.tempDirs.volumes, volumeName,
			"part"+strconv.Itoa(structureNumber)+".img")
		onDiskStruct := onDisk[structure.YamlIndex]
		seek := strconv.FormatInt(int64(onDiskStruct.StartOffset)/sectorSize, 10)
		count := strconv.FormatFloat(math.Ceil(float64(onDiskStruct.Size)/float64(sectorSize)), 'f', 0, 64)
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
			if bytes.Equal(randomBytes, id) {
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

// generateGerminateCmd creates the appropriate germinate command for the
// values configured in the image definition yaml file
func generateGerminateCmd(imageDefinition imagedefinition.ImageDefinition) *exec.Cmd {
	// determine the value for the seed-dist in the form of <archive>.<series>
	seedDist := imageDefinition.Rootfs.Flavor
	if imageDefinition.Rootfs.Seed.SeedBranch != "" {
		seedDist = seedDist + "." + imageDefinition.Rootfs.Seed.SeedBranch
	}

	seedSource := strings.Join(imageDefinition.Rootfs.Seed.SeedURLs, ",")

	germinateCmd := execCommand(
		"germinate",
		"--mirror", imageDefinition.Rootfs.Mirror,
		"--arch", imageDefinition.Architecture,
		"--dist", imageDefinition.Series,
		"--seed-source", seedSource,
		"--seed-dist", seedDist,
		"--no-rdepends",
	)

	if *imageDefinition.Rootfs.Seed.Vcs {
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
		Depth:        1,
	}
	if imageDefinition.Gadget.GadgetBranch != "" {
		cloneOptions.ReferenceName = plumbing.NewBranchReferenceName(imageDefinition.Gadget.GadgetBranch)
	}

	err := cloneOptions.Validate()
	if err != nil {
		return err
	}

	_, err = git.PlainClone(workDir, false, cloneOptions)
	return err
}

// generateDebootstrapCmd generates the debootstrap command used to create a chroot
// environment that will eventually become the rootfs of the resulting image
func generateDebootstrapCmd(imageDefinition imagedefinition.ImageDefinition, targetDir string) *exec.Cmd {
	debootstrapCmd := execCommand("debootstrap",
		"--arch", imageDefinition.Architecture,
		"--variant=minbase",
	)

	if imageDefinition.Customization != nil && len(imageDefinition.Customization.ExtraPPAs) > 0 {
		// ca-certificates is needed to use PPAs
		debootstrapCmd.Args = append(debootstrapCmd.Args, "--include=ca-certificates")
	}

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

	installCmd.Args = append(installCmd.Args, packageList...)

	// Env is sometimes used for mocking command calls in tests,
	// so only overwrite env if it is nil
	if installCmd.Env == nil {
		installCmd.Env = os.Environ()
	}
	installCmd.Env = append(installCmd.Env, "DEBIAN_FRONTEND=noninteractive")

	return []*exec.Cmd{updateCmd, installCmd}
}

func setDenyingPolicyRcD(path string) (func(error) error, error) {
	const policyRcDDisableAll = `#!/bin/sh
echo "All runlevel operations denied by policy" >&2
exit 101
`
	err := osMkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		return nil, fmt.Errorf("Error creating policy-rc.d dir: %s", err.Error())
	}

	err = osWriteFile(path, []byte(policyRcDDisableAll), 0755)
	if err != nil {
		return nil, fmt.Errorf("Error writing to policy-rc.d: %s", err.Error())
	}

	return func(err error) error {
		tmpErr := osRemove(path)
		if tmpErr != nil {
			err = fmt.Errorf("%s\n%s", err, tmpErr)
		}
		return err
	}, nil
}

// divertPolicyRcD dpkg-diverts policy-rc.d to keep it if it already exists
func divertPolicyRcD(targetDir string) (*exec.Cmd, *exec.Cmd) {
	return dpkgDivert(targetDir, "/usr/sbin/policy-rc.d")
}

// dpkgDivert dpkg-diverts the given file in the given baseDir
func dpkgDivert(baseDir string, target string) (*exec.Cmd, *exec.Cmd) {
	dpkgDivert := "dpkg-divert"
	targetDiverted := target + ".dpkg-divert"

	commonArgs := []string{
		"--local",
		"--divert",
		targetDiverted,
		"--rename",
		target,
	}

	divert := append([]string{baseDir, dpkgDivert}, commonArgs...)
	undivert := append([]string{baseDir, dpkgDivert, "--remove"}, commonArgs...)

	return execCommand("chroot", divert...), execCommand("chroot", undivert...)
}

// backupReplaceStartStopDaemon backup start-stop-daemon and replace it with a fake one
// Returns a restore function to put the original one in place
func backupReplaceStartStopDaemon(baseDir string) (func(error) error, error) {
	const startStopDaemonContent = `#!/bin/sh
echo
echo "Warning: Fake start-stop-daemon called, doing nothing"
`

	startStopDaemon := filepath.Join(baseDir, "sbin", "start-stop-daemon")
	return helper.BackupReplace(startStopDaemon, startStopDaemonContent)
}

// backupReplaceInitctl backup initctl and replace it with a fake one
// Returns a restore function to put the original one in place
func backupReplaceInitctl(baseDir string) (func(error) error, error) {
	const initctlContent = `#!/bin/sh
if [ "$1" = version ]; then exec /sbin/initctl.REAL "$@"; fi
echo
echo "Warning: Fake initctl called, doing nothing"
`

	initctl := filepath.Join(baseDir, "sbin", "initctl")
	return helper.BackupReplace(initctl, initctlContent)
}

// execTeardownCmds executes given commands and collects error to join them with an existing error.
// Failure to execute one command will not stop from executing following ones.
func execTeardownCmds(teardownCmds []*exec.Cmd, debug bool, prevErr error) (err error) {
	err = prevErr
	errs := make([]string, 0)
	for _, teardownCmd := range teardownCmds {
		cmdOutput := helper.SetCommandOutput(teardownCmd, debug)
		teardownErr := teardownCmd.Run()
		if teardownErr != nil {
			errs = append(errs, fmt.Sprintf("teardown command  \"%s\" failed. Output: \n%s",
				teardownCmd.String(), cmdOutput.String()))
		}
	}

	if len(errs) > 0 {
		err = fmt.Errorf("teardown failed: %s", strings.Join(errs, "\n"))
		if prevErr != nil {
			errs := append([]string{prevErr.Error()}, errs...)
			err = errors.New(strings.Join(errs, "\n"))
		}
	}

	return err
}

// manualMakeDirs creates a directory (and intermediate directories) into the chroot
func manualMakeDirs(customizations []*imagedefinition.MakeDirs, targetDir string, debug bool) error {
	for _, c := range customizations {
		path := filepath.Join(targetDir, c.Path)
		if debug {
			fmt.Printf("Creating directory \"%s\"\n", path)
		}
		if err := osMkdirAll(path, fs.FileMode(c.Permissions)); err != nil {
			return fmt.Errorf("Error creating directory \"%s\" into chroot: %s",
				path, err.Error())
		}
	}
	return nil
}

// manualCopyFile copies a file into the chroot
func manualCopyFile(customizations []*imagedefinition.CopyFile, confDefPath string, targetDir string, debug bool) error {
	for _, c := range customizations {
		source := filepath.Join(confDefPath, c.Source)
		dest := filepath.Join(targetDir, c.Dest)
		if debug {
			fmt.Printf("Copying file \"%s\" to \"%s\"\n", source, dest)
		}
		if err := osutilCopySpecialFile(source, dest); err != nil {
			return fmt.Errorf("Error copying file \"%s\" into chroot: %s",
				source, err.Error())
		}
	}
	return nil
}

// manualExecute executes executable files in the chroot
func manualExecute(customizations []*imagedefinition.Execute, targetDir string, debug bool) error {
	for _, c := range customizations {
		executeCmd := execCommand("chroot", targetDir, c.ExecutePath)
		if debug {
			fmt.Printf("Executing command \"%s\"\n", executeCmd.String())
		}
		executeOutput := helper.SetCommandOutput(executeCmd, debug)
		err := executeCmd.Run()
		if err != nil {
			return fmt.Errorf("Error running script \"%s\". Error is %s. Full output below:\n%s",
				executeCmd.String(), err.Error(), executeOutput.String())
		}
	}
	return nil
}

// manualTouchFile touches files in the chroot
func manualTouchFile(customizations []*imagedefinition.TouchFile, targetDir string, debug bool) error {
	for _, c := range customizations {
		fullPath := filepath.Join(targetDir, c.TouchPath)
		if debug {
			fmt.Printf("Creating empty file \"%s\"\n", fullPath)
		}
		_, err := osCreate(fullPath)
		if err != nil {
			return fmt.Errorf("Error creating file in chroot: %s", err.Error())
		}
	}
	return nil
}

// manualAddGroup adds groups in the chroot
func manualAddGroup(customizations []*imagedefinition.AddGroup, targetDir string, debug bool) error {
	for _, c := range customizations {
		addGroupCmd := execCommand("chroot", targetDir, "groupadd", c.GroupName)
		debugStatement := fmt.Sprintf("Adding group \"%s\"\n", c.GroupName)
		if c.GroupID != "" {
			addGroupCmd.Args = append(addGroupCmd.Args, []string{"--gid", c.GroupID}...)
			debugStatement = fmt.Sprintf("%s with GID %s\n", strings.TrimSpace(debugStatement), c.GroupID)
		}
		if debug {
			fmt.Print(debugStatement)
		}
		addGroupOutput := helper.SetCommandOutput(addGroupCmd, debug)
		err := addGroupCmd.Run()
		if err != nil {
			return fmt.Errorf("Error adding group. Command used is \"%s\". Error is %s. Full output below:\n%s",
				addGroupCmd.String(), err.Error(), addGroupOutput.String())
		}
	}
	return nil
}

// manualAddUser adds users in the chroot
func manualAddUser(customizations []*imagedefinition.AddUser, targetDir string, debug bool) error {
	for _, c := range customizations {
		debugStatement := fmt.Sprintf("Adding user \"%s\"\n", c.UserName)
		var addUserCmds []*exec.Cmd

		addUserCmd := execCommand("chroot", targetDir, "useradd", c.UserName)
		if c.UserID != "" {
			addUserCmd.Args = append(addUserCmd.Args, []string{"--uid", c.UserID}...)
			debugStatement = fmt.Sprintf("%s with UID %s\n", strings.TrimSpace(debugStatement), c.UserID)
		}

		addUserCmds = append(addUserCmds, addUserCmd)

		if c.Password != "" {
			chPasswordCmd := execCommand("chroot", targetDir, "chpasswd")

			if c.PasswordType == "hash" {
				chPasswordCmd.Args = append(chPasswordCmd.Args, "-e")
			}

			chPasswordCmd.Stdin = strings.NewReader(fmt.Sprintf("%s:%s", c.UserName, c.Password))

			debugStatement = fmt.Sprintf("%s, setting a password\n", strings.TrimSpace(debugStatement))
			addUserCmds = append(addUserCmds, chPasswordCmd)
		}

		debugStatement = fmt.Sprintf("%s, forcing reseting the password at first login\n", strings.TrimSpace(debugStatement))
		addUserCmds = append(addUserCmds,
			execCommand("chroot", targetDir, "passwd", "--expire", c.UserName),
		)

		if debug {
			fmt.Print(debugStatement)
		}

		for _, cmd := range addUserCmds {
			err := runCmd(cmd, debug)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// getPreseedsnaps returns a slice of the snaps that were preseeded in a chroot
// and their channels
func getPreseededSnaps(rootfs string) (seededSnaps map[string]string, err error) {
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
	err = preseed.Iter(func(sn *seed.Snap) error {
		seededSnaps[sn.SnapName()] = sn.Channel
		return nil
	})
	if err != nil {
		return seededSnaps, err
	}

	return seededSnaps, nil
}

// associateLoopDevice associates a file to a loop device and returns the loop device number
// Also returns the command to detach the loop device during teardown
func associateLoopDevice(path string, sectorSize quantity.Size) (string, *exec.Cmd, error) {
	// run the losetup command and read the output to determine which loopback was used
	losetupCmd := execCommand("losetup",
		"--find",
		"--show",
		"--partscan",
		"--sector-size",
		sectorSize.String(),
		path,
	)
	var losetupOutput []byte
	losetupOutput, err := losetupCmd.Output()
	if err != nil {
		err = fmt.Errorf("Error running losetup command \"%s\". Error is %s",
			losetupCmd.String(),
			err.Error(),
		)
		return "", nil, err
	}

	loopUsed := strings.TrimSpace(string(losetupOutput))

	//nolint:gosec,G204
	losetupDetachCmd := execCommand("losetup", "--detach", loopUsed)

	return loopUsed, losetupDetachCmd, nil
}

// divertOSProber divert GRUB's os-prober as we don't want to scan for other OSes on
// the build system
func divertOSProber(mountDir string) (*exec.Cmd, *exec.Cmd) {
	return dpkgDivert(mountDir, "/etc/grub.d/30_os-prober")
}

// updateGrub mounts the resulting image and runs update-grub
func (stateMachine *StateMachine) updateGrub(rootfsVolName string, rootfsPartNum int) (err error) {
	// create a directory in which to mount the rootfs
	mountDir := filepath.Join(stateMachine.tempDirs.scratch, "loopback")
	err = osMkdir(mountDir, 0755)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("Error creating scratch/loopback directory: %s", err.Error())
	}

	// Slice used to store all the commands that need to be run
	// to properly update grub.cfg in the chroot
	// updateGrubCmds should be filled as a FIFO list
	var updateGrubCmds []*exec.Cmd
	// Slice used to store all the commands that need to be run
	// to properly cleanup everything after the update of grub.cfg
	// updateGrubCmds should be filled as a LIFO list (so new entries should added at the start of the slice)
	var teardownCmds []*exec.Cmd

	defer func() {
		err = execTeardownCmds(teardownCmds, stateMachine.commonFlags.Debug, err)
	}()

	imgPath := filepath.Join(stateMachine.commonFlags.OutputDir, stateMachine.VolumeNames[rootfsVolName])

	loopUsed, losetupDetachCmd, err := associateLoopDevice(imgPath, stateMachine.SectorSize)
	if err != nil {
		return err
	}

	// detach the loopback device
	teardownCmds = append(teardownCmds, losetupDetachCmd)

	updateGrubCmds = append(updateGrubCmds,
		// mount the rootfs partition in which to run update-grub
		//nolint:gosec,G204
		execCommand("mount",
			fmt.Sprintf("%sp%d", loopUsed, rootfsPartNum),
			mountDir,
		),
	)

	teardownCmds = append([]*exec.Cmd{execCommand("umount", mountDir)}, teardownCmds...)

	// set up the mountpoints
	mountPoints := []mountPoint{
		{
			src:      "devtmpfs-build",
			basePath: mountDir,
			relpath:  "/dev",
			typ:      "devtmpfs",
		},
		{
			src:      "devpts-build",
			basePath: mountDir,
			relpath:  "/dev/pts",
			typ:      "devpts",
			opts:     []string{"nodev", "nosuid"},
		},
		{
			src:      "proc-build",
			basePath: mountDir,
			relpath:  "/proc",
			typ:      "proc",
		},
		{
			src:      "sysfs-build",
			basePath: mountDir,
			relpath:  "/sys",
			typ:      "sysfs",
		},
	}

	for _, mp := range mountPoints {
		mountCmds, umountCmds, err := mp.getMountCmd()
		if err != nil {
			return fmt.Errorf("Error preparing mountpoint \"%s\": \"%s\"",
				mp.relpath,
				err.Error(),
			)
		}
		updateGrubCmds = append(updateGrubCmds, mountCmds...)
		teardownCmds = append(umountCmds, teardownCmds...)
	}

	teardownCmds = append([]*exec.Cmd{
		execCommand("udevadm", "settle"),
	}, teardownCmds...)

	divert, undivert := divertOSProber(mountDir)

	updateGrubCmds = append(updateGrubCmds, divert)
	teardownCmds = append([]*exec.Cmd{undivert}, teardownCmds...)

	// actually run update-grub
	updateGrubCmds = append(updateGrubCmds,
		execCommand("chroot",
			mountDir,
			"update-grub",
		),
	)

	// now run all the commands
	err = helper.RunCmds(updateGrubCmds, stateMachine.commonFlags.Debug)
	if err != nil {
		return err
	}

	return nil
}
