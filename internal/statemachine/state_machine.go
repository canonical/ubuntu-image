// Package statemachine provides the functions and structs to set up and
// execute a state machine based ubuntu-image build
package statemachine

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/gadget/quantity"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/osutil/mkfs"
)

// define some functions that can be mocked by test cases
var gadgetLayoutVolume = gadget.LayoutVolume
var gadgetNewMountedFilesystemWriter = gadget.NewMountedFilesystemWriter
var helperCopyBlob = helper.CopyBlob
var ioutilReadDir = ioutil.ReadDir
var ioutilReadFile = ioutil.ReadFile
var ioutilWriteFile = ioutil.WriteFile
var osMkdir = os.Mkdir
var osMkdirAll = os.MkdirAll
var osOpenFile = os.OpenFile
var osRemoveAll = os.RemoveAll
var osCreate = os.Create
var osutilCopyFile = osutil.CopyFile
var osutilCopySpecialFile = osutil.CopySpecialFile
var execCommand = exec.Command
var mkfsMakeWithContent = mkfs.MakeWithContent

var mockableBlockSize string = "1" //used for mocking dd calls

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

// temporaryDirectories organizes the state machines, rootfs, unpack, and volumes dirs
type temporaryDirectories struct {
	rootfs  string
	unpack  string
	volumes string
}

// StateMachine will hold the command line data, track the current state, and handle all function calls
type StateMachine struct {
	cleanWorkDir bool   // whether or not to clean up the workDir
	CurrentStep  string // tracks the current progress of the state machine
	StepsTaken   int    // counts the number of steps taken
	yamlFilePath string // the location for the yaml file
	isSeeded     bool   // core 20 images are seeded
	rootfsSize   quantity.Size
	tempDirs     temporaryDirectories

	// The flags that were passed in on the command line
	commonFlags       *commands.CommonOpts
	stateMachineFlags *commands.StateMachineOpts

	states []stateFunc // the state functions

	// used to access image type specific variables from state functions
	parent SmInterface

	// imported from snapd, the info parsed from gadget.yaml
	gadgetInfo *gadget.Info

	// image sizes for parsing the --image-size flags
	imageSizes  map[string]quantity.Size
	volumeOrder []string
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
	} else {
		stateMachine.cleanup()
	}
	return nil
}
