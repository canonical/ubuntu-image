// Package statemachine provides the functions and structs to set up and
// execute a state machine based ubuntu-image build
package statemachine

import (
	"encoding/gob"
	"fmt"
	"os"

	"github.com/canonical/ubuntu-image/internal/commands"
)

// SmInterface allows different image types to implement their own setup/run/teardown functions
type SmInterface interface {
	Setup() error
	Run() error
	Teardown() error
}

// stateFunc allows us easy access to the function names, which will help with --resume and debug statements
type stateFunc struct {
	name     string
	function func(*StateMachine) error
}

// StateMachine will hold the command line data, track the current state, and handle all function calls
type StateMachine struct {
	cleanWorkDir bool   // whether or not to clean up the workDir
	CurrentStep  string // tracks the current progress of the state machine
	StepsTaken   int    // counts the number of steps taken

	// The flags that were passed in on the command line
	commonFlags       commands.CommonOpts
	stateMachineFlags commands.StateMachineOpts

	states []stateFunc // the state functions

	// used only for testing
	tempLocation string // parent directory of temporary workdir
}

// setCommonOpts stores the common options for all image types in the struct
func (stateMachine *StateMachine) setCommonOpts() {
	stateMachine.commonFlags = commands.CommonOptsPassed
	stateMachine.stateMachineFlags = commands.StateMachineOptsPassed
}

// validateInput ensures that command line flags for the state machine are valid. These
// flags are applicable to all image types
func (stateMachine *StateMachine) validateInput() error {
	// Validate command line options
	if stateMachine.stateMachineFlags.Thru != "" && stateMachine.stateMachineFlags.Until != "" {
		return fmt.Errorf("cannot specify both --until and --thru")
	}
	if stateMachine.stateMachineFlags.WorkDir == "" && stateMachine.stateMachineFlags.Resume {
		return fmt.Errorf("must specify workdir when using --resume flag")
	}

	return nil
}

// readMetadata reads info about a partial state machine from disk
func (stateMachine *StateMachine) readMetadata() error {
	// handle the resume case
	if stateMachine.stateMachineFlags.Resume {
		// open the ubuntu-image.gob file and determine the state
		var partialStateMachine = new(StateMachine)
		gobfile, err := os.Open(stateMachine.stateMachineFlags.WorkDir + "/ubuntu-image.gob")
		if err != nil {
			return fmt.Errorf("error reading metadata file: %s", err.Error())
		}
		defer gobfile.Close()
		dec := gob.NewDecoder(gobfile)
		err = dec.Decode(&partialStateMachine)
		if err != nil {
			return fmt.Errorf("failed to parse metadata file: %s", err.Error())
		}
		stateMachine.CurrentStep = partialStateMachine.CurrentStep
		stateMachine.StepsTaken = partialStateMachine.StepsTaken

		// delete all of the stateFuncs that have already run
		stateMachine.states = stateMachine.states[stateMachine.StepsTaken:]
	}
	return nil
}

// writeMetadata writes the state machine info to disk. This will be used when resuming a
// partial state machine run
func (stateMachine *StateMachine) writeMetadata() error {
	fmt.Printf("JAWN writing metadata to directory %s\n", stateMachine.stateMachineFlags.WorkDir)
	gobfile, err := os.OpenFile(stateMachine.stateMachineFlags.WorkDir+"/ubuntu-image.gob", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("error opening metadata file for writing: %s", stateMachine.stateMachineFlags.WorkDir+"/ubuntu-image.gob")
	}
	defer gobfile.Close()
	enc := gob.NewEncoder(gobfile)

	// no need to check errors, as it will panic if there is one
	enc.Encode(stateMachine)
	return nil
}

// cleanup cleans the workdir. For now this is just deleting the temporary directory if necessary
// but will have more functionality added to it later
func (stateMachine *StateMachine) cleanup() error {
	if err := os.RemoveAll(stateMachine.stateMachineFlags.WorkDir); err != nil {
		return err
	}
	return nil
}

// Run iterates through the state functions, stopping when appropriate based on --until and --thru
func (stateMachine *StateMachine) Run() error {
	// iterate through the states
	for _, stateFunc := range stateMachine.states {
		if stateFunc.name == stateMachine.stateMachineFlags.Until {
			break
		}
		if stateMachine.commonFlags.Debug {
			fmt.Printf("[%d] %s\n", stateMachine.StepsTaken, stateFunc.name)
		}
		fmt.Printf("[%d] %s\n", stateMachine.StepsTaken, stateFunc.name)
		//stateMachine.CurrentStep = stateName
		if err := stateFunc.function(stateMachine); err != nil {
			// clean up work dir on error
			stateMachine.cleanup()
			return err
		}
		stateMachine.StepsTaken++
		if stateFunc.name == stateMachine.stateMachineFlags.Thru {
			break
		}
	}
	return nil
}

// Teardown handles anything else that needs to happen after the states have finished running
func (stateMachine *StateMachine) Teardown() error {
	if !stateMachine.cleanWorkDir {
		if err := stateMachine.writeMetadata(); err != nil {
			return err
		}
	}
	return nil
}
