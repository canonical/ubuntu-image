package main

import (
	"flag"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/canonical/ubuntu-image/internal/helper"
)

/* This function tests valid commands. It will need to be updated
 * when real functionality is included */
func TestValidCommands(t *testing.T) {
	testCases := []struct {
		name        string
		command     string
		gadgetModel string
		expected    string
	}{
		{"valid snap command", "snap", "model_assertion.yml", "snap functionality to be added"},
		{"valid classic command", "classic", "gadget_tree.yml", "classic functionality to be added"},
	}
	for _, tc := range testCases {
		tc := tc // capture range variable for parallel execution
		t.Run("test "+tc.name, func(t *testing.T) {

			// capture stdout for the test commands
			stdout, restoreStdout := helper.CaptureStdout(t)
			defer restoreStdout()

			// set up the command
			cmd := generateRootCmd()
			var args []string
			if tc.command != "" {
				args = append(args, tc.command)
			}
			if tc.gadgetModel != "" {
				args = append(args, tc.gadgetModel)
			}
			cmd.SetArgs(args)

			// finally, execute the command and check output
			err := cmd.Execute()
			restoreStdout()
			if err != nil {
				t.Error("Did not expect an error but got", err)
			}

			got, err := ioutil.ReadAll(stdout)
			if err != nil {
				t.Error("Failed to read stdout", err)
			}
			if !strings.Contains(string(got), tc.expected) {
				t.Errorf("Unexpected output. Expected \"%s\" to be in output: \"%s\"",
					tc.expected, string(got))
			}
		})
	}
}

/* This function tests a few invalid commands:
 * ubuntu-image is run with a command that is neither snap nor classic
 * ubuntu-image snap is run with no model assertion
 * ubuntu-image classic is run with no gadget tree
 * ubuntu-image is run with a nonexistent flag
 * ubuntu-image is run with too many commands/args */
func TestInvalidCommands(t *testing.T) {
	testCases := []struct {
		name     string
		command  []string
		flags    []string
		expected string
	}{
		{"Invalid Command", []string{"test"}, nil, "invalid argument \"test\" for \"ubuntu-image\""},
		{"No Model Assertion", []string{"snap"}, nil, "accepts 1 arg(s), received 0"},
		{"No Gadget Tree", []string{"classic"}, nil, "accepts 1 arg(s), received 0"},
		{"Invalid Flag", []string{"classic"}, []string{"--nonexistent"}, "unknown flag: --nonexistent"},
		{"Two Commands", []string{"snap", "classic", "gadget.yml"}, nil, "accepts 1 arg(s), received 2"},
	}
	for _, tc := range testCases {
		tc := tc // capture range variable for parallel execution
		t.Run("test "+tc.name, func(t *testing.T) {

			// capture stderr for the test commands
			stderr, restoreStderr := helper.CaptureStderr(t)
			defer restoreStderr()

			// set up the command
			cmd := generateRootCmd()
			var args []string
			if tc.command != nil {
				args = append(args, tc.command...)
			}
			if tc.flags != nil {
				args = append(args, tc.flags...)
			}
			cmd.SetArgs(args)

			// finally, execute the command and check output
			err := cmd.Execute()
			restoreStderr()
			if err == nil {
				t.Error("Expected an error but none was found")
			}

			got, err := ioutil.ReadAll(stderr)
			if err != nil {
				t.Error("Failed to read stderr", err)
			}
			if !strings.Contains(string(got), tc.expected) {
				t.Errorf("Unexpected output. Expected \"%s\" to be in output: \"%s\"",
					tc.expected, string(got))
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
		{"exit 0", []string{"--help"}, 0},
		{"exit 1", []string{"--help-me"}, 1},
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
		})
	}
}
