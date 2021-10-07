// Package statemachine provides the functions and structs to set up and
// execute a state machine based ubuntu-image build
package statemachine

import (
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/canonical/ubuntu-image/internal/helper"
	diskfs "github.com/diskfs/go-diskfs"
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
var osRename = os.Rename
var osCreate = os.Create
var osutilCopyFile = osutil.CopyFile
var osutilCopySpecialFile = osutil.CopySpecialFile
var execCommand = exec.Command
var mkfsMakeWithContent = mkfs.MakeWithContent
var diskfsCreate = diskfs.Create

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
	YamlFilePath string // the location for the yaml file
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
	GadgetInfo *gadget.Info

	// image sizes for parsing the --image-size flags
	ImageSizes  map[string]quantity.Size
	volumeOrder []string
}

// SetCommonOpts stores the common options for all image types in the struct
func (stateMachine *StateMachine) SetCommonOpts(commonOpts *commands.CommonOpts,
	stateMachineOpts *commands.StateMachineOpts) {
	stateMachine.commonFlags = commonOpts
	stateMachine.stateMachineFlags = stateMachineOpts
}

// parseImageSizes handles the flag --image-size, which is a string in the format
// <volumeName>:<volumeSize>,<volumeName2>:<volumeSize2>. It can also be in the
// format <volumeSize> to signify one size to rule them all
func (stateMachine *StateMachine) parseImageSizes() error {
	// initialize the size map
	stateMachine.ImageSizes = make(map[string]quantity.Size)

	// If --image-size was not used, simply return
	if stateMachine.commonFlags.Size == "" {
		return nil
	}

	if !strings.Contains(stateMachine.commonFlags.Size, ":") {
		// handle the "one size to rule them all" case
		parsedSize, err := quantity.ParseSize(stateMachine.commonFlags.Size)
		if err != nil {
			return fmt.Errorf("Failed to parse argument to --image-size: %s", err.Error())
		}
		for volumeName := range stateMachine.GadgetInfo.Volumes {
			stateMachine.ImageSizes[volumeName] = parsedSize
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
				if volumeNumber < len(stateMachine.volumeOrder) {
					stateName := stateMachine.volumeOrder[volumeNumber]
					stateMachine.ImageSizes[stateName] = parsedSize
				} else {
					return fmt.Errorf("Volume index %d is out of range", volumeNumber)
				}
			} else {
				if _, found := stateMachine.GadgetInfo.Volumes[splitSize[0]]; !found {
					return fmt.Errorf("Volume %s does not exist in gadget.yaml",
						splitSize[0])
				}
				stateMachine.ImageSizes[splitSize[0]] = parsedSize
			}
		}
	}
	return nil
}

// saveVolumeOrder records the order that the volumes appear in gadget.yaml. This is necessary
// to preserve backwards compatibility of the command line syntax --image-size <volume_number>:<size>
func (stateMachine *StateMachine) saveVolumeOrder(gadgetYamlContents string) {
	// don't bother doing this if --image-size was not used
	if stateMachine.commonFlags.Size == "" {
		return
	}

	indexMap := make(map[string]int)
	for volumeName := range stateMachine.GadgetInfo.Volumes {
		searchString := volumeName + ":"
		index := strings.Index(gadgetYamlContents, searchString)
		indexMap[volumeName] = index
	}

	// now sort based on the index
	type volumeNameIndex struct {
		VolumeName string
		Index      int
	}

	var sortable []volumeNameIndex
	for volumeName, volumeIndex := range indexMap {
		sortable = append(sortable, volumeNameIndex{volumeName, volumeIndex})
	}

	sort.Slice(sortable, func(i, j int) bool {
		return sortable[i].Index < sortable[j].Index
	})

	var sortedVolumes []string
	for _, volume := range sortable {
		sortedVolumes = append(sortedVolumes, volume.VolumeName)
	}

	stateMachine.volumeOrder = sortedVolumes
}

// postProcessGadgetYaml adds the rootfs to the partitions list if needed
func (stateMachine *StateMachine) postProcessGadgetYaml() error {
	var rootfsSeen bool = false
	var farthestOffset quantity.Offset = 0
	var lastOffset quantity.Offset = 0
	var lastVolumeName string
	for volumeName, volume := range stateMachine.GadgetInfo.Volumes {
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
			farthestOffset = maxOffset(lastOffset, farthestOffset)
			structure.Offset = &offset
			// we've manually updated the offset, but since structure is
			// not a pointer we need to overwrite the value in volume.Structure
			volume.Structure[ii] = structure
		}
	}

	if !rootfsSeen && len(stateMachine.GadgetInfo.Volumes) == 1 {
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

		// There is only one volume, so lastVolumeName is its name
		// we now add the rootfs structure to the volume
		stateMachine.GadgetInfo.Volumes[lastVolumeName].Structure =
			append(stateMachine.GadgetInfo.Volumes[lastVolumeName].Structure, rootfsStructure)
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
		/*stateMachine.CurrentStep = partialStateMachine.CurrentStep
		stateMachine.StepsTaken = partialStateMachine.StepsTaken
		stateMachine.GadgetInfo = partialStateMachine.GadgetInfo
		stateMachine.YamlFilePath = partialStateMachine.YamlFilePath
		stateMachine.ImageSizes = partialStateMachine.ImageSizes
		stateMachine.tempDirs.rootfs = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "root")
		stateMachine.tempDirs.unpack = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "unpack")
		stateMachine.tempDirs.volumes = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "volumes")*/
		stateMachine = partialStateMachine

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

// handleContentSizes ensures that the sizes of the partitions are large enough and stores
// safe values in the stateMachine struct for use during make_image
func (stateMachine *StateMachine) handleContentSizes(farthestOffset quantity.Offset, volumeName string) {
	// store volume sizes in the stateMachine Struct. These will be used during
	// the make_image step
	calculated := quantity.Size((farthestOffset/quantity.OffsetMiB + 17) * quantity.OffsetMiB)
	volumeSize, found := stateMachine.ImageSizes[volumeName]
	if !found {
		stateMachine.ImageSizes[volumeName] = calculated
	} else {
		if volumeSize < calculated {
			fmt.Printf("WARNING: ignoring image size smaller than "+
				"minimum required size: vol:%s %d < %d",
				volumeName, uint64(volumeSize), uint64(calculated))
			stateMachine.ImageSizes[volumeName] = calculated
		} else {
			stateMachine.ImageSizes[volumeName] = volumeSize
		}
	}
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
