package statemachine

import (
	"github.com/canonical/ubuntu-image/internal/commands"
)

// snapStates are the names and function variables to be executed by the state machine for snap images
var snapStates = []stateFunc{
	{"make_temporary_directories", (*StateMachine).makeTemporaryDirectories},
	{"determine_output_directory", (*StateMachine).determineOutputDirectory},
	{"prepare_image", (*StateMachine).prepareImage},
	{"load_gadget_yaml", (*StateMachine).loadGadgetYaml},
	{"set_artifact_names", (*StateMachine).setArtifactNames},
	{"populate_rootfs_contents", (*StateMachine).populateSnapRootfsContents},
	{"generate_disk_info", (*StateMachine).generateDiskInfo},
	{"calculate_rootfs_size", (*StateMachine).calculateRootfsSize},
	{"populate_bootfs_contents", (*StateMachine).populateBootfsContents},
	{"populate_prepare_partitions", (*StateMachine).populatePreparePartitions},
	{"make_disk", (*StateMachine).makeDisk},
	{"generate_manifest", (*StateMachine).generateSnapManifest},
	{"finish", (*StateMachine).finish},
}

// SnapStateMachine embeds StateMachine and adds the command line flags specific to snap images
type SnapStateMachine struct {
	StateMachine
	Opts commands.SnapOpts
	Args commands.SnapArgs
}

// Setup assigns variables and calls other functions that must be executed before Run(). It is
// exported so it can be used as a polymorphism in main
func (snapStateMachine *SnapStateMachine) Setup() error {
	// set the parent pointer of the embedded struct
	snapStateMachine.parent = snapStateMachine

	// set the states that will be used for this image type
	snapStateMachine.states = snapStates

	// do the validation common to all image types
	if err := snapStateMachine.validateInput(); err != nil {
		return err
	}

	// validate values of until and thru
	if err := snapStateMachine.validateUntilThru(); err != nil {
		return err
	}

	// if --resume was passed, figure out where to start
	if err := snapStateMachine.readMetadataJSON(metadataStateFile); err != nil {
		return err
	}

	return nil
}
