package main

import (
	"errors"
	"flag"
	"io"
	"os"
	"testing"

	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/mock"
)

type MockedStateMachine struct {
	mock.Mock
	whenToFail string
}

func (mockSM *MockedStateMachine) Setup() error {
	if mockSM.whenToFail == "Setup" {
		return errors.New("Testing Error")
	}
	return nil
}

func (mockSM *MockedStateMachine) Run() error {
	if mockSM.whenToFail == "Run" {
		return errors.New("Testing Error")
	}
	return nil
}

func (mockSM *MockedStateMachine) Teardown() error {
	if mockSM.whenToFail == "Teardown" {
		return errors.New("Testing Error")
	}
	return nil
}

var mockedStateMachine MockedStateMachine

// TestValidCommands tests that certain valid commands are parsed correctly
func TestValidCommands(t *testing.T) {
	testCases := []struct {
		name        string
		command     string
		gadgetModel string
		expected    string
		isSnap      bool
	}{
		{"valid_snap_command", "snap", "model_assertion.yml", "snap functionality to be added", true},
		{"valid_classic_command", "classic", "gadget_tree.yml", "classic functionality to be added", false},
	}
	for _, tc := range testCases {
		tc := tc // capture range variable for parallel execution
		t.Run("test "+tc.name, func(t *testing.T) {
			// set up the command
			var args []string
			if tc.command != "" {
				args = append(args, tc.command)
			}
			if tc.gadgetModel != "" {
				args = append(args, tc.gadgetModel)
			}

			// finally, execute the command and check output
			ubuntuImageCommand := new(commands.UbuntuImageCommand)
			_, err := flags.ParseArgs(ubuntuImageCommand, args)
			if err != nil {
				t.Error("Did not expect an error but got", err)
			}

			// check that opts got the correct value
			var comparison string
			if tc.isSnap {
				comparison = ubuntuImageCommand.Snap.SnapArgsPassed.ModelAssertion
			} else {
				comparison = ubuntuImageCommand.Classic.ClassicArgsPassed.GadgetTree
			}
			if comparison != tc.gadgetModel {
				t.Errorf("Unexpected input file value \"%s\". Expected \"%s\"",
					comparison, tc.gadgetModel)
			}
		})
	}
}

// TestInvalidCommands tests a few invalid commands argument/flag combinations:
// ubuntu-image is run with a command that is neither snap nor classic
// ubuntu-image snap is run with no model assertion
// ubuntu-image classic is run with no gadget tree
// ubuntu-image is run with a nonexistent flag
func TestInvalidCommands(t *testing.T) {
	testCases := []struct {
		name     string
		command  []string
		flags    []string
		expected string
	}{
		{"invalid_command", []string{"test"}, nil, "invalid argument \"test\" for \"ubuntu-image\""},
		{"no_model_assertion", []string{"snap"}, nil, "accepts 1 arg(s), received 0"},
		{"no_gadget_tree", []string{"classic"}, nil, "accepts 1 arg(s), received 0"},
		{"invalid_flag", []string{"classic"}, []string{"--nonexistent"}, "unknown flag: --nonexistent"},
	}
	for _, tc := range testCases {
		tc := tc // capture range variable for parallel execution
		t.Run("test "+tc.name, func(t *testing.T) {
			// set up the command
			var args []string
			if tc.command != nil {
				args = append(args, tc.command...)
			}
			if tc.flags != nil {
				args = append(args, tc.flags...)
			}

			// finally, execute the command and check output
			ubuntuImageCommand := new(commands.UbuntuImageCommand)
			_, err := flags.ParseArgs(ubuntuImageCommand, args)
			if err == nil {
				t.Error("Expected an error but none was found")
			}
		})
	}
}

