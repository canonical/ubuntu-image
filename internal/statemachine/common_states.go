package statemachine

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/google/uuid"
	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/gadget/quantity"
	"github.com/snapcore/snapd/osutil"
)

// generate work directory file structure
func (stateMachine *StateMachine) makeTemporaryDirectories() error {
	// if no workdir was specified, open a /tmp dir
	if stateMachine.stateMachineFlags.WorkDir == "" {
		stateMachine.stateMachineFlags.WorkDir = filepath.Join("/tmp", "ubuntu-image-"+uuid.NewString())
		if err := osMkdir(stateMachine.stateMachineFlags.WorkDir, 0755); err != nil {
			return fmt.Errorf("Failed to create temporary directory: %s", err.Error())
		}
		stateMachine.cleanWorkDir = true
	} else {
		err := osMkdirAll(stateMachine.stateMachineFlags.WorkDir, 0755)
		if err != nil && !os.IsExist(err) {
			return fmt.Errorf("Error creating work directory: %s", err.Error())
		}
	}

	stateMachine.tempDirs.rootfs = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "root")
	stateMachine.tempDirs.unpack = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "unpack")
	stateMachine.tempDirs.volumes = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "volumes")

	if err := osMkdir(stateMachine.tempDirs.rootfs, 0755); err != nil {
		return fmt.Errorf("Error creating temporary directory: %s", err.Error())
	}

	return nil
}

// Load gadget.yaml, do some validation, and store the relevant info in the StateMachine struct
func (stateMachine *StateMachine) loadGadgetYaml() error {
	gadgetYamlDst := filepath.Join(stateMachine.stateMachineFlags.WorkDir, "gadget.yaml")
	if err := osutilCopyFile(stateMachine.yamlFilePath,
		gadgetYamlDst, osutil.CopyFlagOverwrite); err != nil {
		return fmt.Errorf("Error loading gadget.yaml: %s", err.Error())
	}

	// read in the gadget.yaml as bytes, because snapd expects it that way
	gadgetYamlBytes, err := ioutilReadFile(stateMachine.yamlFilePath)
	if err != nil {
		return fmt.Errorf("Error loading gadget.yaml: %s", err.Error())
	}

	stateMachine.gadgetInfo, err = gadget.InfoFromGadgetYaml(gadgetYamlBytes, nil)
	if err != nil {
		return fmt.Errorf("Error loading gadget.yaml: %s", err.Error())
	}

	// check if the unpack dir should be preserved
	envar := os.Getenv("UBUNTU_IMAGE_PRESERVE_UNPACK")
	if envar != "" {
		err := osMkdirAll(envar, 0755)
		if err != nil && !os.IsExist(err) {
			return fmt.Errorf("Error creating preserve_unpack directory: %s", err.Error())
		}
		if err := osutilCopySpecialFile(stateMachine.tempDirs.unpack, envar); err != nil {
			return fmt.Errorf("Error preserving unpack dir: %s", err.Error())
		}
	}

	if err := stateMachine.postProcessGadgetYaml(); err != nil {
		return err
	}

	// for the --image-size argument, the order of the volumes specified in gadget.yaml
	// must be preserved. However, since gadget.Info stores the volumes as a map, the
	// order is not preserved. We use the already read-in gadget.yaml file to store the
	// order of the volumes as an array in the StateMachine struct
	stateMachine.saveVolumeOrder(string(gadgetYamlBytes))

	if err := stateMachine.parseImageSizes(); err != nil {
		return err
	}

	return nil
}

// Run hooks specified by --hooks-directory after populating rootfs contents
func (stateMachine *StateMachine) populateRootfsContentsHooks() error {
	if stateMachine.isSeeded {
		if stateMachine.commonFlags.Debug {
			fmt.Println("Building from a seeded gadget - " +
				"skipping the post-populate-rootfs hook execution: unsupported")
		}
		return nil
	}

	if len(stateMachine.commonFlags.HooksDirectories) == 0 {
		// no hooks, move on
		return nil
	}

	err := stateMachine.runHooks("post-populate-rootfs",
		"UBUNTU_IMAGE_HOOK_ROOTFS", stateMachine.tempDirs.rootfs)
	if err != nil {
		return err
	}

	return nil
}

// If --disk-info was used, copy the provided file to the correct location
func (stateMachine *StateMachine) generateDiskInfo() error {
	if stateMachine.commonFlags.DiskInfo != "" {
		diskInfoDir := filepath.Join(stateMachine.tempDirs.rootfs, ".disk")
		if err := osMkdir(diskInfoDir, 0755); err != nil {
			return fmt.Errorf("Failed to create disk info directory: %s", err.Error())
		}
		diskInfoFile := filepath.Join(diskInfoDir, "info")
		err := osutilCopyFile(stateMachine.commonFlags.DiskInfo, diskInfoFile, osutil.CopyFlagDefault)
		if err != nil {
			return fmt.Errorf("Failed to copy Disk Info file: %s", err.Error())
		}
	}
	return nil
}

