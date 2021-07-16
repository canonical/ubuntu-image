package statemachine

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/canonical/ubuntu-image/internal/helper"
)

var testDir = "ubuntu-image-0615c8dd-d3af-4074-bfcb-c3d3c8392b06"

/* This function ensures that the temporary workdir is cleaned up after the
 * state machine has finished running */
func TestCleanup(t *testing.T) {
	t.Run("test cleanup", func(t *testing.T) {
		stateMachine := StateMachine{}
		stateMachine.Run()
		if _, err := os.Stat(stateMachine.stateMachineFlags.WorkDir); err == nil {
			t.Errorf("Error: temporary workdir %s was not cleaned up\n", stateMachine.stateMachineFlags.WorkDir)
		}
	})
}

/* This function tests --until and --thru with each state for both snap and classic */
func TestUntilThru(t *testing.T) {
	testCases := []struct {
		name string
	}{
		{"until"},
		{"thru"},
	}
	for _, tc := range testCases {
		t.Run("test "+tc.name, func(t *testing.T) {
			partialStateMachine := StateMachine{}
			for _, state := range partialStateMachine.states {
				tempDir, err := os.MkdirTemp("", "ubuntu-image-"+tc.name+"-")
				if err != nil {
					t.Errorf("Could not create workdir: %s\n", err.Error())
				}
				defer os.RemoveAll(tempDir)
				partialStateMachine.stateMachineFlags.WorkDir = tempDir
				switch tc.name {
				case "until":
					partialStateMachine.stateMachineFlags.Until = state.name
					break
				case "thru":
					partialStateMachine.stateMachineFlags.Thru = state.name
					break
				}
				if err := partialStateMachine.Run(); err != nil {
					t.Errorf("Failed to run partial state machine")
				}
				resumeStateMachine := StateMachine{}
				resumeStateMachine.stateMachineFlags.Resume = true
				resumeStateMachine.stateMachineFlags.WorkDir = tempDir
				if err := resumeStateMachine.Run(); err != nil {
					t.Errorf("Failed to resume state machine from state: %s\n", state.name)
				}
			}
		})
	}
}

/* state_machine.go validates the state machine specific args to keep main.go cleaner.
 * This function tests that validation with a number of invalid arguments and flags */
func TestInvalidStateMachineArgs(t *testing.T) {
	testCases := []struct {
		name   string
		until  string
		thru   string
		resume bool
	}{
		{"both until and thru", "1", "1", false},
		// TODO: do we want this validation? the python version seems to not have it
		//{"invalid until name", "fake step", "", false},
		//{"invalid thru name", "", "fake step", false},
		{"resume with no workdir", "", "", true},
	}

	for _, tc := range testCases {
		t.Run("test "+tc.name, func(t *testing.T) {
			smFlags := commands.StateMachineOpts{Until: tc.until, Thru: tc.thru, Resume: tc.resume}
			stateMachine := StateMachine{stateMachineFlags: smFlags}

			if err := stateMachine.validateInput(); err == nil {
				t.Error("Expected an error but there was none!")
			}
		})
	}
}

/* The state machine does a fair amount of file io to track state. This function tests
 * failures in these file io attempts by pausing the state machine, messing with
 * files/directories by deleting them ,changing permissions, etc, then resuming */
func TestFileErrors(t *testing.T) {
	testCases := []struct {
		name          string
		workDir       string
		pauseStep     string
		tempLocation  string
		causeProblems func(string)
		cleanUp       func(string)
	}{
		{"error_reading_metadata_file", "tmp", "populate_rootfs_contents", "", func(messWith string) { os.RemoveAll(messWith) }, nil},
		{"error_opening_metadata_file", "tmp", "populate_rootfs_contents", "", func(messWith string) { os.Chmod(messWith+"/ubuntu-image.gob", 0000) }, func(messWith string) { os.Chmod(messWith+"/ubuntu-image.gob", 0777); os.RemoveAll(messWith) }},
		{"error_parsing_metadata", "tmp", "make_temporary_directories", "", func(messWith string) { os.Truncate(messWith+"/ubuntu-image.gob", 0) }, func(messWith string) { os.RemoveAll(messWith) }},
		{"error_creating_tmp", "", "prepare_gadget_tree", "/tmp/this/path/better/not/exist/" + testDir, nil, nil},
	}
	for _, tc := range testCases {
		t.Run("test "+tc.name, func(t *testing.T) {
			var partialStateMachine StateMachine
			var smFlags commands.StateMachineOpts

			partialStateMachine.tempLocation = tc.tempLocation
			if tc.workDir == "tmp" {
				workDir, err := os.MkdirTemp(tc.tempLocation, "ubuntu-image-"+tc.name+"-")
				if err != nil {
					t.Errorf("Failed to create temporary directory %s\n", workDir)
				}
				smFlags.WorkDir = workDir
			} else {
				smFlags.WorkDir = tc.workDir
			}
			smFlags.Until = tc.pauseStep

			// don't check errors as some are expected here
			partialStateMachine.stateMachineFlags = smFlags
			partialStateMachine.states = classicStates
			partialStateMachine.cleanWorkDir = false
			partialStateMachine.Run()
			partialStateMachine.writeMetadata()

			// mess with files or directories
			if tc.causeProblems != nil {
				tc.causeProblems(partialStateMachine.stateMachineFlags.WorkDir)
			}

			// try to resume the state machine
			resumeSmFlags := commands.StateMachineOpts{WorkDir: partialStateMachine.stateMachineFlags.WorkDir, Resume: true}
			var resumeStateMachine StateMachine

			resumeStateMachine.states = classicStates
			resumeStateMachine.stateMachineFlags = resumeSmFlags
			readErr := resumeStateMachine.readMetadata()
			runErr := resumeStateMachine.Run()
			if readErr != nil && runErr != nil {
				t.Error("Expected an error but there was none!")
			}

			if tc.cleanUp != nil {
				tc.cleanUp(resumeStateMachine.stateMachineFlags.WorkDir)
			}
		})
	}
}

