package statemachine

import (
	"path/filepath"
	"testing"

	"github.com/canonical/ubuntu-image/internal/helper"
)

// TestRootfsSetup tests a successful run of the polymorphed Setup function
func TestRootfsSetup(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := helper.SaveCWD()
	defer restoreCWD()

	var stateMachine RootfsStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.Args.ImageDefinition = filepath.Join("testdata", "image_definitions",
		"test_amd64.yaml")

	err := stateMachine.Setup()
	asserter.AssertErrNil(err, true)
}
