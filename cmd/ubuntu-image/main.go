package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/canonical/ubuntu-image/internal/statemachine"
	"github.com/jessevdk/go-flags"
)

// helper variables for unit testing
var osExit = os.Exit
var captureStd = helper.CaptureStd

var stateMachineLongDesc = `Options for controlling the internal state machine.
Other than -w, these options are mutually exclusive. When -u or -t is given,
the state machine can be resumed later with -r, but -w must be given in that
case since the state is saved in a .ubuntu-image.gob file in the working directory.`

func main() {

	parser := flags.NewParser(&commands.UbuntuImageCommand, flags.Default)
	parser.AddGroup("State Machine Options", stateMachineLongDesc, &commands.StateMachineOpts)
	parser.AddGroup("Common Options", "Options common to both commands", &commands.CommonOpts)

	// go-flags can be overzealous about printing errors that aren't actually errors
	// so we capture stdout/stderr while parsing and later decide whether to print
	stdout, restoreStdout, err := captureStd(&os.Stdout)
	if err != nil {
		fmt.Printf("Failed to capture stdout: %s\n", err.Error())
		osExit(1)
	}
	stderr, restoreStderr, err := captureStd(&os.Stderr)
	if err != nil {
		fmt.Printf("Failed to capture stderr: %s\n", err.Error())
		osExit(1)
	}

	// Parse the options provided and handle specific errors
	if _, err := parser.Parse(); err != nil {
		if e, ok := err.(*flags.Error); ok {
			switch e.Type {
			case flags.ErrHelp:
				restoreStdout()
				restoreStderr()
				readStdout, err := ioutil.ReadAll(stdout)
				if err != nil {
					fmt.Printf("Error reading from stdout: %s\n", err.Error())
					osExit(1)
					break
				}
				fmt.Println(string(readStdout))
				osExit(0)
			case flags.ErrCommandRequired:
				// if --resume was given, this is not an error
				if !commands.StateMachineOpts.Resume {
					restoreStdout()
					restoreStderr()
					readStderr, err := ioutil.ReadAll(stderr)
					if err != nil {
						fmt.Printf("Error reading from stderr: %s\n", err.Error())
						osExit(1)
						break
					}
					fmt.Printf("Error: %s", string(readStderr))
					osExit(1)
				}
				break
			default:
				restoreStdout()
				restoreStderr()
				fmt.Printf("Error: %s", err.Error())
				osExit(1)
			}
		}
	}

	// restore stdout
	restoreStdout()
	restoreStderr()

	// Set up the state machine
	var stateMachine statemachine.StateMachine
	stateMachine.Until = commands.StateMachineOpts.Until
	stateMachine.Thru = commands.StateMachineOpts.Thru
	stateMachine.Resume = commands.StateMachineOpts.Resume
	stateMachine.Debug = commands.CommonOpts.Debug
	if commands.StateMachineOpts.WorkDir == "" {
		// set the workdir to a /tmp directory
		stateMachine.CleanWorkDir = true
	} else {
		stateMachine.WorkDir = commands.StateMachineOpts.WorkDir
		stateMachine.CleanWorkDir = false
	}

	// Invalid arguments would have already caused errors, so no need to check
	stateMachine.ImageType = parser.Command.Commands()[0].Name

	// finally, run the state machine
	if err := stateMachine.Run(); err != nil {
		fmt.Println(err.Error())
		osExit(1)
	}
}
