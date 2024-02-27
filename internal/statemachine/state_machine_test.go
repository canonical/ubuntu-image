package statemachine

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	diskfs "github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/invopop/jsonschema"
	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/gadget/quantity"
	"github.com/snapcore/snapd/image"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/seed"
	"github.com/xeipuuv/gojsonschema"

	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/canonical/ubuntu-image/internal/helper"
)

const (
	testDataDir = "testdata"
)

var testDir = "ubuntu-image-0615c8dd-d3af-4074-bfcb-c3d3c8392b06"

// for tests where we don't want to run actual states
var testStates = []stateFunc{
	{"test_succeed", func(*StateMachine) error { return nil }},
}

// for tests where we want to run all the states
var allTestStates = []stateFunc{
	makeTemporaryDirectoriesState,
	{prepareGadgetTreeState.name, func(statemachine *StateMachine) error { return nil }},
	{prepareClassicImageState.name, func(statemachine *StateMachine) error { return nil }},
	{loadGadgetYamlState.name, func(statemachine *StateMachine) error { return nil }},
	{populateClassicRootfsContentsState.name, func(statemachine *StateMachine) error { return nil }},
	{generateDiskInfoState.name, func(statemachine *StateMachine) error { return nil }},
	{calculateRootfsSizeState.name, func(statemachine *StateMachine) error { return nil }},
	{populateBootfsContentsState.name, func(statemachine *StateMachine) error { return nil }},
	{populatePreparePartitionsState.name, func(statemachine *StateMachine) error { return nil }},
	{makeDiskState.name, func(statemachine *StateMachine) error { return nil }},
	{generatePackageManifestState.name, func(statemachine *StateMachine) error { return nil }},
}

func ptrToOffset(offset quantity.Offset) *quantity.Offset {
	return &offset
}

// define some mocked versions of go package functions
func mockCopyBlob([]string) error {
	return fmt.Errorf("Test Error")
}
func mockSetDefaults(interface{}) error {
	return fmt.Errorf("Test Error")
}
func mockCheckEmptyFields(interface{}, *gojsonschema.Result, *jsonschema.Schema) error {
	return fmt.Errorf("Test Error")
}
func mockCheckTags(interface{}, string) (string, error) {
	return "", fmt.Errorf("Test Error")
}
func mockBackupAndCopyResolvConfFail(string) error {
	return fmt.Errorf("Test Error")
}
func mockBackupAndCopyResolvConfSuccess(string) error {
	return nil
}
func mockRestoreResolvConf(string) error {
	return fmt.Errorf("Test Error")
}
func mockCopyBlobSuccess([]string) error {
	return nil
}
func mockLayoutVolume(*gadget.Volume,
	map[int]*gadget.OnDiskStructure,
	*gadget.LayoutOptions) (*gadget.LaidOutVolume, error) {
	return nil, fmt.Errorf("Test Error")
}
func mockNewMountedFilesystemWriter(*gadget.LaidOutStructure,
	gadget.ContentObserver) (*gadget.MountedFilesystemWriter, error) {
	return nil, fmt.Errorf("Test Error")
}
func mockMkfsWithContent(typ, img, label, contentRootDir string, deviceSize, sectorSize quantity.Size) error {
	return fmt.Errorf("Test Error")
}
func mockMkfs(typ, img, label string, deviceSize, sectorSize quantity.Size) error {
	return fmt.Errorf("Test Error")
}
func mockReadAll(io.Reader) ([]byte, error) {
	return []byte{}, fmt.Errorf("Test Error")
}
func mockReadDir(string) ([]os.DirEntry, error) {
	return []os.DirEntry{}, fmt.Errorf("Test Error")
}
func mockReadFile(string) ([]byte, error) {
	return []byte{}, fmt.Errorf("Test Error")
}
func mockWriteFile(string, []byte, os.FileMode) error {
	return fmt.Errorf("Test Error")
}
func mockMkdir(string, os.FileMode) error {
	return fmt.Errorf("Test error")
}
func mockMkdirAll(string, os.FileMode) error {
	return fmt.Errorf("Test error")
}
func mockMkdirTemp(string, string) (string, error) {
	return "", fmt.Errorf("Test error")
}
func mockOpen(string) (*os.File, error) {
	return nil, fmt.Errorf("Test error")
}
func mockOpenFile(string, int, os.FileMode) (*os.File, error) {
	return nil, fmt.Errorf("Test error")
}
func mockOpenFileAppend(name string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag|os.O_APPEND, perm)
}
func mockRemoveAll(string) error {
	return fmt.Errorf("Test error")
}
func mockRename(string, string) error {
	return fmt.Errorf("Test error")
}
func mockTruncate(string, int64) error {
	return fmt.Errorf("Test error")
}
func mockCreate(string) (*os.File, error) {
	return nil, fmt.Errorf("Test error")
}
func mockCopyFile(string, string, osutil.CopyFlag) error {
	return fmt.Errorf("Test error")
}
func mockCopySpecialFile(string, string) error {
	return fmt.Errorf("Test error")
}
func mockDiskfsCreate(string, int64, diskfs.Format, diskfs.SectorSize) (*disk.Disk, error) {
	return nil, fmt.Errorf("Test error")
}
func mockRandRead(output []byte) (int, error) {
	return 0, fmt.Errorf("Test error")
}
func mockSeedOpen(seedDir, label string) (seed.Seed, error) {
	return nil, fmt.Errorf("Test error")
}
func mockImagePrepare(*image.Options) error {
	return fmt.Errorf("Test Error")
}
func mockUnmarshal([]byte, any) error {
	return fmt.Errorf("Test Error")
}
func mockMarshal(interface{}) ([]byte, error) {
	return []byte{}, fmt.Errorf("Test Error")
}
func mockRel(string, string) (string, error) {
	return "", fmt.Errorf("Test error")
}
func mockGojsonschemaValidateError(gojsonschema.JSONLoader, gojsonschema.JSONLoader) (*gojsonschema.Result, error) {
	return nil, fmt.Errorf("Test Error")
}

