package statemachine

import (
	"encoding/gob"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/gadget/quantity"
)

// SetCommonOpts stores the common options for all image types in the struct
func (stateMachine *StateMachine) SetCommonOpts(commonOpts *commands.CommonOpts,
	stateMachineOpts *commands.StateMachineOpts) {
	stateMachine.commonFlags = commonOpts
	stateMachine.stateMachineFlags = stateMachineOpts
}

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

// parseImageSizes handles the flag --image-size, which is a string in the format
// <volumeName>:<volumeSize>,<volumeName2>:<volumeSize2>. It can also be in the
// format <volumeSize> to signify one size to rule them all
func (stateMachine *StateMachine) parseImageSizes() error {
	// initialize the size map
	stateMachine.imageSizes = make(map[string]quantity.Size)

	// If --image-size was not used, simply return
	if stateMachine.commonFlags.Size == "" {
		return nil
	}

	if !strings.Contains(stateMachine.commonFlags.Size, ":") {
		// handle the "one size to rule them all" case
		parsedSize, err := quantity.ParseSize(stateMachine.commonFlags.Size)
		if err != nil {
			return fmt.Errorf("Failed to parse argument to --image-size: %s", err.Error())
		}
		for volumeName := range stateMachine.gadgetInfo.Volumes {
			stateMachine.imageSizes[volumeName] = parsedSize
		}
	} else {
		allSizes := strings.Split(stateMachine.commonFlags.Size, ",")
		for _, size := range allSizes {
			// each of these should be of the form "<name|number>:<size>"
			splitSize := strings.Split(size, ":")
			if len(splitSize) != 2 {
				return fmt.Errorf("Argument to --image-size %s is not "+
					"in the correct format", size)
			}
			parsedSize, err := quantity.ParseSize(splitSize[1])
			if err != nil {
				return fmt.Errorf("Failed to parse argument to --image-size: %s",
					err.Error())
			}
			// the image size parsed successfully, now find which volume to associate it with
			volumeNumber, err := strconv.Atoi(splitSize[0])
			if err == nil {
				// argument passed was numeric.
				if volumeNumber < len(stateMachine.volumeOrder) {
					stateName := stateMachine.volumeOrder[volumeNumber]
					stateMachine.imageSizes[stateName] = parsedSize
				} else {
					return fmt.Errorf("Volume index %d is out of range", volumeNumber)
				}
			} else {
				if _, found := stateMachine.gadgetInfo.Volumes[splitSize[0]]; !found {
					return fmt.Errorf("Volume %s does not exist in gadget.yaml",
						splitSize[0])
				}
				stateMachine.imageSizes[splitSize[0]] = parsedSize
			}
		}
	}
	return nil
}

// saveVolumeOrder records the order that the volumes appear in gadget.yaml. This is necessary
// to preserve backwards compatibility of the command line syntax --image-size <volume_number>:<size>
func (stateMachine *StateMachine) saveVolumeOrder(gadgetYamlContents string) {
	// don't bother doing this if --image-size was not used
	if stateMachine.commonFlags.Size == "" {
		return
	}

	indexMap := make(map[string]int)
	for volumeName := range stateMachine.gadgetInfo.Volumes {
		searchString := volumeName + ":"
		index := strings.Index(gadgetYamlContents, searchString)
		indexMap[volumeName] = index
	}

	// now sort based on the index
	type volumeNameIndex struct {
		VolumeName string
		Index      int
	}

	var sortable []volumeNameIndex
	for volumeName, volumeIndex := range indexMap {
		sortable = append(sortable, volumeNameIndex{volumeName, volumeIndex})
	}

	sort.Slice(sortable, func(i, j int) bool {
		return sortable[i].Index < sortable[j].Index
	})

	var sortedVolumes []string
	for _, volume := range sortable {
		sortedVolumes = append(sortedVolumes, volume.VolumeName)
	}

	stateMachine.volumeOrder = sortedVolumes
}

