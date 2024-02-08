package statemachine

import (
	"fmt"
	"os"
	"path/filepath"
)

var preparePackState = stateFunc{"prepare_pack", (*StateMachine).preparePack}

// preparePack prepare the packStateMachine
// This step must be run first
func (stateMachine *StateMachine) preparePack() error {
	packStateMachine := stateMachine.parent.(*PackStateMachine)

	packStateMachine.YamlFilePath = filepath.Join(packStateMachine.Opts.GadgetDir, gadgetYamlPathInTree)

	return nil
}

var populateTemporaryDirectoriesState = stateFunc{"populate_temporary_directories", (*StateMachine).populateTemporaryDirectories}

// populateTemporaryDirectories fills tempDirs with dirs given as Opts
func (stateMachine *StateMachine) populateTemporaryDirectories() error {
	packStateMachine := stateMachine.parent.(*PackStateMachine)

	files, err := osReadDir(packStateMachine.Opts.RootfsDir)
	if err != nil {
		return fmt.Errorf("Error reading rootfs dir: %s", err.Error())
	}

	for _, srcFile := range files {
		srcFile := filepath.Join(packStateMachine.Opts.RootfsDir, srcFile.Name())
		if err := osutilCopySpecialFile(srcFile, stateMachine.tempDirs.rootfs); err != nil {
			return fmt.Errorf("Error copying rootfs: %s", err.Error())
		}
	}

	// make the gadget directory under unpack
	gadgetDir := filepath.Join(stateMachine.tempDirs.unpack, "gadget")

	err = osMkdir(gadgetDir, 0755)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("Error creating scratch/gadget directory: %s", err.Error())
	}

	gfiles, err := osReadDir(packStateMachine.Opts.GadgetDir)
	if err != nil {
		return fmt.Errorf("Error reading gadget dir: %s", err.Error())
	}

	for _, srcFile := range gfiles {
		srcFile := filepath.Join(packStateMachine.Opts.GadgetDir, srcFile.Name())
		if err := osutilCopySpecialFile(srcFile, gadgetDir); err != nil {
			return fmt.Errorf("Error copying gadget: %s", err.Error())
		}
	}

	return nil
}
