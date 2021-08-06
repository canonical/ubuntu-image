// Package statemachine provides the functions and structs to set up and
// execute a state machine based ubuntu-image build
package statemachine

import (
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/gadget/quantity"
	"github.com/snapcore/snapd/osutil"
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
var osutilCopyFile = osutil.CopyFile
var osutilCopySpecialFile = osutil.CopySpecialFile

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
	imageSizes map[string]quantity.Size
}

// SetCommonOpts stores the common options for all image types in the struct
func (stateMachine *StateMachine) SetCommonOpts(commonOpts *commands.CommonOpts,
	stateMachineOpts *commands.StateMachineOpts) {
	stateMachine.commonFlags = commonOpts
	stateMachine.stateMachineFlags = stateMachineOpts
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

	// if --until or --thru was given, make sure the specified state exists
	var searchState string
	var stateFound bool = false
	if stateMachine.stateMachineFlags.Until != "" {
		searchState = stateMachine.stateMachineFlags.Until
	}
	if stateMachine.stateMachineFlags.Thru != "" {
		searchState = stateMachine.stateMachineFlags.Thru
	}

	if searchState != "" {
		for _, state := range stateMachine.states {
			if state.name == searchState {
				stateFound = true
				break
			}
		}
		if !stateFound {
			return fmt.Errorf("state %s is not a valid state name", searchState)
		}
	}

	return nil
}

// parseImageSizes handles the flag --image-size, which is a string in the format
// <volumeName>:<volumeSize>,<volumeName2>:<volumeSize2>. It can also be in the
// format <volumeSize> to signify one size to rule them all
func (stateMachine *StateMachine) parseImageSizes() error {
	// initialize the size map
	stateMachine.imageSizes = make(map[string]quantity.Size)

	if stateMachine.commonFlags.Size == "" {
		return nil
	}

	if !strings.Contains(stateMachine.commonFlags.Size, ":") {
		// handle the "one size to rule them all" case
		parsedSize, err := quantity.ParseSize(stateMachine.commonFlags.Size)
		if err != nil {
			return fmt.Errorf("Failed to parse argument to --image-size: %s", err.Error())
		}
		for volumeName := range stateMachine.gadgetInfo.Volumes {
			stateMachine.imageSizes[volumeName] = parsedSize
		}
	} else {
		allSizes := strings.Split(stateMachine.commonFlags.Size, ",")
		for _, size := range allSizes {
			// each of these should be of the form "<name|number>:<size>"
			splitSize := strings.Split(size, ":")
			if len(splitSize) != 2 {
				return fmt.Errorf("Argument to --image-size %s is not "+
					"in the correct format", size)
			}
			parsedSize, err := quantity.ParseSize(splitSize[1])
			if err != nil {
				return fmt.Errorf("Failed to parse argument to --image-size: %s",
					err.Error())
			}
			// the image size parsed successfully, now find which volume to associate it with
			volumeNumber, err := strconv.Atoi(splitSize[0])
			if err == nil {
				// argument passed was numeric.
				//TODO
				fmt.Printf("JAWN handle numerics for %d\n", volumeNumber)
			} else {
				if _, found := stateMachine.gadgetInfo.Volumes[splitSize[0]]; !found {
					return fmt.Errorf("Volume %s does not exist in gadget.yaml",
						splitSize[0])
				}
				stateMachine.imageSizes[splitSize[0]] = parsedSize
			}
		}
	}
	return nil
}