func readOnlyDiskfsCreate(diskName string, size int64, format diskfs.Format, sectorSize diskfs.SectorSize) (*disk.Disk, error) {
	diskFile, err := os.OpenFile(diskName, os.O_RDONLY|os.O_CREATE, 0444)
	disk := disk.Disk{
		File:             diskFile,
		LogicalBlocksize: int64(sectorSize),
	}
	return &disk, err
}

// Fake exec command helper
var testCaseName string

func fakeExecCommand(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestExecHelperProcess", "--", command}
	cs = append(cs, args...)
	//nolint:gosec,G204
	cmd := exec.Command(os.Args[0], cs...)
	tc := "TEST_CASE=" + testCaseName
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1", tc}
	return cmd
}

// This is a helper that mocks out any exec calls performed in this package
func TestExecHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)
	args := os.Args

	// We need to get rid of the trailing 'mock' call of our test binary, so
	// that args has the actual command arguments. We can then check their
	// correctness etc.
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}

	// I think the best idea I saw from people is to switch this on test case
	// instead on the actual arguments. And this makes sense to me
	switch os.Getenv("TEST_CASE") {
	case "TestGeneratePackageManifest":
		fmt.Fprint(os.Stdout, "foo 1.2\nbar 1.4-1ubuntu4.1\nlibbaz 0.1.3ubuntu2\n")
	case "TestGenerateFilelist":
		fmt.Fprint(os.Stdout, "/root\n/home\n/var")
	case "TestFailedPreseedClassicImage":
		fallthrough
	case "TestFailedUpdateGrubLosetup":
		fallthrough
	case "TestFailedMakeQcow2Image":
		fallthrough
	case "TestFailedGeneratePackageManifest":
		fallthrough
	case "TestFailedGenerateFilelist":
		fallthrough
	case "TestFailedGerminate":
		fallthrough
	case "TestFailedSetupLiveBuildCommands":
		fallthrough
	case "TestFailedCreateChroot":
		fallthrough
	case "TestStateMachine_installPackages_fail":
		fallthrough
	case "TestFailedPrepareClassicImage":
		fallthrough
	case "TestFailedBuildGadgetTree":
		// throwing an error here simulates the "command" having an error
		os.Exit(1)
	case "TestFailedUpdateGrubOther": // this passes the initial losetup command and fails a later command
		if args[0] != "losetup" {
			os.Exit(1)
		}
	case "TestFailedCreateChrootNoHostname":
		fallthrough
	case "TestFailedCreateChrootSkip":
		fallthrough
	case "TestFailedRunLiveBuild":
		// Do nothing so we don't have to wait for actual lb commands
		break
	}
}

// testStateMachine implements Setup, Run, and Teardown() for testing purposes
type testStateMachine struct {
	StateMachine
}

// testStateMachine needs its own setup
func (TestStateMachine *testStateMachine) Setup() error {
	// set the states that will be used for this image type
	TestStateMachine.states = allTestStates

	// do the validation common to all image types
	if err := TestStateMachine.validateInput(); err != nil {
		return err
	}

	// if --resume was passed, figure out where to start
	if err := TestStateMachine.readMetadata(metadataStateFile); err != nil {
		return err
	}

	return nil
}

