package statemachine

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"

	diskfs "github.com/diskfs/go-diskfs"
	diskutils "github.com/diskfs/go-diskfs/disk"
	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/gadget/quantity"
	"github.com/snapcore/snapd/osutil"

	"github.com/canonical/ubuntu-image/internal/helper"
)

var setArtifactNamesState = stateFunc{"set_artifact_names", (*StateMachine).setArtifactNames}

// for snap/core image builds, the image name is always <volume-name>.img for
// each volume in the gadget. This function stores that info in the struct
func (stateMachine *StateMachine) setArtifactNames() error {
	stateMachine.VolumeNames = make(map[string]string)
	for volumeName := range stateMachine.GadgetInfo.Volumes {
		stateMachine.VolumeNames[volumeName] = volumeName + ".img"
	}
	return nil
}

var loadGadgetYamlState = stateFunc{"load_gadget_yaml", (*StateMachine).loadGadgetYaml}

// Load gadget.yaml, do some validation, and store the relevant info in the StateMachine struct
func (stateMachine *StateMachine) loadGadgetYaml() error {
	gadgetYamlDst := filepath.Join(stateMachine.stateMachineFlags.WorkDir, "gadget.yaml")
	if err := osutilCopyFile(stateMachine.YamlFilePath,
		gadgetYamlDst, osutil.CopyFlagOverwrite); err != nil {
		return fmt.Errorf(`Error copying gadget.yaml to %s: %s
The gadget.yaml file is expected to be located in a "meta" subdirectory of the provided built gadget directory.
`, gadgetYamlDst, err.Error())
	}

	// read in the gadget.yaml as bytes, because snapd expects it that way
	gadgetYamlBytes, err := osReadFile(stateMachine.YamlFilePath)
	if err != nil {
		return fmt.Errorf("Error reading gadget.yaml bytes: %s", err.Error())
	}

	stateMachine.GadgetInfo, err = gadget.InfoFromGadgetYaml(gadgetYamlBytes, nil)
	if err != nil {
		return fmt.Errorf("Error running InfoFromGadgetYaml: %s", err.Error())
	}

	err = gadget.Validate(stateMachine.GadgetInfo, nil, nil)
	if err != nil {
		return fmt.Errorf("invalid gadget: %s", err.Error())
	}

	// check if the unpack dir should be preserved
	err = preserveUnpack(stateMachine.tempDirs.unpack)
	if err != nil {
		return err
	}

	err = stateMachine.validateVolumes()
	if err != nil {
		return err
	}

	// for the --image-size argument, the order of the volumes specified in gadget.yaml
	// must be preserved. However, since gadget.Info stores the volumes as a map, the
	// order is not preserved. We use the already read-in gadget.yaml file to store the
	// order of the volumes in a slice in the StateMachine struct
	stateMachine.saveVolumeOrder(string(gadgetYamlBytes))

	if err := stateMachine.postProcessGadgetYaml(); err != nil {
		return err
	}

	if err := stateMachine.parseImageSizes(); err != nil {
		return err
	}

	// pre-parse the sector size argument here as it's a string and we will be using it
	// in various places
	stateMachine.SectorSize, err = quantity.ParseSize(stateMachine.commonFlags.SectorSize)
	if err != nil {
		return err
	}

	return nil
}

// preserveUnpack checks if and does preserve the gadget unpack directory
func preserveUnpack(unpackDir string) error {
	preserveUnpackDir := os.Getenv("UBUNTU_IMAGE_PRESERVE_UNPACK")
	if len(preserveUnpackDir) == 0 {
		return nil
	}
	err := osMkdirAll(preserveUnpackDir, 0755)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("Error creating preserve unpack directory: %s", err.Error())
	}
	if err := osutilCopySpecialFile(unpackDir, preserveUnpackDir); err != nil {
		return fmt.Errorf("Error preserving unpack dir: %s", err.Error())
	}
	return nil
}