// readMetadata reads info about a partial state machine from disk
func (stateMachine *StateMachine) readMetadata() error {
	// handle the resume case
	if stateMachine.stateMachineFlags.Resume {
		// open the ubuntu-image.gob file and determine the state
		var partialStateMachine = new(StateMachine)
		gobfilePath := filepath.Join(stateMachine.stateMachineFlags.WorkDir, "ubuntu-image.gob")
		gobfile, err := os.Open(gobfilePath)
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
	gobfilePath := filepath.Join(stateMachine.stateMachineFlags.WorkDir, "ubuntu-image.gob")
	gobfile, err := os.OpenFile(gobfilePath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("error opening metadata file for writing: %s", gobfilePath)
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
	if stateMachine.cleanWorkDir {
		if err := osRemoveAll(stateMachine.stateMachineFlags.WorkDir); err != nil {
			return err
		}
	}
	return nil
}

// postProcessGadgetYaml does some additional validation on gadget.yaml beyond what
// snapd does, and adds the rootfs to the partitions list if needed
func (stateMachine *StateMachine) postProcessGadgetYaml() error {
	var rootfsSeen bool = false
	var farthestOffset quantity.Offset = 0
	var lastOffset quantity.Offset = 0
	var lastVolumeName string
	for volumeName, volume := range stateMachine.gadgetInfo.Volumes {
		lastVolumeName = volumeName
		volumeBaseDir := filepath.Join(stateMachine.tempDirs.volumes, volumeName)
		if err := osMkdirAll(volumeBaseDir, 0755); err != nil {
			return fmt.Errorf("Error creating volume dir: %s", err.Error())
		}
		// look for the rootfs and check if the image is seeded
		for ii, structure := range volume.Structure {
			if structure.Role == "" && structure.Label == gadget.SystemBoot {
				fmt.Printf("WARNING: volumes:%s:structure:%d:filesystem_label "+
					"used for defining partition roles; use role instead\n",
					volumeName, ii)
			} else if structure.Role == gadget.SystemData {
				rootfsSeen = true
			} else if structure.Role == gadget.SystemSeed {
				stateMachine.isSeeded = true
			}

			// update farthestOffset if needed
			var offset quantity.Offset
			if structure.Offset == nil {
				if structure.Role != "mbr" && lastOffset < quantity.OffsetMiB {
					offset = quantity.OffsetMiB
				} else {
					offset = lastOffset
				}
			} else {
				offset = *structure.Offset
			}
			lastOffset = offset + quantity.Offset(structure.Size)
			farthestOffset = helper.MaxOffset(lastOffset, farthestOffset)
		}
	}

	if !rootfsSeen && len(stateMachine.gadgetInfo.Volumes) == 1 {
		// We still need to handle the case of unspecified system-data
		// partition where we simply attach the rootfs at the end of the
		// partition list.
		//
		// Since so far we have no knowledge of the rootfs contents, the
		// size is set to 0, and will be calculated later
		rootfsStructure := gadget.VolumeStructure{
			Name:        "",
			Label:       "writable",
			Offset:      &farthestOffset,
			OffsetWrite: new(gadget.RelativeOffset),
			Size:        quantity.Size(0),
			Type:        "83,0FC63DAF-8483-4772-8E79-3D69D8477DE4",
			Role:        gadget.SystemData,
			ID:          "",
			Filesystem:  "ext4",
			Content:     []gadget.VolumeContent{},
			Update:      gadget.VolumeUpdate{},
		}

		// get the name of the volume
		stateMachine.gadgetInfo.Volumes[lastVolumeName].Structure =
			append(stateMachine.gadgetInfo.Volumes[lastVolumeName].Structure, rootfsStructure)
	}
	return nil
}

// runHooks reads through the --hooks-directory flags and calls a helper function to execute the scripts
func (stateMachine *StateMachine) runHooks(hookName, envKey, envVal string) error {
	os.Setenv(envKey, envVal)
	for _, hooksDir := range stateMachine.commonFlags.HooksDirectories {
		hooksDirectoryd := filepath.Join(hooksDir, hookName+".d")
		hookScripts, err := ioutilReadDir(hooksDirectoryd)

		// It's okay for hooks-directory.d to not exist, but if it does exist run all the scripts in it
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("Error reading hooks directory: %s", err.Error())
		}

		for _, hookScript := range hookScripts {
			hookScriptPath := filepath.Join(hooksDirectoryd, hookScript.Name())
			if stateMachine.commonFlags.Debug {
				fmt.Printf("Running hook script: %s\n", hookScriptPath)
			}
			if err := helper.RunScript(hookScriptPath); err != nil {
				return err
			}
		}

		// if hookName exists in the hook directory, run it
		hookScript := filepath.Join(hooksDir, hookName)
		_, err = os.Stat(hookScript)
		if err == nil {
			if stateMachine.commonFlags.Debug {
				fmt.Printf("Running hook script: %s\n", hookScript)
			}
			if err := helper.RunScript(hookScript); err != nil {
				return err
			}
		}
	}
	return nil
}

// handleLkBootloader handles the special "lk" bootloader case where some extra
// files need to be added to the bootfs
func (stateMachine *StateMachine) handleLkBootloader(volume *gadget.Volume) error {
	if volume.Bootloader != "lk" {
		return nil
	}
	// For the LK bootloader we need to copy boot.img and snapbootsel.bin to
	// the gadget folder so they can be used as partition content. The first
	// one comes from the kernel snap, while the second one is modified by
	// the prepare_image step to set the right core and kernel for the kernel
	// command line.
	bootDir := filepath.Join(stateMachine.tempDirs.unpack,
		"image", "boot", "lk")
	gadgetDir := filepath.Join(stateMachine.tempDirs.unpack, "gadget")
	if _, err := os.Stat(bootDir); err != nil {
		return fmt.Errorf("got lk bootloader but directory %s does not exist", bootDir)
	}
	err := osMkdir(gadgetDir, 0755)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("Failed to create gadget dir: %s", err.Error())
	}
	files, err := ioutilReadDir(bootDir)
	if err != nil {
		return fmt.Errorf("Error reading lk bootloader dir: %s", err.Error())
	}
	for _, lkFile := range files {
		srcFile := filepath.Join(bootDir, lkFile.Name())
		if err := osutilCopySpecialFile(srcFile, gadgetDir); err != nil {
			return fmt.Errorf("Error copying lk bootloader dir: %s", err.Error())
		}
	}
	return nil
}

