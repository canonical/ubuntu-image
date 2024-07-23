package statemachine

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	diskfs "github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
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
func mockNewMountedFilesystemWriter(*gadget.LaidOutStructure, *gadget.LaidOutStructure,
	gadget.ContentObserver) (*gadget.MountedFilesystemWriter, error) {
	return nil, fmt.Errorf("Test Error")
}
func mockMkfsWithContent(typ, img, label, contentRootDir string, deviceSize, sectorSize quantity.Size) error {
	return fmt.Errorf("Test Error")
}
func mockMkfs(typ, img, label string, deviceSize, sectorSize quantity.Size) error {
	return fmt.Errorf("Test Error")
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

// helper function to define *quantity.Offsets inline
func createOffsetPointer(x quantity.Offset) *quantity.Offset {
	return &x
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
	case "TestFailedPreseedClassicImage",
		"TestFailedUpdateGrubLosetup",
		"TestFailedMakeQcow2Image",
		"TestFailedGeneratePackageManifest",
		"TestFailedGenerateFilelist",
		"TestFailedGerminate",
		"TestFailedSetupLiveBuildCommands",
		"TestFailedCreateChroot",
		"TestStateMachine_installPackages_fail",
		"TestFailedPrepareClassicImage",
		"TestFailedBuildGadgetTree":
		// throwing an error here simulates the "command" having an error
		os.Exit(1)
	case "TestFailedUpdateGrubOther": // this passes the initial losetup command and fails a later command
		if args[0] != "losetup" {
			os.Exit(1)
		}
	case "TestFailedCreateChrootNoHostname",
		"TestFailedCreateChrootSkip",
		"TestFailedRunLiveBuild":
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
	if err := os.Mkdir(workDir, 0755); err != nil {
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

// TestDryRun ensures that the name of the states is not printed when the --dry-run flag is used
// because nothing should be executed
func TestDryRun(t *testing.T) {
	asserter := helper.Asserter{T: t}
	workDir := "ubuntu-image-test-debug"
	err := os.Mkdir(workDir, 0755)
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(workDir) })

	var stateMachine testStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.stateMachineFlags.WorkDir = workDir
	stateMachine.commonFlags.DryRun = true

	err = stateMachine.Setup()
	asserter.AssertErrNil(err, true)

	// just use the one state
	stateMachine.states = testStates
	stdout, restoreStdout, err := helper.CaptureStd(&os.Stdout)
	asserter.AssertErrNil(err, true)

	err = stateMachine.Run()
	asserter.AssertErrNil(err, true)

	restoreStdout()
	readStdout, err := io.ReadAll(stdout)
	asserter.AssertErrNil(err, true)

	if strings.Contains(string(readStdout), stateMachine.states[0].name) {
		t.Errorf("Expected state name \"%s\" to not appear in output \"%s\"\n", stateMachine.states[0].name, string(readStdout))
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
		{"error_write_metadata", 8, stateFunc{"test_error_write_metadata", func(stateMachine *StateMachine) error {
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

// TestGrowImageSize ensures that using --image-size with a few different values
// results in the correct sizes in stateMachine.ImageSizes
func TestGrowImageSize(t *testing.T) {
	testCases := []struct {
		name   string
		size   string
		result map[string]quantity.Size
	}{
		{
			name:   "size_not_specified",
			size:   "",
			result: map[string]quantity.Size{"pc": 54525952},
		},
		{
			name:   "size_smaller_than_content",
			size:   "pc:123",
			result: map[string]quantity.Size{"pc": 54525952},
		},
		{
			name:   "size_bigger_than_content",
			size:   "pc:4G",
			result: map[string]quantity.Size{"pc": 4 * quantity.SizeGiB},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			var stateMachine StateMachine
			volumeName := "pc"
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.commonFlags.Size = tc.size
			stateMachine.YamlFilePath = filepath.Join("testdata", "gadget_tree",
				"meta", "gadget.yaml")

			// need workdir and loaded gadget.yaml set up for this
			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, false)

			err = stateMachine.loadGadgetYaml()
			asserter.AssertErrNil(err, false)

			v, found := stateMachine.GadgetInfo.Volumes[volumeName]
			if !found {
				t.Fatalf("no volume names pc in the gadget")
			}

			stateMachine.growImageSize(v.MinSize(), volumeName)
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

// TestStateMachine_postProcessGadgetYaml tests postProcessGadgetYaml
func TestStateMachine_postProcessGadgetYaml(t *testing.T) {
	cmpOpts := []cmp.Option{
		cmpopts.IgnoreUnexported(
			gadget.Volume{},
		),
		cmpopts.IgnoreFields(gadget.VolumeStructure{}, "EnclosingVolume"),
	}
	tests := []struct {
		name         string
		gadgetYaml   []byte
		volumeOrder  []string
		wantVolumes  map[string]*gadget.Volume
		wantIsSeeded bool
		expectedErr  string
	}{
		{
			name: "simple full test",
			gadgetYaml: []byte(`volumes:
  pc:
    bootloader: grub
    structure:
      - name: mbr
        type: mbr
        size: 440
        update:
          edition: 1
        content:
          - image: pc-boot.img
      - name: BIOS Boot
        type: DA,21686148-6449-6E6F-744E-656564454649
        size: 1M
        offset: 1M
        offset-write: mbr+92
        update:
          edition: 2
        content:
          - image: pc-core.img
      - name: ubuntu-seed
        role: system-seed
        filesystem: vfat
        # UEFI will boot the ESP partition by default first
        type: EF,C12A7328-F81F-11D2-BA4B-00A0C93EC93B
        size: 1200M
        update:
          edition: 2
        content:
          - source: grubx64.efi
            target: EFI/boot/grubx64.efi
          - source: shim.efi.signed
            target: EFI/boot/bootx64.efi
      - name: ubuntu-boot
        filesystem-label: system-boot
        filesystem: ext4
        type: 83,0FC63DAF-8483-4772-8E79-3D69D8477DE4
        # whats the appropriate size?
        size: 750M
        update:
          edition: 1
        content:
          - source: grubx64.efi
            target: EFI/boot/grubx64.efi
          - source: shim.efi.signed
            target: EFI/boot/bootx64.efi
      - name: ubuntu-save
        role: system-save
        filesystem: ext4
        type: 83,0FC63DAF-8483-4772-8E79-3D69D8477DE4
        size: 16M
      - name: ubuntu-data
        role: system-data
        filesystem: ext4
        type: 83,0FC63DAF-8483-4772-8E79-3D69D8477DE4
        size: 1G
`),
			volumeOrder:  []string{"pc"},
			wantIsSeeded: true,
			wantVolumes: map[string]*gadget.Volume{
				"pc": {
					Schema:     "gpt",
					Bootloader: "grub",
					Structure: []gadget.VolumeStructure{
						{
							VolumeName: "pc",
							Name:       "mbr",
							Offset:     createOffsetPointer(0),
							MinSize:    440,
							Size:       440,
							Type:       "mbr",
							Role:       "mbr",
							Content: []gadget.VolumeContent{
								{
									Image: "pc-boot.img",
								},
							},
							Update: gadget.VolumeUpdate{Edition: 1},
						},
						{
							VolumeName: "pc",
							Name:       "BIOS Boot",
							Offset:     createOffsetPointer(1048576),
							OffsetWrite: &gadget.RelativeOffset{
								RelativeTo: "mbr",
								Offset:     quantity.Offset(92),
							},
							MinSize: 1048576,
							Size:    1048576,
							Type:    "DA,21686148-6449-6E6F-744E-656564454649",
							Content: []gadget.VolumeContent{
								{
									Image: "pc-core.img",
								},
							},
							Update:    gadget.VolumeUpdate{Edition: 2},
							YamlIndex: 1,
						},
						{
							VolumeName: "pc",
							Name:       "ubuntu-seed",
							Label:      "ubuntu-seed",
							Offset:     createOffsetPointer(2097152),
							MinSize:    1258291200,
							Size:       1258291200,
							Type:       "EF,C12A7328-F81F-11D2-BA4B-00A0C93EC93B",
							Role:       "system-seed",
							Filesystem: "vfat",
							Content: []gadget.VolumeContent{
								{
									UnresolvedSource: "grubx64.efi",
									Target:           "EFI/boot/grubx64.efi",
								},
								{
									UnresolvedSource: "shim.efi.signed",
									Target:           "EFI/boot/bootx64.efi",
								},
							},
							Update:    gadget.VolumeUpdate{Edition: 2},
							YamlIndex: 2,
						},
						{
							VolumeName: "pc",
							Name:       "ubuntu-boot",
							Offset:     createOffsetPointer(1260388352),
							MinSize:    786432000,
							Size:       786432000,
							Type:       "83,0FC63DAF-8483-4772-8E79-3D69D8477DE4",
							Label:      "system-boot",
							Filesystem: "ext4",
							Content: []gadget.VolumeContent{
								{
									UnresolvedSource: "grubx64.efi",
									Target:           "EFI/boot/grubx64.efi",
								},
								{
									UnresolvedSource: "shim.efi.signed",
									Target:           "EFI/boot/bootx64.efi",
								},
							},
							Update:    gadget.VolumeUpdate{Edition: 1},
							YamlIndex: 3,
						},
						{
							VolumeName: "pc",
							Name:       "ubuntu-save",
							Offset:     createOffsetPointer(2046820352),
							MinSize:    16777216,
							Size:       16777216,
							Type:       "83,0FC63DAF-8483-4772-8E79-3D69D8477DE4",
							Role:       "system-save",
							Filesystem: "ext4",
							YamlIndex:  4,
						},
						{
							VolumeName: "pc",
							Name:       "ubuntu-data",
							Offset:     createOffsetPointer(2063597568),
							MinSize:    1073741824,
							Size:       1073741824,
							Type:       "83,0FC63DAF-8483-4772-8E79-3D69D8477DE4",
							Role:       "system-data",
							Filesystem: "ext4",
							Content:    []gadget.VolumeContent{},
							YamlIndex:  5,
						},
					},
					Name: "pc",
				},
			},
		},
		{
			name: "minimal configuration, adding a system-data structure and missing content on system-seed",
			gadgetYaml: []byte(`volumes:
  pc:
    bootloader: grub
    structure:
      - name: mbr
        type: mbr
        size: 440
        update:
          edition: 1
        content:
          - image: pc-boot.img
      - name: ubuntu-seed
        role: system-seed
        filesystem: vfat
        # UEFI will boot the ESP partition by default first
        type: EF,C12A7328-F81F-11D2-BA4B-00A0C93EC93B
        size: 1200M
        update:
          edition: 2
`),
			volumeOrder:  []string{"pc"},
			wantIsSeeded: true,
			wantVolumes: map[string]*gadget.Volume{
				"pc": {
					Schema:     "gpt",
					Bootloader: "grub",
					Structure: []gadget.VolumeStructure{
						{
							VolumeName: "pc",
							Name:       "mbr",
							Offset:     createOffsetPointer(0),
							MinSize:    440,
							Size:       440,
							Type:       "mbr",
							Role:       "mbr",
							Content: []gadget.VolumeContent{
								{
									Image: "pc-boot.img",
								},
							},
							Update: gadget.VolumeUpdate{Edition: 1},
						},
						{
							VolumeName: "pc",
							Name:       "ubuntu-seed",
							Label:      "ubuntu-seed",
							Offset:     createOffsetPointer(1048576),
							MinSize:    1258291200,
							Size:       1258291200,
							Type:       "EF,C12A7328-F81F-11D2-BA4B-00A0C93EC93B",
							Role:       "system-seed",
							Filesystem: "vfat",
							Content:    []gadget.VolumeContent{},
							Update:     gadget.VolumeUpdate{Edition: 2},
							YamlIndex:  1,
						},
						{
							VolumeName: "",
							Name:       "",
							Label:      "writable",
							Offset:     createOffsetPointer(1259339776),
							MinSize:    0,
							Size:       0,
							Type:       "83,0FC63DAF-8483-4772-8E79-3D69D8477DE4",
							Role:       "system-data",
							Filesystem: "ext4",
							Content:    []gadget.VolumeContent{},
							YamlIndex:  2,
						},
					},
					Name: "pc",
				},
			},
		},
		{
			name: "do not add a system-data structure if there is not exactly one",
			gadgetYaml: []byte(`volumes:
  pc:
    bootloader: grub
    structure:
      - name: mbr
        type: mbr
        size: 440
        update:
          edition: 1
        content:
          - image: pc-boot.img
      - name: ubuntu-seed
        role: system-seed
        filesystem: vfat
        # UEFI will boot the ESP partition by default first
        type: EF,C12A7328-F81F-11D2-BA4B-00A0C93EC93B
        size: 1200M
        update:
          edition: 2
  pc2:
    structure:
      - name: mbr
        type: mbr
        size: 440
        update:
          edition: 1
        content:
          - image: pc-boot.img
`),
			volumeOrder:  []string{"pc", "pc2"},
			wantIsSeeded: true,
			wantVolumes: map[string]*gadget.Volume{
				"pc": {
					Schema:     "gpt",
					Bootloader: "grub",
					Structure: []gadget.VolumeStructure{
						{
							VolumeName: "pc",
							Name:       "mbr",
							Offset:     createOffsetPointer(0),
							MinSize:    440,
							Size:       440,
							Type:       "mbr",
							Role:       "mbr",
							Content: []gadget.VolumeContent{
								{
									Image: "pc-boot.img",
								},
							},
							Update: gadget.VolumeUpdate{Edition: 1},
						},
						{
							VolumeName: "pc",
							Name:       "ubuntu-seed",
							Label:      "ubuntu-seed",
							Offset:     createOffsetPointer(1048576),
							MinSize:    1258291200,
							Size:       1258291200,
							Type:       "EF,C12A7328-F81F-11D2-BA4B-00A0C93EC93B",
							Role:       "system-seed",
							Filesystem: "vfat",
							Content:    []gadget.VolumeContent{},
							Update:     gadget.VolumeUpdate{Edition: 2},
							YamlIndex:  1,
						},
					},
					Name: "pc",
				},
				"pc2": {
					Schema:     "gpt",
					Bootloader: "",
					Structure: []gadget.VolumeStructure{
						{
							VolumeName: "pc2",
							Name:       "mbr",
							Offset:     createOffsetPointer(0),
							MinSize:    440,
							Size:       440,
							Type:       "mbr",
							Role:       "mbr",
							Content: []gadget.VolumeContent{
								{
									Image: "pc-boot.img",
								},
							},
							Update: gadget.VolumeUpdate{Edition: 1},
						},
					},
					Name: "pc2",
				},
			},
		},
		{
			name: "error with invalid source path",
			gadgetYaml: []byte(`volumes:
  pc:
    bootloader: grub
    structure:
      - name: ubuntu-seed
        role: system-seed
        filesystem: vfat
        # UEFI will boot the ESP partition by default first
        type: EF,C12A7328-F81F-11D2-BA4B-00A0C93EC93B
        size: 1200M
        update:
          edition: 2
        content:
          - source: ../grubx64.efi
            target: EFI/boot/grubx64.efi
`),
			volumeOrder:  []string{"pc"},
			wantIsSeeded: true,
			expectedErr:  "filesystem content source \"../grubx64.efi\" contains \"../\". This is disallowed for security purposes",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			stateMachine := &StateMachine{
				VolumeOrder: tt.volumeOrder,
			}
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, false)
			t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

			stateMachine.GadgetInfo, err = gadget.InfoFromGadgetYaml(tt.gadgetYaml, nil)
			asserter.AssertErrNil(err, false)

			err = stateMachine.postProcessGadgetYaml()

			if len(tt.expectedErr) == 0 {
				asserter.AssertErrNil(err, true)
				asserter.AssertEqual(tt.wantIsSeeded, stateMachine.IsSeeded)
				asserter.AssertEqual(tt.wantVolumes, stateMachine.GadgetInfo.Volumes, cmpOpts...)
			} else {
				asserter.AssertErrContains(err, tt.expectedErr)
			}
		})
	}
}

// TestStateMachine_postProcessGadgetYaml_fail tests failues in the post processing of
// the gadget.yaml file after loading it in.
func TestStateMachine_postProcessGadgetYaml_fail(t *testing.T) {
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
									YamlIndex: 0,
								},
								{
									Name:    "BIOS Boot",
									Offset:  ptrToOffset(quantity.Offset(quantity.Size(1048576))),
									MinSize: quantity.Size(1048576),
									Size:    quantity.Size(1048576),
									Role:    "",
									Type:    "21686148-6449-6E6F-744E-656564454649",
									Content: nil,
									Update: gadget.VolumeUpdate{
										Edition: 2,
									},
									YamlIndex: 1,
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
									YamlIndex: 0,
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
			name: "simple case with 2 states in dry-run mode",
			fields: fields{
				commonFlags: &commands.CommonOpts{
					Debug:  false,
					DryRun: true,
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
Following states would be executed:
[0] stateFunc1
[1] stateFunc2
`,
		},
		{
			name: "simple case with 2 states in dry-run mode and debug",
			fields: fields{
				commonFlags: &commands.CommonOpts{
					Debug:  true,
					DryRun: true,
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
Following states would be executed:
[0] stateFunc1
[1] stateFunc2
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

// TestMakeTemporaryDirectories tests a successful execution of the
// make_temporary_directories state with and without --workdir
func TestMakeTemporaryDirectories(t *testing.T) {
	testCases := []struct {
		name    string
		workdir string
	}{
		{"with_workdir", "/tmp/make_temporary_directories-" + uuid.NewString()},
		{"without_workdir", ""},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.stateMachineFlags.WorkDir = tc.workdir
			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)

			// make sure workdir was successfully created
			if _, err := os.Stat(stateMachine.stateMachineFlags.WorkDir); err != nil {
				t.Errorf("Failed to create workdir %s",
					stateMachine.stateMachineFlags.WorkDir)
			}
			os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
		})
	}
}

// TestFailedMakeTemporaryDirectories tests some failed executions of the make_temporary_directories state
func TestFailedMakeTemporaryDirectories(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine StateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

	// mock os.Mkdir and test with and without a WorkDir
	osMkdir = mockMkdir
	defer func() {
		osMkdir = os.Mkdir
	}()
	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrContains(err, "Failed to create temporary directory")

	stateMachine.stateMachineFlags.WorkDir = testDir
	err = stateMachine.makeTemporaryDirectories()
	asserter.AssertErrContains(err, "Error creating temporary directory")

	// mock os.MkdirAll and only test with a WorkDir
	osMkdirAll = mockMkdirAll
	defer func() {
		osMkdirAll = os.MkdirAll
	}()
	err = stateMachine.makeTemporaryDirectories()
	if err == nil {
		// try adding a workdir to see if that triggers the failure
		stateMachine.stateMachineFlags.WorkDir = testDir
		err = stateMachine.makeTemporaryDirectories()
		asserter.AssertErrContains(err, "Error creating temporary directory")
	}
	os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
}

// TestDetermineOutputDirectory unit tests the determineOutputDirectory function
func TestDetermineOutputDirectory(t *testing.T) {
	testDir1 := "/tmp/determine_output_dir-" + uuid.NewString()
	testDir2 := "/tmp/determine_output_dir-" + uuid.NewString()
	cwd, _ := os.Getwd() // nolint: errcheck
	testCases := []struct {
		name              string
		workDir           string
		outputDir         string
		expectedOutputDir string
		cleanUp           bool
	}{
		{"no_workdir_no_outputdir", "", "", cwd, false},
		{"yes_workdir_no_outputdir", testDir1, "", testDir1, true},
		{"no_workdir_yes_outputdir", "", testDir1, testDir1, true},
		{"different_workdir_and_outputdir", testDir1, testDir2, testDir2, true},
		{"same_workdir_and_outputdir", testDir1, testDir1, testDir1, true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.stateMachineFlags.WorkDir = tc.workDir
			stateMachine.commonFlags.OutputDir = tc.outputDir

			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)

			err = stateMachine.determineOutputDirectory()
			asserter.AssertErrNil(err, true)
			if tc.cleanUp {
				t.Cleanup(func() { os.RemoveAll(stateMachine.commonFlags.OutputDir) })
			}

			// ensure the correct output dir was set and that it exists
			if stateMachine.commonFlags.OutputDir != tc.expectedOutputDir {
				t.Errorf("OutputDir set in in struct \"%s\" does not match expected value \"%s\"",
					stateMachine.commonFlags.OutputDir, tc.expectedOutputDir)
			}
			if _, err := os.Stat(stateMachine.commonFlags.OutputDir); err != nil {
				t.Errorf("Failed to create output directory %s",
					stateMachine.stateMachineFlags.WorkDir)
			}
		})
	}
}

// TestDetermineOutputDirectory_fail tests failures in the determineOutputDirectory function
func TestDetermineOutputDirectory_fail(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine StateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.commonFlags.OutputDir = "testdir"

	// mock os.MkdirAll
	osMkdirAll = mockMkdirAll
	defer func() {
		osMkdirAll = os.MkdirAll
	}()
	err := stateMachine.determineOutputDirectory()
	asserter.AssertErrContains(err, "Error creating OutputDir")
	osMkdirAll = os.MkdirAll
}