// Calculate the size of the root filesystem
// on a 100MiB filesystem, ext4 takes a little over 7MiB for the
// metadata. Use 8MB as a minimum padding here
func (stateMachine *StateMachine) calculateRootfsSize() error {
	rootfsSize, err := helper.Du(stateMachine.tempDirs.rootfs)
	if err != nil {
		return fmt.Errorf("Error getting rootfs size: %s", err.Error())
	}
	var rootfsQuantity quantity.Size = rootfsSize
	rootfsPadding := 8 * quantity.SizeMiB
	rootfsQuantity += rootfsPadding

	// fudge factor for incidentals
	rootfsQuantity += (rootfsQuantity / 2)

	stateMachine.rootfsSize = rootfsQuantity
	return nil
}

// Populate the Bootfs Contents by using snapd's MountedFilesystemWriter
func (stateMachine *StateMachine) populateBootfsContents() error {
	// find the name of the system volume. snapd functions have already verified it exists
	var systemVolumeName string
	var systemVolume *gadget.Volume
	for volumeName, volume := range stateMachine.gadgetInfo.Volumes {
		for _, structure := range volume.Structure {
			// use the system-boot role to identify the system volume
			if structure.Role == gadget.SystemBoot || structure.Label == gadget.SystemBoot {
				systemVolumeName = volumeName
				systemVolume = volume
			}
		}
	}

	// now call LayoutVolume to get a LaidOutVolume we can use
	// with a mountedFilesystemWriter
	layoutConstraints := gadget.LayoutConstraints{SkipResolveContent: false}
	laidOutVolume, err := gadgetLayoutVolume(
		filepath.Join(stateMachine.tempDirs.unpack, "gadget"),
		filepath.Join(stateMachine.tempDirs.unpack, "kernel"),
		systemVolume, layoutConstraints)
	if err != nil {
		return fmt.Errorf("Error laying out bootfs contents: %s", err.Error())
	}

	for ii, laidOutStructure := range laidOutVolume.LaidOutStructure {
		var targetDir string
		if laidOutStructure.Role == gadget.SystemSeed {
			targetDir = stateMachine.tempDirs.rootfs
		} else {
			targetDir = filepath.Join(stateMachine.tempDirs.volumes,
				systemVolumeName,
				"part"+strconv.Itoa(ii))
		}
		// Bad special-casing.  snapd's image.Prepare currently
		// installs to /boot/grub, but we need to map this to
		// /EFI/ubuntu.  This is because we are using a SecureBoot
		// signed bootloader image which has this path embedded, so
		// we need to install our files to there.
		if !stateMachine.isSeeded &&
			(laidOutStructure.Role == gadget.SystemBoot ||
				laidOutStructure.Label == gadget.SystemBoot) {
			if err := stateMachine.handleSecureBoot(systemVolume, targetDir); err != nil {
				return err
			}
		}
		if laidOutStructure.HasFilesystem() {
			mountedFilesystemWriter, err := gadgetNewMountedFilesystemWriter(&laidOutStructure, nil)
			if err != nil {
				return fmt.Errorf("Error creating NewMountedFilesystemWriter: %s", err.Error())
			}

			err = mountedFilesystemWriter.Write(targetDir, []string{})
			if err != nil {
				return fmt.Errorf("Error in mountedFilesystem.Write(): %s", err.Error())
			}
		}
	}
	return nil
}

// Populate and prepare the partitions. For partitions without filesystem: specified in
// gadget.yaml, this involves using dd to copy the content blobs into a .img file. For
// partitions that do have filesystem: specified, we use the Mkfs functions from snapd.
// Throughout this process, the offset is tracked to ensure partitions are not overlapping.
func (stateMachine *StateMachine) populatePreparePartitions() error {
	// iterate through all the volumes
	for volumeName, volume := range stateMachine.gadgetInfo.Volumes {
		if err := stateMachine.handleLkBootloader(volume); err != nil {
			return err
		}
		var farthestOffset quantity.Offset = 0
		for structureNumber, structure := range volume.Structure {
			var contentRoot string
			if structure.Role == gadget.SystemData || structure.Role == gadget.SystemSeed {
				contentRoot = stateMachine.tempDirs.rootfs
			} else {
				contentRoot = filepath.Join(stateMachine.tempDirs.volumes, volumeName,
					"part"+strconv.Itoa(structureNumber))
			}
			var offset quantity.Offset
			if structure.Offset != nil {
				offset = *structure.Offset
			} else {
				offset = 0
			}
			farthestOffset = maxOffset(farthestOffset,
				quantity.Offset(structure.Size)+offset)
			if shouldSkipStructure(structure, stateMachine.isSeeded) {
				continue
			}

			// copy the data
			partImg := filepath.Join(stateMachine.tempDirs.volumes, volumeName,
				"part"+strconv.Itoa(structureNumber)+".img")
			if err := stateMachine.copyStructureContent(structure,
				contentRoot, partImg); err != nil {
				return err
			}
		}
		// set the image size values to be used by make_disk
		stateMachine.handleContentSizes(farthestOffset, volumeName)
	}
	return nil
}

// Make the disk
func (stateMachine *StateMachine) makeDisk() error {
	return nil
}

// Finish step to show that the build was successful
func (stateMachine *StateMachine) finish() error {
	return nil
}