// handleContentSizes ensures that the sizes of the partitions are large enough and stores
// safe values in the stateMachine struct for use during make_image
func (stateMachine *StateMachine) handleContentSizes(farthestOffset quantity.Offset, volumeName string) {
	// store volume sizes in the stateMachineStruct. These will be used during
	// the make_image step
	// TODO: check the value of calculated is accurate
	calculated := quantity.Size((farthestOffset/quantity.OffsetMiB + 17) * quantity.OffsetMiB)
	volumeSize, found := stateMachine.imageSizes[volumeName]
	if !found {
		stateMachine.imageSizes[volumeName] = calculated
	} else {
		if volumeSize < calculated {
			fmt.Printf("WARNING: ignoring image size smaller than "+
				"minimum required size: vol:%s %d < %d",
				volumeName, uint64(volumeSize), uint64(calculated))
			stateMachine.imageSizes[volumeName] = calculated
		} else {
			stateMachine.imageSizes[volumeName] = volumeSize
		}
	}
}

// shouldSkipStructure returns whether a structure should be skipped during certain processing
func (stateMachine *StateMachine) shouldSkipStructure(structure gadget.VolumeStructure) bool {
	if stateMachine.isSeeded &&
		(structure.Role == gadget.SystemBoot ||
			structure.Role == gadget.SystemData ||
			structure.Role == gadget.SystemSave ||
			structure.Label == gadget.SystemBoot) {
		return true
	}
	return false
}

// copyStructureContent handles copying raw blobs or creating formatted filesystems
func (stateMachine *StateMachine) copyStructureContent(structure gadget.VolumeStructure,
	contentRoot, partImg string) error {
	if structure.Filesystem == "" {
		// copy the contents to the new location
		var runningOffset quantity.Offset = 0
		for _, content := range structure.Content {
			if content.Offset != nil {
				runningOffset = *content.Offset
			}
			// first zero it out
			ddArgs := []string{"if=/dev/zero", "of=" + partImg, "count=0",
				"bs=" + strconv.FormatUint(uint64(structure.Size), 10),
				"seek=1"}
			if err := helperCopyBlob(ddArgs); err != nil {
				return fmt.Errorf("Error zeroing partition: %s",
					err.Error())
			}

			// now write the real value
			inFile := filepath.Join(stateMachine.tempDirs.unpack,
				"gadget", content.Image)
			ddArgs = []string{"if=" + inFile, "of=" + partImg, "bs=1",
				"seek=" + strconv.FormatUint(uint64(runningOffset), 10),
				"conv=sparse,notrunc"}
			if err := helperCopyBlob(ddArgs); err != nil {
				return fmt.Errorf("Error copying image blob: %s",
					err.Error())
			}
			runningOffset += quantity.Offset(content.Size)
		}
	} else {
		var blockSize quantity.Size
		if structure.Role == gadget.SystemData || structure.Role == gadget.SystemSeed {
			// system-data and system-seed structures are not required to have
			// an explicit size set in the yaml file
			if structure.Size < stateMachine.rootfsSize {
				fmt.Printf("WARNING: rootfs structure size %s smaller "+
					"than actual rootfs contents %s\n",
					structure.Size.IECString(),
					stateMachine.rootfsSize.IECString())
				blockSize = stateMachine.rootfsSize
			} else {
				blockSize = structure.Size
			}
		} else {
			blockSize = structure.Size
		}
		// use mkfs funcitons from snapd to create the filesystems
		ddArgs := []string{"if=/dev/zero", "of=" + partImg, "count=0",
			"bs=" + strconv.FormatUint(uint64(blockSize), 10), "seek=1"}
		if err := helperCopyBlob(ddArgs); err != nil {
			return fmt.Errorf("Error zeroing image file %s: %s",
				partImg, err.Error())
		}
		err := helper.MkfsWithContent(structure.Filesystem, partImg, structure.Label,
			contentRoot, structure.Size, quantity.Size(512))
		if err != nil {
			return fmt.Errorf("Error running mkfs: %s", err.Error())
		}
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
