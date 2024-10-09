// Package statemachine provides the functions and structs to set up and
// execute a state machine based ubuntu-image build
package statemachine

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	diskfs "github.com/diskfs/go-diskfs"
	"github.com/google/uuid"
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
var gojsonschemaValidate = gojsonschema.Validate
var filepathRel = filepath.Rel

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
	rootfs  string // finale location of the built rootfs
	unpack  string // directory holding the unpacked gadget tree (and thus boot assets)
	volumes string // directory holding resulting partial images associated to volumes declared in the gadget.yaml
	chroot  string // place where the rootfs is built and modified
	scratch string // place to build and mount some directories at various stage
}

// StateMachine will hold the command line data, track the current state, and handle all function calls
type StateMachine struct {
	cleanWorkDir  bool          // whether or not to clean up the workDir
	CurrentStep   string        // tracks the current progress of the state machine
	StepsTaken    int           // counts the number of steps taken
	ConfDefPath   string        // directory holding the model assertion / image definition file
	YamlFilePath  string        // the location for the gadget yaml file
	IsSeeded      bool          // core 20 images are seeded
	RootfsVolName string        // volume on which the rootfs is located
	RootfsPartNum int           // rootfs partition number
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

	// Initially filled with the parsing of --image-size flags
	// Will then track the required size
	ImageSizes  map[string]quantity.Size
	VolumeOrder []string

	// names of images for each volume
	VolumeNames map[string]string

	// name of the "main volume"
	MainVolumeName string

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
	stateMachine.ImageSizes = make(map[string]quantity.Size)

	if stateMachine.commonFlags.Size == "" {
		return nil
	}

	if stateMachine.hasSingleImageSizeValue() {
		err := stateMachine.handleSingleImageSize()
		if err != nil {
			return err
		}
	} else {
		err := stateMachine.handleMultipleImageSizes()
		if err != nil {
			return err
		}
	}
	return nil
}

// hasSingleImageSizeValue determines if the provided --image-size flags contains
// a single value to use for every volumes or a list of values for some volumes
func (stateMachine *StateMachine) hasSingleImageSizeValue() bool {
	return !strings.Contains(stateMachine.commonFlags.Size, ":")
}

// handleSingleImageSize parses as a single value and applies the image size given in
// the flag --image-size
func (stateMachine *StateMachine) handleSingleImageSize() error {
	parsedSize, err := quantity.ParseSize(stateMachine.commonFlags.Size)
	if err != nil {
		return fmt.Errorf("Failed to parse argument to --image-size: %s", err.Error())
	}
	for volumeName := range stateMachine.GadgetInfo.Volumes {
		stateMachine.ImageSizes[volumeName] = parsedSize
	}
	return nil
}

