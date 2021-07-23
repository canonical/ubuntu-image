package statemachine

import (
	"fmt"
	"os"
)

// generate work directory file structure
func (stateMachine *StateMachine) makeTemporaryDirectories() error {
	// if no workdir was specified, open a /tmp dir
	if stateMachine.stateMachineFlags.WorkDir == "" {
		workDir, err := os.MkdirTemp(stateMachine.tempLocation, "ubuntu-image-")
		if err != nil {
			return fmt.Errorf("Failed to create temporary directory: %s", err.Error())
		}
		stateMachine.stateMachineFlags.WorkDir = workDir
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
