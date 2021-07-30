package statemachine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/google/uuid"
	"github.com/inhies/go-bytesize"
	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/osutil"
)

// generate work directory file structure
func (stateMachine *StateMachine) makeTemporaryDirectories() error {
	// if no workdir was specified, open a /tmp dir
	fmt.Println("JAWN entered makeTemporaryDirectories")
	if stateMachine.stateMachineFlags.WorkDir == "" {
		fmt.Println("JAWN no workdir")
		stateMachine.stateMachineFlags.WorkDir = filepath.Join("/tmp", "ubuntu-image-"+uuid.NewString())
		if err := osMkdir(stateMachine.stateMachineFlags.WorkDir, 0755); err != nil {
			return fmt.Errorf("Failed to create temporary directory: %s", err.Error())
		}
		stateMachine.cleanWorkDir = true
	} else {
		fmt.Printf("JAWN workdir is %s", stateMachine.stateMachineFlags.WorkDir)
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

// Load the gadget yaml passed in via command line
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

	for volumeName := range stateMachine.gadgetInfo.Volumes {
		volumeBaseDir := filepath.Join(stateMachine.tempDirs.volumes, volumeName)
		if err := osMkdirAll(volumeBaseDir, 0755); err != nil {
			return fmt.Errorf("Error creating volume dir: %s", err.Error())
		}
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

// Run hooks for populating rootfs contents
func (stateMachine *StateMachine) populateRootfsContentsHooks() error {
	if !stateMachine.hooksAllowed {
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

// Generate the disk info
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
	rootfsBytes := bytesize.New(rootfsSize)
	rootfsPadding, _ := bytesize.Parse("8MB")
	rootfsBytes += rootfsPadding

	// fudge factor for incidentals
	rootfsBytes += (rootfsBytes / 2)

	stateMachine.rootfsSize = rootfsBytes
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

// Clean up and organize files
func (stateMachine *StateMachine) finish() error {
	if stateMachine.cleanWorkDir {
		if err := stateMachine.cleanup(); err != nil {
			return err
		}
	}
	return nil
}
