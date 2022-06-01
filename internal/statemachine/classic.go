package statemachine

import (
	"github.com/canonical/ubuntu-image/internal/commands"
)

// startingClassicStates are the names and function variables that exist at the beginning
// of every classic state machine run
var startingClassicStates = []stateFunc{
	{"parse_image_definition", (*StateMachine).parseImageDefinition},
	{"calculate_states", (*StateMachine).calculateStates},
	{"make_temporary_directories", (*StateMachine).makeTemporaryDirectories},
}

// possibleRootfsStates is a list of all possible states for creating the rootfs.
// They are listed IN ORDER that they will be run. If a struct tag is present
// in the image definition naming one of these functions, it will be added to the
//state machine and executed IN THE ORDER LISTED HERE.
var possibleClassicStates = []stateFunc{
	{"build_gadget_tree", (*StateMachine).buildGadgetTree},
	{"prepare_gadget_tree", (*StateMachine).prepareGadgetTree},
	{"load_gadget_yaml", (*StateMachine).loadGadgetYaml},
	{"extract_rootfs_tar", (*StateMachine).extractRootfsTar},
	{"germinate", (*StateMachine).germinate},
	{"expand_tasks", (*StateMachine).expandTasks},
	{"create_chroot", (*StateMachine).createChroot},
	{"configure_extra_ppas", (*StateMachine).setupExtraPPAs},
	{"configure_extra_packages", (*StateMachine).configureExtraPackages},
	{"install_packages", (*StateMachine).installPackages},
	{"install_extra_packages", (*StateMachine).installExtraPackages},
	{"install_extra_snaps", (*StateMachine).prepareClassicImage},
	{"copy_custom_files", (*StateMachine).copyCustomFiles},
	{"execute_custom_files", (*StateMachine).executeCustomFiles},
	{"touch_custom_files", (*StateMachine).touchCustomFiles},
	{"add_groups", (*StateMachine).addGroups},
	{"add_users", (*StateMachine).addUsers},
}

var imageCreationStates = []stateFunc{
	{"populate_rootfs_contents", (*StateMachine).populateClassicRootfsContents},
	{"generate_disk_info", (*StateMachine).generateDiskInfo},
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
}

// Setup assigns variables and calls other functions that must be executed before Run()
func (classicStateMachine *ClassicStateMachine) Setup() error {
	//TODO: this is a temporary way to skip some states while we
	// implement the classic image redesign. remove it when possible
	classicStateMachine.stateSkip = true

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
