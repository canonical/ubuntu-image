package statemachine

import (
	"crypto/rand"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"github.com/canonical/ubuntu-image/internal/helper"
	diskfs "github.com/diskfs/go-diskfs"
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
	if err := osutilCopyFile(stateMachine.YamlFilePath,
		gadgetYamlDst, osutil.CopyFlagOverwrite); err != nil {
		return fmt.Errorf("Error copying gadget.yaml to %s: %s", gadgetYamlDst, err.Error())
	}

	// read in the gadget.yaml as bytes, because snapd expects it that way
	gadgetYamlBytes, err := ioutilReadFile(stateMachine.YamlFilePath)
	if err != nil {
		return fmt.Errorf("Error reading gadget.yaml bytes: %s", err.Error())
	}

	stateMachine.GadgetInfo, err = gadget.InfoFromGadgetYaml(gadgetYamlBytes, nil)
	if err != nil {
		return fmt.Errorf("Error running InfoFromGadgetYaml: %s", err.Error())
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
	if stateMachine.IsSeeded {
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
	RootfsSize, err := helper.Du(stateMachine.tempDirs.rootfs)
	if err != nil {
		return fmt.Errorf("Error getting rootfs size: %s", err.Error())
	}
	var rootfsQuantity quantity.Size = RootfsSize
	rootfsPadding := 8 * quantity.SizeMiB

	// fudge factor for incidentals
	rootfsQuantity = quantity.Size(math.Ceil(float64(rootfsQuantity) * 1.5))
	rootfsQuantity += rootfsPadding

	stateMachine.RootfsSize = rootfsQuantity

	// we have already saved the rootfs size in the state machine struct, but we
	// should also set it in the gadget.Structure that represents the rootfs
	for _, volume := range stateMachine.GadgetInfo.Volumes {
		for structureNumber, structure := range volume.Structure {
			if structure.Size == 0 {
				structure.Size = rootfsQuantity
			}
			volume.Structure[structureNumber] = structure
		}
	}
	return nil
}

// Populate the Bootfs Contents by using snapd's MountedFilesystemWriter
func (stateMachine *StateMachine) populateBootfsContents() error {
	// find the name of the system volume. snapd functions have already verified it exists
	var systemVolumeName string
	var systemVolume *gadget.Volume
	for volumeName, volume := range stateMachine.GadgetInfo.Volumes {
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
		if !stateMachine.IsSeeded &&
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
	for volumeName, volume := range stateMachine.GadgetInfo.Volumes {
		if err := stateMachine.handleLkBootloader(volume); err != nil {
			return err
		}
		var farthestOffset quantity.Offset = 0
		//for structureNumber, structure := range volume.Structure {
		for structureNumber, structure := range volume.Structure {
			var contentRoot string
			if structure.Role == gadget.SystemData || structure.Role == gadget.SystemSeed {
				contentRoot = stateMachine.tempDirs.rootfs
			} else {
				contentRoot = filepath.Join(stateMachine.tempDirs.volumes, volumeName,
					"part"+strconv.Itoa(structureNumber))
			}
			farthestOffset = maxOffset(farthestOffset,
				quantity.Offset(structure.Size)+getStructureOffset(structure))
			if shouldSkipStructure(structure, stateMachine.IsSeeded) {
				continue
			}

			// copy the data
			partImg := filepath.Join(stateMachine.tempDirs.volumes, volumeName,
				"part"+strconv.Itoa(structureNumber)+".img")
			if err := stateMachine.copyStructureContent(volume, structure,
				structureNumber, contentRoot, partImg); err != nil {
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
	// ensure the output dir exists
	if stateMachine.commonFlags.OutputDir == "" {
		if stateMachine.cleanWorkDir { // no workdir specified, so create the image in the pwd
			stateMachine.commonFlags.OutputDir, _ = os.Getwd()
		} else {
			stateMachine.commonFlags.OutputDir = stateMachine.stateMachineFlags.WorkDir
		}
	} else {
		err := osMkdirAll(stateMachine.commonFlags.OutputDir, 0755)
		if err != nil && !os.IsExist(err) {
			return fmt.Errorf("Error creating OutputDir: %s", err.Error())
		}
	}
	for volumeName, volume := range stateMachine.GadgetInfo.Volumes {
		imgName := filepath.Join(stateMachine.commonFlags.OutputDir, volumeName+".img")

		// Create the disk image
		imgSize, _ := stateMachine.calculateImageSize()

		diskImg, err := diskfsCreate(imgName, imgSize, diskfs.Raw)
		if err != nil {
			return fmt.Errorf("Error creating disk image: %s", err.Error())
		}

		// snapd always populates Schema, so it cannot be empty. Use the blocksize of the created disk
		sectorSize := uint64(diskImg.LogicalBlocksize)

		// set up the partitions on the device
		partitionTable := createPartitionTable(volumeName, volume, sectorSize, stateMachine.IsSeeded)

		// Write the partition table to disk
		if err := diskImg.Partition(*partitionTable); err != nil {
			return fmt.Errorf("Error partitioning image file: %s", err.Error())
		}

		// TODO: go-diskfs doesn't set the disk ID when using an MBR partition table.
		// this function is a temporary workaround, but we should change upstream go-diskfs
		if volume.Schema == "mbr" {
			randomBytes := make([]byte, 4)
			rand.Read(randomBytes)
			diskFile, err := osOpenFile(imgName, os.O_RDWR, 0755)
			defer diskFile.Close()
			if err != nil {
				return fmt.Errorf("Error opening disk to write MBR disk identifier: %s",
					err.Error())
			}
			_, err = diskFile.WriteAt(randomBytes, 440)
			if err != nil {
				return fmt.Errorf("Error writing MBR disk identifier: %s", err.Error())
			}
			diskFile.Close()
		}

		// After the partitions have been created, copy the data into the correct locations
		if err := stateMachine.copyDataToImage(volumeName, volume, diskImg); err != nil {
			return err
		}

		// Open the file and write any OffsetWrite values
		if err := writeOffsetValues(volume, imgName, sectorSize, uint64(imgSize)); err != nil {
			return err
		}
	}
	return nil
}

// Finish step to show that the build was successful
func (stateMachine *StateMachine) finish() error {
	return nil
}
