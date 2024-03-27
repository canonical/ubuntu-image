package main

import (
	"fmt"
	"io"
	"os"

	"github.com/jessevdk/go-flags"

	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/canonical/ubuntu-image/internal/statemachine"
)

// Version holds the ubuntu-image version number
// this is usually overridden at build time
var Version string = ""

// helper variables for unit testing
var osExit = os.Exit
var captureStd = helper.CaptureStd

var stateMachineLongDesc = `Options for controlling the internal state machine.
Other than -w, these options are mutually exclusive. When -u or -t is given,
the state machine can be resumed later with -r, but -w must be given in that
case since the state is saved in a ubuntu-image.json file in the working directory.`

func initStateMachine(imageType string, commonOpts *commands.CommonOpts, stateMachineOpts *commands.StateMachineOpts, ubuntuImageCommand *commands.UbuntuImageCommand) (statemachine.SmInterface, error) {
	var stateMachine statemachine.SmInterface
	switch imageType {
	case "snap":
		stateMachine = &statemachine.SnapStateMachine{
			Opts: ubuntuImageCommand.Snap.SnapOptsPassed,
			Args: ubuntuImageCommand.Snap.SnapArgsPassed,
		}
	case "classic":
		stateMachine = &statemachine.ClassicStateMachine{
			Args: ubuntuImageCommand.Classic.ClassicArgsPassed,
		}
	case "pack":
		stateMachine = &statemachine.PackStateMachine{
			Opts: ubuntuImageCommand.Pack.PackOptsPassed,
		}
	default:
		return nil, fmt.Errorf("unsupported command\n")
	}

	stateMachine.SetCommonOpts(commonOpts, stateMachineOpts)

	return stateMachine, nil
}

func executeStateMachine(sm statemachine.SmInterface) error {
	if err := sm.Setup(); err != nil {
		return err
	}

	if err := sm.Run(); err != nil {
		return err
	}

	if err := sm.Teardown(); err != nil {
		return err
	}

	return nil
}

// unhidePackOpts make pack options visible in help if the pack command is used
// This should be removed when the pack command is made visible to everyone
func unhidePackOpts(parser *flags.Parser) {
	// Save given options before removing them temporarily
	// otherwise the help will be displayed twice
	opts := parser.Options
	parser.Options = 0
	defer func() { parser.Options = opts }()
	// parse once to determine the active command
	// we do not care about error here since we will reparse again
	_, _ = parser.Parse() // nolint: errcheck

	if parser.Active != nil {
		if parser.Active.Name == "pack" {
			parser.Active.Hidden = false
		}
	}
}

// parseFlags parses received flags and returns error code accordingly
func parseFlags(parser *flags.Parser, restoreStdout, restoreStderr func(), stdout, stderr io.Reader, resume, version bool) (error, int) {
	if _, err := parser.Parse(); err != nil {
		if e, ok := err.(*flags.Error); ok {
			switch e.Type {
			case flags.ErrHelp:
				restoreStdout()
				restoreStderr()
				readStdout, err := io.ReadAll(stdout)
				if err != nil {
					fmt.Printf("Error reading from stdout: %s\n", err.Error())
					return err, 1
				}
				fmt.Println(string(readStdout))
				return e, 0
			case flags.ErrCommandRequired:
				// if --resume was given, this is not an error
				if !resume && !version {
					restoreStdout()
					restoreStderr()
					readStderr, err := io.ReadAll(stderr)
					if err != nil {
						fmt.Printf("Error reading from stderr: %s\n", err.Error())
						return err, 1
					}
					fmt.Printf("Error: %s\n", string(readStderr))
					return e, 1
				}
			default:
				restoreStdout()
				restoreStderr()
				fmt.Printf("Error: %s\n", err.Error())
				return e, 1
			}
		}
	}
	return nil, 0
}

func main() { //nolint: gocyclo
	commonOpts := new(commands.CommonOpts)
	stateMachineOpts := new(commands.StateMachineOpts)
	ubuntuImageCommand := new(commands.UbuntuImageCommand)

	// set up the go-flags parser for command line options
	parser := flags.NewParser(ubuntuImageCommand, flags.Default)
	_, err := parser.AddGroup("State Machine Options", stateMachineLongDesc, stateMachineOpts)
	if err != nil {
		fmt.Printf("Error: %s\n", err.Error())
		osExit(1)
		return
	}
	_, err = parser.AddGroup("Common Options", "Options common to every command", commonOpts)
	if err != nil {
		fmt.Printf("Error: %s\n", err.Error())
		osExit(1)
		return
	}

	// go-flags can be overzealous about printing errors that aren't actually errors
	// so we capture stdout/stderr while parsing and later decide whether to print
	stdout, restoreStdout, err := captureStd(&os.Stdout)
	if err != nil {
		fmt.Printf("Failed to capture stdout: %s\n", err.Error())
		osExit(1)
		return
	}
	defer restoreStdout()

	stderr, restoreStderr, err := captureStd(&os.Stderr)
	if err != nil {
		fmt.Printf("Failed to capture stderr: %s\n", err.Error())
		osExit(1)
		return
	}
	defer restoreStderr()

	unhidePackOpts(parser)

	// Parse the options provided and handle specific errors
	err, code := parseFlags(parser, restoreStdout, restoreStderr, stdout, stderr, stateMachineOpts.Resume, commonOpts.Version)
	if err != nil {
		osExit(code)
		return
	}

	// restore stdout
	restoreStdout()
	restoreStderr()

	// in case user only requested version number, print and exit
	if commonOpts.Version {
		// we expect Version to be supplied at build time or fetched from the snap environment
		if Version == "" {
			Version = os.Getenv("SNAP_VERSION")
		}
		fmt.Printf("ubuntu-image %s\n", Version)
		osExit(0)
		return
	}

	var imageType string
	if parser.Command.Active != nil {
		imageType = parser.Command.Active.Name
	}

	// init the state machine
	sm, err := initStateMachine(imageType, commonOpts, stateMachineOpts, ubuntuImageCommand)
	if err != nil {
		fmt.Printf("Error: %s\n", err.Error())
		osExit(1)
		return
	}

	// let the state machine handle the image build
	err = executeStateMachine(sm)
	if err != nil {
		fmt.Printf("Error: %s\n", err.Error())
		osExit(1)
		return
	}
}
