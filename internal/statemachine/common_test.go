// This file contains unit tests for all of the common state functions
package statemachine

import (
	"os"
	"testing"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/google/uuid"
)

// TestMakeTemporaryDirectories tests a successful execution of the
// make_temporary_directories state with and without --workdir
func TestMakeTemporaryDirectories(t *testing.T) {
	testCases := []struct {
		name    string
		workdir string
	}{
		{"with_workdir", "/tmp/make_temporary_directories-" + uuid.NewString()},
		{"without_workdir", ""},
	}
	for _, tc := range testCases {
		t.Run("test_"+tc.name, func(t *testing.T) {
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.stateMachineFlags.WorkDir = tc.workdir
			if err := stateMachine.makeTemporaryDirectories(); err != nil {
				t.Errorf("Did not expect an error, got %s", err.Error())
			}
			if _, err := os.Stat(stateMachine.stateMachineFlags.WorkDir); err != nil {
				t.Errorf("Failed to create workdir %s",
					stateMachine.stateMachineFlags.WorkDir)
			}
			os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
		})
	}
}

// TestFailedMakeTemporaryDirectories tests some failed executions of the make_temporary_directories state
func TestFailedMakeTemporaryDirectories(t *testing.T) {
	t.Run("test_failed_mkdir", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		// mock os.Mkdir and test with and without a WorkDir
		osMkdir = mockMkdir
		defer func() {
			osMkdir = os.Mkdir
		}()
		if err := stateMachine.makeTemporaryDirectories(); err == nil {
			t.Error("Expected an error, but got none")
		}
		stateMachine.stateMachineFlags.WorkDir = testDir
		if err := stateMachine.makeTemporaryDirectories(); err == nil {
			t.Error("Expected an error, but got none")
		}

		// mock os.MkdirAll and only test with a WorkDir
		osMkdirAll = mockMkdirAll
		defer func() {
			osMkdirAll = os.MkdirAll
		}()
		if err := stateMachine.makeTemporaryDirectories(); err == nil {
			// try adding a workdir to see if that triggers the failure
			stateMachine.stateMachineFlags.WorkDir = testDir
			if err := stateMachine.makeTemporaryDirectories(); err == nil {
				t.Error("Expected an error, but got none")
			}
		}
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}
