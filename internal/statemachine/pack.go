package statemachine

import (
	"fmt"

	"github.com/canonical/ubuntu-image/internal/commands"
)

var packStates = []stateFunc{
	{"prepare_pack", (*StateMachine).preparePack},
	{"make_temporary_directories", (*StateMachine).makeTemporaryDirectories},
	{"populate_temporary_directories", (*StateMachine).populateTemporaryDirectories},
	{"load_gadget_yaml", (*StateMachine).loadGadgetYaml},
	{"set_artifact_names", (*StateMachine).setArtifactNames},
	{"calculate_rootfs_size", (*StateMachine).calculateRootfsSize},
	{"populate_bootfs_contents", (*StateMachine).populateBootfsContents},
	{"populate_prepare_partitions", (*StateMachine).populatePreparePartitions},
	{"make_disk", (*StateMachine).makeDisk},
	{"update_bootloader", (*StateMachine).updateBootloader},
	{"finish", (*StateMachine).finish},
}

// PackStateMachine embeds StateMachine and adds the command line flags specific to pack images
type PackStateMachine struct {
	StateMachine
	Opts commands.PackOpts
}

// Setup assigns variables and calls other functions that must be executed before Run()
func (packStateMachine *PackStateMachine) Setup() error {
	fmt.Print("WARNING: this is an experimental feature.\n")

	// set the parent pointer of the embedded struct
	packStateMachine.parent = packStateMachine

	// set the beginning states that will be used by all pack image builds
	packStateMachine.states = packStates

	// do the validation common to all image types
	if err := packStateMachine.validateInput(); err != nil {
		return err
	}

	// validate values of until and thru
	if err := packStateMachine.validateUntilThru(); err != nil {
		return err
	}

	// if --resume was passed, figure out where to start
	if err := packStateMachine.readMetadata(metadataStateFile); err != nil {
		return err
	}

	return nil
}

// Placeholder method to satisfy the interface. This is not used when packing.
func (packStateMachine *PackStateMachine) SetSeries() error {
	return nil
}