// TestUntilThru tests --until and --thru with each state
func TestUntilThru(t *testing.T) {
	testCases := []struct {
		name string
	}{
		{"until"},
		{"thru"},
	}
	for _, tc := range testCases {
		t.Run("test "+tc.name, func(t *testing.T) {
			for _, state := range allTestStates {
				asserter := helper.Asserter{T: t}
				// run a partial state machine
				var partialStateMachine testStateMachine
				partialStateMachine.commonFlags, partialStateMachine.stateMachineFlags = helper.InitCommonOpts()
				tempDir := filepath.Join("/tmp", "ubuntu-image-"+tc.name)
				if err := os.Mkdir(tempDir, 0755); err != nil {
					t.Errorf("Could not create workdir: %s\n", err.Error())
				}
				t.Cleanup(func() { os.RemoveAll(tempDir) })
				partialStateMachine.stateMachineFlags.WorkDir = tempDir

				if tc.name == "until" {
					partialStateMachine.stateMachineFlags.Until = state.name
				} else {
					partialStateMachine.stateMachineFlags.Thru = state.name
				}

				err := partialStateMachine.Setup()
				asserter.AssertErrNil(err, false)

				err = partialStateMachine.Run()
				asserter.AssertErrNil(err, false)

				err = partialStateMachine.Teardown()
				asserter.AssertErrNil(err, false)

				// now resume
				var resumeStateMachine testStateMachine
				resumeStateMachine.commonFlags, resumeStateMachine.stateMachineFlags = helper.InitCommonOpts()
				resumeStateMachine.stateMachineFlags.Resume = true
				resumeStateMachine.stateMachineFlags.WorkDir = partialStateMachine.stateMachineFlags.WorkDir

				err = resumeStateMachine.Setup()
				asserter.AssertErrNil(err, false)

				err = resumeStateMachine.Run()
				asserter.AssertErrNil(err, false)

				err = resumeStateMachine.Teardown()
				asserter.AssertErrNil(err, false)

				os.RemoveAll(tempDir)
			}
		})
	}
}

// TestDebug ensures that the name of the states is printed when the --debug flag is used
func TestDebug(t *testing.T) {
	asserter := helper.Asserter{T: t}
	workDir := "ubuntu-image-test-debug"
	if err := os.Mkdir("ubuntu-image-test-debug", 0755); err != nil {
		t.Errorf("Failed to create temporary directory %s\n", workDir)
	}

	t.Cleanup(func() { os.RemoveAll(workDir) })

	var stateMachine testStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.stateMachineFlags.WorkDir = workDir
	stateMachine.commonFlags.Debug = true

	err := stateMachine.Setup()
	asserter.AssertErrNil(err, true)

	// just use the one state
	stateMachine.states = testStates
	stdout, restoreStdout, err := helper.CaptureStd(&os.Stdout)
	asserter.AssertErrNil(err, true)

	err = stateMachine.Run()
	asserter.AssertErrNil(err, true)

	// restore stdout and check that the debug info was printed
	restoreStdout()
	readStdout, err := io.ReadAll(stdout)
	asserter.AssertErrNil(err, true)

	if !strings.Contains(string(readStdout), stateMachine.states[0].name) {
		t.Errorf("Expected state name \"%s\" to appear in output \"%s\"\n", stateMachine.states[0].name, string(readStdout))
	}
}

// TestFunction replaces some of the stateFuncs to test various error scenarios
func TestFunctionErrors(t *testing.T) {
	testCases := []struct {
		name          string
		overrideState int
		newStateFunc  stateFunc
	}{
		{"error_state_func", 0, stateFunc{"test_error_state_func", func(stateMachine *StateMachine) error { return fmt.Errorf("Test Error") }}},
		{"error_write_metadata", 10, stateFunc{"test_error_write_metadata", func(stateMachine *StateMachine) error {
			os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
			return nil
		}}},
	}
	for _, tc := range testCases {
		t.Run("test "+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			workDir := filepath.Join("/tmp", "ubuntu-image-"+tc.name)
			err := os.Mkdir(workDir, 0755)
			asserter.AssertErrNil(err, true)

			t.Cleanup(func() { os.RemoveAll(workDir) })

			var stateMachine testStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.stateMachineFlags.WorkDir = workDir
			err = stateMachine.Setup()
			asserter.AssertErrNil(err, true)

			// override the function, but save the old one
			oldStateFunc := stateMachine.states[tc.overrideState]
			stateMachine.states[tc.overrideState] = tc.newStateFunc
			defer func() {
				stateMachine.states[tc.overrideState] = oldStateFunc
			}()
			if err := stateMachine.Run(); err == nil {
				if err := stateMachine.Teardown(); err == nil {
					t.Errorf("Expected an error but there was none")
				}
			}
		})
	}
}