// TestExit code runs a number of commands, both valid and invalid, and ensures that the
// program exits with the correct exit code
func TestExitCode(t *testing.T) {
	testCases := []struct {
		name     string
		flags    []string
		expected int
	}{
		{"help_exit_0", []string{"--help"}, 0},
		{"snap_exit_0", []string{"snap", "model_assertion.yml"}, 0},
		{"classic_exit_0", []string{"classic", "gadget_tree.yml", "--project", "ubuntu-cpc"}, 0},
		{"workdir_exit_0", []string{"classic", "gadget_tree.yml", "--project", "ubuntu-cpc", "--workdir", "/tmp/ubuntu-image-0615c8dd-d3af-4074-bfcb-c3d3c8392b06"}, 0},
		{"invalid_flag_exit_1", []string{"--help-me"}, 1},
		{"bad_state_machine_args", []string{"classic", "gadget_tree.yaml", "-u", "5", "-t", "6"}, 1},
		{"no_command_given", []string{}, 1},
		{"resume_without_workdir", []string{"--resume"}, 1},
	}
	for _, tc := range testCases {
		t.Run("test "+tc.name, func(t *testing.T) {
			// Override os.Exit temporarily
			oldOsExit := osExit
			defer func() {
				osExit = oldOsExit
			}()

			var got int
			tmpExit := func(code int) {
				got = code
			}

			osExit = tmpExit

			// set up the flags for the test cases
			flag.CommandLine = flag.NewFlagSet(tc.name, flag.ExitOnError)
			os.Args = append([]string{tc.name}, tc.flags...)

			// os.Exit will be captured. Run the command with no flags to trigger an error
			imageType = ""
			main()
			if got != tc.expected {
				t.Errorf("Expected exit code: %d, got: %d", tc.expected, got)
			}
			os.RemoveAll("/tmp/ubuntu-image-0615c8dd-d3af-4074-bfcb-c3d3c8392b06")
		})
	}
}

// TestFailedStdoutStderrCapture tests that scenarios involving failed stdout
// and stderr captures and reads fail gracefully
func TestFailedStdoutStderrCapture(t *testing.T) {
	testCases := []struct {
		name     string
		stdCap   *os.File
		readFrom *os.File
		flags    []string
	}{
		{"error_capture_stdout", os.Stdout, os.Stdout, []string{}},
		{"error_capture_stderr", os.Stderr, os.Stderr, []string{}},
		{"error_read_stdout", os.Stdout, nil, []string{"--help"}},
		{"error_read_stderr", os.Stderr, nil, []string{}},
	}
	for _, tc := range testCases {
		t.Run("test "+tc.name, func(t *testing.T) {
			// Override os.Exit temporarily
			oldOsExit := osExit
			defer func() {
				osExit = oldOsExit
			}()

			var got int
			tmpExit := func(code int) {
				got = code
			}

			osExit = tmpExit

			// os.Exit will be captured. set the captureStd function
			captureStd = func(toCap **os.File) (io.Reader, func(), error) {
				var err error
				if *toCap == tc.readFrom {
					err = errors.New("Testing Error")
				} else {
					err = nil
				}
				return tc.readFrom, func() { return }, err
			}

			// set up the flags for the test cases
			flag.CommandLine = flag.NewFlagSet(tc.name, flag.ExitOnError)
			os.Args = append([]string{tc.name}, tc.flags...)

			// run main and check the exit code
			imageType = ""
			main()
			if got != 1 {
				t.Errorf("Expected error code on exit, got: %d", got)
			}

		})
	}
}

// TestFailedStateMachine tests fails for all implemented functions to ensure
// that main fails gracefully
func TestFailedStateMachine(t *testing.T) {
	testCases := []struct {
		name       string
		whenToFail string
	}{
		{"error_statemachine_setup", "Setup"},
		{"error_statemachine_run", "Run"},
		{"error_statemachine_teardown", "Teardown"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Override os.Exit temporarily
			oldOsExit := osExit
			defer func() {
				osExit = oldOsExit
			}()

			var got int
			tmpExit := func(code int) {
				got = code
			}

			osExit = tmpExit

			flags := []string{"snap", "model_assertion"}
			// set up the flags for the test cases
			flag.CommandLine = flag.NewFlagSet("failed_state_machine", flag.ExitOnError)
			os.Args = append([]string{"failed_state_machine"}, flags...)

			// this stops main from using the snapSM or classicSm
			imageType = "test"

			mockedStateMachine.whenToFail = tc.whenToFail
			stateMachineInterface = &mockedStateMachine
			main()
			if got != 1 {
				t.Errorf("Expected error code on exit, got: %d", got)
			}
		})
	}
}
