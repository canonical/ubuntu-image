package main

import (
	"errors"
	"flag"
	"io"
	"os"
	"testing"

	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/jessevdk/go-flags"
)

/* This function tests valid commands. It will need to be updated
 * when real functionality is included */
func TestValidCommands(t *testing.T) {
	testCases := []struct {
		name        string
		command     string
		gadgetModel string
		expected    string
		isSnap      bool
	}{
		{"valid snap command", "snap", "model_assertion.yml", "snap functionality to be added", true},
		{"valid classic command", "classic", "gadget_tree.yml", "classic functionality to be added", false},
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
			args, err := flags.ParseArgs(&commands.UbuntuImageCommand, args)
			if err != nil {
				t.Error("Did not expect an error but got", err)
			}

			// check that opts got the correct value
			var comparison string
			if tc.isSnap {
				comparison = commands.UbuntuImageCommand.Snap.SnapArgs.ModelAssertion
			} else {
				comparison = commands.UbuntuImageCommand.Classic.ClassicArgs.GadgetTree
			}
			if comparison != tc.gadgetModel {
				t.Errorf("Unexpected input file value \"%s\". Expected \"%s\"",
					comparison, tc.gadgetModel)
			}
		})
	}
}

/* This function tests a few invalid commands:
 * ubuntu-image is run with a command that is neither snap nor classic
 * ubuntu-image snap is run with no model assertion
 * ubuntu-image classic is run with no gadget tree
 * ubuntu-image is run with a nonexistent flag */
func TestInvalidCommands(t *testing.T) {
	testCases := []struct {
		name     string
		command  []string
		flags    []string
		expected string
	}{
		{"invalid command", []string{"test"}, nil, "invalid argument \"test\" for \"ubuntu-image\""},
		{"no model assertion", []string{"snap"}, nil, "accepts 1 arg(s), received 0"},
		{"no gadget tree", []string{"classic"}, nil, "accepts 1 arg(s), received 0"},
		{"invalid flag", []string{"classic"}, []string{"--nonexistent"}, "unknown flag: --nonexistent"},
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
			args, err := flags.ParseArgs(&commands.UbuntuImageCommand, args)
			if err == nil {
				t.Error("Expected an error but none was found")
			}
		})
	}
}

/* This function tests that running the "ubuntu-image" command
 * with valid arguments results in an exit code of 0 and that
 * running with invalid arguments results in an exit code of 1 */
func TestExitCode(t *testing.T) {
	testCases := []struct {
		name     string
		flags    []string
		expected int
	}{
		{"snap exit 0", []string{"snap", "model_assertion.yml"}, 0},
		{"classic exit 0", []string{"classic", "gadget_tree.yml"}, 0},
		{"workdir exit 0", []string{"classic", "gadget_tree.yml", "--workdir", "/tmp/ubuntu-image"}, 0},
		{"exit 1", []string{"--help-me"}, 1},
		{"help exit 0", []string{"--help"}, 0},
		{"bad state machine args", []string{"classic", "gadget_tree.yaml", "-u", "5", "-t", "6"}, 1},
		{"no command given", []string{}, 1},
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
			main()
			if got != tc.expected {
				t.Errorf("Expected exit code: %d, got: %d", tc.expected, got)
			}
			os.RemoveAll("/tmp/ubuntu-image")
		})
	}
}

/* This function tests error scenarios in utility functions like capturing
 * from stdout or stderr. It also checks the scenario where stdout and stderr
 * are successfully captured, but a subsequent read from them fails */
func TestFailedStdoutStderrCapture(t *testing.T) {
	testCases := []struct {
		name     string
		stdCap   *os.File
		readFrom *os.File
		flags    []string
	}{
		{"error capture stdout", os.Stdout, os.Stdout, []string{}},
		{"error capture stderr", os.Stderr, os.Stderr, []string{}},
		{"error read stdout", os.Stdout, nil, []string{"--help"}},
		{"error read stderr", os.Stderr, nil, []string{}},
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
				if *toCap == tc.stdCap {
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
			main()
			if got != 1 {
				t.Errorf("Expected error code on exit, got: %d", got)
			}

		})
	}
}