// readMetadata reads info about a partial state machine from disk
func (stateMachine *StateMachine) readMetadata() error {
	// handle the resume case
	if stateMachine.stateMachineFlags.Resume {
		// open the ubuntu-image.gob file and determine the state
		var partialStateMachine = new(StateMachine)
		gobfilePath := filepath.Join(stateMachine.stateMachineFlags.WorkDir, "ubuntu-image.gob")
		gobfile, err := os.Open(gobfilePath)
		if err != nil {
			return fmt.Errorf("error reading metadata file: %s", err.Error())
		}
		defer gobfile.Close()
		dec := gob.NewDecoder(gobfile)
		err = dec.Decode(&partialStateMachine)
		if err != nil {
			return fmt.Errorf("failed to parse metadata file: %s", err.Error())
		}
		stateMachine.CurrentStep = partialStateMachine.CurrentStep
		stateMachine.StepsTaken = partialStateMachine.StepsTaken

		// delete all of the stateFuncs that have already run
		stateMachine.states = stateMachine.states[stateMachine.StepsTaken:]
	}
	return nil
}

// writeMetadata writes the state machine info to disk. This will be used when resuming a
// partial state machine run
func (stateMachine *StateMachine) writeMetadata() error {
	gobfilePath := filepath.Join(stateMachine.stateMachineFlags.WorkDir, "ubuntu-image.gob")
	gobfile, err := os.OpenFile(gobfilePath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("error opening metadata file for writing: %s", gobfilePath)
	}
	defer gobfile.Close()
	enc := gob.NewEncoder(gobfile)

	// no need to check errors, as it will panic if there is one
	enc.Encode(stateMachine)
	return nil
}

// cleanup cleans the workdir. For now this is just deleting the temporary directory if necessary
// but will have more functionality added to it later
func (stateMachine *StateMachine) cleanup() error {
	if stateMachine.cleanWorkDir {
		if err := osRemoveAll(stateMachine.stateMachineFlags.WorkDir); err != nil {
			return err
		}
	}
	return nil
}