// TestSetCommonOpts ensures that the function actually sets the correct values in the struct
func TestSetCommonOpts(t *testing.T) {
	asserter := helper.Asserter{T: t}
	type args struct {
		stateMachine     SmInterface
		commonOpts       *commands.CommonOpts
		stateMachineOpts *commands.StateMachineOpts
	}

	cmpOpts := []cmp.Option{
		cmp.AllowUnexported(
			StateMachine{},
			temporaryDirectories{},
		),
		cmpopts.IgnoreUnexported(
			gadget.Info{},
		),
	}

	tests := []struct {
		name        string
		args        args
		want        SmInterface
		expectedErr string
	}{
		{
			name: "set options on a snap state machine",
			args: args{
				stateMachine: &SnapStateMachine{},
				commonOpts: &commands.CommonOpts{
					Debug: true,
				},
				stateMachineOpts: &commands.StateMachineOpts{
					WorkDir: "workdir",
				},
			},
			want: &SnapStateMachine{
				StateMachine: StateMachine{
					commonFlags: &commands.CommonOpts{
						Debug: true,
					},
					stateMachineFlags: &commands.StateMachineOpts{
						WorkDir: "workdir",
					},
				},
			},
		},
		{
			name: "set options on a classic state machine",
			args: args{
				stateMachine: &ClassicStateMachine{},
				commonOpts: &commands.CommonOpts{
					Debug: true,
				},
				stateMachineOpts: &commands.StateMachineOpts{
					WorkDir: "workdir",
				},
			},
			want: &ClassicStateMachine{
				StateMachine: StateMachine{
					commonFlags: &commands.CommonOpts{
						Debug: true,
					},
					stateMachineFlags: &commands.StateMachineOpts{
						WorkDir: "workdir",
					},
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.args.stateMachine.SetCommonOpts(tc.args.commonOpts, tc.args.stateMachineOpts)
			asserter.AssertEqual(tc.want, tc.args.stateMachine, cmpOpts...)
		})
	}
}

// TestParseImageSizes tests a successful image size parse with all of the different allowed syntaxes
func TestParseImageSizes(t *testing.T) {
	testCases := []struct {
		name   string
		size   string
		result map[string]quantity.Size
	}{
		{"one_size", "4G", map[string]quantity.Size{
			"first":  4 * quantity.SizeGiB,
			"second": 4 * quantity.SizeGiB,
			"third":  4 * quantity.SizeGiB,
			"fourth": 4 * quantity.SizeGiB}},
		{"size_per_image_name", "first:1G,second:2G,third:3G,fourth:4G", map[string]quantity.Size{
			"first":  1 * quantity.SizeGiB,
			"second": 2 * quantity.SizeGiB,
			"third":  3 * quantity.SizeGiB,
			"fourth": 4 * quantity.SizeGiB}},
		{"size_per_image_number", "0:1G,1:2G,2:3G,3:4G", map[string]quantity.Size{
			"first":  1 * quantity.SizeGiB,
			"second": 2 * quantity.SizeGiB,
			"third":  3 * quantity.SizeGiB,
			"fourth": 4 * quantity.SizeGiB}},
		{"mixed_size_syntax", "0:1G,second:2G,2:3G,fourth:4G", map[string]quantity.Size{
			"first":  1 * quantity.SizeGiB,
			"second": 2 * quantity.SizeGiB,
			"third":  3 * quantity.SizeGiB,
			"fourth": 4 * quantity.SizeGiB}},
	}
	for _, tc := range testCases {
		t.Run("test_parse_image_sizes_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.YamlFilePath = filepath.Join("testdata", "gadget-multi.yaml")
			stateMachine.commonFlags.Size = tc.size

			// need workdir and loaded gadget.yaml set up for this
			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, false)

			err = stateMachine.loadGadgetYaml()
			asserter.AssertErrNil(err, false)

			err = stateMachine.parseImageSizes()
			asserter.AssertErrNil(err, false)

			// ensure the correct size was set
			for volumeName := range stateMachine.GadgetInfo.Volumes {
				setSize := stateMachine.ImageSizes[volumeName]
				if setSize != tc.result[volumeName] {
					t.Errorf("Volume %s has the wrong size set: %d", volumeName, setSize)
				}
			}

		})
	}
}

// TestFailedParseImageSizes tests failures in parsing the image sizes
func TestFailedParseImageSizes(t *testing.T) {
	testCases := []struct {
		name   string
		size   string
		errMsg string
	}{
		{"invalid_size", "4test", "Failed to parse argument to --image-size"},
		{"too_many_args", "first:1G:2G", "Argument to --image-size first:1G:2G is not in the correct format"},
		{"multiple_invalid", "first:1test", "Failed to parse argument to --image-size"},
		{"volume_not_exist", "fifth:1G", "Volume fifth does not exist in gadget.yaml"},
		{"index_out_of_range", "9:1G", "Volume index 9 is out of range"},
	}
	for _, tc := range testCases {
		t.Run("test_failed_parse_image_sizes_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.YamlFilePath = filepath.Join("testdata", "gadget-multi.yaml")

			// need workdir and loaded gadget.yaml set up for this
			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, false)

			err = stateMachine.loadGadgetYaml()
			asserter.AssertErrNil(err, false)

			// run parseImage size and make sure it failed
			stateMachine.commonFlags.Size = tc.size
			err = stateMachine.parseImageSizes()
			asserter.AssertErrContains(err, tc.errMsg)
		})
	}
}

