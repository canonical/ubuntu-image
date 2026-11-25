package statemachine

import (
	"errors"
	"fmt"

	"github.com/snapcore/snapd/asserts"

	"github.com/canonical/ubuntu-image/internal/commands"
)

// snapStates are the names and function variables to be executed by the state machine for snap images
var snapStates = []stateFunc{
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

// Setup assigns variables and calls other functions that must be executed before Run().
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

	if err := snapStateMachine.SetSeries(); err != nil {
		return err
	}

	snapStateMachine.displayStates()

	if snapStateMachine.commonFlags.DryRun {
		return nil
	}

	if err := snapStateMachine.makeTemporaryDirectories(); err != nil {
		return err
	}

	return snapStateMachine.determineOutputDirectory()
}

func (snapStateMachine *SnapStateMachine) SetSeries() error {
	model, err := snapStateMachine.decodeModelAssertion()
	if err != nil {
		return err
	}

	// we extracted a "core" series, containing only the major version, not minor
	// In this case we will always end up using the LTS, so .04 minor version.
	snapStateMachine.series = fmt.Sprintf("%s.04", model.Series())

	return nil
}

func (snapStateMachine *SnapStateMachine) Architecture() (string, error) {
	model, err := snapStateMachine.decodeModelAssertion()
	if err != nil {
		return "", err
	}

	arch := model.Architecture()
	if len(arch) == 0 {
		return "", errors.New("unable to identify the arch")
	}

	return arch, nil
}

// decodeModelAssertion() was copied and slightly adapted from image/image_linux.go
// in https://github.com/snapcore/snapd/
// Commit: 6ab16e24bc7e2ee386a07716a5b3eeb520ffc022
func (snapStateMachine *SnapStateMachine) decodeModelAssertion() (*asserts.Model, error) {
	return decodeModelAssertionFile(snapStateMachine.Args.ModelAssertion)
}
