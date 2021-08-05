package statemachine

import (
	"fmt"

	"github.com/canonical/ubuntu-image/internal/commands"
)

// classicStates are the names and function variables to be executed by the state machine for classic images
var classicStates = []stateFunc{
	{"make_temporary_directories", (*StateMachine).makeTemporaryDirectories},
	{"prepare_gadget_tree", (*StateMachine).prepareGadgetTree},
	{"run_live_build", (*StateMachine).runLiveBuild},
	{"load_gadget_yaml", (*StateMachine).loadGadgetYaml},
	{"populate_rootfs_contents", (*StateMachine).populateClassicRootfsContents},
	{"populate_rootfs_contents_hooks", (*StateMachine).populateRootfsContentsHooks},
	{"generate_disk_info", (*StateMachine).generateDiskInfo},
	{"calculate_rootfs_size", (*StateMachine).calculateRootfsSize},
	{"prepopulate_bootfs_contents", (*StateMachine).prepopulateBootfsContents},
	{"populate_bootfs_contents", (*StateMachine).populateBootfsContents},
	{"populate_prepare_partitions", (*StateMachine).populatePreparePartitions},
	{"make_disk", (*StateMachine).makeDisk},
	{"generate_manifest", (*StateMachine).generateManifest},
	{"finish", (*StateMachine).finish},
}

// ClassicStateMachine embeds StateMachine and adds the command line flags specific to classic images
type ClassicStateMachine struct {
	StateMachine
	Opts commands.ClassicOpts
	Args commands.ClassicArgs
}

// validateClassicInput validates command line flags specific to classic images
func (classicStateMachine *ClassicStateMachine) validateClassicInput() error {
	// --project or --filesystem must be specified, but not both
	if classicStateMachine.Opts.Project == "" && classicStateMachine.Opts.Filesystem == "" {
		return fmt.Errorf("project or filesystem is required")
	} else if classicStateMachine.Opts.Project != "" && classicStateMachine.Opts.Filesystem != "" {
		return fmt.Errorf("project and filesystem are mutually exclusive")
	}

	return nil
}

// Setup assigns variables and calls other functions that must be executed before Run()
func (classicStateMachine *ClassicStateMachine) Setup() error {
	// set the parent pointer of the embedded struct
	classicStateMachine.parent = classicStateMachine

	// set the states that will be used for this image type
	classicStateMachine.states = classicStates

	// do the validation common to all image types
	if err := classicStateMachine.validateInput(); err != nil {
		return err
	}

	// if --resume was passed, figure out where to start
	if err := classicStateMachine.readMetadata(); err != nil {
		return err
	}

	// do the validation specific to classic images
	if err := classicStateMachine.validateClassicInput(); err != nil {
		return err
	}
	return nil
}
