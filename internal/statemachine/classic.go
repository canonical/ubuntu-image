package statemachine

import (
	"fmt"

	"github.com/canonical/ubuntu-image/internal/commands"
)

// classicStates are the names and function variables to be executed by the state machine for classic images
var classicStates = []stateFunc{
	stateFunc{"make_temporary_directories", (*StateMachine).makeTemporaryDirectories},
	stateFunc{"prepare_gadget_tree", (*StateMachine).prepareGadgetTree},
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

// classicStateMachine embeds StateMachine and adds the command line flags specific to classic images
type classicStateMachine struct {
	StateMachine
	Opts commands.ClassicOpts
	Args commands.ClassicArgs
}

// validateClassicInput validates command line flags specific to classic images
func (ClassicStateMachine *classicStateMachine) validateClassicInput() error {
	// --project or --filesystem must be specified, but not both
	if ClassicStateMachine.Opts.Project == "" && ClassicStateMachine.Opts.Filesystem == "" {
		return fmt.Errorf("project or filesystem is required")
	} else if ClassicStateMachine.Opts.Project != "" && ClassicStateMachine.Opts.Filesystem != "" {
		return fmt.Errorf("project and filesystem are mutually exclusive")
	}

	// TODO: more validation, probably
	return nil
}

// Setup assigns variables and calls other functions that must be executed before Run()
func (ClassicStateMachine *classicStateMachine) Setup() error {
	// Set the struct variables specific to classic images
	ClassicStateMachine.Opts = commands.UICommand.Classic.ClassicOptsPassed
	ClassicStateMachine.Args = commands.UICommand.Classic.ClassicArgsPassed

	// get the common options for all image types
	ClassicStateMachine.setCommonOpts()

	// set the states that will be used for this image type
	ClassicStateMachine.states = classicStates

	// do the validation common to all image types
	if err := ClassicStateMachine.validateInput(); err != nil {
		return err
	}

	// if --resume was passed, figure out where to start
	if err := ClassicStateMachine.readMetadata(); err != nil {
		return err
	}

	// do the validation specific to classic images
	if err := ClassicStateMachine.validateClassicInput(); err != nil {
		return err
	}
	return nil
}

// ClassicSM is the interface used for polymorphisms on Setup, Run And Teardown when building classic images
var ClassicSM classicStateMachine
