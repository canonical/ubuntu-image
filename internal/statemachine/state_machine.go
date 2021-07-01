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
var stateFuncs = map[int]func(StateMachine) bool{
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
}

/* For state machine runs that will be resumed,
 * We need to write the state machine info to disk */
func (stateMachine StateMachine) writeMetadata() bool {
	gobfile, err := os.OpenFile(stateMachine.WorkDir+"/ubuntu-image.gob", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil && !os.IsExist(err) {
		fmt.Printf("Error opening metadata file for writing: %s\n", stateMachine.WorkDir+"/ubuntu-image.gob")
		return false
	}
	defer gobfile.Close()
	enc := gob.NewEncoder(gobfile)

	// no need to check errors, as it will panic if there is one
	enc.Encode(stateMachine)
	return true
}

// Step 0: generate work directory file structure
func (stateMachine StateMachine) makeTemporaryDirectories() bool {
	if stateMachine.Debug {
		fmt.Println("[ 0] make_temporary_directories")
	}
	err := os.Mkdir(stateMachine.WorkDir, 0755)
	if err != nil && !os.IsExist(err) {
		fmt.Println("Error creating work directory")
		return false
	}

	return true
}

// Step 1: Prepare the gadget tree
func (stateMachine StateMachine) prepareGadgetTree() bool {
	if stateMachine.Debug {
		fmt.Println("[ 1] prepare_gadget_tree")
	}
	return true
}

// Step 2: Prepare the image
func (stateMachine StateMachine) prepareImage() bool {
	if stateMachine.Debug {
		fmt.Println("[ 2] prepare_image")
	}
	return true
}

// Step 3: Load the gadget yaml passed in via command line
func (stateMachine StateMachine) loadGadgetYaml() bool {
	if stateMachine.Debug {
		fmt.Println("[ 3] load_gadget_yaml")
	}
	return true
}

// Step 4: Populate the image's rootfs contents
func (stateMachine StateMachine) populateRootfsContents() bool {
	if stateMachine.Debug {
		fmt.Println("[ 4] populate_rootfs_contents")
	}
	return true
}

// Step 5: Run hooks for populating rootfs contents
func (stateMachine StateMachine) populateRootfsContentsHooks() bool {
	if stateMachine.Debug {
		fmt.Println("[ 5] populate_rootfs_contents_hooks")
	}
	return true
}

// Step 6: Generate the disk info
func (stateMachine StateMachine) generateDiskInfo() bool {
	if stateMachine.Debug {
		fmt.Println("[ 6] generate_disk_info")
	}
	return true
}

// Step 7: Calculate the rootfs size
func (stateMachine StateMachine) calculateRootfsSize() bool {
	if stateMachine.Debug {
		fmt.Println("[ 7] calculate_rootfs_size")
	}
	return true
}

// Step 8: Pre populate the bootfs contents
func (stateMachine StateMachine) prepopulateBootfsContents() bool {
	if stateMachine.Debug {
		fmt.Println("[ 8] pre_populate_bootfs_contents")
	}
	return true
}

// Step 9: Populate the Bootfs Contents
func (stateMachine StateMachine) populateBootfsContents() bool {
	if stateMachine.Debug {
		fmt.Println("[ 9] populate_bootfs_contents")
	}
	return true
}

// Step 10: Populate and prepare the partitions
func (stateMachine StateMachine) populatePreparePartitions() bool {
	if stateMachine.Debug {
		fmt.Println("[10] populate_prepare_partitions")
	}
	return true
}

// Step 11: Make the disk
func (stateMachine StateMachine) makeDisk() bool {
	if stateMachine.Debug {
		fmt.Println("[11] make_disk")
	}
	return true
}

// Step 12: Generate the manifest
func (stateMachine StateMachine) generateManifest() bool {
	if stateMachine.Debug {
		fmt.Println("[12] generate_manifest")
	}
	return true
}

// Step 13: Clean up and organize files
func (stateMachine StateMachine) finish() bool {
	if stateMachine.Debug {
		fmt.Println("[13] finish")
	}
	if stateMachine.CleanWorkDir {
		os.RemoveAll(stateMachine.WorkDir)
	}
	return true
}

// Run parses the command line options and iterates through the states
func (stateMachine StateMachine) Run() bool {
	// Validate command line options
	if stateMachine.Thru != "" && stateMachine.Until != "" {
		fmt.Println("Cannot specify both --until and --thru!")
		return false
	}
	if stateMachine.WorkDir == "" && stateMachine.Resume {
		fmt.Println("Must specify workdir when using --resume flag!")
		return false
	}

	// attempt to parse thru and until
	if stateMachine.Until != "" {
		// first check if it is a digit
		if val, err := strconv.Atoi(stateMachine.Until); err == nil {
			// got a digit, make sure it's within range
			if val >= 0 && val <= len(stateNames) {
				stateMachine.UntilOrdinal = val
			} else {
				return false
			}
		} else if val, exists := stateNames[stateMachine.Until]; exists {
			stateMachine.UntilOrdinal = val
		} else {
			fmt.Printf("Invalid value for Until: %s\n", stateMachine.Until)
			return false
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
				return false
			}
		} else if val, exists := stateNames[stateMachine.Thru]; exists {
			stateMachine.ThruOrdinal = val
		} else {
			fmt.Printf("Invalid value for Thru: %s\n", stateMachine.Thru)
			return false
		}
	} else {
		// go "thru" the end
		stateMachine.ThruOrdinal = len(stateFuncs)
	}

	// handle the resume case
	var startingState int
	if stateMachine.Resume {
		// open the ubuntu-image.gob file and determine the state
		var partialStateMachine = new(StateMachine)
		gobfile, err := os.Open(stateMachine.WorkDir + "/ubuntu-image.gob")
		if err != nil {
			fmt.Printf("Error reading metadata file: %s\n", err.Error())
			return false
		}
		defer gobfile.Close()
		dec := gob.NewDecoder(gobfile)
		err = dec.Decode(&partialStateMachine)
		if err != nil {
			fmt.Printf("Failed to parse metadata file: %s\n", err.Error())
			return false
		}
		startingState = partialStateMachine.CurrentStep
	} else {
		// start from the beginning
		startingState = 0
	}

	// if no workdir was specified, open a /tmp dir
	if stateMachine.WorkDir == "" {
		workDir, err := os.MkdirTemp(stateMachine.tempLocation, "ubuntu-image-")
		if err != nil {
			fmt.Println("Failed to create temporary directory")
			return false
		}
		stateMachine.WorkDir = workDir
	}

	// iterate through the states
	for state := startingState; state < len(stateFuncs); state++ {
		stateMachine.CurrentStep = state
		if state < stateMachine.UntilOrdinal && state <= stateMachine.ThruOrdinal {
			if !stateFuncs[state](stateMachine) {
				return false
			}
		} else {
			break
		}
	}

	if !stateMachine.CleanWorkDir {
		if !stateMachine.writeMetadata() {
			return false
		}
	}
	return true
}
