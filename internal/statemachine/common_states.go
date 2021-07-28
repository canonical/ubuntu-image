package statemachine

import (
	"fmt"
	"os"

	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/osutil"
	"github.com/google/uuid"
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

// Load the gadget yaml passed in via command line
func (stateMachine *StateMachine) loadGadgetYaml() error {
	if err := osutil.CopySpecialFile(stateMachine.yamlFilePath,
		stateMachine.stateMachineFlags.WorkDir); err != nil {
		return fmt.Errorf("Error loading gadget.yaml: %s", err.Error())
	}

	// read in the gadget.yaml as bytes, because snapd expects it that way
	gadgetYamlBytes, err := os.ReadFile(stateMachine.yamlFilePath)
	if err != nil {
		return fmt.Errorf("Error loading gadget.yaml: %s", err.Error())
	}

	stateMachine.gadgetInfo, err = gadget.InfoFromGadgetYaml(gadgetYamlBytes, nil)
	if err != nil {
		return fmt.Errorf("Error loading gadget.yaml: %s", err.Error())
	}

	for volumeName, _ := range(stateMachine.gadgetInfo.Volumes) {
		volumeBaseDir := stateMachine.tempDirs.volumes + "/" + volumeName
		if err := os.MkdirAll(volumeBaseDir, 0755); err != nil {
			return fmt.Errorf("Error creating volume dir: %s", err.Error())
		}
	}

	// check if the unpack dir should be preserved
	envar := os.Getenv("UBUNTU_IMAGE_PRESERVE_UNPACK")
	if envar != "" {
		preserveDir := envar + "/unpack"
		if err := osutil.CopySpecialFile(stateMachine.tempDirs.unpack, preserveDir); err != nil {
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

// Clean up and organize files
func (stateMachine *StateMachine) finish() error {
	if stateMachine.cleanWorkDir {
		if err := stateMachine.cleanup(); err != nil {
			return err
		}
	}
	return nil
}
