package main

import (
	"errors"
	"flag"
	"io"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/jessevdk/go-flags"
	"github.com/snapcore/snapd/gadget"

	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/canonical/ubuntu-image/internal/statemachine"
	"github.com/canonical/ubuntu-image/internal/testhelper"
)

var (
	ErrAtSetup    = errors.New("Fail at Setup")
	ErrAtRun      = errors.New("Fail at Run")
	ErrAtTeardown = errors.New("Fail at Teardown")
)

type mockedStateMachine struct {
	whenToFail string
}

func (mockSM *mockedStateMachine) Setup() error {
	if mockSM.whenToFail == "Setup" {
		return ErrAtSetup
	}
	return nil
}

func (mockSM *mockedStateMachine) Run() error {
	if mockSM.whenToFail == "Run" {
		return ErrAtRun
	}
	return nil
}

func (mockSM *mockedStateMachine) Teardown() error {
	if mockSM.whenToFail == "Teardown" {
		return ErrAtTeardown
	}
	return nil
}

func (mockSM *mockedStateMachine) SetCommonOpts(commonOpts *commands.CommonOpts, stateMachineOpts *commands.StateMachineOpts) {
}

func (mockSM *mockedStateMachine) SetSeries() error {
	return nil
}

// TestValidCommands tests that certain valid commands are parsed correctly
func TestValidCommands(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name    string
		command string
		flags   []string
		field   func(*commands.UbuntuImageCommand) string
		want    string
	}{
		{
			name:    "valid_snap_command",
			command: "snap",
			flags:   []string{"model_assertion.yml"},
			field: func(u *commands.UbuntuImageCommand) string {
				return u.Snap.SnapArgsPassed.ModelAssertion
			},
			want: "model_assertion.yml",
		},
		{
			name:    "valid_classic_command",
			command: "classic",
			flags:   []string{"image_defintion.yml"},
			field: func(u *commands.UbuntuImageCommand) string {
				return u.Classic.ClassicArgsPassed.ImageDefinition
			},
			want: "image_defintion.yml",
		},
		{
			name:    "valid_pack_command",
			command: "pack",
			flags:   []string{"--artifact-type", "raw", "--gadget-dir", "./test-gadget-dir", "--rootfs-dir", "./test"},
			field: func(u *commands.UbuntuImageCommand) string {
				return u.Pack.PackOptsPassed.GadgetDir
			},
			want: "./test-gadget-dir",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var args []string
			if tc.command != "" {
				args = append(args, tc.command)
			}
			if tc.flags != nil {
				args = append(args, tc.flags...)
			}

			ubuntuImageCommand := &commands.UbuntuImageCommand{}
			_, err := flags.ParseArgs(ubuntuImageCommand, args)
			if err != nil {
				t.Error("Did not expect an error but got", err)
			}

			got := tc.field(ubuntuImageCommand)
			if tc.want != got {
				t.Errorf("Unexpected parsed value \"%s\". Expected \"%s\"",
					got, tc.want)
			}
		})
	}
}

