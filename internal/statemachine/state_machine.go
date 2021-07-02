package statemachine

import (
	"encoding/gob"
	"fmt"
	"os"
	"strconv"
)

// used for --resume to convert a named step to an ordinal
var stateNames = map[string]int{
	"make_temporary_directories":     0,
	"prepare_gadget_tree":            1,
	"prepare_image":                  2,
	"load_gadget_yaml":               3,
	"populate_rootfs_contents":       4,
	"populate_rootfs_contents_hooks": 5,
	"generate_disk_info":             6,
	"calculate_rootfs_size":          7,
	"pre_populate_bootfs_contents":   8,
	"populate_bootfs_contents":       9,
	"populate_prepare_partitions":    10,
	"make_disk":                      11,
	"generate_manifest":              12,
	"finish":                         13,
}

// iterated over by the state machine, and individual functions can be overridden for tests
var stateFuncs = map[int]func(StateMachine) error{
	0:  StateMachine.makeTemporaryDirectories,
	1:  StateMachine.prepareGadgetTree,
	2:  StateMachine.prepareImage,
	3:  StateMachine.loadGadgetYaml,
	4:  StateMachine.populateRootfsContents,
	5:  StateMachine.populateRootfsContentsHooks,
	6:  StateMachine.generateDiskInfo,
	7:  StateMachine.calculateRootfsSize,
	8:  StateMachine.prepopulateBootfsContents,
	9:  StateMachine.populateBootfsContents,
	10: StateMachine.populatePreparePartitions,
	11: StateMachine.makeDisk,
	12: StateMachine.generateManifest,
	13: StateMachine.finish,
}

// StateMachine will hold the command line data, track the current state, and handle all function calls
type StateMachine struct {
	WorkDir      string // where the state files and file structures are placed
	ImageType    string // snap or classic. Certain states will need to know this
	Until        string // run Until a certain step, not inclusively
	Thru         string // run Thru a certain step, inclusively
	Debug        bool   // verbose logging
	Resume       bool   // pick up where we left off
	CleanWorkDir bool   // whether or not to clean up the workDir
	UntilOrdinal int    // numeric step to stop before
	ThruOrdinal  int    // numeric step to stop after
	CurrentStep  int    // tracks the current progress of the state machine
	tempLocation string // used for testing, "" in non test cases
	startingStep int    // when --resume is used we need to know what step to start from
}

/* Certain combinations of arguments are not allowed. Validate proper
 * state machine arguments were provided */
func (stateMachine *StateMachine) validateInput() error {
	// Validate command line options
	if stateMachine.Thru != "" && stateMachine.Until != "" {
		return fmt.Errorf("Cannot specify both --until and --thru!")
	}
	if stateMachine.WorkDir == "" && stateMachine.Resume {
		return fmt.Errorf("Must specify workdir when using --resume flag!")
	}

	if err := stateMachine.getUntilThruOrdinals(); err != nil {
		return err
	}

	// handle the resume case
	if stateMachine.Resume {
		// open the ubuntu-image.gob file and determine the state
		var partialStateMachine = new(StateMachine)
		gobfile, err := os.Open(stateMachine.WorkDir + "/ubuntu-image.gob")
		if err != nil {
			return fmt.Errorf("Error reading metadata file: %s\n", err.Error())
		}
		defer gobfile.Close()
		dec := gob.NewDecoder(gobfile)
		err = dec.Decode(&partialStateMachine)
		if err != nil {
			return fmt.Errorf("Failed to parse metadata file: %s\n", err.Error())
		}
		stateMachine.startingStep = partialStateMachine.CurrentStep
	} else {
		// start from the beginning
		stateMachine.startingStep = 0
	}
	return nil
}

func (stateMachine *StateMachine) getUntilThruOrdinals() error {
	// attempt to parse thru and until
	if stateMachine.Until != "" {
		// first check if it is a digit
		if val, err := strconv.Atoi(stateMachine.Until); err == nil {
			// got a digit, make sure it's within range
			if val >= 0 && val <= len(stateNames) {
				stateMachine.UntilOrdinal = val
			} else {
				return fmt.Errorf("Provided \"until\" step out of range")
			}
		} else if val, exists := stateNames[stateMachine.Until]; exists {
			stateMachine.UntilOrdinal = val
		} else {
			return fmt.Errorf("Invalid value for Until: %s\n", stateMachine.Until)
		}
	} else {
		// go "until" the end
		stateMachine.UntilOrdinal = len(stateFuncs) + 1
	}

	if stateMachine.Thru != "" {
		// first check if it is a digit
		if val, err := strconv.Atoi(stateMachine.Thru); err == nil {
			// got a digit, make sure it's within range
			if val >= 0 && val <= len(stateNames) {
				stateMachine.ThruOrdinal = val
			} else {
				return fmt.Errorf("Provided \"thru\" step out of range")
			}
		} else if val, exists := stateNames[stateMachine.Thru]; exists {
			stateMachine.ThruOrdinal = val
		} else {
			return fmt.Errorf("Invalid value for Thru: %s\n", stateMachine.Thru)
		}
	} else {
		// go "thru" the end
		stateMachine.ThruOrdinal = len(stateFuncs)
	}
	return nil
}