// handleMultipleImageSizes parses and applies the image size given in
// the flag --image-size in the format <volumeName>:<volumeSize>,<volumeName2>:<volumeSize2>
func (stateMachine *StateMachine) handleMultipleImageSizes() error {
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
				volumeName := stateMachine.VolumeOrder[volumeNumber]
				stateMachine.ImageSizes[volumeName] = parsedSize
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

// postProcessGadgetYaml gathers several addition information from the volumes and
// operates several fixes on the volumes in the GadgetInfo.
// - Adds the rootfs to the partitions list if needed
// - Adds missing content
func (stateMachine *StateMachine) postProcessGadgetYaml() error {
	var rootfsSeen bool = false
	var farthestOffset quantity.Offset
	var lastOffset quantity.Offset
	var farthestOffsetUnknown bool = false
	lastVolumeName := ""

	for _, volumeName := range stateMachine.VolumeOrder {
		lastVolumeName = volumeName
		volume := stateMachine.GadgetInfo.Volumes[volumeName]
		volumeBaseDir := filepath.Join(stateMachine.tempDirs.volumes, volumeName)
		if err := osMkdirAll(volumeBaseDir, 0755); err != nil {
			return fmt.Errorf("Error creating volume dir: %s", err.Error())
		}
		for i := range volume.Structure {
			structure := &volume.Structure[i]
			stateMachine.warnUsageOfSystemLabel(volumeName, structure, i)

			if helper.IsRootfsStructure(structure) {
				rootfsSeen = true
			}

			stateMachine.handleSystemSeed(volume, structure, i)

			err := checkStructureContent(structure)
			if err != nil {
				return err
			}

			err = stateMachine.handleRootfsScheme(structure, volume, i)
			if err != nil {
				return err
			}

			// update farthestOffset if needed
			if structure.Offset == nil {
				farthestOffsetUnknown = true
			} else {
				offset := *structure.Offset
				lastOffset = offset + quantity.Offset(structure.Size)
				farthestOffset = maxOffset(lastOffset, farthestOffset)
			}

			fixMissingContent(volume, structure, i)
		}
	}

	stateMachine.fixMissingSystemData(lastVolumeName, farthestOffset, farthestOffsetUnknown, rootfsSeen)

	return nil
}

func (stateMachine *StateMachine) warnUsageOfSystemLabel(volumeName string, structure *gadget.VolumeStructure, structIndex int) {
	if structure.Role == "" && structure.Label == gadget.SystemBoot && !stateMachine.commonFlags.Quiet {
		fmt.Printf("WARNING: volumes:%s:structure:%d:filesystem_label "+
			"used for defining partition roles; use role instead\n",
			volumeName, structIndex)
	}
}

// handleSystemSeed checks if the structure is a system-seed one and fixes the Label if needed
func (stateMachine *StateMachine) handleSystemSeed(volume *gadget.Volume, structure *gadget.VolumeStructure, structIndex int) {
	if !helper.IsSystemSeedStructure(structure) {
		return
	}
	stateMachine.IsSeeded = true
	// The "main" volume is the one with a system-seed structure
	stateMachine.MainVolumeName = structure.VolumeName

	if structure.Label == "" {
		structure.Label = structure.Name
		volume.Structure[structIndex] = *structure
	}
}

// checkStructureContent makes sure there are no "../" paths in the structure's contents
func checkStructureContent(structure *gadget.VolumeStructure) error {
	for _, content := range structure.Content {
		if strings.Contains(content.UnresolvedSource, "../") {
			return fmt.Errorf("filesystem content source \"%s\" contains \"../\". "+
				"This is disallowed for security purposes",
				content.UnresolvedSource)
		}
	}
	return nil
}

// handleRootfsScheme handles special syntax of rootfs:/<file path> in structure
// content. This is needed to allow images such as raspberry pi to source their
// kernel and initrd from the staged rootfs later in the build process.
func (stateMachine *StateMachine) handleRootfsScheme(structure *gadget.VolumeStructure, volume *gadget.Volume, structIndex int) error {
	if helper.IsSystemBootStructure(structure) {
		relativeRootfsPath, err := filepathRel(
			filepath.Join(stateMachine.tempDirs.unpack, "gadget"),
			stateMachine.tempDirs.rootfs,
		)
		if err != nil {
			return fmt.Errorf("Error creating relative path from unpack/gadget to rootfs: \"%s\"", err.Error())
		}
		for j, content := range structure.Content {
			content.UnresolvedSource = strings.ReplaceAll(content.UnresolvedSource,
				"rootfs:",
				relativeRootfsPath,
			)
			volume.Structure[structIndex].Content[j] = content
		}
	}
	return nil
}

// fixMissingContent adds Content to system-data and system-seed.
// It may not be defined for these roles, so Content is a nil slice leading
// copyStructureContent() skip the rootfs copying later.
// So we need to make an empty slice here to avoid this situation.
func fixMissingContent(volume *gadget.Volume, structure *gadget.VolumeStructure, structIndex int) {
	if (helper.IsRootfsStructure(structure) || helper.IsSystemSeedStructure(structure)) && structure.Content == nil {
		structure.Content = make([]gadget.VolumeContent, 0)
	}

	volume.Structure[structIndex] = *structure
}

// fixMissingSystemData handles the case of unspecified system-data partition
// where we simply attach the rootfs at the end of the partition list.
// Since so far we have no knowledge of the rootfs contents, the size is set
// to 0, and will be calculated later
func (stateMachine *StateMachine) fixMissingSystemData(lastVolumeName string, farthestOffset quantity.Offset, farthestOffsetUnknown bool, rootfsSeen bool) {
	// We only add the structure if there is a single volume because we cannot
	// be sure which volume is considered the "main one" by the user, even though
	// we have a way to find it (see comment about the MainVolume field). We
	// should revisit this if we want to be stricter in the future about that.
	if farthestOffsetUnknown || rootfsSeen || len(stateMachine.GadgetInfo.Volumes) != 1 {
		return
	}
	// So for now we consider the main volume to be the last one
	volume := stateMachine.GadgetInfo.Volumes[lastVolumeName]

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

	volume.Structure = append(volume.Structure, rootfsStructure)
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
	stateMachine.YamlFilePath = partialStateMachine.YamlFilePath
	stateMachine.IsSeeded = partialStateMachine.IsSeeded
	stateMachine.RootfsVolName = partialStateMachine.RootfsVolName
	stateMachine.RootfsPartNum = partialStateMachine.RootfsPartNum

	stateMachine.SectorSize = partialStateMachine.SectorSize
	stateMachine.RootfsSize = partialStateMachine.RootfsSize

	stateMachine.tempDirs.rootfs = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "root")
	stateMachine.tempDirs.unpack = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "unpack")
	stateMachine.tempDirs.volumes = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "volumes")
	stateMachine.tempDirs.chroot = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "chroot")
	stateMachine.tempDirs.scratch = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "scratch")

	stateMachine.GadgetInfo = partialStateMachine.GadgetInfo
	stateMachine.ImageSizes = partialStateMachine.ImageSizes
	stateMachine.VolumeOrder = partialStateMachine.VolumeOrder
	stateMachine.VolumeNames = partialStateMachine.VolumeNames
	stateMachine.MainVolumeName = partialStateMachine.MainVolumeName

	stateMachine.Packages = partialStateMachine.Packages
	stateMachine.Snaps = partialStateMachine.Snaps

	if stateMachine.GadgetInfo != nil {
		// Due to https://github.com/golang/go/issues/10415 we need to set back the volume
		// structs we reset before encoding (see writeMetadata())
		gadget.SetEnclosingVolumeInStructs(stateMachine.GadgetInfo.Volumes)

		rebuildYamlIndex(stateMachine.GadgetInfo)
	}

	return nil
}

