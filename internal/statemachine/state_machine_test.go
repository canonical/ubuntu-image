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

// for tests where we don't want to run actual states
var testStates = []stateFunc{
	{"test_succeed", func(*StateMachine) error { return nil }},
}

// for tests where we want to run all the states
var allTestStates = []stateFunc{
	{"make_temporary_directories", (*StateMachine).makeTemporaryDirectories},
	{"prepare_gadget_tree", (*StateMachine).prepareGadgetTree},
	{"prepare_image", (*StateMachine).prepareImage},
	{"load_gadget_yaml", (*StateMachine).loadGadgetYaml},
	{"populate_rootfs_contents", (*StateMachine).populateRootfsContents},
	{"populate_rootfs_contents_hooks", (*StateMachine).populateRootfsContentsHooks},
	{"generate_disk_info", (*StateMachine).generateDiskInfo},
	{"calculate_rootfs_size", (*StateMachine).calculateRootfsSize},
	{"prepopulate_bootfs_contents", (*StateMachine).prepopulateBootfsContents},
	{"populate_bootfs_contents", (*StateMachine).populateBootfsContents},
	{"populate_prepare_partitions", (*StateMachine).populatePreparePartitions},
	{"make_disk", (*StateMachine).makeDisk},
	{"generate_manifest", (*StateMachine).generateManifest},
	{"finish", (*StateMachine).finish},
}

// testStateMachine implements Setup, Run, and Teardown() for testing purposes
type testStateMachine struct {
	StateMachine
}