// TestHandleContentSizes ensures that using --image-size with a few different values
// results in the correct sizes in stateMachine.ImageSizes
func TestHandleContentSizes(t *testing.T) {
	testCases := []struct {
		name   string
		size   string
		result map[string]quantity.Size
	}{
		{"size_not_specified", "", map[string]quantity.Size{"pc": 17825792}},
		{"size_smaller_than_content", "pc:123", map[string]quantity.Size{"pc": 17825792}},
		{"size_bigger_than_content", "pc:4G", map[string]quantity.Size{"pc": 4 * quantity.SizeGiB}},
	}
	for _, tc := range testCases {
		t.Run("test_handle_content_sizes_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.commonFlags.Size = tc.size
			stateMachine.YamlFilePath = filepath.Join("testdata", "gadget_tree",
				"meta", "gadget.yaml")

			// need workdir and loaded gadget.yaml set up for this
			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, false)

			err = stateMachine.loadGadgetYaml()
			asserter.AssertErrNil(err, false)

			stateMachine.handleContentSizes(0, "pc")
			// ensure the correct size was set
			for volumeName := range stateMachine.GadgetInfo.Volumes {
				setSize := stateMachine.ImageSizes[volumeName]
				if setSize != tc.result[volumeName] {
					t.Errorf("Volume %s has the wrong size set: %d. "+
						"Should be %d", volumeName, setSize, tc.result[volumeName])
				}
			}
		})
	}
}

