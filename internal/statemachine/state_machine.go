// Package statemachine provides the functions and structs to set up and
// execute a state machine based ubuntu-image build
package statemachine

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	diskfs "github.com/diskfs/go-diskfs"
	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/gadget/quantity"
	"github.com/snapcore/snapd/image"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/osutil/mkfs"
	"github.com/snapcore/snapd/seed"
	"github.com/xeipuuv/gojsonschema"

	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/canonical/ubuntu-image/internal/helper"
)

const (
	metadataStateFile = "ubuntu-image.json"
)

var gadgetYamlPathInTree = filepath.Join("meta", "gadget.yaml")

// define some functions that can be mocked by test cases
var gadgetLayoutVolume = gadget.LayoutVolume
var gadgetNewMountedFilesystemWriter = gadget.NewMountedFilesystemWriter
var helperCopyBlob = helper.CopyBlob
var helperSetDefaults = helper.SetDefaults
var helperCheckEmptyFields = helper.CheckEmptyFields
var helperCheckTags = helper.CheckTags
var helperBackupAndCopyResolvConf = helper.BackupAndCopyResolvConf
var helperRestoreResolvConf = helper.RestoreResolvConf
var ioReadAll = io.ReadAll
var osReadDir = os.ReadDir
var osReadFile = os.ReadFile
var osWriteFile = os.WriteFile
var osMkdir = os.Mkdir
var osMkdirAll = os.MkdirAll
var osMkdirTemp = os.MkdirTemp
var osOpen = os.Open
var osOpenFile = os.OpenFile
var osRemoveAll = os.RemoveAll
var osRemove = os.Remove
var osRename = os.Rename
var osCreate = os.Create
var osTruncate = os.Truncate
var osGetenv = os.Getenv
var osSetenv = os.Setenv
var osutilCopyFile = osutil.CopyFile
var osutilCopySpecialFile = osutil.CopySpecialFile
var execCommand = exec.Command
var mkfsMakeWithContent = mkfs.MakeWithContent
var mkfsMake = mkfs.Make
var diskfsCreate = diskfs.Create
var randRead = rand.Read
var seedOpen = seed.Open
var imagePrepare = image.Prepare
var httpGet = http.Get
var jsonUnmarshal = json.Unmarshal
var gojsonschemaValidate = gojsonschema.Validate
var filepathRel = filepath.Rel

var mockableBlockSize string = "1" //used for mocking dd calls

// SmInterface allows different image types to implement their own setup/run/teardown functions
type SmInterface interface {
	Setup() error
	Run() error
	Teardown() error
	SetCommonOpts(commonOpts *commands.CommonOpts, stateMachineOpts *commands.StateMachineOpts)
	SetSeries() error
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
	chroot  string
	scratch string
}

