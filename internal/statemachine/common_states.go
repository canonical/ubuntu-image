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
	"github.com/canonical/ubuntu-image/internal/partition"
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

// Semi-arbitrary value, probably larger than needed but big enough to not have issues
// and low enough to stay in the same magnitude
const ext4FudgeFactor = 1.5

// On a 100MiB filesystem, ext4 takes a little over 7MiB for the
// metadata, so use 8MB as a minimum padding
const ext4Padding = 8 * quantity.SizeMiB

var calculateRootfsSizeState = stateFunc{"calculate_rootfs_size", (*StateMachine).calculateRootfsSize}

// calculateRootfsSize calculates the size needed by the root filesystem.
// If an image size was specified, make sure it is big enough to contain the
// rootfs and try to allocate it to the rootfs
func (stateMachine *StateMachine) calculateRootfsSize() error {
	rootfsMinSize, err := stateMachine.getRootfsMinSize()
	if err != nil {
		return err
	}

	stateMachine.RootfsSize = rootfsMinSize

	rootfsVolume, rootfsStructure := stateMachine.findRootfsVolumeStructure()

	gadgetRootfsMinSize := stateMachine.getGadgetRootfsMinSize(rootfsStructure)
	if gadgetRootfsMinSize > stateMachine.RootfsSize {
		stateMachine.RootfsSize = gadgetRootfsMinSize
	}

	desiredSize, foundDesiredSize := stateMachine.getRootfsDesiredSize(rootfsVolume)

	if foundDesiredSize {
		if stateMachine.RootfsSize > desiredSize {
			fmt.Printf("WARNING: rootfs content %d is bigger "+
				"than requested image size (%d). Try using a larger value of "+
				"--image-size",
				stateMachine.RootfsSize, desiredSize,
			)
		} else {
			stateMachine.RootfsSize = desiredSize
		}
	}

	if rootfsStructure != nil {
		raiseStructureSizes(rootfsStructure, stateMachine.RootfsSize)
	}

	return nil
}

// getRootfsMinSize gets the minimum size needed to hold the built rootfs directory
func (stateMachine *StateMachine) getRootfsMinSize() (quantity.Size, error) {
	rootfsMinSize, err := helper.Du(stateMachine.tempDirs.rootfs)
	if err != nil {
		return quantity.Size(0), fmt.Errorf("Error getting rootfs size: %s", err.Error())
	}

	// Take into account ext4 filesystems metadata size
	rootfsPadding := ext4Padding
	rootfsMinSize = quantity.Size(math.Ceil(float64(rootfsMinSize) * ext4FudgeFactor))
	rootfsMinSize += rootfsPadding

	return stateMachine.alignToSectorSize(rootfsMinSize), nil
}

// getGadgetRootfsMinSize gets the minimum size of the rootfs requested in the
// gadget YAML
func (stateMachine *StateMachine) getGadgetRootfsMinSize(rootfsStructure *gadget.VolumeStructure) quantity.Size {
	if rootfsStructure == nil {
		return quantity.Size(0)
	}

	return stateMachine.alignToSectorSize(rootfsStructure.MinSize)
}

// getRootfsDesiredSize subtracts the size and offsets of the existing structures
// from the requested image size to determine the maximum possible size of the rootfs
func (stateMachine *StateMachine) getRootfsDesiredSize(rootfsVolume *gadget.Volume) (quantity.Size, bool) {
	if rootfsVolume == nil {
		return quantity.Size(0), false
	}

	desiredSize, found := stateMachine.ImageSizes[rootfsVolume.Name]
	if !found {
		// So far we do not know of a desired size for the rootfs
		return quantity.Size(0), false
	}

	reservedSize := calculateNoRootfsSize(rootfsVolume)
	partitionSize := partition.PartitionTableSizeFromVolume(rootfsVolume, uint64(stateMachine.SectorSize), 0)
	reservedSize += quantity.Size(partitionSize)

	desiredSize = helper.SafeQuantitySubtraction(desiredSize, reservedSize)
	desiredSize = stateMachine.alignToSectorSize(desiredSize)

	return desiredSize, found
}

// calculateNoRootfsSize determines the needed space for existing structures
// except for the rootfs
func calculateNoRootfsSize(v *gadget.Volume) quantity.Size {
	var size quantity.Size
	for _, s := range v.Structure {
		if helper.IsRootfsStructure(&s) { //nolint:gosec,G301
			continue
		}
		if s.Offset != nil && quantity.Size(*s.Offset) > size {
			size = quantity.Size(*s.Offset)
		}
		size += s.MinSize
	}

	return size
}

// findRootfsVolumeStructure finds the volume and the structure associated to the rootfs
func (stateMachine *StateMachine) findRootfsVolumeStructure() (*gadget.Volume, *gadget.VolumeStructure) {
	for _, volume := range stateMachine.GadgetInfo.Volumes {
		for i := range volume.Structure {
			s := &volume.Structure[i]
			if helper.IsRootfsStructure(s) { //nolint:gosec,G301
				return volume, s
			}
		}
	}
	return nil, nil
}