// TestPostProcessGadgetYaml runs through a variety of gadget.yaml files
// and ensures the volume/structures are as expected
func TestPostProcessGadgetYaml(t *testing.T) {
	// helper function to define *quantity.Offsets inline
	createOffsetPointer := func(x quantity.Offset) *quantity.Offset {
		return &x
	}
	testCases := []struct {
		name           string
		gadgetYaml     string
		expectedResult gadget.Volume
	}{
		{
			"rootfs_gadget_source",
			filepath.Join("testdata", "gadget_rootfs_source.yaml"),
			gadget.Volume{
				Schema:     "mbr",
				Bootloader: "u-boot",
				Name:       "pc",
				Structure: []gadget.VolumeStructure{
					{
						VolumeName: "pc",
						Type:       "0C",
						Offset:     createOffsetPointer(1048576),
						MinSize:    536870912,
						Size:       536870912,
						Label:      "system-boot",
						Filesystem: "vfat",
						Content: []gadget.VolumeContent{
							{
								UnresolvedSource: "install/boot-assets/",
								Target:           "/",
							},
							{
								UnresolvedSource: "../../root/boot/vmlinuz",
								Target:           "/",
							},
							{
								UnresolvedSource: "../../root/boot/initrd.img",
								Target:           "/",
							},
						},
					},
					{
						Type:       "83,0FC63DAF-8483-4772-8E79-3D69D8477DE4",
						Role:       "system-data",
						Filesystem: "ext4",
						Label:      "writable",
						Offset:     createOffsetPointer(537919488),
						Content:    []gadget.VolumeContent{},
						YamlIndex:  1,
					},
				},
			},
		},
		{
			"rootfs_unspecified",
			filepath.Join("testdata", "gadget_no_rootfs.yaml"),
			gadget.Volume{
				Schema:     "gpt",
				Bootloader: "grub",
				Name:       "pc",
				Structure: []gadget.VolumeStructure{
					{
						VolumeName: "pc",
						Name:       "mbr",
						Type:       "mbr",
						Offset:     createOffsetPointer(0),
						Role:       "mbr",
						MinSize:    440,
						Size:       440,
						Content: []gadget.VolumeContent{
							{
								Image:  "pc-boot.img",
								Offset: createOffsetPointer(0),
							},
						},
					},
					{
						VolumeName: "pc",
						Name:       "BIOS Boot",
						Type:       "DA,21686148-6449-6E6F-744E-656564454649",
						MinSize:    1048576,
						Size:       1048576,
						OffsetWrite: &gadget.RelativeOffset{
							RelativeTo: "mbr",
							Offset:     quantity.Offset(92),
						},
						Offset: createOffsetPointer(1048576),
						Content: []gadget.VolumeContent{
							{
								Image: "pc-core.img",
							},
						},
						YamlIndex: 1,
					},
					{
						VolumeName: "pc",
						Name:       "EFI System",
						Type:       "EF,C12A7328-F81F-11D2-BA4B-00A0C93EC93B",
						MinSize:    52428800,
						Size:       52428800,
						Filesystem: "vfat",
						Offset:     createOffsetPointer(2097152),
						Label:      "system-boot",
						Content: []gadget.VolumeContent{
							{
								UnresolvedSource: "grubx64.efi",
								Target:           "EFI/boot/grubx64.efi",
							},
							{
								UnresolvedSource: "shim.efi.signed",
								Target:           "EFI/boot/bootx64.efi",
							},
							{
								UnresolvedSource: "grub-cpc.cfg",
								Target:           "EFI/ubuntu/grub.cfg",
							},
						},
						YamlIndex: 2,
					},
					{
						Type:       "83,0FC63DAF-8483-4772-8E79-3D69D8477DE4",
						Role:       "system-data",
						Filesystem: "ext4",
						Label:      "writable",
						Offset:     createOffsetPointer(54525952),
						Content:    []gadget.VolumeContent{},
						YamlIndex:  3,
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run("test_post_process_gadget_yaml_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

			// need workdir and loaded gadget.yaml set up for this
			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, false)

			// load in the gadget.yaml file
			stateMachine.YamlFilePath = tc.gadgetYaml

			// ensure unpack exists and load gadget.yaml
			err = os.MkdirAll(stateMachine.tempDirs.unpack, 0755)
			asserter.AssertErrNil(err, true)
			err = stateMachine.loadGadgetYaml()
			asserter.AssertErrNil(err, false)

			// we now need to also ensure the expectedResult to have properly set volume pointers
			for i := range tc.expectedResult.Structure {
				if tc.expectedResult.Structure[i].VolumeName != "" {
					tc.expectedResult.Structure[i].EnclosingVolume = stateMachine.GadgetInfo.Volumes[tc.expectedResult.Structure[i].VolumeName]
				}
			}

			if !reflect.DeepEqual(*stateMachine.GadgetInfo.Volumes["pc"], tc.expectedResult) {
				t.Errorf("GadgetInfo after postProcessGadgetYaml:\n%+v "+
					"does not match expected result:\n%+v",
					*stateMachine.GadgetInfo.Volumes["pc"],
					tc.expectedResult,
				)
			}
		})
	}
}

// TestFailedPostProcessGadgetYaml tests failues in the post processing of
// the gadget.yaml file after loading it in. This is accomplished by mocking
// os.MkdirAll
func TestFailedPostProcessGadgetYaml(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine StateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, false)

	// set a valid yaml file and load it in
	stateMachine.YamlFilePath = filepath.Join("testdata",
		"gadget_tree", "meta", "gadget.yaml")
	// ensure unpack exists
	err = os.MkdirAll(stateMachine.tempDirs.unpack, 0755)
	asserter.AssertErrNil(err, true)
	err = stateMachine.loadGadgetYaml()
	asserter.AssertErrNil(err, false)

	// mock filepath.Rel
	filepathRel = mockRel
	defer func() {
		filepathRel = filepath.Rel
	}()
	err = stateMachine.postProcessGadgetYaml()
	asserter.AssertErrContains(err, "Error creating relative path")
	filepathRel = filepath.Rel

	// mock os.MkdirAll
	osMkdirAll = mockMkdirAll
	defer func() {
		osMkdirAll = os.MkdirAll
	}()
	err = stateMachine.postProcessGadgetYaml()
	asserter.AssertErrContains(err, "Error creating volume dir")
	osMkdirAll = os.MkdirAll

	// use a gadget with a disallowed string in the content field
	stateMachine.YamlFilePath = filepath.Join("testdata", "gadget_invalid_content.yaml")
	err = stateMachine.loadGadgetYaml()
	asserter.AssertErrContains(err, "disallowed for security purposes")
}

func TestStateMachine_readMetadata(t *testing.T) {
	type args struct {
		metadataFile string
		resume       bool
	}

	cmpOpts := []cmp.Option{
		cmp.AllowUnexported(
			StateMachine{},
			gadget.Info{},
			temporaryDirectories{},
			stateFunc{},
		),
		cmpopts.IgnoreFields(stateFunc{}, "function"),
		cmpopts.IgnoreFields(gadget.VolumeStructure{}, "EnclosingVolume"),
	}

	testCases := []struct {
		name             string
		wantStateMachine *StateMachine
		args             args
		shouldPass       bool
		expectedError    string
	}{
		{
			name: "successful read",
			args: args{
				metadataFile: "successful_read.json",
				resume:       true,
			},
			wantStateMachine: &StateMachine{
				stateMachineFlags: &commands.StateMachineOpts{
					Resume:  true,
					WorkDir: filepath.Join(testDataDir, "metadata"),
				},
				CurrentStep:  "",
				StepsTaken:   2,
				YamlFilePath: "/tmp/ubuntu-image-2329554237/unpack/gadget/meta/gadget.yaml",
				IsSeeded:     true,
				SectorSize:   quantity.Size(512),
				RootfsSize:   quantity.Size(775915520),
				states:       allTestStates[2:],
				GadgetInfo: &gadget.Info{
					Volumes: map[string]*gadget.Volume{
						"pc": {
							Schema:     "gpt",
							Bootloader: "grub",
							Structure: []gadget.VolumeStructure{
								{
									Name:    "mbr",
									Offset:  ptrToOffset(quantity.Offset(quantity.Size(0))),
									MinSize: quantity.Size(440),
									Size:    quantity.Size(440),
									Role:    "mbr",
									Type:    "mbr",
									Content: []gadget.VolumeContent{
										{
											Image: "pc-boot.img",
										},
									},
									Update: gadget.VolumeUpdate{
										Edition: 1,
									},
								},
							},
						},
					},
				},
				ImageSizes:  map[string]quantity.Size{"pc": 3155165184},
				VolumeOrder: []string{"pc"},
				VolumeNames: map[string]string{"pc": "pc.img"},
				Packages:    []string{"nginx", "apache2"},
				Snaps:       []string{"core", "lxd"},
				tempDirs: temporaryDirectories{
					rootfs:  filepath.Join(testDataDir, "metadata", "root"),
					unpack:  filepath.Join(testDataDir, "metadata", "unpack"),
					volumes: filepath.Join(testDataDir, "metadata", "volumes"),
					chroot:  filepath.Join(testDataDir, "metadata", "chroot"),
					scratch: filepath.Join(testDataDir, "metadata", "scratch"),
				},
			},
			shouldPass: true,
		},
		{
			name: "invalid format",
			args: args{
				metadataFile: "invalid_format.json",
				resume:       true,
			},
			wantStateMachine: nil,
			shouldPass:       false,
			expectedError:    "failed to parse metadata file",
		},
		{
			name: "missing state file",
			args: args{
				metadataFile: "inexistent.json",
				resume:       true,
			},
			wantStateMachine: nil,
			shouldPass:       false,
			expectedError:    "error reading metadata file",
		},
		{
			name: "do nothing if not resuming",
			args: args{
				metadataFile: "unimportant.json",
				resume:       false,
			},
			wantStateMachine: &StateMachine{
				stateMachineFlags: &commands.StateMachineOpts{
					Resume:  false,
					WorkDir: filepath.Join(testDataDir, "metadata"),
				},
				states: allTestStates,
			},
			shouldPass:    true,
			expectedError: "error reading metadata file",
		},
		{
			name: "state file with too many steps",
			args: args{
				metadataFile: "too_many_steps.json",
				resume:       true,
			},
			wantStateMachine: nil,
			shouldPass:       false,
			expectedError:    "invalid steps taken count",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			gotStateMachine := &StateMachine{
				stateMachineFlags: &commands.StateMachineOpts{
					Resume:  tc.args.resume,
					WorkDir: filepath.Join(testDataDir, "metadata"),
				},
				states: allTestStates,
			}

			err := gotStateMachine.readMetadata(tc.args.metadataFile)
			if tc.shouldPass {
				asserter.AssertErrNil(err, true)
				asserter.AssertEqual(tc.wantStateMachine, gotStateMachine, cmpOpts...)
			} else {
				asserter.AssertErrContains(err, tc.expectedError)
			}
		})
	}
}