/* This test iterates through each state function individually and ensures
 * that the name of each state is printed when the --debug flag is in use */
func TestDebug(t *testing.T) {
	t.Run("test debug", func(t *testing.T) {
		workDir, err := os.MkdirTemp("", "ubuntu-image-test-debug-")
		if err != nil {
			t.Errorf("Failed to create temporary directory %s\n", workDir)
		}

		smFlags := commands.StateMachineOpts{WorkDir: workDir}
		commonFlags := commands.CommonOpts{Debug: true}
		stateMachine := StateMachine{stateMachineFlags: smFlags, commonFlags: commonFlags}
		for _, state := range stateMachine.states {
			stdout, restoreStdout, err := helper.CaptureStd(&os.Stdout)
			if err != nil {
				t.Errorf("Failed to capture stdout: %s\n", err.Error())
			}

			// ignore the return value since we're just looking for the printed name
			state.function(&stateMachine)

			// restore stdout and check that the debug info was printed
			restoreStdout()
			readStdout, err := ioutil.ReadAll(stdout)
			if err != nil {
				t.Errorf("Failed to read stdout: %s\n", err.Error())
			}
			if !strings.Contains(string(readStdout), state.name) {
				t.Errorf("Expected state name %s to appear in output %s\n", state.name, string(readStdout))
			}
		}
		// clean up
		os.RemoveAll(workDir)
	})
}

/* This function overrides some of the state functions to test various error scenarios */
func TestFunctionErrors(t *testing.T) {
	testCases := []struct {
		name          string
		overrideState int
		newStateFunc  stateFunc
	}{
		{"error_state_func", 0, stateFunc{"test_error_state_func", func(stateMachine *StateMachine) error { return fmt.Errorf("Test Error") }}},
		{"error_write_metadata", 13, stateFunc{"test_error_write_metadata", func(stateMachine *StateMachine) error { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir); return nil }}},
		{"error_cleanup", 12, stateFunc{"test_error_cleanup", func(stateMachine *StateMachine) error {
			stateMachine.cleanWorkDir = true
			stateMachine.stateMachineFlags.WorkDir = "."
			return nil
		}}},
	}
	for _, tc := range testCases {
		t.Run("test "+tc.name, func(t *testing.T) {
			workDir, err := os.MkdirTemp("", "ubuntu-image-"+tc.name+"-")
			if err != nil {
				t.Errorf("Failed to create temporary directory %s\n", workDir)
			}

			// override the function, but save the old one
			smFlags := commands.StateMachineOpts{WorkDir: workDir}
			var stateMachine classicStateMachine
			stateMachine.Setup()
			stateMachine.stateMachineFlags = smFlags
			stateMachine.states[tc.overrideState] = tc.newStateFunc
			if err := stateMachine.Run(); err == nil {
				if err := stateMachine.Teardown(); err == nil {
					t.Errorf("Expected an error but there was none")
				}
			}

			// clean up the workdir
			os.RemoveAll(workDir)
		})
	}
}

/* This function tests a failure in the "MkdirAll" call that happens when --workdir is used */
func TestFailedCreateWorkDir(t *testing.T) {
	t.Run("test error creating workdir", func(t *testing.T) {
		/* create the parent dir of the workdir with restrictive permissions */
		parentDir := "/tmp/workdir_error-5c919639-b972-4265-807a-19cd23fd1936/"
		workDir := parentDir + testDir
		os.Mkdir(parentDir, 0000)

		smFlags := commands.StateMachineOpts{WorkDir: workDir, Thru: "make_temporary_directories"}
		stateMachine := StateMachine{stateMachineFlags: smFlags}
		stateMachine.states = classicStates
		if err := stateMachine.Run(); err == nil {
			t.Errorf("Expected an error but there was none")
		}

		os.Chmod(parentDir, 0777)
		os.RemoveAll(parentDir)
	})
}