// alignToSectorSize align the given size to the SectorSize of the stateMachine
func (stateMachine *StateMachine) alignToSectorSize(size quantity.Size) quantity.Size {
	return quantity.Size(math.Ceil(float64(size)/float64(stateMachine.SectorSize))) *
		quantity.Size(stateMachine.SectorSize)
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

// populatePreparePartitions populates and prepares the partitions. For partitions without
// "filesystem:" specified in gadget.yaml, this involves using dd to copy the content blobs
// into a .img file. For partitions that do have "filesystem:" specified, we use the Mkfs
// functions from snapd.
// Throughout this process, the offset is tracked to ensure partitions are not overlapping.
func (stateMachine *StateMachine) populatePreparePartitions() error {
	for _, volumeName := range stateMachine.VolumeOrder {
		volume := stateMachine.GadgetInfo.Volumes[volumeName]
		if err := stateMachine.handleLkBootloader(volume); err != nil {
			return err
		}
		for structIndex := range volume.Structure {
			structure := &volume.Structure[structIndex]
			if helper.ShouldSkipStructure(structure, stateMachine.IsSeeded) {
				continue
			}

			var contentRoot string
			if helper.IsRootfsStructure(structure) || helper.IsSystemSeedStructure(structure) { //nolint:gosec,G301
				contentRoot = stateMachine.tempDirs.rootfs
			} else {
				contentRoot = filepath.Join(stateMachine.tempDirs.volumes, volumeName,
					"part"+strconv.Itoa(structIndex))
			}

			// copy the data
			partImg := filepath.Join(stateMachine.tempDirs.volumes, volumeName,
				"part"+strconv.Itoa(structIndex)+".img")
			if err := stateMachine.copyStructureContent(structure,
				contentRoot, partImg); err != nil {
				return err
			}
		}
		// Make sure the register image size grows with the content
		stateMachine.growImageSize(volume.MinSize(), volumeName)
	}
	return nil
}

// growImageSize checks if the current size still fits in the requested
// image size and grows it if necessary
func (stateMachine *StateMachine) growImageSize(currentSize quantity.Size, volumeName string) {
	desiredMinSize, found := stateMachine.ImageSizes[volumeName]
	if !found {
		stateMachine.ImageSizes[volumeName] = currentSize
	} else {
		if desiredMinSize < currentSize {
			fmt.Printf("WARNING: ignoring image size smaller than "+
				"minimum required size: vol:%s %d < %d\n",
				volumeName, uint64(desiredMinSize), uint64(currentSize))
			stateMachine.ImageSizes[volumeName] = currentSize
		}
	}
}

var makeDiskState = stateFunc{"make_disk", (*StateMachine).makeDisk}

// makeDisk makes the disk image
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

		if err := stateMachine.partitionDisk(diskImg, volume, volumeName); err != nil {
			return err
		}

		// TODO: go-diskfs doesn't set the disk ID when using an MBR partition table.
		// this function is a temporary workaround, but we should change upstream go-diskfs
		if volume.Schema == partition.SchemaMBR {
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
	// Calculate the minimum size that would be needed according to gadget.yaml.
	imgSize := volume.Size()

	desiredImgSize, found := stateMachine.ImageSizes[volumeName]
	if found && desiredImgSize > imgSize {
		imgSize = desiredImgSize
	}
	if err := osRemoveAll(imgName); err != nil {
		return nil, fmt.Errorf("Error removing old disk image: %s", err.Error())
	}

	sectorSizeDiskfs := diskfs.SectorSize(int(stateMachine.SectorSize))
	imgSize = stateMachine.alignToSectorSize(imgSize)

	partitionSize := partition.PartitionTableSizeFromVolume(volume, uint64(stateMachine.SectorSize), 0)
	totalImgSize := int64(imgSize) + int64(partitionSize)

	diskImg, err := diskfsCreate(imgName, totalImgSize, diskfs.Raw, sectorSizeDiskfs)
	if err != nil {
		return nil, fmt.Errorf("Error creating disk image: %s", err.Error())
	}

	return diskImg, nil
}

// partitionDisk generates a partition table and applies it to the disk
func (stateMachine *StateMachine) partitionDisk(diskImg *diskutils.Disk, volume *gadget.Volume, volumeName string) error {
	partitionTable, rootfsPartitionNumber, err := partition.GeneratePartitionTable(volume, uint64(stateMachine.SectorSize), uint64(diskImg.Size), stateMachine.IsSeeded)
	if err != nil {
		return err
	}

	// Save the rootfs partition number, for later use
	// Store in any case, even if value is -1 to make it clear later it was not found
	stateMachine.RootfsPartNum = rootfsPartitionNumber
	if rootfsPartitionNumber != -1 {
		stateMachine.RootfsVolName = volumeName
	}

	if err := diskImg.Partition(partitionTable); err != nil {
		return fmt.Errorf("Error partitioning image file: %s", err.Error())
	}
	return nil
}
