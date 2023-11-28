package statemachine

import (
	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/canonical/ubuntu-image/internal/imagedefinition"
)

// classicStates are the names and function variables to be executed by the state machine for classic images
var startingClassicStates = []stateFunc{
	{"parse_image_definition", (*StateMachine).parseImageDefinition},
	{"calculate_states", (*StateMachine).calculateStates},
	{"make_temporary_directories", (*StateMachine).makeTemporaryDirectories},
	{"determine_output_directory", (*StateMachine).determineOutputDirectory},
}

var rootfsSeedStates = []stateFunc{
	{"germinate", (*StateMachine).germinate},
	{"create_chroot", (*StateMachine).createChroot},
}

var imageCreationStates = []stateFunc{
	{"calculate_rootfs_size", (*StateMachine).calculateRootfsSize},
	{"populate_bootfs_contents", (*StateMachine).populateBootfsContents},
	{"populate_prepare_partitions", (*StateMachine).populatePreparePartitions},
}

// ClassicStateMachine embeds StateMachine and adds the command line flags specific to classic images
type ClassicStateMachine struct {
	StateMachine
	ImageDef imagedefinition.ImageDefinition
	Opts     commands.ClassicOpts
	Args     commands.ClassicArgs
}

// Setup assigns variables and calls other functions that must be executed before Run()
func (classicStateMachine *ClassicStateMachine) Setup() error {
	// set the parent pointer of the embedded struct
	classicStateMachine.parent = classicStateMachine

	// set the beginning states that will be used by all classic image builds
	classicStateMachine.states = startingClassicStates

	if err := classicStateMachine.setConfDefDir(classicStateMachine.parent.(*ClassicStateMachine).Args.ImageDefinition); err != nil {
		return err
	}

	// do the validation common to all image types
	if err := classicStateMachine.validateInput(); err != nil {
		return err
	}

	// if --resume was passed, figure out where to start
	if err := classicStateMachine.readMetadata(metadataStateFile); err != nil {
		return err
	}

	return nil
}
