package statemachine

import (
	"errors"
	"fmt"
	"os"

	"github.com/snapcore/snapd/asserts"
	"github.com/snapcore/snapd/seed/seedwriter"
	"github.com/snapcore/snapd/snap"

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

	// manifest, when set by the --manifest pipeline, is the parsed
	// input manifest. Stashed so the end-of-prepareImage step can
	// emit a build.yaml that reflects what was actually built.
	manifest *commands.OnlineManifest

	// manifestStoreURL, when set by the --manifest pipeline, is the
	// store URL discovered via `m2cp user status --json`. Emitted
	// into build.yaml as appstore-url.
	manifestStoreURL string

	// manifestSeedManifest, when set by the --manifest pipeline, takes
	// precedence over Opts.Revisions in imageOptsSeedManifest.
	manifestSeedManifest *seedwriter.Manifest

	// manifestSnapURL, when set by the --manifest pipeline, becomes
	// image.Options.SnapDownloadURL so the snapd-fork URL hook
	// resolves blob URLs via m2cp instead of the Canonical store API.
	manifestSnapURL func(name string, revision snap.Revision, snapID string) (string, error)

	// manifestAssertionRetrieve, when set by the --manifest pipeline,
	// becomes image.Options.AssertionRetrieve so snapd resolves
	// snap-revision / snap-declaration / account / account-key from
	// an m2cp-prefetched in-memory index instead of the store.
	manifestAssertionRetrieve func(ref *asserts.Ref) (asserts.Assertion, error)
}

// Setup assigns variables and calls other functions that must be executed before Run().
func (snapStateMachine *SnapStateMachine) Setup() error {
	// set the parent pointer of the embedded struct
	snapStateMachine.parent = snapStateMachine

	// set the states that will be used for this image type
	snapStateMachine.states = snapStates

	if snapStateMachine.Opts.Manifest != "" {
		if err := snapStateMachine.prepareFromManifest(); err != nil {
			return err
		}
	}

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

// these are postponed, not implemented or abandoned, not finalized,
// don't let them sneak in into a used model assertion
var reserved = []string{"core", "os", "class", "allowed-modes"}

func (snapStateMachine *SnapStateMachine) decodeModelAssertion() (*asserts.Model, error) {
	fn := snapStateMachine.Args.ModelAssertion

	rawAssert, err := os.ReadFile(fn)
	if err != nil {
		return nil, fmt.Errorf("cannot read model assertion: %s", err)
	}

	assertion, err := asserts.Decode(rawAssert)
	if err != nil {
		return nil, fmt.Errorf("cannot decode model assertion %q: %s", fn, err)
	}
	modelAssertion, ok := assertion.(*asserts.Model)
	if !ok {
		return nil, fmt.Errorf("assertion in %q is not a model assertion", fn)
	}

	for _, rsvd := range reserved {
		if modelAssertion.Header(rsvd) != nil {
			return nil, fmt.Errorf("model assertion cannot have reserved/unsupported header %q set", rsvd)
		}
	}

	return modelAssertion, nil
}