// validateVolumes checks there is at least one volume in the gadget
func (stateMachine *StateMachine) validateVolumes() error {
	if len(stateMachine.GadgetInfo.Volumes) < 1 {
		return fmt.Errorf("no volume in the gadget.yaml. Specify at least one volume.")
	}
	return nil
}

var generateDiskInfoState = stateFunc{"generate_disk_info", (*StateMachine).generateDiskInfo}

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

var calculateRootfsSizeState = stateFunc{"calculate_rootfs_size", (*StateMachine).calculateRootfsSize}

// calculateRootfsSize calculates the size of the root filesystem.
// On a 100MiB filesystem, ext4 takes a little over 7MiB for the
// metadata, so use 8MB as a minimum padding.
func (stateMachine *StateMachine) calculateRootfsSize() error {
	rootfsSize, err := helper.Du(stateMachine.tempDirs.rootfs)
	if err != nil {
		return fmt.Errorf("Error getting rootfs size: %s", err.Error())
	}

	// fudge factor for incidentals
	rootfsPadding := 8 * quantity.SizeMiB
	rootfsSize = quantity.Size(math.Ceil(float64(rootfsSize) * 1.5))
	rootfsSize += rootfsPadding

	stateMachine.RootfsSize = stateMachine.alignToSectorSize(rootfsSize)

	if stateMachine.commonFlags.Size != "" {
		rootfsVolume, rootfsVolumeName := stateMachine.findRootfsVolume()
		desiredSize := stateMachine.ImageSizes[rootfsVolumeName]

		// subtract the size and offsets of the existing volumes
		if rootfsVolume != nil {
			for _, structure := range rootfsVolume.Structure {
				desiredSize = helper.SafeQuantitySubtraction(desiredSize, structure.Size)
				if structure.Offset != nil {
					desiredSize = helper.SafeQuantitySubtraction(desiredSize,
						quantity.Size(*structure.Offset))
				}
			}

			desiredSize = stateMachine.alignToSectorSize(desiredSize)

			if desiredSize < stateMachine.RootfsSize {
				return fmt.Errorf("Error: calculated rootfs partition size %d is smaller "+
					"than actual rootfs contents (%d). Try using a larger value of "+
					"--image-size",
					desiredSize, stateMachine.RootfsSize,
				)
			}

			stateMachine.RootfsSize = desiredSize
		}
	}

	stateMachine.syncGadgetStructureRootfsSize()
	return nil
}

// findRootfsVolume finds the volume associated to the rootfs
func (stateMachine *StateMachine) findRootfsVolume() (*gadget.Volume, string) {
	for volumeName, volume := range stateMachine.GadgetInfo.Volumes {
		for _, structure := range volume.Structure {
			if structure.Size == 0 {
				return volume, volumeName
			}
		}
	}
	return nil, ""
}

// alignToSectorSize align the given size to the SectorSize of the stateMachine
func (stateMachine *StateMachine) alignToSectorSize(size quantity.Size) quantity.Size {
	return quantity.Size(math.Ceil(float64(size)/float64(stateMachine.SectorSize))) *
		quantity.Size(stateMachine.SectorSize)
}

// syncGadgetStructureRootfsSize synchronizes size of the gadget.Structure that
// represents the rootfs with the RootfsSize value of the statemachine
// This functions assumes stateMachine.RootfsSize was previously correctly updated.
func (stateMachine *StateMachine) syncGadgetStructureRootfsSize() {
	for _, volume := range stateMachine.GadgetInfo.Volumes {
		for structIndex, structure := range volume.Structure {
			if structure.Size == 0 {
				structure.Size = stateMachine.RootfsSize
			}
			volume.Structure[structIndex] = structure
		}
	}
}

var populateBootfsContentsState = stateFunc{"populate_bootfs_contents", (*StateMachine).populateBootfsContents}