// TestInvalidCommands tests invalid commands argument/flag combinations
func TestInvalidCommands(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name          string
		command       []string
		flags         []string
		expectedError string
	}{
		{"invalid_command", []string{"test"}, nil, "Unknown command `test'. Please specify one command of: classic or snap"},
		{"no_model_assertion", []string{"snap"}, nil, "the required argument `model_assertion` was not provided"},
		{"no_gadget_tree", []string{"classic"}, nil, "the required argument `image_definition` was not provided"},
		{"invalid_flag", []string{"classic"}, []string{"--nonexistent"}, "unknown flag `nonexistent'"},
		{"invalid_validation", []string{"snap"}, []string{"--validation=test"}, "unknown flag `validation'"},
		{"invalid_sector_size", []string{"snap"}, []string{"--sector_size=123"}, "unknown flag `sector_size'"},
		{"missing_one_flag", []string{"pack"}, []string{"--artifact-type=raw"}, "the required flags `--gadget-dir' and `--rootfs-dir' were not specified"},
		{"missing_flags", []string{"pack"}, []string{"--artifact-type=raw", "--gadget-dir=./test"}, "the required flag `--rootfs-dir' was not specified"},
	}
	for _, tc := range testCases {
		tc := tc // capture range variable for parallel execution
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}

			var args []string
			if tc.command != nil {
				args = append(args, tc.command...)
			}
			if tc.flags != nil {
				args = append(args, tc.flags...)
			}

			// finally, execute the command and check output
			ubuntuImageCommand := &commands.UbuntuImageCommand{}
			_, gotErr := flags.ParseArgs(ubuntuImageCommand, args)
			asserter.AssertErrContains(gotErr, tc.expectedError)

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
		{"invalid_flag_exit_1", []string{"--help-me"}, 1},
		{"bad_state_machine_args_classic", []string{"classic", "gadget_tree.yaml", "-u", "5", "-t", "6"}, 1},
		{"bad_state_machine_args_snap", []string{"snap", "model_assertion.yaml", "-u", "5", "-t", "6"}, 1},
		{"bad_state_machine_args_pack", []string{"pack", "--artifact-type", "raw", "--gadget-dir", "./test-gadget-dir", "--rootfs-dir", "./test", "-u", "5", "-t", "6"}, 1},
		{"no_command_given", []string{}, 1},
		{"resume_without_workdir", []string{"--resume"}, 1},
		{"invalid_sector_size", []string{"--sector-size", "128", "--help"}, 1}, // Cheap trick with the --help to make the test work
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			restoreCWD := testhelper.SaveCWD()
			defer restoreCWD()
			// Override os.Exit temporarily
			oldOsExit := osExit
			t.Cleanup(func() {
				osExit = oldOsExit
			})

			var got int
			tmpExit := func(code int) {
				got = code
			}

			osExit = tmpExit

			// set up the flags for the test cases
			flag.CommandLine = flag.NewFlagSet(tc.name, flag.ExitOnError)
			os.Args = append([]string{tc.name}, tc.flags...)

			// os.Exit will be captured. Run the command with no flags to trigger an error
			main()
			if got != tc.expected {
				t.Errorf("Expected exit code: %d, got: %d", tc.expected, got)
			}
			os.RemoveAll("/tmp/ubuntu-image-0615c8dd-d3af-4074-bfcb-c3d3c8392b06")
		})
	}
}

// TestVersion code runs ubuntu-image --version and checks if the resulting
// version makes sense
func TestVersion(t *testing.T) {
	testCases := []struct {
		name      string
		hardcoded string
		snapEnv   string
		expected  string
	}{
		{"hardcoded_version", "2.0ubuntu1", "", "2.0ubuntu1"},
		{"snap_version", "", "2.0+snap1", "2.0+snap1"},
		{"both_hardcoded_and_snap", "2.0ubuntu1", "2.0+snap1", "2.0ubuntu1"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			restoreCWD := testhelper.SaveCWD()
			defer restoreCWD()
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
			os.Args = append([]string{tc.name}, "--version")

			// pre-set the test-case environment
			Version = tc.hardcoded
			os.Setenv("SNAP_VERSION", tc.snapEnv)

			main()
			if got != 0 {
				t.Errorf("Expected exit code: 0, got: %d", got)
			}
			os.Unsetenv("SNAP_VERSION")

			// since we're printing the Version variable, no need to capture
			// and analyze the output
			if Version != tc.expected {
				t.Errorf("Expected version string: '%s', got: '%s'", tc.expected, Version)
			}
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

			// os.Exit will be captured. set the captureStd function
			captureStd = func(toCap **os.File) (io.Reader, func(), error) {
				var err error
				if *toCap == tc.readFrom {
					err = errors.New("Testing Error")
				} else {
					err = nil
				}
				return tc.readFrom, func() {}, err
			}

			// set up the flags for the test cases
			flag.CommandLine = flag.NewFlagSet(tc.name, flag.ExitOnError)
			os.Args = append([]string{tc.name}, tc.flags...)

			// run main and check the exit code
			main()
			if got != 1 {
				t.Errorf("Expected error code on exit, got: %d", got)
			}

		})
	}
}

