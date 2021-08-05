package statemachine

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/google/uuid"
	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/osutil"
)

// generate work directory file structure
func (stateMachine *StateMachine) makeTemporaryDirectories() error {
	// if no workdir was specified, open a /tmp dir
	if stateMachine.stateMachineFlags.WorkDir == "" {
		stateMachine.stateMachineFlags.WorkDir = "/tmp/ubuntu-image-" + uuid.NewString()
		if err := os.Mkdir(stateMachine.stateMachineFlags.WorkDir, 0755); err != nil {
			return fmt.Errorf("Failed to create temporary directory: %s", err.Error())
		}
		stateMachine.cleanWorkDir = true
	} else {
		err := os.MkdirAll(stateMachine.stateMachineFlags.WorkDir, 0755)
		if err != nil && !os.IsExist(err) {
			return fmt.Errorf("Error creating work directory: %s", err.Error())
		}
	}

	stateMachine.tempDirs.rootfs = stateMachine.stateMachineFlags.WorkDir + "/root"
	stateMachine.tempDirs.unpack = stateMachine.stateMachineFlags.WorkDir + "/unpack"
	stateMachine.tempDirs.volumes = stateMachine.stateMachineFlags.WorkDir + "/volumes"

	if err := os.Mkdir(stateMachine.tempDirs.rootfs, 0755); err != nil {
		return fmt.Errorf("Error creating temporary directory: %s", err.Error())
	}

	return nil
}

// Load gadget.yaml, do some validation, and store the relevant info in the StateMachine struct
func (stateMachine *StateMachine) loadGadgetYaml() error {
        if err := osutilCopySpecialFile(stateMachine.yamlFilePath,
                stateMachine.stateMachineFlags.WorkDir); err != nil {
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

        var rootfsSeen bool = false
        var farthestOffset quantity.Offset = 0
        var lastOffset quantity.Offset = 0
        for volumeName, volume := range stateMachine.gadgetInfo.Volumes {
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
                                stateMachine.hooksAllowed = false
                        }

                        fmt.Println(structure)
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
                        if lastOffset > farthestOffset {
                                farthestOffset = lastOffset
                                fmt.Printf("JAWN setting farthestOffset to %s. offset is %s and size is %s\n", strconv.FormatUint(uint64(farthestOffset), 10), strconv.FormatUint(uint64(offset), 10), strconv.FormatUint(uint64(structure.Size), 10))
                        }
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

                // TODO: un-hardcode this
                stateMachine.gadgetInfo.Volumes["pc"].Structure =
                        append(stateMachine.gadgetInfo.Volumes["pc"].Structure, rootfsStructure)
        }

        // check if the unpack dir should be preserved
        envar := os.Getenv("UBUNTU_IMAGE_PRESERVE_UNPACK")
        if envar != "" {
                preserveDir := filepath.Join(envar, "unpack")
                if err := osutilCopySpecialFile(stateMachine.tempDirs.unpack, preserveDir); err != nil {
                        return fmt.Errorf("Error preserving unpack dir: %s", err.Error())
                }
        }

        return nil
}

// Populate the image's rootfs contents
func (stateMachine *StateMachine) populateRootfsContents() error {
	return nil
}

// Run hooks for populating rootfs contents
func (stateMachine *StateMachine) populateRootfsContentsHooks() error {
	return nil
}

// Generate the disk info
func (stateMachine *StateMachine) generateDiskInfo() error {
	return nil
}

// Calculate the rootfs size
func (stateMachine *StateMachine) calculateRootfsSize() error {
	return nil
}

// Pre populate the bootfs contents
func (stateMachine *StateMachine) prepopulateBootfsContents() error {
	return nil
}

// Populate the Bootfs Contents
func (stateMachine *StateMachine) populateBootfsContents() error {
	return nil
}

// Populate and prepare the partitions
func (stateMachine *StateMachine) populatePreparePartitions() error {
	return nil
}

// Make the disk
func (stateMachine *StateMachine) makeDisk() error {
	return nil
}

// Generate the manifest
func (stateMachine *StateMachine) generateManifest() error {
	return nil
}

// Finish step to show that the build was successful
func (stateMachine *StateMachine) finish() error {
	return nil
}