/* For state machine runs that will be resumed,
 * We need to write the state machine info to disk */
func (stateMachine StateMachine) writeMetadata() error {
	gobfile, err := os.OpenFile(stateMachine.WorkDir+"/ubuntu-image.gob", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil && !os.IsExist(err) {
		fmt.Errorf("Error opening metadata file for writing: %s\n", stateMachine.WorkDir+"/ubuntu-image.gob")
	}
	defer gobfile.Close()
	enc := gob.NewEncoder(gobfile)

	// no need to check errors, as it will panic if there is one
	enc.Encode(stateMachine)
	return nil
}

// Step 0: generate work directory file structure
func (stateMachine StateMachine) makeTemporaryDirectories() error {
	if stateMachine.Debug {
		fmt.Println("[ 0] make_temporary_directories")
	}

	// if no workdir was specified, open a /tmp dir
	if stateMachine.WorkDir == "" {
		workDir, err := os.MkdirTemp(stateMachine.tempLocation, "ubuntu-image-")
		if err != nil {
			fmt.Errorf("Failed to create temporary directory")
		}
		stateMachine.WorkDir = workDir
	} else {
		err := os.Mkdir(stateMachine.WorkDir, 0755)
		if err != nil && !os.IsExist(err) {
			fmt.Errorf("Error creating work directory")
		}
	}

	return nil
}

// Step 1: Prepare the gadget tree
func (stateMachine StateMachine) prepareGadgetTree() error {
	if stateMachine.Debug {
		fmt.Println("[ 1] prepare_gadget_tree")
	}
	return nil
}

// Step 2: Prepare the image
func (stateMachine StateMachine) prepareImage() error {
	if stateMachine.Debug {
		fmt.Println("[ 2] prepare_image")
	}
	return nil
}

// Step 3: Load the gadget yaml passed in via command line
func (stateMachine StateMachine) loadGadgetYaml() error {
	if stateMachine.Debug {
		fmt.Println("[ 3] load_gadget_yaml")
	}
	return nil
}

// Step 4: Populate the image's rootfs contents
func (stateMachine StateMachine) populateRootfsContents() error {
	if stateMachine.Debug {
		fmt.Println("[ 4] populate_rootfs_contents")
	}
	return nil
}

// Step 5: Run hooks for populating rootfs contents
func (stateMachine StateMachine) populateRootfsContentsHooks() error {
	if stateMachine.Debug {
		fmt.Println("[ 5] populate_rootfs_contents_hooks")
	}
	return nil
}

// Step 6: Generate the disk info
func (stateMachine StateMachine) generateDiskInfo() error {
	if stateMachine.Debug {
		fmt.Println("[ 6] generate_disk_info")
	}
	return nil
}

// Step 7: Calculate the rootfs size
func (stateMachine StateMachine) calculateRootfsSize() error {
	if stateMachine.Debug {
		fmt.Println("[ 7] calculate_rootfs_size")
	}
	return nil
}

// Step 8: Pre populate the bootfs contents
func (stateMachine StateMachine) prepopulateBootfsContents() error {
	if stateMachine.Debug {
		fmt.Println("[ 8] pre_populate_bootfs_contents")
	}
	return nil
}

// Step 9: Populate the Bootfs Contents
func (stateMachine StateMachine) populateBootfsContents() error {
	if stateMachine.Debug {
		fmt.Println("[ 9] populate_bootfs_contents")
	}
	return nil
}

// Step 10: Populate and prepare the partitions
func (stateMachine StateMachine) populatePreparePartitions() error {
	if stateMachine.Debug {
		fmt.Println("[10] populate_prepare_partitions")
	}
	return nil
}

// Step 11: Make the disk
func (stateMachine StateMachine) makeDisk() error {
	if stateMachine.Debug {
		fmt.Println("[11] make_disk")
	}
	return nil
}

// Step 12: Generate the manifest
func (stateMachine StateMachine) generateManifest() error {
	if stateMachine.Debug {
		fmt.Println("[12] generate_manifest")
	}
	return nil
}

// Step 13: Clean up and organize files
func (stateMachine StateMachine) finish() error {
	if stateMachine.Debug {
		fmt.Println("[13] finish")
	}
	if stateMachine.CleanWorkDir {
		os.RemoveAll(stateMachine.WorkDir)
	}
	return nil
}

// Run parses the command line options and iterates through the states
func (stateMachine StateMachine) Run() error {
	if err := stateMachine.validateInput(); err != nil {
		return err
	}

	// iterate through the states
	for state := stateMachine.startingStep; state < len(stateFuncs); state++ {
		stateMachine.CurrentStep = state
		if state < stateMachine.UntilOrdinal && state <= stateMachine.ThruOrdinal {
			if err := stateFuncs[state](stateMachine); err != nil {
				return err
			}
		} else {
			break
		}
	}

	if !stateMachine.CleanWorkDir {
		if err := stateMachine.writeMetadata(); err != nil {
			return err
		}
	}
	return nil
}
