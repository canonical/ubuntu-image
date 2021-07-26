// TODO: some tests are commented out. They now fail because "needs-root" was added to d/tests/control
// they can be "mocked" similar to how os.Exit is mocked in main to reach 100% coverage
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
	{"prepare_gadget_tree", func(statemachine *StateMachine) error { return nil }},
	{"prepare_image", func(statemachine *StateMachine) error { return nil }},
	{"load_gadget_yaml", func(statemachine *StateMachine) error { return nil }},
	{"populate_rootfs_contents", func(statemachine *StateMachine) error { return nil }},
	{"populate_rootfs_contents_hooks", func(statemachine *StateMachine) error { return nil }},
	{"generate_disk_info", func(statemachine *StateMachine) error { return nil }},
	{"calculate_rootfs_size", func(statemachine *StateMachine) error { return nil }},
	{"prepopulate_bootfs_contents", func(statemachine *StateMachine) error { return nil }},
	{"populate_bootfs_contents", func(statemachine *StateMachine) error { return nil }},
	{"populate_prepare_partitions", func(statemachine *StateMachine) error { return nil }},
	{"make_disk", func(statemachine *StateMachine) error { return nil }},
	{"generate_manifest", func(statemachine *StateMachine) error { return nil }},
	{"finish", (*StateMachine).finish},
}

// testStateMachine implements Setup, Run, and Teardown() for testing purposes
type testStateMachine struct {
	StateMachine
}

// testStateMachine needs its own setup
func (TestStateMachine *testStateMachine) Setup() error {
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

// TestCleanup ensures that the temporary workdir is cleaned up after the
// state machine has finished running
func TestCleanup(t *testing.T) {
	t.Run("test_cleanup", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.Run()
		if _, err := os.Stat(stateMachine.stateMachineFlags.WorkDir); err == nil {
			t.Errorf("Error: temporary workdir %s was not cleaned up\n", stateMachine.stateMachineFlags.WorkDir)
		}
	})
}

// TestUntilThru tests --until and --thru with each state
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
				// run a partial state machine
				var partialStateMachine testStateMachine
				partialStateMachine.commonFlags, partialStateMachine.stateMachineFlags = helper.InitCommonOpts()
				tempDir := "ubuntu-image-" + tc.name
				if err := os.Mkdir(tempDir, 0755); err != nil {
					t.Errorf("Could not create workdir: %s\n", err.Error())
				}
				defer os.RemoveAll(tempDir)
				partialStateMachine.stateMachineFlags.WorkDir = tempDir

				if tc.name == "until" {
					partialStateMachine.stateMachineFlags.Until = state.name
				} else {
					partialStateMachine.stateMachineFlags.Thru = state.name
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
				resumeStateMachine.commonFlags, resumeStateMachine.stateMachineFlags = helper.InitCommonOpts()
				resumeStateMachine.stateMachineFlags.Resume = true
				resumeStateMachine.stateMachineFlags.WorkDir = partialStateMachine.stateMachineFlags.WorkDir

				if err := resumeStateMachine.Setup(); err != nil {
					t.Errorf("Failed to resume state machine: %s", err.Error())
				}
				if err := resumeStateMachine.Run(); err != nil {
					t.Errorf("Failed to resume state machine from state: %s\n", state.name)
				}
				if err := resumeStateMachine.Teardown(); err != nil {
					t.Errorf("Failed to resume state machine: %s", err.Error())
				}
				os.RemoveAll(tempDir)
			}
		})
	}
}

// TestInvalidStateMachineArgs tests that invalid state machine command line arguments result in a failure
func TestInvalidStateMachineArgs(t *testing.T) {
	testCases := []struct {
		name   string
		until  string
		thru   string
		resume bool
	}{
		{"both_until_and_thru", "make_temporary_directories", "calculate_rootfs_size", false},
		{"invalid_until_name", "fake step", "", false},
		{"invalid_thru_name", "", "fake step", false},
		{"resume_with_no_workdir", "", "", true},
	}

	for _, tc := range testCases {
		t.Run("test "+tc.name, func(t *testing.T) {
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.stateMachineFlags.Until = tc.until
			stateMachine.stateMachineFlags.Thru = tc.thru
			stateMachine.stateMachineFlags.Resume = tc.resume

			if err := stateMachine.validateInput(); err == nil {
				t.Error("Expected an error but there was none!")
			}
		})
	}
}