func TestStateMachine_writeMetadata(t *testing.T) {
	tests := []struct {
		name          string
		stateMachine  *StateMachine
		shouldPass    bool
		expectedError string
	}{
		{
			name: "successful write",
			stateMachine: &StateMachine{
				stateMachineFlags: &commands.StateMachineOpts{
					Resume:  true,
					WorkDir: filepath.Join(testDataDir, "metadata"),
				},
				CurrentStep:  "",
				StepsTaken:   2,
				YamlFilePath: "/tmp/ubuntu-image-2329554237/unpack/gadget/meta/gadget.yaml",
				IsSeeded:     true,
				SectorSize:   quantity.Size(512),
				RootfsSize:   quantity.Size(775915520),
				states:       allTestStates[2:],
				GadgetInfo: &gadget.Info{
					Volumes: map[string]*gadget.Volume{
						"pc": {
							Schema:     "gpt",
							Bootloader: "grub",
							Structure: []gadget.VolumeStructure{
								{
									Name:    "mbr",
									Offset:  ptrToOffset(quantity.Offset(quantity.Size(0))),
									MinSize: quantity.Size(440),
									Size:    quantity.Size(440),
									Role:    "mbr",
									Type:    "mbr",
									Content: []gadget.VolumeContent{
										{
											Image: "pc-boot.img",
										},
									},
									Update: gadget.VolumeUpdate{
										Edition: 1,
									},
								},
							},
						},
					},
				},
				Packages:    nil,
				Snaps:       nil,
				ImageSizes:  map[string]quantity.Size{"pc": 3155165184},
				VolumeOrder: []string{"pc"},
				VolumeNames: map[string]string{"pc": "pc.img"},
				tempDirs: temporaryDirectories{
					rootfs:  filepath.Join(testDataDir, "metadata", "root"),
					unpack:  filepath.Join(testDataDir, "metadata", "unpack"),
					volumes: filepath.Join(testDataDir, "metadata", "volumes"),
					chroot:  filepath.Join(testDataDir, "metadata", "chroot"),
					scratch: filepath.Join(testDataDir, "metadata", "scratch"),
				},
			},
			shouldPass: true,
		},
		{
			name: "fail to marshall an invalid stateMachine - use a GadgetInfo with a channel",
			stateMachine: &StateMachine{
				stateMachineFlags: &commands.StateMachineOpts{
					Resume:  true,
					WorkDir: filepath.Join(testDataDir, "metadata"),
				},
				GadgetInfo: &gadget.Info{
					Defaults: map[string]map[string]interface{}{
						"key": {
							"key": make(chan int),
						},
					},
				},
				CurrentStep:  "",
				StepsTaken:   2,
				YamlFilePath: "/tmp/ubuntu-image-2329554237/unpack/gadget/meta/gadget.yaml",
			},
			shouldPass:    false,
			expectedError: "failed to JSON encode metadata",
		},
		{
			name: "fail to write in inexistent directory",
			stateMachine: &StateMachine{
				stateMachineFlags: &commands.StateMachineOpts{
					Resume:  true,
					WorkDir: filepath.Join("non-existent", "metadata"),
				},
				CurrentStep:  "",
				StepsTaken:   2,
				YamlFilePath: "/tmp/ubuntu-image-2329554237/unpack/gadget/meta/gadget.yaml",
			},
			shouldPass:    false,
			expectedError: "error opening JSON metadata file for writing",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			tName := strings.ReplaceAll(tc.name, " ", "_")
			err := tc.stateMachine.writeMetadata(fmt.Sprintf("result_%s.json", tName))

			if tc.shouldPass {
				want, err := os.ReadFile(filepath.Join(testDataDir, "metadata", fmt.Sprintf("reference_%s.json", tName)))
				if err != nil {
					t.Fatal("Unable to load reference metadata file: %w", err)
				}

				got, err := os.ReadFile(filepath.Join(testDataDir, "metadata", fmt.Sprintf("result_%s.json", tName)))
				if err != nil {
					t.Fatal("Unable to load metadata file: %w", err)
				}

				asserter.AssertEqual(want, got)

			} else {
				asserter.AssertErrContains(err, tc.expectedError)
			}

		})
	}
}