// TestExecuteStateMachine tests fails for all implemented functions to ensure
// that main fails gracefully
func TestExecuteStateMachine(t *testing.T) {
	testCases := []struct {
		name          string
		whenToFail    string
		expectedError string
	}{
		{
			name:          "error_statemachine_setup",
			whenToFail:    "Setup",
			expectedError: ErrAtSetup.Error(),
		},
		{
			name:          "error_statemachine_run",
			whenToFail:    "Run",
			expectedError: ErrAtRun.Error(),
		},
		{
			name:          "error_statemachine_teardown",
			whenToFail:    "Teardown",
			expectedError: ErrAtTeardown.Error(),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}

			flags := []string{"snap", "model_assertion"}
			// set up the flags for the test cases
			flag.CommandLine = flag.NewFlagSet("failed_state_machine", flag.ExitOnError)
			os.Args = flags

			gotErr := executeStateMachine(&mockedStateMachine{
				whenToFail: tc.whenToFail,
			})
			asserter.AssertErrContains(gotErr, tc.expectedError)
		})
	}
}

func Test_initStateMachine(t *testing.T) {
	asserter := helper.Asserter{T: t}
	type args struct {
		imageType          string
		commonOpts         *commands.CommonOpts
		stateMachineOpts   *commands.StateMachineOpts
		ubuntuImageCommand *commands.UbuntuImageCommand
	}

	cmpOpts := []cmp.Option{
		cmpopts.IgnoreUnexported(
			statemachine.SnapStateMachine{},
			statemachine.StateMachine{},
			gadget.Info{},
		),
	}

	tests := []struct {
		name        string
		args        args
		want        statemachine.SmInterface
		expectedErr string
	}{
		{
			name: "init a snap state machine",
			args: args{
				imageType:        "snap",
				commonOpts:       &commands.CommonOpts{},
				stateMachineOpts: &commands.StateMachineOpts{},
				ubuntuImageCommand: &commands.UbuntuImageCommand{
					Snap: commands.SnapCommand{
						SnapOptsPassed: commands.SnapOpts{},
						SnapArgsPassed: commands.SnapArgs{},
					},
				},
			},
			want: &statemachine.SnapStateMachine{
				StateMachine: statemachine.StateMachine{},
				Opts:         commands.SnapOpts{},
				Args:         commands.SnapArgs{},
			},
		},
		{
			name: "init a classic state machine",
			args: args{
				imageType:        "classic",
				commonOpts:       &commands.CommonOpts{},
				stateMachineOpts: &commands.StateMachineOpts{},
				ubuntuImageCommand: &commands.UbuntuImageCommand{
					Classic: commands.ClassicCommand{
						ClassicArgsPassed: commands.ClassicArgs{},
					},
				},
			},
			want: &statemachine.ClassicStateMachine{
				Args: commands.ClassicArgs{},
			},
		},
		{
			name: "fail to init an unknown statemachine",
			args: args{
				imageType:          "unknown",
				commonOpts:         &commands.CommonOpts{},
				stateMachineOpts:   &commands.StateMachineOpts{},
				ubuntuImageCommand: &commands.UbuntuImageCommand{},
			},
			want:        nil,
			expectedErr: "unsupported command",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := initStateMachine(tc.args.imageType, tc.args.commonOpts, tc.args.stateMachineOpts, tc.args.ubuntuImageCommand)

			if err != nil || len(tc.expectedErr) != 0 {
				asserter.AssertErrContains(err, tc.expectedErr)
			}

			asserter.AssertEqual(tc.want, got, cmpOpts...)

		})
	}
}
