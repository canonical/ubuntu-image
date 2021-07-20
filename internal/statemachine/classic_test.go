package statemachine

import (
	"testing"

	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/canonical/ubuntu-image/internal/helper"
)

// TestInvalidCommandLineClassic tests invalid command line input for classic images
func TestInvalidCommandLineClassic(t *testing.T) {
	testCases := []struct {
		name       string
		project    string
		filesystem string
	}{
		{"neither_project_nor_filesystem", "", ""},
		{"both_project_and_filesystem", "ubuntu-cpc", "/tmp"},
	}
	for _, tc := range testCases {
		t.Run("test "+tc.name, func(t *testing.T) {
			restoreArgs := helper.Setup()
			defer restoreArgs()

			var stateMachine classicStateMachine
			commands.UICommand.Classic.ClassicOptsPassed.Project = tc.project
			commands.UICommand.Classic.ClassicOptsPassed.Filesystem = tc.filesystem

			if err := stateMachine.Setup(); err == nil {
				t.Error("Expected an error but there was none")
			}
		})
	}
}

// TestFailedValidateInputClassic tests a failure in the Setup() function when validating common input
func TestFailedValidateInputClassic(t *testing.T) {
	t.Run("test_failed_validate_input", func(t *testing.T) {
		restoreArgs := helper.Setup()
		defer restoreArgs()

		// use both --until and --thru to trigger this failure
		commands.StateMachineOptsPassed.Until = "until-test"
		commands.StateMachineOptsPassed.Thru = "thru-test"

		var stateMachine classicStateMachine
		if err := stateMachine.Setup(); err == nil {
			t.Error("Expected an error but there was none")
		}
	})
}

// TestFailedReadMetadataClassic tests a failed metadata read by passing --resume with no previous partial state machine run
func TestFailedReadMetadataClassic(t *testing.T) {
	t.Run("test_failed_read_metadata", func(t *testing.T) {
		restoreArgs := helper.Setup()
		defer restoreArgs()

		// start a --resume with no previous SM run
		commands.StateMachineOptsPassed.Resume = true
		commands.StateMachineOptsPassed.WorkDir = testDir

		var stateMachine classicStateMachine
		if err := stateMachine.Setup(); err == nil {
			t.Error("Expected an error but there was none")
		}
	})
}

// TestSuccessfulClassicRun runs through all states ensuring none failed
func TestSuccessfulClassicRun(t *testing.T) {
	t.Run("test_successful_classic_run", func(t *testing.T) {
		restoreArgs := helper.Setup()
		defer restoreArgs()

		var stateMachine classicStateMachine
		commands.UICommand.Classic.ClassicOptsPassed.Project = "ubuntu-cpc"

		if err := stateMachine.Setup(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}

		if err := stateMachine.Run(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}

		if err := stateMachine.Teardown(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}
	})
}