// rebuildYamlIndex reset the YamlIndex field in VolumeStructure
// This field is not serialized (for a good reason) so it is lost when saving the metadata
// We consider here the JSON serialization keeps the struct order and we can naively
// consider the YamlIndex value is the same as the index of the structure in the structure slice.
func rebuildYamlIndex(info *gadget.Info) {
	for _, v := range info.Volumes {
		for i, s := range v.Structure {
			s.YamlIndex = i
			v.Structure[i] = s
		}
	}
}

// displayStates print the calculated states
func (s *StateMachine) displayStates() {
	if !s.commonFlags.Debug && !s.commonFlags.DryRun {
		return
	}

	verb := "will"
	if s.commonFlags.DryRun {
		verb = "would"
	}
	fmt.Printf("\nFollowing states %s be executed:\n", verb)

	for i, state := range s.states {
		if state.name == s.stateMachineFlags.Until {
			break
		}
		fmt.Printf("[%d] %s\n", i, state.name)

		if state.name == s.stateMachineFlags.Thru {
			break
		}
	}

	if s.commonFlags.DryRun {
		return
	}
	fmt.Println("\nContinuing")
}

// writeMetadata writes the state machine info to disk, encoded as JSON. This will be used when resuming a
// partial state machine run
func (stateMachine *StateMachine) writeMetadata(metadataFile string) error {
	jsonfilePath := filepath.Join(stateMachine.stateMachineFlags.WorkDir, metadataFile)
	jsonfile, err := os.OpenFile(jsonfilePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
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

// generate work directory file structure
func (stateMachine *StateMachine) makeTemporaryDirectories() error {
	// if no workdir was specified, open a /tmp dir
	if stateMachine.stateMachineFlags.WorkDir == "" {
		stateMachine.stateMachineFlags.WorkDir = filepath.Join("/tmp", "ubuntu-image-"+uuid.NewString())
		if err := osMkdir(stateMachine.stateMachineFlags.WorkDir, 0755); err != nil {
			return fmt.Errorf("Failed to create temporary directory: %s", err.Error())
		}
		stateMachine.cleanWorkDir = true
	} else {
		err := osMkdirAll(stateMachine.stateMachineFlags.WorkDir, 0755)
		if err != nil && !os.IsExist(err) {
			return fmt.Errorf("Error creating work directory: %s", err.Error())
		}
	}

	stateMachine.tempDirs.rootfs = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "root")
	stateMachine.tempDirs.unpack = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "unpack")
	stateMachine.tempDirs.volumes = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "volumes")
	stateMachine.tempDirs.chroot = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "chroot")
	stateMachine.tempDirs.scratch = filepath.Join(stateMachine.stateMachineFlags.WorkDir, "scratch")

	tempDirs := []string{stateMachine.tempDirs.scratch, stateMachine.tempDirs.rootfs, stateMachine.tempDirs.unpack}
	for _, tempDir := range tempDirs {
		err := osMkdir(tempDir, 0755)
		if err != nil && !os.IsExist(err) {
			return fmt.Errorf("Error creating temporary directory \"%s\": \"%s\"", tempDir, err.Error())
		}
	}

	return nil
}

// determineOutputDirectory sets the directory in which to place artifacts
// and creates it if it doesn't already exist
func (stateMachine *StateMachine) determineOutputDirectory() error {
	if stateMachine.commonFlags.OutputDir == "" {
		if stateMachine.cleanWorkDir { // no workdir specified, so create the image in the pwd
			var err error
			stateMachine.commonFlags.OutputDir, err = os.Getwd()
			if err != nil {
				return fmt.Errorf("Error creating OutputDir: %s", err.Error())
			}
		} else {
			stateMachine.commonFlags.OutputDir = stateMachine.stateMachineFlags.WorkDir
		}
	} else {
		err := osMkdirAll(stateMachine.commonFlags.OutputDir, 0755)
		if err != nil && !os.IsExist(err) {
			return fmt.Errorf("Error creating OutputDir: %s", err.Error())
		}
	}
	return nil
}

// Run iterates through the state functions, stopping when appropriate based on --until and --thru
func (stateMachine *StateMachine) Run() error {
	if stateMachine.commonFlags.DryRun {
		return nil
	}
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
	fmt.Println("Build successful")
	return nil
}

// Teardown handles anything else that needs to happen after the states have finished running
func (stateMachine *StateMachine) Teardown() error {
	if stateMachine.commonFlags.DryRun {
		return nil
	}
	if stateMachine.cleanWorkDir {
		return stateMachine.cleanup()
	}
	return stateMachine.writeMetadata(metadataStateFile)
}
