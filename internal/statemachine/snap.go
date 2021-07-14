package statemachine

import (
	"github.com/canonical/ubuntu-image/internal/commands"
)

// snapStates are the names and function variables to be executed by the state machine for snap images
var snapStates = []stateFunc{
	stateFunc{"make_temporary_directories", (*StateMachine).makeTemporaryDirectories},
	stateFunc{"prepare_image", (*StateMachine).prepareImage},
	stateFunc{"load_gadget_yaml", (*StateMachine).loadGadgetYaml},
	stateFunc{"populate_rootfs_contents", (*StateMachine).populateRootfsContents},
	stateFunc{"populate_rootfs_contents_hooks", (*StateMachine).populateRootfsContentsHooks},
	stateFunc{"generate_disk_info", (*StateMachine).generateDiskInfo},
	stateFunc{"calculate_rootfs_size", (*StateMachine).calculateRootfsSize},
	stateFunc{"prepopulate_bootfs_contents", (*StateMachine).prepopulateBootfsContents},
	stateFunc{"populate_bootfs_contents", (*StateMachine).populateBootfsContents},
	stateFunc{"populate_prepare_partitions", (*StateMachine).populatePreparePartitions},
	stateFunc{"make_disk", (*StateMachine).makeDisk},
	stateFunc{"generate_manifest", (*StateMachine).generateManifest},
	stateFunc{"finish", (*StateMachine).finish},
}

// snapStateMachine embeds StateMachine and adds the command line flags specific to snap images
type snapStateMachine struct {
	StateMachine
	Opts commands.SnapOpts
	Args commands.SnapArgs
}

// Setup assigns variables and calls other functions that must be executed before Run(). It is
// exported so it can be used as a polymorphism in main
func (SnapStateMachine *snapStateMachine) Setup() error {
	// Set the struct variables specific to snap images
	SnapStateMachine.Opts = commands.UICommand.Snap.SnapOptsPassed
	SnapStateMachine.Args = commands.UICommand.Snap.SnapArgsPassed

	// get the common options for all image types
	SnapStateMachine.setCommonOpts()

	// set the states that will be used for this image type
	SnapStateMachine.states = snapStates

	// do the validation common to all image types
	if err := SnapStateMachine.validateInput(); err != nil {
		return err
	}

	// if --resume was passed, figure out where to start
	if err := SnapStateMachine.readMetadata(); err != nil {
		return err
	}

	// TODO: is there any validation specific to snap images?
	return nil
}

// SnapSM is the interface used for polymorphisms on Setup, Run And Teardown when building snap images
var SnapSM snapStateMachine