func (TestStateMachine *testStateMachine) Setup() error {
	// get the common options for all image types
	TestStateMachine.setCommonOpts()

	// set the states that will be used for this image type
	TestStateMachine.states = allTestStates

	// do the validation common to all image types
	if err := TestStateMachine.validateInput(); err != nil {
		return err
	}

	// if --resume was passed, figure out where to start
	if err := TestStateMachine.readMetadata(); err != nil {
		return err
	}

	return nil
}

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
			for _, state := range allTestStates {
				restoreArgs := helper.Setup()
				defer restoreArgs()

				// run a partial state machine
				var partialStateMachine testStateMachine
				tempDir, err := os.MkdirTemp("", "ubuntu-image-"+tc.name+"-")
				if err != nil {
					t.Errorf("Could not create workdir: %s\n", err.Error())
				}
				defer os.RemoveAll(tempDir)
				commands.StateMachineOptsPassed.WorkDir = tempDir

				switch tc.name {
				case "until":
					commands.StateMachineOptsPassed.Until = state.name
					break
				case "thru":
					commands.StateMachineOptsPassed.Thru = state.name
					break
				}
				if err := partialStateMachine.Setup(); err != nil {
					t.Errorf("Failed to set up partial state machine: %s", err.Error())
				}
				if err := partialStateMachine.Run(); err != nil {
					t.Errorf("Failed to run partial state machine: %s", err.Error())
				}
				if err := partialStateMachine.Teardown(); err != nil {
					t.Errorf("Failed to teardown partial state machine: %s", err.Error())
				}

				// now resume
				var resumeStateMachine testStateMachine
				commands.StateMachineOptsPassed.Until = ""
				commands.StateMachineOptsPassed.Thru = ""
				commands.StateMachineOptsPassed.Resume = true

				if err := resumeStateMachine.Setup(); err != nil {
					t.Errorf("Failed to resume state machine: %s", err.Error())
				}
				if err := resumeStateMachine.Run(); err != nil {
					t.Errorf("Failed to resume state machine from state: %s\n", state.name)
				}
				if err := resumeStateMachine.Teardown(); err != nil {
					t.Errorf("Failed to resume state machine: %s", err.Error())
				}
				restoreArgs()
				os.RemoveAll(tempDir)
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
	}
	for _, tc := range testCases {
		t.Run("test "+tc.name, func(t *testing.T) {
			var partialStateMachine testStateMachine

			restoreArgs := helper.Setup()
			defer restoreArgs()

			partialStateMachine.tempLocation = tc.tempLocation
			workDir, err := os.MkdirTemp(tc.tempLocation, "ubuntu-image-"+tc.name+"-")
			if err != nil {
				t.Errorf("Failed to create temporary directory %s\n", workDir)
			}
			commands.StateMachineOptsPassed.WorkDir = workDir
			commands.StateMachineOptsPassed.Until = tc.pauseStep

			// don't check errors as some are expected here
			if err := partialStateMachine.Setup(); err != nil {
				t.Errorf("error in partialStateMachine.Setup(): %s", err.Error())
			}
			partialStateMachine.cleanWorkDir = false
			if err := partialStateMachine.Run(); err != nil {
				t.Errorf("error in partialStateMachine.Run(): %s", err.Error())
			}
			if err := partialStateMachine.Teardown(); err != nil {
				t.Errorf("error in partialStateMachine.Teardown(): %s", err.Error())
			}

			// mess with files or directories
			if tc.causeProblems != nil {
				tc.causeProblems(partialStateMachine.stateMachineFlags.WorkDir)
			}

			// try to resume the state machine
			commands.StateMachineOptsPassed.Resume = true
			commands.StateMachineOptsPassed.Until = ""
			var resumeStateMachine testStateMachine

			setupErr := resumeStateMachine.Setup()
			if setupErr == nil {
				runErr := resumeStateMachine.Run()
				if runErr == nil {
					t.Error("Expected an error but there was none!")
				}
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

		restoreArgs := helper.Setup()
		defer restoreArgs()

		var stateMachine testStateMachine
		commands.StateMachineOptsPassed.WorkDir = workDir
		commands.CommonOptsPassed.Debug = true

		stateMachine.Setup()

		// just use the one state
		stateMachine.states = testStates
		stdout, restoreStdout, err := helper.CaptureStd(&os.Stdout)
		if err != nil {
			t.Errorf("Failed to capture stdout: %s\n", err.Error())
		}

		stateMachine.Run()

		// restore stdout and check that the debug info was printed
		restoreStdout()
		readStdout, err := ioutil.ReadAll(stdout)
		if err != nil {
			t.Errorf("Failed to read stdout: %s\n", err.Error())
		}
		if !strings.Contains(string(readStdout), stateMachine.states[0].name) {
			t.Errorf("Expected state name \"%s\" to appear in output \"%s\"\n", stateMachine.states[0].name, string(readStdout))
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
		{"error_write_metadata", 13, stateFunc{"test_error_write_metadata", func(stateMachine *StateMachine) error {
			os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
			return nil
		}}},
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

			restoreArgs := helper.Setup()
			defer restoreArgs()

			commands.StateMachineOptsPassed.WorkDir = workDir
			var stateMachine testStateMachine
			stateMachine.Setup()

			// override the function, but save the old one
			oldStateFunc := stateMachine.states[tc.overrideState]
			stateMachine.states[tc.overrideState] = tc.newStateFunc
			defer func() {
				stateMachine.states[tc.overrideState] = oldStateFunc
			}()
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
		restoreArgs := helper.Setup()
		defer restoreArgs()

		/* create the parent dir of the workdir with restrictive permissions */
		parentDir := "/tmp/workdir_error-5c919639-b972-4265-807a-19cd23fd1936/"
		workDir := parentDir + testDir
		os.Mkdir(parentDir, 0000)

		commands.StateMachineOptsPassed.WorkDir = workDir
		commands.StateMachineOptsPassed.Thru = "make_temporary_directories"

		var stateMachine StateMachine
		stateMachine.states = classicStates

		stateMachine.setCommonOpts()
		if err := stateMachine.makeTemporaryDirectories(); err == nil {
			t.Errorf("Expected an error but there was none")
		}

		os.Chmod(parentDir, 0777)
		os.RemoveAll(parentDir)
	})
}

// TestErrorCreateTmp tests the scenario where creating a temporary workdir fails
func TestFailedCreateTmp(t *testing.T) {
	t.Run("test_error_creating_tmp", func(t *testing.T) {
		restoreArgs := helper.Setup()
		defer restoreArgs()

		var stateMachine testStateMachine
		stateMachine.tempLocation = "/tmp/this/path/better/not/exist/" + testDir

		if err := stateMachine.Setup(); err != nil {
			t.Errorf("Failed to set up stateMachine: %s", err.Error())
		}
		if err := stateMachine.Run(); err == nil {
			t.Error("Expected an error but there was none")
		}
	})
}