// Populate the Bootfs Contents by using snapd's MountedFilesystemWriter
func (stateMachine *StateMachine) populateBootfsContents() error {
	var preserve []string
	for _, volumeName := range stateMachine.VolumeOrder {
		volume := stateMachine.GadgetInfo.Volumes[volumeName]
		// piboot modifies the original config.txt from the gadget,
		// avoid overwriting with the one coming from the gadget
		if volume.Bootloader == "piboot" {
			preserve = append(preserve, "config.txt")
		}

		// Get a LaidOutVolume we can use with a mountedFilesystemWriter
		laidOutVolume, err := stateMachine.layoutVolume(volume)
		if err != nil {
			return err
		}

		for i, laidOutStructure := range laidOutVolume.LaidOutStructure {
			err = stateMachine.populateBootfsLayoutStructure(laidOutStructure, laidOutVolume, i, volume, volumeName, preserve)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// layoutVolume generates a LaidOutVolume to be used with a gadget.NewMountedFilesystemWriter
func (stateMachine *StateMachine) layoutVolume(volume *gadget.Volume) (*gadget.LaidOutVolume, error) {
	layoutOptions := &gadget.LayoutOptions{
		SkipResolveContent: false,
		IgnoreContent:      false,
		GadgetRootDir:      filepath.Join(stateMachine.tempDirs.unpack, "gadget"),
		KernelRootDir:      filepath.Join(stateMachine.tempDirs.unpack, "kernel"),
	}
	laidOutVolume, err := gadgetLayoutVolume(volume,
		gadget.OnDiskStructsFromGadget(volume), layoutOptions)
	if err != nil {
		return nil, fmt.Errorf("Error laying out bootfs contents: %s", err.Error())
	}

	return laidOutVolume, nil
}

// populateBootfsLayoutStructure write a laidOutStructure to the associated target directory
func (stateMachine *StateMachine) populateBootfsLayoutStructure(laidOutStructure gadget.LaidOutStructure, laidOutVolume *gadget.LaidOutVolume, index int, volume *gadget.Volume, volumeName string, preserve []string) error {
	var targetDir string
	if laidOutStructure.Role() == gadget.SystemSeed {
		targetDir = stateMachine.tempDirs.rootfs
	} else {
		targetDir = filepath.Join(stateMachine.tempDirs.volumes,
			volumeName,
			"part"+strconv.Itoa(index))
	}
	// Bad special-casing.  snapd's image.Prepare currently
	// installs to /boot/grub, but we need to map this to
	// /EFI/ubuntu.  This is because we are using a SecureBoot
	// signed bootloader image which has this path embedded, so
	// we need to install our files to there.
	if !stateMachine.IsSeeded &&
		(laidOutStructure.Role() == gadget.SystemBoot ||
			laidOutStructure.Label() == gadget.SystemBoot) {
		if err := stateMachine.handleSecureBoot(volume, targetDir); err != nil {
			return err
		}
	}
	if laidOutStructure.HasFilesystem() {
		mountedFilesystemWriter, err := gadgetNewMountedFilesystemWriter(nil, &laidOutVolume.LaidOutStructure[index], nil)
		if err != nil {
			return fmt.Errorf("Error creating NewMountedFilesystemWriter: %s", err.Error())
		}

		err = mountedFilesystemWriter.Write(targetDir, preserve)
		if err != nil {
			return fmt.Errorf("Error in mountedFilesystem.Write(): %s", err.Error())
		}
	}
	return nil
}

var populatePreparePartitionsState = stateFunc{"populate_prepare_partitions", (*StateMachine).populatePreparePartitions}

// Populate and prepare the partitions. For partitions without "filesystem:" specified in
// gadget.yaml, this involves using dd to copy the content blobs into a .img file. For
// partitions that do have "filesystem:" specified, we use the Mkfs functions from snapd.
// Throughout this process, the offset is tracked to ensure partitions are not overlapping.
func (stateMachine *StateMachine) populatePreparePartitions() error {
	for _, volumeName := range stateMachine.VolumeOrder {
		volume := stateMachine.GadgetInfo.Volumes[volumeName]
		if err := stateMachine.handleLkBootloader(volume); err != nil {
			return err
		}
		for structIndex, structure := range volume.Structure {
			var contentRoot string
			if structure.Role == gadget.SystemData || structure.Role == gadget.SystemSeed {
				contentRoot = stateMachine.tempDirs.rootfs
			} else {
				contentRoot = filepath.Join(stateMachine.tempDirs.volumes, volumeName,
					"part"+strconv.Itoa(structIndex))
			}
			if shouldSkipStructure(structure, stateMachine.IsSeeded) {
				continue
			}

			// copy the data
			partImg := filepath.Join(stateMachine.tempDirs.volumes, volumeName,
				"part"+strconv.Itoa(structIndex)+".img")
			if err := stateMachine.copyStructureContent(volume, structure,
				structIndex, contentRoot, partImg); err != nil {
				return err
			}
		}
		// Set the image size values to be used by make_disk, by using
		// the minimum size that would be valid according to gadget.yaml.
		stateMachine.handleContentSizes(quantity.Offset(volume.MinSize()), volumeName)
	}
	return nil
}

var makeDiskState = stateFunc{"make_disk", (*StateMachine).makeDisk}

// Make the disk
func (stateMachine *StateMachine) makeDisk() error {
	for volumeName, volume := range stateMachine.GadgetInfo.Volumes {
		_, found := stateMachine.VolumeNames[volumeName]
		if !found {
			continue
		}
		imgName := filepath.Join(stateMachine.commonFlags.OutputDir, stateMachine.VolumeNames[volumeName])

		diskImg, err := stateMachine.createDiskImage(volumeName, volume, imgName)
		if err != nil {
			return err
		}

		partitionTable, rootfsPartitionNumber := generatePartitionTable(volume, uint64(stateMachine.SectorSize), stateMachine.IsSeeded)

		// Save the rootfs partition number, if found, for later use
		if rootfsPartitionNumber != -1 {
			stateMachine.RootfsVolName = volumeName
			stateMachine.RootfsPartNum = rootfsPartitionNumber
		}

		if err := diskImg.Partition(*partitionTable); err != nil {
			return fmt.Errorf("Error partitioning image file: %s", err.Error())
		}

		// TODO: go-diskfs doesn't set the disk ID when using an MBR partition table.
		// this function is a temporary workaround, but we should change upstream go-diskfs
		if volume.Schema == schemaMBR {
			err = fixDiskIDOnMBR(imgName)
			if err != nil {
				return err
			}
		}

		// After the partitions have been created, copy the data into the correct locations
		if err := stateMachine.copyDataToImage(volumeName, volume, diskImg); err != nil {
			return err
		}

		// Open the file and write any OffsetWrite values
		if err := writeOffsetValues(volume, imgName, uint64(stateMachine.SectorSize), uint64(diskImg.Size)); err != nil {
			return err
		}
	}
	return nil
}

// createDiskImage creates a disk image and makes sure the size respects the configuration and
// the SectorSize
func (stateMachine *StateMachine) createDiskImage(volumeName string, volume *gadget.Volume, imgName string) (*diskutils.Disk, error) {
	imgSize, found := stateMachine.ImageSizes[volumeName]
	if !found {
		// Calculate the minimum size that would be valid according to gadget.yaml.
		imgSize = volume.MinSize()
	}
	if err := osRemoveAll(imgName); err != nil {
		return nil, fmt.Errorf("Error removing old disk image: %s", err.Error())
	}

	sectorSizeDiskfs := diskfs.SectorSize(int(stateMachine.SectorSize))
	imgSize = stateMachine.alignToSectorSize(imgSize)

	diskImg, err := diskfsCreate(imgName, int64(imgSize), diskfs.Raw, sectorSizeDiskfs)
	if err != nil {
		return nil, fmt.Errorf("Error creating disk image: %s", err.Error())
	}

	return diskImg, nil
}