func TestMinSize(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine StateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

	stateMachine.YamlFilePath = filepath.Join("testdata", "gadget-gpt-minsize.yaml")

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, false)
	err = stateMachine.loadGadgetYaml()
	asserter.AssertErrNil(err, false)
}

func TestStateMachine_displayStates(t *testing.T) {
	asserter := helper.Asserter{T: t}
	type fields struct {
		commonFlags       *commands.CommonOpts
		stateMachineFlags *commands.StateMachineOpts
		states            []stateFunc
	}
	tests := []struct {
		name       string
		fields     fields
		wantOutput string
	}{
		{
			name: "simple case with 2 states",
			fields: fields{
				commonFlags: &commands.CommonOpts{
					Debug: true,
				},
				stateMachineFlags: &commands.StateMachineOpts{},
				states: []stateFunc{
					{
						name: "stateFunc1",
					},
					{
						name: "stateFunc2",
					},
				},
			},
			wantOutput: `
Following states will be executed:
[0] stateFunc1
[1] stateFunc2

Continuing
`,
		},
		{
			name: "3 states with until",
			fields: fields{
				commonFlags: &commands.CommonOpts{
					Debug: true,
				},
				stateMachineFlags: &commands.StateMachineOpts{
					Until: "stateFunc3",
				},
				states: []stateFunc{
					{
						name: "stateFunc1",
					},
					{
						name: "stateFunc2",
					},
					{
						name: "stateFunc3",
					},
				},
			},
			wantOutput: `
Following states will be executed:
[0] stateFunc1
[1] stateFunc2

Continuing
`,
		},
		{
			name: "3 states with thru",
			fields: fields{
				commonFlags: &commands.CommonOpts{
					Debug: true,
				},
				stateMachineFlags: &commands.StateMachineOpts{
					Thru: "stateFunc2",
				},
				states: []stateFunc{
					{
						name: "stateFunc1",
					},
					{
						name: "stateFunc2",
					},
					{
						name: "stateFunc3",
					},
				},
			},
			wantOutput: `
Following states will be executed:
[0] stateFunc1
[1] stateFunc2

Continuing
`,
		},
		{
			name: "3 states without debug",
			fields: fields{
				commonFlags: &commands.CommonOpts{
					Debug: false,
				},
				stateMachineFlags: &commands.StateMachineOpts{
					Thru: "stateFunc2",
				},
				states: []stateFunc{
					{
						name: "stateFunc1",
					},
					{
						name: "stateFunc2",
					},
					{
						name: "stateFunc3",
					},
				},
			},
			wantOutput: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// capture stdout, calculate the states, and ensure they were printed
			stdout, restoreStdout, err := helper.CaptureStd(&os.Stdout)
			defer restoreStdout()
			asserter.AssertErrNil(err, true)

			s := &StateMachine{
				commonFlags:       tt.fields.commonFlags,
				stateMachineFlags: tt.fields.stateMachineFlags,
				states:            tt.fields.states,
			}
			s.displayStates()

			restoreStdout()
			readStdout, err := io.ReadAll(stdout)
			asserter.AssertErrNil(err, true)

			asserter.AssertEqual(tt.wantOutput, string(readStdout))
		})
	}
}