// StateMachine will hold the command line data, track the current state, and handle all function calls
type StateMachine struct {
	cleanWorkDir  bool          // whether or not to clean up the workDir
	CurrentStep   string        // tracks the current progress of the state machine
	StepsTaken    int           // counts the number of steps taken
	ConfDefPath   string        // directory holding the model assertion / image definition file
	YamlFilePath  string        // the location for the gadget yaml file
	IsSeeded      bool          // core 20 images are seeded
	rootfsVolName string        // volume on which the rootfs is located
	rootfsPartNum int           // rootfs partition number
	SectorSize    quantity.Size // parsed (converted) sector size
	RootfsSize    quantity.Size
	tempDirs      temporaryDirectories

	series string

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
	VolumeOrder []string

	// names of images for each volume
	VolumeNames map[string]string

	Packages []string
	Snaps    []string
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
				if volumeNumber < len(stateMachine.VolumeOrder) {
					stateName := stateMachine.VolumeOrder[volumeNumber]
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

	stateMachine.VolumeOrder = sortedVolumes
}

// postProcessGadgetYaml adds the rootfs to the partitions list if needed
func (stateMachine *StateMachine) postProcessGadgetYaml() error {
	var rootfsSeen bool = false
	var farthestOffset quantity.Offset
	var lastOffset quantity.Offset
	farthestOffsetUnknown := false
	var volume *gadget.Volume
	for _, volumeName := range stateMachine.VolumeOrder {
		volume = stateMachine.GadgetInfo.Volumes[volumeName]
		volumeBaseDir := filepath.Join(stateMachine.tempDirs.volumes, volumeName)
		if err := osMkdirAll(volumeBaseDir, 0755); err != nil {
			return fmt.Errorf("Error creating volume dir: %s", err.Error())
		}
		// look for the rootfs and check if the image is seeded
		for ii, structure := range volume.Structure {
			if structure.Role == "" && structure.Label == gadget.SystemBoot {
				if !stateMachine.commonFlags.Quiet {
					fmt.Printf("WARNING: volumes:%s:structure:%d:filesystem_label "+
						"used for defining partition roles; use role instead\n",
						volumeName, ii)
				}
			} else if structure.Role == gadget.SystemData {
				rootfsSeen = true
			} else if structure.Role == gadget.SystemSeed {
				stateMachine.IsSeeded = true
				if structure.Label == "" {
					structure.Label = "ubuntu-seed"
					volume.Structure[ii] = structure
				}
			}

			// make sure there are no "../" paths in the structure's contents
			for _, content := range structure.Content {
				if strings.Contains(content.UnresolvedSource, "../") {
					return fmt.Errorf("filesystem content source \"%s\" contains \"../\". "+
						"This is disallowed for security purposes",
						content.UnresolvedSource)
				}
			}
			if structure.Role == gadget.SystemBoot || structure.Label == gadget.SystemBoot {
				// handle special syntax of rootfs:/<file path> in
				// structure content. This is needed to allow images
				// such as raspberry pi to source their kernel and
				// initrd from the staged rootfs later in the build
				// process.
				relativeRootfsPath, err := filepathRel(
					filepath.Join(stateMachine.tempDirs.unpack, "gadget"),
					stateMachine.tempDirs.rootfs,
				)
				if err != nil {
					return fmt.Errorf("Error creating relative path from unpack/gadget to rootfs: \"%s\"", err.Error())
				}
				for jj, content := range structure.Content {
					content.UnresolvedSource = strings.ReplaceAll(content.UnresolvedSource,
						"rootfs:",
						relativeRootfsPath,
					)
					volume.Structure[ii].Content[jj] = content
				}
			}

			// update farthestOffset if needed
			if structure.Offset == nil {
				farthestOffsetUnknown = true
			} else {
				offset := *structure.Offset
				lastOffset = offset + quantity.Offset(structure.Size)
				farthestOffset = maxOffset(lastOffset, farthestOffset)
			}

			// system-data and system-seed do not always have content defined.
			// this makes Content be a nil slice and lead copyStructureContent() skip the rootfs copying later.
			// so we need to make an empty slice here to avoid this situation.
			if (structure.Role == gadget.SystemData || structure.Role == gadget.SystemSeed) && structure.Content == nil {
				structure.Content = make([]gadget.VolumeContent, 0)
			}

			// we've manually updated the offset, but since structure is
			// not a pointer we need to overwrite the value in volume.Structure
			volume.Structure[ii] = structure
		}
	}

	if !farthestOffsetUnknown && !rootfsSeen && len(stateMachine.GadgetInfo.Volumes) == 1 {
		// We still need to handle the case of unspecified system-data
		// partition where we simply attach the rootfs at the end of the
		// partition list.
		//
		// Since so far we have no knowledge of the rootfs contents, the
		// size is set to 0, and will be calculated later

		// Note that there is only one volume, so "volume" points to it
		rootfsStructure := gadget.VolumeStructure{
			Name:       "",
			Label:      "writable",
			Offset:     &farthestOffset,
			Size:       quantity.Size(0),
			Type:       "83,0FC63DAF-8483-4772-8E79-3D69D8477DE4",
			Role:       gadget.SystemData,
			ID:         "",
			Filesystem: "ext4",
			Content:    []gadget.VolumeContent{},
			Update:     gadget.VolumeUpdate{},
			// "virtual" yaml index for the new structure (it would
			// be the last one in gadget.yaml)
			YamlIndex: len(volume.Structure),
		}

		// we now add the rootfs structure to the volume
		volume.Structure = append(volume.Structure, rootfsStructure)
	}
	return nil
}

// readMetadata reads info about a partial state machine encoded as JSON from disk
func (stateMachine *StateMachine) readMetadata(metadataFile string) error {
	if !stateMachine.stateMachineFlags.Resume {
		return nil
	}
	// open the ubuntu-image.json file and load the state
	var partialStateMachine = &StateMachine{}
	jsonfilePath := filepath.Join(stateMachine.stateMachineFlags.WorkDir, metadataFile)
	jsonfile, err := os.ReadFile(jsonfilePath)
	if err != nil {
		return fmt.Errorf("error reading metadata file: %s", err.Error())
	}

	err = json.Unmarshal(jsonfile, partialStateMachine)
	if err != nil {
		return fmt.Errorf("failed to parse metadata file: %s", err.Error())
	}

	return stateMachine.loadState(partialStateMachine)
}

func (stateMachine *StateMachine) loadState(partialStateMachine *StateMachine) error {
	stateMachine.StepsTaken = partialStateMachine.StepsTaken

	if stateMachine.StepsTaken > len(stateMachine.states) {
		return fmt.Errorf("invalid steps taken count (%d). The state machine only have %d steps", stateMachine.StepsTaken, len(stateMachine.states))
	}

	// delete all of the stateFuncs that have already run
	stateMachine.states = stateMachine.states[stateMachine.StepsTaken:]

	stateMachine.CurrentStep = partialStateMachine.CurrentStep
	stateMachine.Packages = partialStateMachine.Packages
	stateMachine.Snaps = partialStateMachine.Snaps
	stateMachine.GadgetInfo = partialStateMachine.GadgetInfo
	stateMachine.YamlFilePath = partialStateMachine.YamlFilePath
	stateMachine.ImageSizes = partialStateMachine.ImageSizes
	stateMachine.RootfsSize = partialStateMachine.RootfsSize
	stateMachine.IsSeeded = partialStateMachine.IsSeeded
	stateMachine.VolumeOrder = partialStateMachine.VolumeOrder
	stateMachine.SectorSize = partialStateMachine.SectorSize
	stateMachine.VolumeNames = partialStateMachine.VolumeNames
	stateMachine.tempDirs.rootfs = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "root")
	stateMachine.tempDirs.unpack = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "unpack")
	stateMachine.tempDirs.volumes = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "volumes")
	stateMachine.tempDirs.chroot = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "chroot")
	stateMachine.tempDirs.scratch = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "scratch")

	// due to https://github.com/golang/go/issues/10415 we need to set back the volume
	// structs we reset before encoding (see writeMetadata())
	if stateMachine.GadgetInfo != nil {
		gadget.SetEnclosingVolumeInStructs(stateMachine.GadgetInfo.Volumes)
	}

	return nil
}

