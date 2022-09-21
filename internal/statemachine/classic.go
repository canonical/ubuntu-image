package statemachine

import (
	"github.com/canonical/ubuntu-image/internal/commands"
)

// classicStates are the names and function variables to be executed by the state machine for classic images
var startingClassicStates = []stateFunc{
	{"parse_image_definition", (*StateMachine).parseImageDefinition},
	{"calculate_states", (*StateMachine).calculateStates},
	{"make_temporary_directories", (*StateMachine).makeTemporaryDirectories},
}

var rootfsSeedStates = []stateFunc{
	{"germinate", (*StateMachine).germinate},
	{"create_chroot", (*StateMachine).createChroot},
}

var imageCreationStates = []stateFunc{
	{"calculate_rootfs_size", (*StateMachine).calculateRootfsSize},
	{"populate_bootfs_contents", (*StateMachine).populateBootfsContents},
	{"populate_prepare_partitions", (*StateMachine).populatePreparePartitions},
	{"make_disk", (*StateMachine).makeDisk},
	{"generate_manifest", (*StateMachine).generatePackageManifest},
	{"finish", (*StateMachine).finish},
}

// ClassicStateMachine embeds StateMachine and adds the command line flags specific to classic images
type ClassicStateMachine struct {
	StateMachine
	ImageDef ImageDefinition
	Opts     commands.ClassicOpts
	Args     commands.ClassicArgs
	Packages []string
	Snaps    []string
}

// Setup assigns variables and calls other functions that must be executed before Run()
func (classicStateMachine *ClassicStateMachine) Setup() error {
	// set the parent pointer of the embedded struct
	classicStateMachine.parent = classicStateMachine

	// set the beginning states that will be used by all classic image builds
	classicStateMachine.states = startingClassicStates

	// do the validation common to all image types
	if err := classicStateMachine.validateInput(); err != nil {
		return err
	}

	// if --resume was passed, figure out where to start
	if err := classicStateMachine.readMetadata(); err != nil {
		return err
	}

	return nil
}