// TestFileErrors tests a number of different file i/o and permissions errors to ensure
// that the program errors cleanly
/*func TestFileErrors(t *testing.T) {
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
			partialStateMachine.commonFlags, partialStateMachine.stateMachineFlags = helper.InitCommonOpts()

			partialStateMachine.tempLocation = tc.tempLocation
			workDir, err := os.MkdirTemp(tc.tempLocation, "ubuntu-image-"+tc.name+"-")
			if err != nil {
				t.Errorf("Failed to create temporary directory %s\n", workDir)
			}
			partialStateMachine.stateMachineFlags.WorkDir = workDir
			partialStateMachine.stateMachineFlags.Until = tc.pauseStep

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
			var resumeStateMachine testStateMachine
			resumeStateMachine.commonFlags, resumeStateMachine.stateMachineFlags = helper.InitCommonOpts()
			resumeStateMachine.stateMachineFlags.Resume = true
			resumeStateMachine.stateMachineFlags.WorkDir = partialStateMachine.stateMachineFlags.WorkDir

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
}*/

// TestDebug ensures that the name of the states is printed when the --debug flag is used
func TestDebug(t *testing.T) {
	t.Run("test_debug", func(t *testing.T) {
		workDir := "ubuntu-image-test-debug"
		if err := os.Mkdir("ubuntu-image-test-debug", 0755); err != nil {
			t.Errorf("Failed to create temporary directory %s\n", workDir)
		}

		var stateMachine testStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.stateMachineFlags.WorkDir = workDir
		stateMachine.commonFlags.Debug = true

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

// TestFunction replaces some of the stateFuncs to test various error scenarios
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
			workDir := "ubuntu-image-" + tc.name
			if err := os.Mkdir(workDir, 0755); err != nil {
				t.Errorf("Failed to create temporary directory %s\n", workDir)
			}

			var stateMachine testStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.stateMachineFlags.WorkDir = workDir
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

// TestFailedCreateWorkdir tests a failure in the "MkdirAll" call that happens when --workdir is used
/*func TestFailedCreateWorkDir(t *testing.T) {
	t.Run("test_error_creating_workdir", func(t *testing.T) {
		// create the parent dir of the workdir with restrictive permissions
		parentDir := "/tmp/workdir_error-5c919639-b972-4265-807a-19cd23fd1936/"
		workDir := parentDir + testDir
		os.Mkdir(parentDir, 0000)

		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.stateMachineFlags.WorkDir = workDir
		stateMachine.stateMachineFlags.Thru = "make_temporary_directories"
		stateMachine.states = allTestStates

		if err := stateMachine.makeTemporaryDirectories(); err == nil {
			t.Errorf("Expected an error but there was none")
		}

		os.Chmod(parentDir, 0777)
		os.RemoveAll(parentDir)
	})
}*/

// TestFailedCreateTmp tests the scenario where creating a temporary workdir fails
/*func TestFailedCreateTmp(t *testing.T) {
	t.Run("test_error_creating_tmp", func(t *testing.T) {
		var stateMachine testStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.tempLocation = "/tmp/this/path/better/not/exist/" + testDir

		if err := stateMachine.Setup(); err != nil {
			t.Errorf("Failed to set up stateMachine: %s", err.Error())
		}
		if err := stateMachine.Run(); err == nil {
			t.Error("Expected an error but there was none")
		}
	})
}*/

// TestSetCommonOpts ensures that the function actually sets the correct values in the struct
func TestSetCommonOpts(t *testing.T) {
	t.Run("test_set_common_opts", func(t *testing.T) {
		commonOpts := new(commands.CommonOpts)
		stateMachineOpts := new(commands.StateMachineOpts)
		commonOpts.Debug = true
		stateMachineOpts.WorkDir = testDir

		var stateMachine testStateMachine
		stateMachine.SetCommonOpts(commonOpts, stateMachineOpts)

		if !stateMachine.commonFlags.Debug || stateMachine.stateMachineFlags.WorkDir != testDir {
			t.Error("SetCommonOpts failed to set the correct options")
		}
	})
}
