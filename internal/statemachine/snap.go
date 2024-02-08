package statemachine

import (
	"github.com/canonical/ubuntu-image/internal/commands"
)

// snapStates are the names and function variables to be executed by the state machine for snap images
var snapStates = []stateFunc{
	makeTemporaryDirectoriesState,
	determineOutputDirectoryState,
	prepareImageState,
	loadGadgetYamlState,
	setArtifactNamesState,
	populateSnapRootfsContentsState,
	generateDiskInfoState,
	calculateRootfsSizeState,
	populateBootfsContentsState,
	populatePreparePartitionsState,
	makeDiskState,
	generateSnapManifestState,
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

	if err := snapStateMachine.setConfDefDir(snapStateMachine.parent.(*SnapStateMachine).Args.ModelAssertion); err != nil {
		return err
	}

	// do the validation common to all image types
	if err := snapStateMachine.validateInput(); err != nil {
		return err
	}

	// validate values of until and thru
	if err := snapStateMachine.validateUntilThru(); err != nil {
		return err
	}

	// if --resume was passed, figure out where to start
	if err := snapStateMachine.readMetadata(metadataStateFile); err != nil {
		return err
	}

	return nil
}
