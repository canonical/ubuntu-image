package statemachine

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
	stateMachine.tempDirs.chroot = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "chroot")
	stateMachine.tempDirs.scratch = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "scratch")

	tempDirs := []string{stateMachine.tempDirs.scratch, stateMachine.tempDirs.rootfs}
	for _, tempDir := range tempDirs {
		err := osMkdir(tempDir, 0755)
		if err != nil && !os.IsExist(err) {
			return fmt.Errorf("Error creating temporary directory \"%s\": \"%s\"", tempDir, err.Error())
		}
	}

	return nil
}

// determineOutputDirectory sets the directory in which to place artifacts
// and creates it if it doesn't already exist
func (stateMachine *StateMachine) determineOutputDirectory() error {
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
	gadgetYamlBytes, err := osReadFile(stateMachine.YamlFilePath)
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

	// for the --image-size argument, the order of the volumes specified in gadget.yaml
	// must be preserved. However, since gadget.Info stores the volumes as a map, the
	// order is not preserved. We use the already read-in gadget.yaml file to store the
	// order of the volumes as an array in the StateMachine struct
	stateMachine.saveVolumeOrder(string(gadgetYamlBytes))

	if err := stateMachine.postProcessGadgetYaml(); err != nil {
		return err
	}

	if err := stateMachine.parseImageSizes(); err != nil {
		return err
	}

	// pre-parse the sector size argument here as it's a string and we will be using it
	// in various places
	stateMachine.SectorSize, _ = quantity.ParseSize(stateMachine.commonFlags.SectorSize)

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
	// use `du` to calculate the size of the rootfs
	rootfsSize, err := helper.Du(stateMachine.tempDirs.rootfs)
	if err != nil {
		return fmt.Errorf("Error getting rootfs size: %s", err.Error())
	}
	var rootfsQuantity quantity.Size = rootfsSize

	// fudge factor for incidentals
	rootfsPadding := 8 * quantity.SizeMiB
	rootfsQuantity = quantity.Size(math.Ceil(float64(rootfsQuantity) * 1.5))
	rootfsQuantity += rootfsPadding

	// align the size of the rootfs to sector size
	rootfsQuantity = quantity.Size(math.Ceil(float64(rootfsQuantity)/float64(stateMachine.SectorSize))) *
		quantity.Size(stateMachine.SectorSize)

	stateMachine.RootfsSize = rootfsQuantity

	if stateMachine.commonFlags.Size != "" {
		var parsedSize quantity.Size

		// identify which structure has the rootfs
		var rootfsVolume *gadget.Volume
		var rootfsVolumeName string
		for volumeName, volume := range stateMachine.GadgetInfo.Volumes {
			for _, structure := range volume.Structure {
				if structure.Size == 0 {
					rootfsVolume = volume
					rootfsVolumeName = volumeName
					break
				}
			}
		}

		if !strings.Contains(stateMachine.commonFlags.Size, ":") {
			// this scenario has just one size for each volume
			// no need to check error as it has already been done by
			// the parseImageSizes function
			parsedSize, _ = quantity.ParseSize(stateMachine.commonFlags.Size)
		} else {
			parsedSize = stateMachine.ImageSizes[rootfsVolumeName]
		}

		// subtract the size and offsets of the existing volumes
		if rootfsVolume != nil {
			for _, structure := range rootfsVolume.Structure {
				parsedSize = helper.SafeQuantitySubtraction(parsedSize, structure.Size)
				if structure.Offset != nil {
					parsedSize = helper.SafeQuantitySubtraction(parsedSize,
						quantity.Size(*structure.Offset))
				}
			}

			// align the size of the rootfs to sector size
			parsedSize = quantity.Size(math.Ceil(float64(parsedSize)/float64(stateMachine.SectorSize))) *
				quantity.Size(stateMachine.SectorSize)

			if parsedSize < stateMachine.RootfsSize {
				return fmt.Errorf("Error: calculated rootfs partition size %d is smaller "+
					"than actual rootfs contents (%d). Try using a larger value of "+
					"--image-size",
					parsedSize, stateMachine.RootfsSize,
				)
			}

			stateMachine.RootfsSize = parsedSize
		}
	}

	// we have already saved the rootfs size in the state machine struct, but we
	// should also set it in the gadget.Structure that represents the rootfs
	for _, volume := range stateMachine.GadgetInfo.Volumes {
		for structureNumber, structure := range volume.Structure {
			if structure.Size == 0 {
				structure.Size = stateMachine.RootfsSize
			}
			volume.Structure[structureNumber] = structure
		}
	}
	return nil
}

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

		// now call LayoutVolume to get a LaidOutVolume we can use
		// with a mountedFilesystemWriter
		layoutOptions := &gadget.LayoutOptions{
			SkipResolveContent: false,
			IgnoreContent:      false,
			GadgetRootDir:      filepath.Join(stateMachine.tempDirs.unpack, "gadget"),
			KernelRootDir:      filepath.Join(stateMachine.tempDirs.unpack, "kernel"),
		}
		laidOutVolume, err := gadgetLayoutVolume(volume, nil, layoutOptions)
		if err != nil {
			return fmt.Errorf("Error laying out bootfs contents: %s", err.Error())
		}

		for ii, laidOutStructure := range laidOutVolume.LaidOutStructure {
			var targetDir string
			if laidOutStructure.Role() == gadget.SystemSeed {
				targetDir = stateMachine.tempDirs.rootfs
			} else {
				targetDir = filepath.Join(stateMachine.tempDirs.volumes,
					volumeName,
					"part"+strconv.Itoa(ii))
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
				mountedFilesystemWriter, err := gadgetNewMountedFilesystemWriter(&laidOutStructure, nil)
				if err != nil {
					return fmt.Errorf("Error creating NewMountedFilesystemWriter: %s", err.Error())
				}

				err = mountedFilesystemWriter.Write(targetDir, preserve)
				if err != nil {
					return fmt.Errorf("Error in mountedFilesystem.Write(): %s", err.Error())
				}
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
	for _, volumeName := range stateMachine.VolumeOrder {
		volume := stateMachine.GadgetInfo.Volumes[volumeName]
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
	// TODO: this is only temporarily needed until go-diskfs is fixed - see below
	var existingDiskIds [][]byte
	for volumeName, volume := range stateMachine.GadgetInfo.Volumes {
		if _, found := stateMachine.VolumeNames[volumeName]; found {
			imgName := filepath.Join(stateMachine.commonFlags.OutputDir, stateMachine.VolumeNames[volumeName])

			// Create the disk image
			imgSize, found := stateMachine.ImageSizes[volumeName]
			if !found {
				imgSize, _ = stateMachine.calculateImageSize()
			}

			if err := osRemoveAll(imgName); err != nil {
				return fmt.Errorf("Error removing old disk image: %s", err.Error())
			}
			sectorSizeFlag := diskfs.SectorSize(int(stateMachine.SectorSize))
			diskImg, err := diskfsCreate(imgName, int64(imgSize), diskfs.Raw, sectorSizeFlag)
			if err != nil {
				return fmt.Errorf("Error creating disk image: %s", err.Error())
			}

			// make sure the disk image size is a multiple of its block/sector size
			imgSize = quantity.Size(math.Ceil(float64(imgSize)/float64(stateMachine.SectorSize))) *
				stateMachine.SectorSize
			if err := osTruncate(diskImg.File.Name(), int64(imgSize)); err != nil {
				return fmt.Errorf("Error resizing disk image to a multiple of its block size: %s",
					err.Error())
			}

			// set up the partitions on the device
			partitionTable, rootfsPartitionNumber := createPartitionTable(volumeName, volume, uint64(stateMachine.SectorSize), stateMachine.IsSeeded)

			// Save the rootfs partition number, if found, for later use
			if rootfsPartitionNumber != -1 {
				stateMachine.rootfsVolName = volumeName
				stateMachine.rootfsPartNum = rootfsPartitionNumber
			}

			// Write the partition table to disk
			if err := diskImg.Partition(*partitionTable); err != nil {
				return fmt.Errorf("Error partitioning image file: %s", err.Error())
			}

			// TODO: go-diskfs doesn't set the disk ID when using an MBR partition table.
			// this function is a temporary workaround, but we should change upstream go-diskfs
			if volume.Schema == "mbr" {
				randomBytes, err := generateUniqueDiskID(&existingDiskIds)
				if err != nil {
					return fmt.Errorf("Error generating disk ID: %s", err.Error())
				}
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
			if err := writeOffsetValues(volume, imgName, uint64(stateMachine.SectorSize), uint64(imgSize)); err != nil {
				return err
			}
		}
	}
	return nil
}

// Finish step to show that the build was successful
func (stateMachine *StateMachine) finish() error {
	return nil
}
