package statemachine

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/canonical/ubuntu-image/internal/helper"
)

var nonExistentPath string = "/tmp/this/path/better/not/exist"

/* This function ensures that the temporary workdir is cleaned up after the
 * state machine has finished running */
func TestCleanup(t *testing.T) {
	t.Run("test cleanup", func(t *testing.T) {
		stateMachine := StateMachine{WorkDir: ""}
		stateMachine.Run()
		if _, err := os.Stat(stateMachine.WorkDir); err == nil {
			t.Errorf("Error: temporary workdir %s was not cleaned up\n", stateMachine.WorkDir)
		}
	})
}

/* This function tests --until and --thru with each state for both snap and classic */
func TestUntilFlag(t *testing.T) {
	testCases := []struct {
		name string
	}{
		{"until_digit"},
		{"thru_digit"},
		{"until_name"},
		{"thru_name"},
	}
	for _, tc := range testCases {
		t.Run("test "+tc.name, func(t *testing.T) {
			for stateName, stateNum := range stateNames {
				tempDir, err := os.MkdirTemp("", "ubuntu-image-"+tc.name+"-")
				if err != nil {
					t.Errorf("Could not create workdir: %s\n", err.Error())
				}
				defer os.RemoveAll(tempDir)
				partialStateMachine := StateMachine{WorkDir: tempDir}
				switch tc.name {
				case "until_digit":
					partialStateMachine.Until = strconv.Itoa(stateNum)
					break
				case "thru_digit":
					partialStateMachine.Thru = strconv.Itoa(stateNum)
					break
				case "until_name":
					partialStateMachine.Until = stateName
					break
				case "thru_name":
					partialStateMachine.Thru = stateName
					break
				}
				if err := partialStateMachine.Run(); err != nil {
					t.Errorf("Failed to run partial state machine")
				}
				resumeStateMachine := StateMachine{WorkDir: tempDir, Resume: true}
				if err := resumeStateMachine.Run(); err != nil {
					t.Errorf("Failed to resume state machine from state: %s\n", stateName)
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
		{"invalid until digit", "99", "", false},
		{"invalid thru digit", "", "99", false},
		{"invalid until name", "fake step", "", false},
		{"invalid thru name", "", "fake step", false},
		{"resume with no workdir", "", "", true},
	}

	for _, tc := range testCases {
		t.Run("test "+tc.name, func(t *testing.T) {
			var stateMachine StateMachine
			stateMachine.WorkDir = ""
			stateMachine.Until = tc.until
			stateMachine.Thru = tc.thru
			stateMachine.Resume = tc.resume

			if err := stateMachine.Run(); err == nil {
				t.Error("Expected an error but there was none!")
			}
		})
	}
}

/* The state machine does a fair amount of file io to track state. This function tests
 * failures in these file io attempts by pausing the state machine, messing with
 * files/directories by deleting them ,chanigng permissions, etc, then resuming */
func TestFileErrors(t *testing.T) {
	testCases := []struct {
		name          string
		workDir       string
		pauseStep     string
		tempLocation  string
		causeProblems func(string)
		cleanUp       func(string)
	}{
		{"error_reading_metadata_file", "tmp", "5", "", func(messWith string) { os.RemoveAll(messWith) }, nil},
		{"error_opening_metadata_file", "tmp", "5", "", func(messWith string) { os.Chmod(messWith+"/ubuntu-image.gob", 0000) }, func(messWith string) { os.Chmod(messWith+"/ubuntu-image.gob", 0777); os.RemoveAll(messWith) }},
		{"error_creating_workdir", nonExistentPath, "5", "", nil, nil},
		{"error_parsing_metadata", "tmp", "1", "", func(messWith string) { os.Truncate(messWith+"/ubuntu-image.gob", 0) }, func(messWith string) { os.RemoveAll(messWith) }},
		{"error_creating_tmp", "", "2", nonExistentPath, nil, nil},
	}
	for _, tc := range testCases {
		t.Run("test "+tc.name, func(t *testing.T) {
			var partialStateMachine StateMachine

			partialStateMachine.tempLocation = tc.tempLocation
			if tc.workDir == "tmp" {
				workDir, err := os.MkdirTemp(tc.tempLocation, "ubuntu-image-"+tc.name+"-")
				if err != nil {
					t.Errorf("Failed to create temporary directory %s\n", workDir)
				}
				partialStateMachine.WorkDir = workDir
			} else {
				partialStateMachine.WorkDir = tc.workDir
			}
			partialStateMachine.Until = tc.pauseStep

			// don't check errors as some are expected here
			partialStateMachine.Run()

			// mess with files or directories
			if tc.causeProblems != nil {
				tc.causeProblems(partialStateMachine.WorkDir)
			}

			// try to resume the state machine
			resumeStateMachine := StateMachine{WorkDir: partialStateMachine.WorkDir, Resume: true}

			if err := resumeStateMachine.Run(); err == nil {
				t.Error("Expected an error but there was none!")
			}

			if tc.cleanUp != nil {
				tc.cleanUp(resumeStateMachine.WorkDir)
			}
		})
	}
}

/* This test iterates through each state individually using --resume and ensures
 * that the name of each state is printed when the --debug flag is in use */
func TestDebug(t *testing.T) {
	t.Run("test debug", func(t *testing.T) {
		workDir, err := os.MkdirTemp("", "ubuntu-image-test-debug-")
		if err != nil {
			t.Errorf("Failed to create temporary directory %s\n", workDir)
		}
		for stateName, stateNum := range stateNames {
			stdout, restoreStdout, err := helper.CaptureStd(&os.Stdout)
			if err != nil {
				t.Errorf("Failed to capture stdout: %s\n", err.Error())
			}

			stateMachine := StateMachine{WorkDir: workDir, Debug: true}

			// ignore the return value since we're just looking for the printed name
			stateFuncs[stateNum](&stateMachine)

			// restore stdout and check that the debug info was printed
			restoreStdout()
			readStdout, err := ioutil.ReadAll(stdout)
			if err != nil {
				t.Errorf("Failed to read stdout: %s\n", err.Error())
			}
			if !strings.Contains(string(readStdout), stateName) {
				t.Errorf("Expected state name %s to appear in output %s\n", stateName, string(readStdout))
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
		newStateFunc  func(stateMachine *StateMachine) error
	}{
		{"error_state_func", 0, func(stateMachine *StateMachine) error { return fmt.Errorf("Test Error") }},
		{"error_write_metadata", 13, func(stateMachine *StateMachine) error { os.RemoveAll(stateMachine.WorkDir); return nil }},
		{"error_cleanup", 12, func(stateMachine *StateMachine) error {
			stateMachine.CleanWorkDir = true
			stateMachine.WorkDir = "."
			return nil
		}},
	}
	for _, tc := range testCases {
		t.Run("test "+tc.name, func(t *testing.T) {
			workDir, err := os.MkdirTemp("", "ubuntu-image-"+tc.name+"-")
			if err != nil {
				t.Errorf("Failed to create temporary directory %s\n", workDir)
			}

			// override the function, but save the old one
			oldStateFunc := stateFuncs[tc.overrideState]
			stateFuncs[tc.overrideState] = tc.newStateFunc
			stateMachine := StateMachine{WorkDir: workDir}
			if err := stateMachine.Run(); err == nil {
				t.Errorf("Expected an error but there was none")
			}
			// restore the function
			stateFuncs[tc.overrideState] = oldStateFunc

			// clean up the workdir
			os.RemoveAll(workDir)
		})
	}
}
