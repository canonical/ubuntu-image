package statemachine

import (
	"fmt"
	"os"

	"github.com/snapcore/snapd/asserts"

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

	return snapStateMachine.SetSeries()
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

// decodeModelAssertion() was copied and slightly adapted from image/image_linux.go
// in https://github.com/snapcore/snapd/
// Commit: 6ab16e24bc7e2ee386a07716a5b3eeb520ffc022

// these are postponed, not implemented or abandoned, not finalized,
// don't let them sneak in into a used model assertion
var reserved = []string{"core", "os", "class", "allowed-modes"}

func (snapStateMachine *SnapStateMachine) decodeModelAssertion() (*asserts.Model, error) {
	fn := snapStateMachine.Args.ModelAssertion

	rawAssert, err := os.ReadFile(fn)
	if err != nil {
		return nil, fmt.Errorf("cannot read model assertion: %s", err)
	}

	ass, err := asserts.Decode(rawAssert)
	if err != nil {
		return nil, fmt.Errorf("cannot decode model assertion %q: %s", fn, err)
	}
	modela, ok := ass.(*asserts.Model)
	if !ok {
		return nil, fmt.Errorf("assertion in %q is not a model assertion", fn)
	}

	for _, rsvd := range reserved {
		if modela.Header(rsvd) != nil {
			return nil, fmt.Errorf("model assertion cannot have reserved/unsupported header %q set", rsvd)
		}
	}

	return modela, nil
}
