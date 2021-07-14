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

	parser := flags.NewParser(&commands.UICommand, flags.Default)
	parser.AddGroup("State Machine Options", stateMachineLongDesc, &commands.StateMachineOptsPassed)
	parser.AddGroup("Common Options", "Options common to both commands", &commands.CommonOptsPassed)

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
				if !commands.StateMachineOptsPassed.Resume {
					restoreStdout()
					restoreStderr()
					readStderr, err := ioutil.ReadAll(stderr)
					if err != nil {
						fmt.Printf("Error reading from stderr: %s\n", err.Error())
						osExit(1)
						break
					}
					fmt.Printf("Error: %s\n", string(readStderr))
					osExit(1)
				}
				break
			default:
				restoreStdout()
				restoreStderr()
				fmt.Printf("Error: %s\n", err.Error())
				osExit(1)
			}
		}
	}

	// restore stdout
	restoreStdout()
	restoreStderr()

	// this can't be nil because go-flags has already validated the active command
	imageType := parser.Command.Active.Name

	var stateMachine statemachine.SmInterface
	// Set up the state machine
	if imageType == "snap" {
		stateMachine = &statemachine.SnapSM
	} else {
		stateMachine = &statemachine.ClassicSM
	}

	// set up, run, and tear down the state machine
	if err := stateMachine.Setup(); err != nil {
		fmt.Printf("Error: %s\n", err.Error())
		osExit(1)
	}

	if err := stateMachine.Run(); err != nil {
		fmt.Printf("Error: %s\n", err.Error())
		osExit(1)
	}

	if err := stateMachine.Teardown(); err != nil {
		fmt.Printf("Error: %s\n", err.Error())
		osExit(1)
	}
}