// writeMetadata writes the state machine info to disk, encoded as JSON. This will be used when resuming a
// partial state machine run
func (stateMachine *StateMachine) writeMetadata(metadataFile string) error {
	jsonfilePath := filepath.Join(stateMachine.stateMachineFlags.WorkDir, metadataFile)
	jsonfile, err := os.OpenFile(jsonfilePath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("error opening JSON metadata file for writing: %s", jsonfilePath)
	}
	defer jsonfile.Close()

	b, err := json.Marshal(stateMachine)
	if err != nil {
		return fmt.Errorf("failed to JSON encode metadata: %w", err)
	}

	_, err = jsonfile.Write(b)
	if err != nil {
		return fmt.Errorf("failed to write metadata to file: %w", err)
	}
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
				"minimum required size: vol:%s %d < %d\n",
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
	for i := 0; i < len(stateMachine.states); i++ {
		stateFunc := stateMachine.states[i]
		stateMachine.CurrentStep = stateFunc.name
		if stateFunc.name == stateMachine.stateMachineFlags.Until {
			break
		}
		if !stateMachine.commonFlags.Quiet {
			fmt.Printf("[%d] %s\n", stateMachine.StepsTaken, stateFunc.name)
		}
		start := time.Now()
		err := stateFunc.function(stateMachine)
		if stateMachine.commonFlags.Debug {
			fmt.Printf("duration: %v\n", time.Since(start))
		}
		if err != nil {
			// clean up work dir on error
			cleanupErr := stateMachine.cleanup()
			if cleanupErr != nil {
				return fmt.Errorf("error during cleanup: %s while cleaning after stateFunc error: %w", cleanupErr.Error(), err)
			}
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
	if stateMachine.cleanWorkDir {
		return stateMachine.cleanup()
	}
	return stateMachine.writeMetadata(metadataStateFile)
}