// postProcessGadgetYaml adds the rootfs to the partitions list if needed
func (stateMachine *StateMachine) postProcessGadgetYaml() error {
	var rootfsSeen bool = false
	var farthestOffset quantity.Offset = 0
	var lastOffset quantity.Offset = 0
	var lastVolumeName string
	for volumeName, volume := range stateMachine.gadgetInfo.Volumes {
		lastVolumeName = volumeName
		volumeBaseDir := filepath.Join(stateMachine.tempDirs.volumes, volumeName)
		if err := osMkdirAll(volumeBaseDir, 0755); err != nil {
			return fmt.Errorf("Error creating volume dir: %s", err.Error())
		}
		// look for the rootfs and check if the image is seeded
		for ii, structure := range volume.Structure {
			if structure.Role == "" && structure.Label == gadget.SystemBoot {
				fmt.Printf("WARNING: volumes:%s:structure:%d:filesystem_label "+
					"used for defining partition roles; use role instead\n",
					volumeName, ii)
			} else if structure.Role == gadget.SystemData {
				rootfsSeen = true
			} else if structure.Role == gadget.SystemSeed {
				stateMachine.isSeeded = true
			}

			// update farthestOffset if needed
			var offset quantity.Offset
			if structure.Offset == nil {
				if structure.Role != "mbr" && lastOffset < quantity.OffsetMiB {
					offset = quantity.OffsetMiB
				} else {
					offset = lastOffset
				}
			} else {
				offset = *structure.Offset
			}
			lastOffset = offset + quantity.Offset(structure.Size)
			farthestOffset = maxOffset(lastOffset, farthestOffset)
		}
	}

	if !rootfsSeen && len(stateMachine.gadgetInfo.Volumes) == 1 {
		// We still need to handle the case of unspecified system-data
		// partition where we simply attach the rootfs at the end of the
		// partition list.
		//
		// Since so far we have no knowledge of the rootfs contents, the
		// size is set to 0, and will be calculated later
		rootfsStructure := gadget.VolumeStructure{
			Name:        "",
			Label:       "writable",
			Offset:      &farthestOffset,
			OffsetWrite: new(gadget.RelativeOffset),
			Size:        quantity.Size(0),
			Type:        "83,0FC63DAF-8483-4772-8E79-3D69D8477DE4",
			Role:        gadget.SystemData,
			ID:          "",
			Filesystem:  "ext4",
			Content:     []gadget.VolumeContent{},
			Update:      gadget.VolumeUpdate{},
		}

		// There is only one volume, so lastVolumeName is its name
		// we now add the rootfs structure to the volume
		stateMachine.gadgetInfo.Volumes[lastVolumeName].Structure =
			append(stateMachine.gadgetInfo.Volumes[lastVolumeName].Structure, rootfsStructure)
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
				return err
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
				return err
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

// handleContentSizes ensures that the sizes of the partitions are large enough and stores
// safe values in the stateMachine struct for use during make_image
func (stateMachine *StateMachine) handleContentSizes(farthestOffset quantity.Offset, volumeName string) {
	// store volume sizes in the stateMachine Struct. These will be used during
	// the make_image step
	calculated := quantity.Size((farthestOffset/quantity.OffsetMiB + 17) * quantity.OffsetMiB)
	volumeSize, found := stateMachine.imageSizes[volumeName]
	if !found {
		stateMachine.imageSizes[volumeName] = calculated
	} else {
		if volumeSize < calculated {
			fmt.Printf("WARNING: ignoring image size smaller than "+
				"minimum required size: vol:%s %d < %d",
				volumeName, uint64(volumeSize), uint64(calculated))
			stateMachine.imageSizes[volumeName] = calculated
		} else {
			stateMachine.imageSizes[volumeName] = volumeSize
		}
	}
}

// shouldSkipStructure returns whether a structure should be skipped during certain processing
func (stateMachine *StateMachine) shouldSkipStructure(structure gadget.VolumeStructure) bool {
	if stateMachine.isSeeded &&
		(structure.Role == gadget.SystemBoot ||
			structure.Role == gadget.SystemData ||
			structure.Role == gadget.SystemSave ||
			structure.Label == gadget.SystemBoot) {
		return true
	}
	return false
}

// copyStructureContent handles copying raw blobs or creating formatted filesystems
func (stateMachine *StateMachine) copyStructureContent(structure gadget.VolumeStructure,
	contentRoot, partImg string) error {
	if structure.Filesystem == "" {
		// copy the contents to the new location
		var runningOffset quantity.Offset = 0
		for _, content := range structure.Content {
			if content.Offset != nil {
				runningOffset = *content.Offset
			}
			// first zero it out. Structures without filesystem specified in the gadget
			// yaml must have the size specified, so the bs= argument below is valid
			ddArgs := []string{"if=/dev/zero", "of=" + partImg, "count=0",
				"bs=" + strconv.FormatUint(uint64(structure.Size), 10),
				"seek=1"}
			if err := helperCopyBlob(ddArgs); err != nil {
				return fmt.Errorf("Error zeroing partition: %s",
					err.Error())
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
			if structure.Size < stateMachine.rootfsSize {
				fmt.Printf("WARNING: rootfs structure size %s smaller "+
					"than actual rootfs contents %s\n",
					structure.Size.IECString(),
					stateMachine.rootfsSize.IECString())
				blockSize = stateMachine.rootfsSize
			} else {
				blockSize = structure.Size
			}
		} else {
			blockSize = structure.Size
		}
		// use mkfs functions from snapd to create the filesystems
		ddArgs := []string{"if=/dev/zero", "of=" + partImg, "count=0",
			"bs=" + strconv.FormatUint(uint64(blockSize), 10), "seek=1"}
		if err := helperCopyBlob(ddArgs); err != nil {
			return fmt.Errorf("Error zeroing image file %s: %s",
				partImg, err.Error())
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
		if err := osutilCopySpecialFile(srcFile, ubuntuDir); err != nil {
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
