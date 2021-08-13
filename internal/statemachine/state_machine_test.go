package statemachine

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/snapcore/snapd/osutil"
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

// define some mocked versions of go package functions
func mockReadDir(string) ([]os.FileInfo, error) {
	return []os.FileInfo{}, fmt.Errorf("Test Error")
}
func mockReadFile(string) ([]byte, error) {
	return []byte{}, fmt.Errorf("Test Error")
}
func mockMkdir(string, os.FileMode) error {
	return fmt.Errorf("Test error")
}
func mockMkdirAll(string, os.FileMode) error {
	return fmt.Errorf("Test error")
}
func mockRemoveAll(string) error {
	return fmt.Errorf("Test error")
}
func mockCreate(string) (*os.File, error) {
	return nil, fmt.Errorf("Test error")
}
func mockCopyFile(string, string, osutil.CopyFlag) error {
	return fmt.Errorf("Test error")
}
func mockCopySpecialFile(string, string) error {
	return fmt.Errorf("Test error")
}

// getStateNumberByName returns the numeric order of a state based on its name
func (stateMachine *StateMachine) getStateNumberByName(name string) int {
	for i, stateFunc := range stateMachine.states {
		if name == stateFunc.name {
			return i
		}
	}
	return -1
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
		stateMachine.Teardown()
		if _, err := os.Stat(stateMachine.stateMachineFlags.WorkDir); err == nil {
			t.Errorf("Error: temporary workdir %s was not cleaned up\n", stateMachine.stateMachineFlags.WorkDir)
		}
	})
}

// TestFailedCleanup tests a failure in os.RemoveAll while deleting the temporary directory
func TestFailedCleanup(t *testing.T) {
	t.Run("test_failed_cleanup", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.cleanWorkDir = true

		osRemoveAll = mockRemoveAll
		defer func() {
			osRemoveAll = os.RemoveAll
		}()
		if err := stateMachine.cleanup(); err == nil {
			t.Error("Expected an error, but there was none!")
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

// TestFailedMetadataParse tests a failure in parsing the metadata file. This is accomplished
// by giving the state machine a syntactically invalid metadata file to parse
func TestFailedMetadataParse(t *testing.T) {
	t.Run("test_failed_metadata_parse", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.stateMachineFlags.Resume = true
		stateMachine.stateMachineFlags.WorkDir = "testdata"

		if err := stateMachine.readMetadata(); err == nil {
			t.Errorf("Expected an error but there was none")
		}
	})
}
