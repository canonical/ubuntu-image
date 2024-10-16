// This test file tests a successful classic run and success/error scenarios for all states
// that are specific to the classic builds
package statemachine

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/snapcore/snapd/image"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/seed"
	"github.com/snapcore/snapd/store"
	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v2"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/canonical/ubuntu-image/internal/imagedefinition"
	"github.com/canonical/ubuntu-image/internal/testhelper"
)

var yamlMarshal = yaml.Marshal

func TestMain(m *testing.M) {
	basicChroot = NewBasicChroot()
	code := m.Run()
	basicChroot.Clean()
	os.Exit(code)
}

// TestClassicSetup tests a successful run of the polymorphed Setup function
func TestClassicSetup(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.Args.ImageDefinition = filepath.Join("testdata", "image_definitions",
		"test_amd64.yaml")

	err := stateMachine.Setup()
	asserter.AssertErrNil(err, true)
}

// TestYAMLSchemaParsing attempts to parse a variety of image definition files, both
// valid and invalid, and ensures the correct result/errors are returned
func TestYAMLSchemaParsing(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name            string
		imageDefinition string
		shouldPass      bool
		expectedError   string
	}{
		{"valid_image_definition", "test_raspi.yaml", true, ""},
		{"valid_image_definition_no_gadget_no_artifact", "test_image_without_gadget_artifact.yaml", true, ""},
		{"invalid_class", "test_bad_class.yaml", false, "Class must be one of the following"},
		{"invalid_url", "test_bad_url.yaml", false, "Does not match format 'uri'"},
		{"invalid_model_assertion_url", "test_invalid_model_assertion_url.yaml", false, "Does not match format 'uri'"},
		{"invalid_ppa_name", "test_bad_ppa_name.yaml", false, "PPAName: Does not match pattern"},
		{"invalid_ppa_auth", "test_bad_ppa_name.yaml", false, "Auth: Does not match pattern"},
		{"both_seed_and_tasks", "test_both_seed_and_tasks.yaml", false, "Must validate one and only one schema"},
		{"git_gadget_without_url", "test_git_gadget_without_url.yaml", false, "When key gadget:type is specified as git, a URL must be provided"},
		{"file_doesnt_exist", "test_not_exist.yaml", false, "no such file or directory"},
		{"not_valid_yaml", "test_invalid_yaml.yaml", false, "yaml: unmarshal errors"},
		{"missing_yaml_fields", "test_missing_name.yaml", false, "Key \"name\" is required in struct \"ImageDefinition\", but is not in the YAML file!"},
		{"private_ppa_without_fingerprint", "test_private_ppa_without_fingerprint.yaml", false, "Fingerprint is required for private PPAs"},
		{"invalid_paths_in_manual_copy", "test_invalid_paths_in_manual_copy.yaml", false, "needs to be an absolute path (../../malicious)"},
		{"invalid_paths_in_manual_copy_bug", "test_invalid_paths_in_manual_copy.yaml", false, "needs to be an absolute path (/../../malicious)"},
		{"invalid_paths_in_manual_mkdir", "test_invalid_paths_in_manual_mkdir.yaml", false, "needs to be an absolute path (../../malicious)"},
		{"invalid_paths_in_manual_mkdir_bug", "test_invalid_paths_in_manual_mkdir.yaml", false, "needs to be an absolute path (/../../malicious)"},
		{"invalid_paths_in_manual_touch_file", "test_invalid_paths_in_manual_touch_file.yaml", false, "needs to be an absolute path (../../malicious)"},
		{"invalid_paths_in_manual_touch_file_bug", "test_invalid_paths_in_manual_touch_file.yaml", false, "needs to be an absolute path (/../../malicious)"},
		{"img_specified_without_gadget", "test_image_without_gadget.yaml", false, "Key img cannot be used without key gadget:"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			restoreCWD := testhelper.SaveCWD()
			defer restoreCWD()

			var stateMachine ClassicStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine
			stateMachine.Args.ImageDefinition = filepath.Join("testdata", "image_definitions",
				tc.imageDefinition)
			err := stateMachine.parseImageDefinition()

			if tc.shouldPass {
				asserter.AssertErrNil(err, false)
			} else {
				asserter.AssertErrContains(err, tc.expectedError)
			}
		})
	}
}

// TestFailedParseImageDefinition mocks function calls to test
// failure cases in the parseImageDefinition state
func TestFailedParseImageDefinition(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.Args.ImageDefinition = filepath.Join("testdata", "image_definitions",
		"test_raspi.yaml")

	// mock helper.SetDefaults
	helperSetDefaults = mockSetDefaults
	t.Cleanup(func() {
		helperSetDefaults = helper.SetDefaults
	})
	err := stateMachine.parseImageDefinition()
	asserter.AssertErrContains(err, "Test Error")
	helperSetDefaults = helper.SetDefaults

	// mock helper.CheckEmptyFields
	helperCheckEmptyFields = mockCheckEmptyFields
	t.Cleanup(func() {
		helperCheckEmptyFields = helper.CheckEmptyFields
	})
	err = stateMachine.parseImageDefinition()
	asserter.AssertErrContains(err, "Test Error")
	helperCheckEmptyFields = helper.CheckEmptyFields

	// mock gojsonschema.Validate
	gojsonschemaValidate = mockGojsonschemaValidateError
	t.Cleanup(func() {
		gojsonschemaValidate = gojsonschema.Validate
	})
	err = stateMachine.parseImageDefinition()
	asserter.AssertErrContains(err, "Schema validation returned an error")
	gojsonschemaValidate = gojsonschema.Validate

	// mock helper.CheckTags
	// the gadget must be set to nil for this test to work
	stateMachine.Args.ImageDefinition = filepath.Join("testdata", "image_definitions",
		"test_image_without_gadget.yaml")
	helperCheckTags = mockCheckTags
	t.Cleanup(func() {
		helperCheckTags = helper.CheckTags
	})
	err = stateMachine.parseImageDefinition()
	asserter.AssertErrContains(err, "Test Error")
	helperCheckTags = helper.CheckTags
}

// TestClassicStateMachine_calculateStates reads in a variety of yaml files and ensures
// that the correct states are added to the state machine
// TODO: manually assemble the image definitions instead of relying on the parseImageDefinition() function to make this more of a unit test
func TestClassicStateMachine_calculateStates(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name            string
		imageDefinition string
		expectedStates  []string
	}{
		{
			name:            "state_build_gadget",
			imageDefinition: "test_build_gadget.yaml",
			expectedStates: []string{
				"build_gadget_tree",
				"prepare_gadget_tree",
				"load_gadget_yaml",
				"verify_artifact_names",
				"germinate",
				"create_chroot",
				"install_packages",
				"prepare_image",
				"preseed_image",
				"clean_rootfs",
				"customize_sources_list",
				"customize_cloud_init",
				"set_default_locale",
				"populate_rootfs_contents",
				"calculate_rootfs_size",
				"populate_bootfs_contents",
				"populate_prepare_partitions",
				"make_disk",
				"update_bootloader",
				"generate_package_manifest",
			},
		},
		{
			name:            "state_prebuilt_gadget",
			imageDefinition: "test_prebuilt_gadget.yaml",
			expectedStates: []string{
				"build_gadget_tree",
				"prepare_gadget_tree",
				"load_gadget_yaml",
				"verify_artifact_names",
				"germinate",
				"create_chroot",
				"install_packages",
				"prepare_image",
				"preseed_image",
				"clean_rootfs",
				"customize_sources_list",
				"customize_cloud_init",
				"set_default_locale",
				"populate_rootfs_contents",
				"calculate_rootfs_size",
				"populate_bootfs_contents",
				"populate_prepare_partitions",
				"make_disk",
				"update_bootloader",
				"generate_package_manifest",
			},
		},
		{
			name:            "state_prebuilt_rootfs_extras",
			imageDefinition: "test_prebuilt_rootfs_extras.yaml",
			expectedStates: []string{
				"build_gadget_tree",
				"prepare_gadget_tree",
				"load_gadget_yaml",
				"verify_artifact_names",
				"extract_rootfs_tar",
				"add_extra_ppas",
				"install_packages",
				"clean_extra_ppas",
				"prepare_image",
				"preseed_image",
				"clean_rootfs",
				"customize_sources_list",
				"customize_cloud_init",
				"set_default_locale",
				"populate_rootfs_contents",
				"calculate_rootfs_size",
				"populate_bootfs_contents",
				"populate_prepare_partitions",
				"make_disk",
				"update_bootloader",
				"generate_package_manifest",
			},
		},
		{
			name:            "state_ppa",
			imageDefinition: "test_amd64.yaml",
			expectedStates: []string{
				"build_gadget_tree",
				"prepare_gadget_tree",
				"load_gadget_yaml",
				"verify_artifact_names",
				"germinate",
				"create_chroot",
				"add_extra_ppas",
				"install_packages",
				"clean_extra_ppas",
				"prepare_image",
				"preseed_image",
				"clean_rootfs",
				"customize_sources_list",
				"customize_cloud_init",
				"perform_manual_customization",
				"set_default_locale",
				"populate_rootfs_contents",
				"calculate_rootfs_size",
				"populate_bootfs_contents",
				"populate_prepare_partitions",
				"make_disk",
				"update_bootloader",
				"make_qcow2_image",
				"generate_package_manifest",
				"generate_filelist",
			},
		},
		{
			name:            "extract_rootfs_tar",
			imageDefinition: "test_extract_rootfs_tar.yaml",
			expectedStates: []string{
				"build_gadget_tree",
				"prepare_gadget_tree",
				"load_gadget_yaml",
				"verify_artifact_names",
				"extract_rootfs_tar",
				"install_packages",
				"clean_rootfs",
				"customize_sources_list",
				"customize_cloud_init",
				"set_default_locale",
				"populate_rootfs_contents",
				"calculate_rootfs_size",
				"populate_bootfs_contents",
				"populate_prepare_partitions",
				"make_disk",
				"update_bootloader",
				"generate_package_manifest",
			},
		},
		{
			name:            "extract_rootfs_tar_no_customization",
			imageDefinition: "test_extract_rootfs_tar_no_customization.yaml",
			expectedStates: []string{
				"build_gadget_tree",
				"prepare_gadget_tree",
				"load_gadget_yaml",
				"verify_artifact_names",
				"extract_rootfs_tar",
				"clean_rootfs",
				"customize_sources_list",
				"set_default_locale",
				"populate_rootfs_contents",
				"calculate_rootfs_size",
				"populate_bootfs_contents",
				"populate_prepare_partitions",
				"make_disk",
				"update_bootloader",
				"generate_package_manifest",
			},
		},
		{
			name:            "build_rootfs_from_seed",
			imageDefinition: "test_rootfs_seed.yaml",
			expectedStates: []string{
				"build_gadget_tree",
				"prepare_gadget_tree",
				"load_gadget_yaml",
				"verify_artifact_names",
				"germinate",
				"create_chroot",
				"install_packages",
				"prepare_image",
				"preseed_image",
				"clean_rootfs",
				"customize_sources_list",
				"customize_cloud_init",
				"set_default_locale",
				"populate_rootfs_contents",
				"calculate_rootfs_size",
				"populate_bootfs_contents",
				"populate_prepare_partitions",
				"make_disk",
				"update_bootloader",
				"generate_package_manifest",
			},
		},
		{
			name:            "build_rootfs_from_tasks",
			imageDefinition: "test_rootfs_tasks.yaml",
			expectedStates: []string{
				"build_gadget_tree",
				"prepare_gadget_tree",
				"load_gadget_yaml",
				"verify_artifact_names",
				"build_rootfs_from_tasks",
				"clean_rootfs",
				"customize_sources_list",
				"customize_cloud_init",
				"set_default_locale",
				"populate_rootfs_contents",
				"calculate_rootfs_size",
				"populate_bootfs_contents",
				"populate_prepare_partitions",
				"make_disk",
				"update_bootloader",
				"generate_package_manifest",
			},
		},
		{
			name:            "customization_states",
			imageDefinition: "test_customization.yaml",
			expectedStates: []string{
				"build_gadget_tree",
				"prepare_gadget_tree",
				"load_gadget_yaml",
				"verify_artifact_names",
				"germinate",
				"create_chroot",
				"add_extra_ppas",
				"install_packages",
				"clean_extra_ppas",
				"prepare_image",
				"preseed_image",
				"clean_rootfs",
				"customize_sources_list",
				"customize_cloud_init",
				"perform_manual_customization",
				"set_default_locale",
				"populate_rootfs_contents",
				"calculate_rootfs_size",
				"populate_bootfs_contents",
				"populate_prepare_partitions",
				"make_disk",
				"update_bootloader",
				"generate_package_manifest",
			},
		},
		{
			name:            "qcow2",
			imageDefinition: "test_qcow2.yaml",
			expectedStates: []string{
				"build_gadget_tree",
				"prepare_gadget_tree",
				"load_gadget_yaml",
				"verify_artifact_names",
				"germinate",
				"create_chroot",
				"add_extra_ppas",
				"install_packages",
				"clean_extra_ppas",
				"prepare_image",
				"preseed_image",
				"clean_rootfs",
				"customize_sources_list",
				"customize_cloud_init",
				"set_default_locale",
				"populate_rootfs_contents",
				"calculate_rootfs_size",
				"populate_bootfs_contents",
				"populate_prepare_partitions",
				"make_disk",
				"update_bootloader",
				"make_qcow2_image",
			},
		},
		{
			name:            "no artifact",
			imageDefinition: "test_no_artifact.yaml",
			expectedStates: []string{
				"build_gadget_tree",
				"prepare_gadget_tree",
				"load_gadget_yaml",
				"germinate",
				"create_chroot",
				"add_extra_ppas",
				"install_packages",
				"clean_extra_ppas",
				"prepare_image",
				"preseed_image",
				"clean_rootfs",
				"customize_sources_list",
				"customize_cloud_init",
				"perform_manual_customization",
				"set_default_locale",
				"populate_rootfs_contents",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			restoreCWD := testhelper.SaveCWD()
			defer restoreCWD()

			var stateMachine ClassicStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine
			stateMachine.Args.ImageDefinition = filepath.Join("testdata", "image_definitions", tc.imageDefinition)
			err := stateMachine.parseImageDefinition()
			asserter.AssertErrNil(err, true)

			err = stateMachine.calculateStates()
			asserter.AssertErrNil(err, true)

			stateNames := make([]string, 0)
			for _, f := range stateMachine.states {
				stateNames = append(stateNames, f.name)
			}

			asserter.AssertEqual(tc.expectedStates, stateNames)
		})
	}
}

// TestFailedCalculateStates tests failure scenarios in the
// calculateStates function
func TestFailedCalculateStates(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Gadget: &imagedefinition.Gadget{
			GadgetType: "git",
		},
		Rootfs: &imagedefinition.Rootfs{
			ArchiveTasks: []string{"test"},
		},
		Customization: &imagedefinition.Customization{},
		Artifacts:     &imagedefinition.Artifact{},
	}

	// mock helper.CheckTags
	// the gadget must be set to nil for this test to work
	helperCheckTags = mockCheckTags
	t.Cleanup(func() {
		helperCheckTags = helper.CheckTags
	})
	err := stateMachine.calculateStates()
	asserter.AssertErrContains(err, "Test Error")
}

// TestDisplayStates ensures the states are printed to stdout when the --debug flag is set
func TestDisplayStates(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.commonFlags.Debug = true
	stateMachine.commonFlags.DiskInfo = "test" // for coverage!
	stateMachine.Args.ImageDefinition = filepath.Join("testdata", "image_definitions", "test_raspi.yaml")
	err := stateMachine.parseImageDefinition()
	asserter.AssertErrNil(err, true)

	// capture stdout, calculate the states, and ensure they were printed
	stdout, restoreStdout, err := helper.CaptureStd(&os.Stdout)
	defer restoreStdout()
	asserter.AssertErrNil(err, true)

	err = stateMachine.calculateStates()
	asserter.AssertErrNil(err, true)

	stateMachine.displayStates()
	asserter.AssertErrNil(err, true)

	// restore stdout and examine what was printed
	restoreStdout()
	readStdout, err := io.ReadAll(stdout)
	asserter.AssertErrNil(err, true)

	expectedStates := `Following states will be executed:
[0] build_gadget_tree
[1] prepare_gadget_tree
[2] load_gadget_yaml
[3] verify_artifact_names
[4] germinate
[5] create_chroot
[6] install_packages
[7] prepare_image
[8] preseed_image
[9] clean_rootfs
[10] customize_sources_list
[11] customize_fstab
[12] perform_manual_customization
[13] set_default_locale
[14] populate_rootfs_contents
[15] generate_disk_info
[16] calculate_rootfs_size
[17] populate_bootfs_contents
[18] populate_prepare_partitions
[19] make_disk
[20] update_bootloader
[21] generate_package_manifest
`
	if !strings.Contains(string(readStdout), expectedStates) {
		t.Errorf("Expected states to be printed in output:\n\"%s\"\n but got \n\"%s\"\n instead",
			expectedStates, string(readStdout))
	}
}

// TestClassicStateMachine_Setup_Fail_setConfDefDir tests a failure in the Setup() function when setting the configuration definition directory
func TestClassicStateMachine_Setup_Fail_setConfDefDir(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

	tmpDirPath := filepath.Join("/tmp", "test_failed_set_conf_dir")
	err := os.Mkdir(tmpDirPath, 0755)
	t.Cleanup(func() {
		os.RemoveAll(tmpDirPath)
	})
	asserter.AssertErrNil(err, true)

	err = os.Chdir(tmpDirPath)
	asserter.AssertErrNil(err, true)

	_ = os.RemoveAll(tmpDirPath)

	err = stateMachine.Setup()
	asserter.AssertErrContains(err, "unable to determine the configuration definition directory")
	os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
}

// TestFailedValidateInputClassic tests a failure in the Setup() function when validating common input
func TestFailedValidateInputClassic(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	// use both --until and --thru to trigger this failure
	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.stateMachineFlags.Until = "until-test"
	stateMachine.stateMachineFlags.Thru = "thru-test"

	err := stateMachine.Setup()
	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })
	asserter.AssertErrContains(err, "cannot specify both --until and --thru")
}

// TestFailedReadMetadataClassic tests a failed metadata read by passing --resume with no previous partial state machine run
func TestFailedReadMetadataClassic(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	// start a --resume with no previous SM run
	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.stateMachineFlags.Resume = true
	stateMachine.stateMachineFlags.WorkDir = testDir
	stateMachine.Args.ImageDefinition = filepath.Join("testdata", "image_definitions",
		"test_amd64.yaml")

	err := stateMachine.Setup()
	asserter.AssertErrContains(err, "error reading metadata file")
	os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
}

// TestClassicStateMachine_Setup_Fail_makeTemporaryDirectories tests the Setup function
// with makeTemporaryDirectories failing
func TestClassicStateMachine_Setup_Fail_makeTemporaryDirectories(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.stateMachineFlags.WorkDir = testDir
	stateMachine.Args.ImageDefinition = filepath.Join("testdata", "image_definitions",
		"test_amd64.yaml")

	// mock os.MkdirAll
	osMkdirAll = mockMkdirAll
	t.Cleanup(func() {
		osMkdirAll = os.MkdirAll
	})
	err := stateMachine.Setup()
	asserter.AssertErrContains(err, "Error creating work directory")
}

// TestClassicStateMachine_Setup_Fail_determineOutputDirectory tests the Setup function
// with determineOutputDirectory failing
func TestClassicStateMachine_Setup_Fail_determineOutputDirectory(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.Args.ImageDefinition = filepath.Join("testdata", "image_definitions",
		"test_amd64.yaml")
	stateMachine.commonFlags.OutputDir = "/tmp/test"

	// mock os.MkdirAll
	osMkdirAll = mockMkdirAll
	t.Cleanup(func() {
		osMkdirAll = os.MkdirAll
	})
	err := stateMachine.Setup()
	asserter.AssertErrContains(err, "Error creating OutputDir")
}

// TestClassicStateMachine_DryRun tests a successful dry-run execution
func TestClassicStateMachine_DryRun(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	workDir := "ubuntu-image-test-dry-run"
	err := os.Mkdir(workDir, 0755)
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(workDir) })

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.Args.ImageDefinition = filepath.Join("testdata", "image_definitions",
		"test_amd64.yaml")
	stateMachine.stateMachineFlags.WorkDir = workDir
	stateMachine.commonFlags.DryRun = true

	err = stateMachine.Setup()
	asserter.AssertErrNil(err, true)

	files, err := osReadDir(workDir)
	asserter.AssertErrNil(err, true)

	if len(files) != 0 {
		t.Errorf("Some files were created in the workdir but should not. Created files: %s", files)
	}

	err = stateMachine.Run()
	asserter.AssertErrNil(err, true)

	err = stateMachine.Teardown()
	asserter.AssertErrNil(err, true)
}

// TestPrepareGadgetTree runs prepareGadgetTree() and ensures the gadget_tree files
// are placed in the correct locations
func TestPrepareGadgetTree(t *testing.T) {
	t.Parallel()
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Gadget:       &imagedefinition.Gadget{},
	}

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	// place a test gadget tree in the  scratch directory so we don't have to build one
	gadgetDir := filepath.Join(stateMachine.tempDirs.scratch, "gadget")
	err = os.MkdirAll(gadgetDir, 0755)
	asserter.AssertErrNil(err, true)

	gadgetSource := filepath.Join("testdata", "gadget_tree")
	err = osutil.CopySpecialFile(gadgetSource, filepath.Join(gadgetDir, "install"))
	asserter.AssertErrNil(err, true)

	err = stateMachine.prepareGadgetTree()
	asserter.AssertErrNil(err, true)

	gadgetTreeFiles := []string{"grub.conf", "pc-boot.img", "meta/gadget.yaml"}
	for _, file := range gadgetTreeFiles {
		_, err := os.Stat(filepath.Join(stateMachine.tempDirs.unpack, "gadget", file))
		if err != nil {
			t.Errorf("File %s should be in unpack, but is missing", file)
		}
	}
}

// TestPrepareGadgetTreePrebuilt tests the prepareGadgetTree function with prebuilt gadgets
func TestPrepareGadgetTreePrebuilt(t *testing.T) {
	t.Parallel()
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Gadget: &imagedefinition.Gadget{
			GadgetType: "prebuilt",
			GadgetURL:  "testdata/gadget_tree/",
		},
	}

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	err = stateMachine.prepareGadgetTree()
	asserter.AssertErrNil(err, true)

	gadgetTreeFiles := []string{"grub.conf", "pc-boot.img", "meta/gadget.yaml"}
	for _, file := range gadgetTreeFiles {
		_, err := os.Stat(filepath.Join(stateMachine.tempDirs.unpack, "gadget", file))
		if err != nil {
			t.Errorf("File %s should be in unpack, but is missing", file)
		}
	}
}

// TestFailedPrepareGadgetTree tests failures in the prepareGadgetTree function
func TestFailedPrepareGadgetTree(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Gadget:       &imagedefinition.Gadget{},
	}

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	// place a test gadget tree in the  scratch directory so we don't have to build one
	gadgetDir := filepath.Join(stateMachine.tempDirs.scratch, "gadget")
	err = os.MkdirAll(gadgetDir, 0755)
	asserter.AssertErrNil(err, true)

	gadgetSource := filepath.Join("testdata", "gadget_tree")
	err = osutil.CopySpecialFile(gadgetSource, filepath.Join(gadgetDir, "install"))
	asserter.AssertErrNil(err, true)

	// mock os.Mkdir
	osMkdirAll = mockMkdirAll
	t.Cleanup(func() {
		osMkdirAll = os.MkdirAll
	})
	err = stateMachine.prepareGadgetTree()
	asserter.AssertErrContains(err, "Error creating unpack directory")
	osMkdirAll = os.MkdirAll

	// mock os.ReadDir
	osReadDir = mockReadDir
	t.Cleanup(func() {
		osReadDir = os.ReadDir
	})
	err = stateMachine.prepareGadgetTree()
	asserter.AssertErrContains(err, "Error reading gadget tree")
	osReadDir = os.ReadDir

	// mock osutil.CopySpecialFile
	osutilCopySpecialFile = mockCopySpecialFile
	t.Cleanup(func() {
		osutilCopySpecialFile = osutil.CopySpecialFile
	})
	err = stateMachine.prepareGadgetTree()
	asserter.AssertErrContains(err, "Error copying gadget tree")
	osutilCopySpecialFile = osutil.CopySpecialFile
}

// TestVerifyArtifactNames unit tests the verifyArtifactNames function
func TestVerifyArtifactNames(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name             string
		gadgetYAML       string
		artifacts        *imagedefinition.Artifact
		img              *[]imagedefinition.Img
		qcow2            *[]imagedefinition.Qcow2
		expectedVolNames map[string]string
		shouldPass       bool
	}{
		{
			name:             "no artifact ",
			gadgetYAML:       "gadget_tree/meta/gadget.yaml",
			artifacts:        nil,
			expectedVolNames: nil,
			shouldPass:       true,
		},
		{
			name:       "single_volume_specified",
			gadgetYAML: "gadget_tree/meta/gadget.yaml",
			artifacts: &imagedefinition.Artifact{
				Img: &[]imagedefinition.Img{
					{
						ImgName:   "test1.img",
						ImgVolume: "pc",
					},
				},
			},
			expectedVolNames: map[string]string{
				"pc": "test1.img",
			},
			shouldPass: true,
		},
		{
			name:       "single_volume_not_specified",
			gadgetYAML: "gadget_tree/meta/gadget.yaml",
			artifacts: &imagedefinition.Artifact{
				Img: &[]imagedefinition.Img{
					{
						ImgName: "test-single.img",
					},
				},
			},
			expectedVolNames: map[string]string{
				"pc": "test-single.img",
			},
			shouldPass: true,
		},
		{
			name:       "mutli_volume_specified",
			gadgetYAML: "gadget-multi.yaml",
			artifacts: &imagedefinition.Artifact{
				Img: &[]imagedefinition.Img{
					{
						ImgName:   "test1.img",
						ImgVolume: "first",
					},
					{
						ImgName:   "test2.img",
						ImgVolume: "second",
					},
					{
						ImgName:   "test3.img",
						ImgVolume: "third",
					},
					{
						ImgName:   "test4.img",
						ImgVolume: "fourth",
					},
				},
			},
			expectedVolNames: map[string]string{
				"first":  "test1.img",
				"second": "test2.img",
				"third":  "test3.img",
				"fourth": "test4.img",
			},
			shouldPass: true,
		},
		{
			name:       "mutli_volume_not_specified",
			gadgetYAML: "gadget-multi.yaml",
			artifacts: &imagedefinition.Artifact{
				Img: &[]imagedefinition.Img{
					{
						ImgName: "test1.img",
					},
					{
						ImgName: "test2.img",
					},
					{
						ImgName: "test3.img",
					},
					{
						ImgName: "test4.img",
					},
				},
			},
			expectedVolNames: map[string]string{},
			shouldPass:       false,
		},
		{
			name:       "mutli_volume_some_specified",
			gadgetYAML: "gadget-multi.yaml",
			artifacts: &imagedefinition.Artifact{
				Img: &[]imagedefinition.Img{
					{
						ImgName:   "test1.img",
						ImgVolume: "first",
					},
					{
						ImgName:   "test2.img",
						ImgVolume: "second",
					},
					{
						ImgName: "test3.img",
					},
					{
						ImgName: "test4.img",
					},
				},
			},
			expectedVolNames: map[string]string{},
			shouldPass:       false,
		},
		{
			name:       "mutli_volume_only_create_some_images",
			gadgetYAML: "gadget-multi.yaml",
			artifacts: &imagedefinition.Artifact{
				Img: &[]imagedefinition.Img{
					{
						ImgName:   "test1.img",
						ImgVolume: "first",
					},
					{
						ImgName:   "test2.img",
						ImgVolume: "second",
					},
				},
			},
			expectedVolNames: map[string]string{
				"first":  "test1.img",
				"second": "test2.img",
			},
			shouldPass: true,
		},
		{
			name:       "qcow2_single_volume_no_img",
			gadgetYAML: "gadget_tree/meta/gadget.yaml",
			artifacts: &imagedefinition.Artifact{
				Qcow2: &[]imagedefinition.Qcow2{
					{
						Qcow2Name:   "test1.qcow2",
						Qcow2Volume: "pc",
					},
				},
			},
			expectedVolNames: map[string]string{
				"pc": "test1.qcow2.img",
			},
			shouldPass: true,
		},
		{
			name:       "qcow2_single_volume_not_specified_no_img",
			gadgetYAML: "gadget_tree/meta/gadget.yaml",
			artifacts: &imagedefinition.Artifact{
				Qcow2: &[]imagedefinition.Qcow2{
					{
						Qcow2Name: "test1.qcow2",
					},
				},
			},
			expectedVolNames: map[string]string{
				"pc": "test1.qcow2.img",
			},
			shouldPass: true,
		},
		{
			name:       "qcow2_single_volume_yes_img",
			gadgetYAML: "gadget_tree/meta/gadget.yaml",
			artifacts: &imagedefinition.Artifact{
				Img: &[]imagedefinition.Img{
					{
						ImgName:   "test1.img",
						ImgVolume: "pc",
					},
				},
				Qcow2: &[]imagedefinition.Qcow2{
					{
						Qcow2Name:   "test1.img",
						Qcow2Volume: "pc",
					},
				},
			},
			expectedVolNames: map[string]string{
				"pc": "test1.img",
			},
			shouldPass: true,
		},
		{
			name:       "qcow2_mutli_volume_not_specified",
			gadgetYAML: "gadget-multi.yaml",
			artifacts: &imagedefinition.Artifact{
				Qcow2: &[]imagedefinition.Qcow2{
					{
						Qcow2Name: "test1.img",
					},
					{
						Qcow2Name: "test2.img",
					},
					{
						Qcow2Name: "test3.img",
					},
					{
						Qcow2Name: "test4.img",
					},
				},
			},
			expectedVolNames: map[string]string{},
			shouldPass:       false,
		},
		{
			name:       "qcow2_mutli_volume_no_img",
			gadgetYAML: "gadget-multi.yaml",
			artifacts: &imagedefinition.Artifact{
				Qcow2: &[]imagedefinition.Qcow2{
					{
						Qcow2Name:   "test1.qcow2",
						Qcow2Volume: "first",
					},
					{
						Qcow2Name:   "test2.qcow2",
						Qcow2Volume: "second",
					},
					{
						Qcow2Name:   "test3.qcow2",
						Qcow2Volume: "third",
					},
					{
						Qcow2Name:   "test4.qcow2",
						Qcow2Volume: "fourth",
					},
				},
			},
			expectedVolNames: map[string]string{
				"first":  "test1.qcow2.img",
				"second": "test2.qcow2.img",
				"third":  "test3.qcow2.img",
				"fourth": "test4.qcow2.img",
			},
			shouldPass: true,
		},
		{
			name:       "qcow2_mutli_volume_yes_img",
			gadgetYAML: "gadget-multi.yaml",
			artifacts: &imagedefinition.Artifact{
				Img: &[]imagedefinition.Img{
					{
						ImgName:   "test1.img",
						ImgVolume: "first",
					},
					{
						ImgName:   "test2.img",
						ImgVolume: "second",
					},
					{
						ImgName:   "test3.img",
						ImgVolume: "third",
					},
					{
						ImgName:   "test4.img",
						ImgVolume: "fourth",
					},
				},
				Qcow2: &[]imagedefinition.Qcow2{
					{
						Qcow2Name:   "test1.img",
						Qcow2Volume: "first",
					},
					{
						Qcow2Name:   "test2.img",
						Qcow2Volume: "second",
					},
					{
						Qcow2Name:   "test3.img",
						Qcow2Volume: "third",
					},
					{
						Qcow2Name:   "test4.img",
						Qcow2Volume: "fourth",
					},
				},
			},
			expectedVolNames: map[string]string{
				"first":  "test1.img",
				"second": "test2.img",
				"third":  "test3.img",
				"fourth": "test4.img",
			},
			shouldPass: true,
		},
		{
			name:       "qcow2_mutli_volume_img_for_different_volume",
			gadgetYAML: "gadget-multi.yaml",
			artifacts: &imagedefinition.Artifact{
				Img: &[]imagedefinition.Img{
					{
						ImgName:   "test1.img",
						ImgVolume: "first",
					},
					{
						ImgName:   "test2.img",
						ImgVolume: "second",
					},
				},
				Qcow2: &[]imagedefinition.Qcow2{
					{
						Qcow2Name:   "test3.qcow2",
						Qcow2Volume: "third",
					},
					{
						Qcow2Name:   "test4.qcow2",
						Qcow2Volume: "fourth",
					},
				},
			},
			expectedVolNames: map[string]string{
				"first":  "test1.img",
				"second": "test2.img",
				"third":  "test3.qcow2.img",
				"fourth": "test4.qcow2.img",
			},
			shouldPass: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			restoreCWD := testhelper.SaveCWD()
			defer restoreCWD()

			var stateMachine ClassicStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine

			stateMachine.YamlFilePath = filepath.Join("testdata", tc.gadgetYAML)
			stateMachine.ImageDef = imagedefinition.ImageDefinition{
				Architecture: getHostArch(),
				Series:       getHostSuite(),
				Rootfs: &imagedefinition.Rootfs{
					Archive: "ubuntu",
				},
				Customization: &imagedefinition.Customization{},
				Artifacts:     tc.artifacts,
			}

			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)
			t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

			// load gadget yaml
			err = stateMachine.loadGadgetYaml()
			asserter.AssertErrNil(err, true)

			// verify artifact names
			err = stateMachine.verifyArtifactNames()
			if tc.shouldPass {
				asserter.AssertErrNil(err, true)
				if !reflect.DeepEqual(tc.expectedVolNames, stateMachine.VolumeNames) {
					fmt.Println(tc.expectedVolNames)
					fmt.Println(stateMachine.VolumeNames)
					t.Errorf("Expected volume names does not match calculated volume names")
				}
			} else {
				asserter.AssertErrContains(err, "Volume names must be specified for each image")
			}
		})
	}
}

// TestBuildRootfsFromTasks unit tests the buildRootfsFromTasks function
func TestBuildRootfsFromTasks(t *testing.T) {
	t.Parallel()
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

	err := stateMachine.buildRootfsFromTasks()
	asserter.AssertErrNil(err, true)

	os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
}

// TestExtractRootfsTar unit tests the extractRootfsTar function
func TestExtractRootfsTar(t *testing.T) {
	t.Parallel()
	wd, _ := os.Getwd() // nolint: errcheck
	testCases := []struct {
		name          string
		rootfsTar     string
		SHA256sum     string
		expectedFiles []string
	}{
		{
			name:      "vanilla_tar",
			rootfsTar: filepath.Join("testdata", "rootfs_tarballs", "rootfs.tar"),
			SHA256sum: "ec01fd8488b0f35d2ca69e6f82edfaecef5725da70913bab61240419ce574918",
			expectedFiles: []string{
				"test_tar1",
				"test_tar2",
			},
		},
		{
			name:      "vanilla_tar respecting absolute path",
			rootfsTar: filepath.Join(wd, "testdata", "rootfs_tarballs", "rootfs.tar"),
			SHA256sum: "ec01fd8488b0f35d2ca69e6f82edfaecef5725da70913bab61240419ce574918",
			expectedFiles: []string{
				"test_tar1",
				"test_tar2",
			},
		},
		{
			name:      "vanilla_tar relative path even with dot dot",
			rootfsTar: filepath.Join("testdata", "../..", filepath.Base(wd), "testdata", "rootfs_tarballs", "rootfs.tar"),
			SHA256sum: "ec01fd8488b0f35d2ca69e6f82edfaecef5725da70913bab61240419ce574918",
			expectedFiles: []string{
				"test_tar1",
				"test_tar2",
			},
		},
		{
			name:      "gz",
			rootfsTar: filepath.Join("testdata", "rootfs_tarballs", "rootfs.tar.gz"),
			SHA256sum: "29152fd9cadbc92f174815ec642ab3aea98f08f902a4f317ec037f8fe60e40c3",
			expectedFiles: []string{
				"test_tar_gz1",
				"test_tar_gz2",
			},
		},
		{
			name:      "xz",
			rootfsTar: filepath.Join("testdata", "rootfs_tarballs", "rootfs.tar.xz"),
			SHA256sum: "e3708f1d98ccea0e0c36843d9576580505ee36d523bfcf78b0f73a035ae9a14e",
			expectedFiles: []string{
				"test_tar_xz1",
				"test_tar_xz2",
			},
		},
		{
			name:      "bz2",
			rootfsTar: filepath.Join("testdata", "rootfs_tarballs", "rootfs.tar.bz2"),
			SHA256sum: "a1180a73b652d85d7330ef21d433b095363664f2f808363e67f798fae15abf0c",
			expectedFiles: []string{
				"test_tar_bz1",
				"test_tar_bz2",
			},
		},
		{
			name:      "zst",
			rootfsTar: filepath.Join("testdata", "rootfs_tarballs", "rootfs.tar.zst"),
			SHA256sum: "5fb00513f84e28225a3155fd78c59a6a923b222e1c125aab35bbfd4091281829",
			expectedFiles: []string{
				"test_tar_zstd1",
				"test_tar_zstd2",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			restoreCWD := testhelper.SaveCWD()
			defer restoreCWD()

			var stateMachine ClassicStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine
			stateMachine.ImageDef = imagedefinition.ImageDefinition{
				Architecture: getHostArch(),
				Series:       getHostSuite(),
				Rootfs: &imagedefinition.Rootfs{
					Tarball: &imagedefinition.Tarball{
						TarballURL: fmt.Sprintf("file://%s", tc.rootfsTar),
					},
				},
			}

			err := stateMachine.setConfDefDir(filepath.Join(wd, "image_definition.yaml"))
			asserter.AssertErrNil(err, true)

			err = stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)

			t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

			err = stateMachine.extractRootfsTar()
			asserter.AssertErrNil(err, true)

			for _, testFile := range tc.expectedFiles {
				_, err := os.Stat(filepath.Join(stateMachine.tempDirs.chroot, testFile))
				if err != nil {
					t.Errorf("File %s should be in chroot, but is missing", testFile)
				}
			}
		})
	}
}

// TestFailedExtractRootfsTar tests failures in the extractRootfsTar function
func TestFailedExtractRootfsTar(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	tarPath := filepath.Join("testdata", "rootfs_tarballs", "rootfs.tar")
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Rootfs: &imagedefinition.Rootfs{
			Tarball: &imagedefinition.Tarball{
				TarballURL: fmt.Sprintf("file://%s", tarPath),
				SHA256sum:  "fail",
			},
		},
	}

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	// mock os.Mkdir
	osMkdir = mockMkdir
	t.Cleanup(func() {
		osMkdir = os.Mkdir
	})
	err = stateMachine.extractRootfsTar()
	asserter.AssertErrContains(err, "Failed to create chroot directory")
	osMkdir = os.Mkdir

	// clean up chroot directory
	os.RemoveAll(stateMachine.tempDirs.chroot)

	// now test with the incorrect SHA256sum
	err = stateMachine.extractRootfsTar()
	asserter.AssertErrContains(err, "Calculated SHA256 sum of rootfs tarball")

	// clean up chroot directory
	os.RemoveAll(stateMachine.tempDirs.chroot)

	// use a tarball that doesn't exist to trigger a failure in computing
	// the SHA256 sum
	stateMachine.ImageDef.Rootfs.Tarball.TarballURL = "file:///fakefile"
	err = stateMachine.extractRootfsTar()
	asserter.AssertErrContains(err, "Error opening file \"/fakefile\" to calculate SHA256 sum")
	os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
}

// TestStateMachine_customizeCloudInit unit tests the customizeCloudInit method
func TestStateMachine_customizeCloudInit(t *testing.T) {
	testCases := []struct {
		name                   string
		cloudInitCustomization imagedefinition.CloudInit
		wantMetaData           string
		wantUserData           string
		wantNetworkConfig      string
	}{
		{
			name: "full cloudinit conf",
			cloudInitCustomization: imagedefinition.CloudInit{
				MetaData: `#cloud-config

foo: bar`,
				UserData: `#cloud-config

foo: baz`,
				NetworkConfig: `#cloud-config

foobar: foobar`,
			},
			wantMetaData: `#cloud-config

foo: bar`,
			wantUserData: `#cloud-config

foo: baz`,
			wantNetworkConfig: `#cloud-config

foobar: foobar`,
		},
		{
			name: "empty user data",
			cloudInitCustomization: imagedefinition.CloudInit{
				MetaData: `#cloud-config

foo: bar`,
				UserData: "",
				NetworkConfig: `#cloud-config

foobar: foobar`,
			},
			wantMetaData: `#cloud-config

foo: bar`,
			wantUserData: "",
			wantNetworkConfig: `#cloud-config

foobar: foobar`,
		},
		{
			name: "empty metadata",
			cloudInitCustomization: imagedefinition.CloudInit{
				UserData: "",
				NetworkConfig: `#cloud-config

foobar: foobar`,
			},
			wantMetaData: "",
			wantUserData: "",
			wantNetworkConfig: `#cloud-config

foobar: foobar`,
		},
		{
			name: "multiline user data",
			cloudInitCustomization: imagedefinition.CloudInit{
				UserData: `#cloud-config

chpasswd:
	expire: true
	users:
		- name: ubuntu
		password: ubuntu
		type: text
`,
			},
			wantMetaData: "",
			wantUserData: `#cloud-config

chpasswd:
	expire: true
	users:
		- name: ubuntu
		password: ubuntu
		type: text
`,
			wantNetworkConfig: "",
		},
	}

	for i, tc := range testCases {
		t.Run("test_customize_cloud_init_"+tc.name, func(t *testing.T) {
			// Test setup
			asserter := helper.Asserter{T: t}
			restoreCWD := testhelper.SaveCWD()
			defer restoreCWD()

			var stateMachine ClassicStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine
			tmpDir, err := os.MkdirTemp("", "")
			asserter.AssertErrNil(err, true)
			t.Cleanup(func() {
				if tmpErr := osRemoveAll(tmpDir); tmpErr != nil {
					if err != nil {
						err = fmt.Errorf("%s after previous error: %w", tmpErr, err)
					} else {
						err = tmpErr
					}
				}
			})
			stateMachine.tempDirs.chroot = tmpDir

			// this directory is expected to be present as it is installed by cloud-init
			err = os.MkdirAll(path.Join(tmpDir, "etc/cloud/cloud.cfg.d"), 0777)
			asserter.AssertErrNil(err, true)

			stateMachine.ImageDef.Customization = &imagedefinition.Customization{
				CloudInit: &testCases[i].cloudInitCustomization,
			}

			// Running function to test
			err = stateMachine.customizeCloudInit()
			asserter.AssertErrNil(err, true)

			// Validation
			seedPath := path.Join(tmpDir, "var/lib/cloud/seed/nocloud")

			metaDataFile, err := os.Open(path.Join(seedPath, "meta-data"))
			if tc.cloudInitCustomization.MetaData != "" {
				asserter.AssertErrNil(err, false)

				metaDataFileContent, err := io.ReadAll(metaDataFile)
				asserter.AssertErrNil(err, false)

				if string(metaDataFileContent) != tc.wantMetaData {
					t.Errorf("un-expected meta-data content found: expected:\n%v\ngot:%v", tc.wantMetaData, string(metaDataFileContent))
				}
			} else {
				asserter.AssertErrContains(err, "no such file or directory")
			}

			networkConfigFile, err := os.Open(path.Join(seedPath, "network-config"))
			if tc.cloudInitCustomization.NetworkConfig != "" {
				asserter.AssertErrNil(err, false)

				networkConfigFileContent, err := io.ReadAll(networkConfigFile)
				asserter.AssertErrNil(err, false)
				if string(networkConfigFileContent) != tc.wantNetworkConfig {
					t.Errorf("un-expected network-config found: expected:\n%v\ngot:%v", tc.wantNetworkConfig, string(networkConfigFileContent))
				}
			} else {
				asserter.AssertErrContains(err, "no such file or directory")
			}

			userDataFile, err := os.Open(path.Join(seedPath, "user-data"))
			if tc.cloudInitCustomization.UserData != "" {
				asserter.AssertErrNil(err, false)

				userDataFileContent, err := io.ReadAll(userDataFile)
				asserter.AssertErrNil(err, false)

				if string(userDataFileContent) != tc.wantUserData {
					t.Errorf("un-expected user-data content found: expected:\n%v\ngot:%v", tc.wantUserData, string(userDataFileContent))
				}
			} else {
				asserter.AssertErrContains(err, "no such file or directory")
			}

			os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
		})
	}
}

// TestStatemachine_customizeCloudInit_failed tests failure modes of customizeCloudInit method
func TestStatemachine_customizeCloudInit_failed(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	tmpDir, err := os.MkdirTemp("", "")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(tmpDir) })
	stateMachine.tempDirs.chroot = tmpDir

	stateMachine.ImageDef.Customization = &imagedefinition.Customization{
		CloudInit: &imagedefinition.CloudInit{
			MetaData:      `foo: bar`,
			NetworkConfig: `foobar: foobar`,
			UserData: `#cloud-config

chpasswd:
  expire: true
  users:
    - name: ubuntu
      password: ubuntu
      type: text
`,
		},
	}

	// Test if osCreate fails
	fileList := []string{"meta-data", "user-data", "network-config", "90_dpkg.cfg"}
	for _, file := range fileList {
		t.Run("test_failed_customize_cloud_init_"+file, func(t *testing.T) {
			// this directory is expected to be present as it is installed by cloud-init
			cloudInitConfigDirPath := path.Join(tmpDir, "etc/cloud/cloud.cfg.d")
			err = os.MkdirAll(cloudInitConfigDirPath, 0777)
			asserter.AssertErrNil(err, true)
			t.Cleanup(func() {
				os.RemoveAll(cloudInitConfigDirPath)
			})

			osCreate = func(name string) (*os.File, error) {
				if strings.Contains(name, file) {
					return nil, errors.New("test error: failed to create file")
				}
				return os.Create(name)
			}

			err := stateMachine.customizeCloudInit()
			asserter.AssertErrContains(err, "test error: failed to create file")
		})
	}

	// Test if Write fails (file is read only)
	for _, file := range fileList {
		t.Run("test_failed_customize_cloud_init_"+file, func(t *testing.T) {
			// this directory is expected to be present as it is installed by cloud-init
			cloudInitConfigDirPath := path.Join(tmpDir, "etc/cloud/cloud.cfg.d")
			err = os.MkdirAll(cloudInitConfigDirPath, 0777)
			asserter.AssertErrNil(err, true)
			t.Cleanup(func() {
				os.RemoveAll(cloudInitConfigDirPath)
			})

			osCreate = func(name string) (*os.File, error) {
				if strings.Contains(name, file) {
					fileReadWrite, err := os.Create(name)
					asserter.AssertErrNil(err, true)
					fileReadWrite.Close()
					return os.Open(name)
				}
				return os.Create(name)
			}

			err := stateMachine.customizeCloudInit()
			if err == nil {
				t.Errorf("expected error but got nil")
			}
		})
	}

	// Test if os.MkdirAll fails
	t.Run("test_failed_customize_cloud_init_mkdir", func(t *testing.T) {
		// this directory is expected to be present as it is installed by cloud-init
		cloudInitConfigDirPath := path.Join(tmpDir, "etc/cloud/cloud.cfg.d")
		err = os.MkdirAll(cloudInitConfigDirPath, 0777)
		asserter.AssertErrNil(err, true)
		t.Cleanup(func() {
			os.RemoveAll(cloudInitConfigDirPath)
		})

		osMkdirAll = mockMkdirAll
		t.Cleanup(func() {
			osMkdirAll = os.MkdirAll
		})

		err := stateMachine.customizeCloudInit()
		if err == nil {
			t.Error()
		}
	})

	// Test if yaml.Marshal fails
	t.Run("test_failed_customize_cloud_init_yaml_marshal", func(t *testing.T) {
		// this directory is expected to be present as it is installed by cloud-init
		cloudInitConfigDirPath := path.Join(tmpDir, "etc/cloud/cloud.cfg.d")
		err = os.MkdirAll(cloudInitConfigDirPath, 0777)
		asserter.AssertErrNil(err, true)
		t.Cleanup(func() {
			os.RemoveAll(cloudInitConfigDirPath)
		})

		yamlMarshal = mockMarshal
		defer func() {
			yamlMarshal = yaml.Marshal
		}()

		err := stateMachine.customizeCloudInit()
		if err == nil {
			t.Error()
		}
	})

	// Test cloud-init customization is invalid
	testCases := []struct {
		name                   string
		cloudInitCustomization imagedefinition.CloudInit
	}{
		{
			name: "invalid userdata",
			cloudInitCustomization: imagedefinition.CloudInit{
				UserData: "foo: bar",
			},
		},
	}

	for i, tc := range testCases {
		t.Run("test_failed_customize_cloud_init_invalid_config_"+tc.name, func(t *testing.T) {
			// this directory is expected to be present as it is installed by cloud-init
			cloudInitConfigDirPath := path.Join(tmpDir, "etc/cloud/cloud.cfg.d")
			err = os.MkdirAll(cloudInitConfigDirPath, 0777)
			asserter.AssertErrNil(err, true)
			t.Cleanup(func() {
				os.RemoveAll(cloudInitConfigDirPath)
			})

			stateMachine.ImageDef.Customization.CloudInit = &testCases[i].cloudInitCustomization

			err := stateMachine.customizeCloudInit()
			asserter.AssertErrContains(err, "is missing proper header")
		})
	}
}

// TestStateMachine_manualCustomization unit tests the manualCustomization function
func TestStateMachine_manualCustomization(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine

	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Rootfs: &imagedefinition.Rootfs{
			Archive: "ubuntu",
		},
		Customization: &imagedefinition.Customization{
			Manual: &imagedefinition.Manual{
				MakeDirs: []*imagedefinition.MakeDirs{
					{
						Path:        "/etc/foo/bar",
						Permissions: 0755,
					},
					{
						Path:        "/etc/baz/test",
						Permissions: 0644,
					},
				},
				CopyFile: []*imagedefinition.CopyFile{
					{
						Source: filepath.Join("testdata", "test_script"),
						Dest:   "/test_copy_file",
					},
				},
				TouchFile: []*imagedefinition.TouchFile{
					{
						TouchPath: "/test_touch_file",
					},
				},
				Execute: []*imagedefinition.Execute{
					{
						// the file we already copied creates a file /test_execute
						ExecutePath: "/test_copy_file",
					},
				},
				AddUser: []*imagedefinition.AddUser{
					{
						UserName: "testuser",
						UserID:   "123456",
					},
				},
				AddGroup: []*imagedefinition.AddGroup{
					{
						GroupName: "testgroup",
						GroupID:   "456789",
					},
				},
			},
		},
	}

	d, err := os.Getwd()
	asserter.AssertErrNil(err, true)
	err = stateMachine.setConfDefDir(filepath.Join(d, "image_definition.yaml"))
	asserter.AssertErrNil(err, true)

	err = stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	err = getBasicChroot(stateMachine.StateMachine)
	asserter.AssertErrNil(err, true)

	err = stateMachine.manualCustomization()
	asserter.AssertErrNil(err, true)

	// Check that the correct directories exist
	testDirectories := []string{"/etc/foo/bar", "/etc/baz/test"}
	for _, dirName := range testDirectories {
		_, err := os.Stat(filepath.Join(stateMachine.tempDirs.chroot, dirName))
		if err != nil {
			t.Errorf("directory %s should exist, but it does not", dirName)
		}
	}

	// Check that the correct files exist
	testFiles := []string{"test_copy_file", "test_touch_file", "test_execute"}
	for _, fileName := range testFiles {
		_, err := os.Stat(filepath.Join(stateMachine.tempDirs.chroot, fileName))
		if err != nil {
			t.Errorf("file %s should exist, but it does not", fileName)
		}
	}

	// Check that the test user exists with the correct uid
	passwdFile := filepath.Join(stateMachine.tempDirs.chroot, "etc", "passwd")
	passwdContents, err := os.ReadFile(passwdFile)
	asserter.AssertErrNil(err, true)
	if !strings.Contains(string(passwdContents), "testuser:x:123456") {
		t.Errorf("Test user was not created in the chroot")
	}

	// Check that the test group exists with the correct gid
	groupFile := filepath.Join(stateMachine.tempDirs.chroot, "etc", "group")
	groupContents, err := os.ReadFile(groupFile)
	asserter.AssertErrNil(err, true)
	if !strings.Contains(string(groupContents), "testgroup:x:456789") {
		t.Errorf("Test group was not created in the chroot")
	}
}

// TestStateMachine_manualCustomization_fail tests failures in the manualCustomization function
func TestStateMachine_manualCustomization_fail(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	t.Run("test_failed_manual_customization", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		restoreCWD := testhelper.SaveCWD()
		t.Cleanup(restoreCWD)

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine

		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// mock helper.BackupAndCopyResolvConf
		helperBackupAndCopyResolvConf = mockBackupAndCopyResolvConfFail
		t.Cleanup(func() {
			helperBackupAndCopyResolvConf = helper.BackupAndCopyResolvConf
		})
		err = stateMachine.manualCustomization()
		asserter.AssertErrContains(err, "Error setting up /etc/resolv.conf")
	})

	tests := []struct {
		name                 string
		expectedErr          string
		manualCustomizations *imagedefinition.Manual
	}{
		{
			name:        "failing manualMakeDirs",
			expectedErr: "not a directory",
			manualCustomizations: &imagedefinition.Manual{
				MakeDirs: []*imagedefinition.MakeDirs{
					{
						Path:        filepath.Join("/etc", "resolv.conf"),
						Permissions: 0755,
					},
				},
			},
		},
		{
			name:        "failing manualCopyFile",
			expectedErr: "cp: cannot stat 'this/path/does/not/exist'",
			manualCustomizations: &imagedefinition.Manual{
				CopyFile: []*imagedefinition.CopyFile{
					{
						Source: filepath.Join("this", "path", "does", "not", "exist"),
						Dest:   filepath.Join("this", "path", "does", "not", "exist"),
					},
				},
			},
		},
		{
			name:        "failing manualExecute",
			expectedErr: "chroot: failed to run command",
			manualCustomizations: &imagedefinition.Manual{
				Execute: []*imagedefinition.Execute{
					{
						ExecutePath: filepath.Join("this", "path", "does", "not", "exist"),
					},
				},
			},
		},
		{
			name:        "failing manualTouchFile",
			expectedErr: "no such file or directory",
			manualCustomizations: &imagedefinition.Manual{
				TouchFile: []*imagedefinition.TouchFile{
					{
						TouchPath: filepath.Join("this", "path", "does", "not", "exist"),
					},
				},
			},
		},
		{
			name:        "failing manualAddGroup",
			expectedErr: "group 'root' already exists",
			manualCustomizations: &imagedefinition.Manual{
				AddGroup: []*imagedefinition.AddGroup{
					{
						GroupName: "root",
						GroupID:   "0",
					},
				},
			},
		},
		{
			name:        "failing manualAddUser",
			expectedErr: "user 'root' already exists",
			manualCustomizations: &imagedefinition.Manual{
				AddUser: []*imagedefinition.AddUser{
					{
						UserName: "root",
						UserID:   "0",
					},
				},
			},
		},
	}
	asserter := helper.Asserter{T: t}

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Rootfs: &imagedefinition.Rootfs{
			Archive: "ubuntu",
		},
	}

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	err = getBasicChroot(stateMachine.StateMachine)
	asserter.AssertErrNil(err, true)

	// create an /etc/resolv.conf in the chroot
	err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.chroot, "etc"), 0755)
	asserter.AssertErrNil(err, true)
	_, err = os.Create(filepath.Join(stateMachine.tempDirs.chroot, "etc", "resolv.conf"))
	asserter.AssertErrNil(err, true)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			restoreCWD := testhelper.SaveCWD()
			t.Cleanup(restoreCWD)

			stateMachine.ImageDef.Customization = &imagedefinition.Customization{
				Manual: tc.manualCustomizations,
			}

			err = stateMachine.manualCustomization()

			if len(tc.expectedErr) == 0 {
				asserter.AssertErrNil(err, true)
			} else {
				asserter.AssertErrContains(err, tc.expectedErr)
			}
		})
	}
}

// TestPrepareClassicImage unit tests the prepareClassicImage function
func TestPrepareClassicImage(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.Snaps = []string{"core20"}
	stateMachine.commonFlags.Channel = "stable"
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Customization: &imagedefinition.Customization{
			ExtraSnaps: []*imagedefinition.Snap{
				{
					SnapName: "hello",
					Channel:  "candidate",
				},
				{
					SnapName: "lxd",
					Channel:  "latest/stable",
				},
				{
					SnapName: "core22",
				},
			},
		},
	}

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	err = stateMachine.prepareClassicImage()
	asserter.AssertErrNil(err, true)

	// check that the lxd and hello snaps, as well as lxd's base, core20
	// were prepared in the correct location
	snaps := map[string]string{"lxd": "stable", "hello": "candidate", "core20": "stable", "core22": "stable"}
	for snapName, snapChannel := range snaps {
		// reach out to the snap store to find the revision
		// of the snap for the specified channel
		snapStore := store.New(nil, nil)
		snapSpec := store.SnapSpec{Name: snapName}
		context := context.TODO()
		snapInfo, err := snapStore.SnapInfo(context, snapSpec, nil)
		asserter.AssertErrNil(err, true)

		storeRevision := snapInfo.Channels["latest/"+snapChannel].Revision.N
		snapFileName := fmt.Sprintf("%s_%d.snap", snapName, storeRevision)

		snapPath := filepath.Join(stateMachine.tempDirs.chroot,
			"var", "lib", "snapd", "seed", "snaps", snapFileName)
		_, err = os.Stat(snapPath)
		if err != nil {
			if os.IsNotExist(err) {
				t.Errorf("File %s should exist, but does not", snapPath)
			}
		}
	}
}

// TestClassicSnapRevisions tests that if revisions are specified in the image definition
// that the corresponding revisions are staged in the chroot
func TestClassicSnapRevisions(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	if runtime.GOARCH != "amd64" {
		t.Skip("Test for amd64 only")
	}
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.Snaps = []string{"lxd"}
	stateMachine.commonFlags.Channel = "stable"
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Customization: &imagedefinition.Customization{
			ExtraSnaps: []*imagedefinition.Snap{
				{
					SnapName:     "hello",
					SnapRevision: 38,
				},
				{
					SnapName:     "ubuntu-image",
					SnapRevision: 330,
				},
				{
					SnapName:     "core20",
					SnapRevision: 1852,
				},
			},
		},
	}

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	err = stateMachine.prepareClassicImage()
	asserter.AssertErrNil(err, true)

	for _, snapInfo := range stateMachine.ImageDef.Customization.ExtraSnaps {
		// compile a regex used to get revision numbers from seed.manifest
		revRegex, err := regexp.Compile(fmt.Sprintf("%s_(.*?).snap\n", snapInfo.SnapName))
		asserter.AssertErrNil(err, true)
		seedData, err := os.ReadFile(filepath.Join(
			stateMachine.tempDirs.chroot,
			"var",
			"lib",
			"snapd",
			"seed",
			"seed.yaml",
		))
		asserter.AssertErrNil(err, true)
		revString := revRegex.FindStringSubmatch(string(seedData))
		if len(revString) != 2 {
			t.Fatal("Error finding snap revision via regex")
		}
		seededRevision, err := strconv.Atoi(revString[1])
		asserter.AssertErrNil(err, true)

		if seededRevision != snapInfo.SnapRevision {
			t.Errorf("Error, expected snap %s to "+
				"be revision %d, but it was %d",
				snapInfo.SnapName, snapInfo.SnapRevision, seededRevision)
		}
	}
}

// TestFailedPrepareClassicImage tests failures in the prepareClassicImage function
func TestFailedPrepareClassicImage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Customization: &imagedefinition.Customization{
			ExtraSnaps: []*imagedefinition.Snap{},
		},
	}

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	// include an invalid snap snap name to trigger a failure in
	// parseSnapsAndChannels
	stateMachine.Snaps = []string{"lxd=test=invalid=name"}
	err = stateMachine.prepareClassicImage()
	asserter.AssertErrContains(err, "Invalid syntax")

	// try to include a nonexistent snap to trigger a failure
	// in snapStore.SnapInfo
	stateMachine.Snaps = []string{"test-this-snap-name-should-never-exist"}
	err = stateMachine.prepareClassicImage()
	asserter.AssertErrContains(err, "Error getting info for snap")

	// mock image.Prepare
	stateMachine.Snaps = []string{"hello", "core"}
	imagePrepare = mockImagePrepare
	t.Cleanup(func() {
		imagePrepare = image.Prepare
	})
	err = stateMachine.prepareClassicImage()
	asserter.AssertErrContains(err, "Error preparing image")
	imagePrepare = image.Prepare

	// Test with a model assertion file
	stateMachine.ImageDef.ModelAssertion = filepath.Join("testdata", "modelAssertionClassic")
	err = stateMachine.prepareClassicImage()
	asserter.AssertErrNil(err, true)

	path, err := filepath.Abs(filepath.Join("testdata", "modelAssertionClassic"))
	asserter.AssertErrNil(err, true)
	stateMachine.ImageDef.ModelAssertion = path
	err = stateMachine.prepareClassicImage()
	asserter.AssertErrNil(err, true)

	stateMachine.ImageDef.ModelAssertion = ""
	// preseed the chroot, create a state.json file to trigger a reset, and mock some related functions
	err = stateMachine.prepareClassicImage()
	asserter.AssertErrNil(err, true)
	_, err = os.Create(filepath.Join(stateMachine.tempDirs.chroot, "var", "lib", "snapd", "state.json"))
	asserter.AssertErrNil(err, true)

	seedOpen = mockSeedOpen
	t.Cleanup(func() {
		seedOpen = seed.Open
	})
	err = stateMachine.prepareClassicImage()
	asserter.AssertErrContains(err, "Error getting list of preseeded snaps")
	seedOpen = seed.Open

	// Setup the exec.Command mock
	testCaseName = "TestFailedPrepareClassicImage"
	execCommand = fakeExecCommand
	t.Cleanup(func() {
		execCommand = exec.Command
	})
	err = stateMachine.prepareClassicImage()
	asserter.AssertErrContains(err, "Error resetting preseeding")
}

// TestStateMachine_PopulateClassicRootfsContents runs the state machine through populate_rootfs_contents and examines
// the rootfs to ensure at least some of the correct file are in place
func TestStateMachine_PopulateClassicRootfsContents(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	if runtime.GOARCH != "amd64" {
		t.Skip("Test for amd64 only")
	}
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Rootfs: &imagedefinition.Rootfs{
			Archive: "ubuntu",
		},
		Customization: &imagedefinition.Customization{},
	}

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	err = getBasicChroot(stateMachine.StateMachine)
	asserter.AssertErrNil(err, true)

	err = stateMachine.populateClassicRootfsContents()
	asserter.AssertErrNil(err, true)

	// check the files before Teardown
	fileList := []string{filepath.Join("etc", "shadow"),
		filepath.Join("etc", "systemd"),
		filepath.Join("usr", "lib")}
	for _, file := range fileList {
		_, err := os.Stat(filepath.Join(stateMachine.tempDirs.rootfs, file))
		if err != nil {
			if os.IsNotExist(err) {
				t.Errorf("File %s should exist, but does not", file)
			}
		}
	}

	// return when Customization.Fstab is not empty
	stateMachine.ImageDef.Customization.Fstab = []*imagedefinition.Fstab{
		{
			Label:        "writable",
			Mountpoint:   "/",
			FSType:       "ext4",
			MountOptions: "defaults",
			Dump:         true,
			FsckOrder:    1,
		},
	}

	err = stateMachine.populateClassicRootfsContents()
	asserter.AssertErrNil(err, true)

	// return when no Customization
	stateMachine.ImageDef.Customization = nil

	err = stateMachine.populateClassicRootfsContents()
	asserter.AssertErrNil(err, true)
}

// TestStateMachine_FailedPopulateClassicRootfsContents tests failed scenarios in populateClassicRootfsContents
// this is accomplished by mocking functions
func TestStateMachine_FailedPopulateClassicRootfsContents(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	asserter := helper.Asserter{T: t}
	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Rootfs: &imagedefinition.Rootfs{
			Archive: "ubuntu",
		},
		Customization: &imagedefinition.Customization{},
	}

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	err = getBasicChroot(stateMachine.StateMachine)
	asserter.AssertErrNil(err, true)

	// mock os.ReadDir
	osReadDir = mockReadDir
	t.Cleanup(func() {
		osReadDir = os.ReadDir
	})
	err = stateMachine.populateClassicRootfsContents()
	asserter.AssertErrContains(err, "Error reading chroot dir")
	osReadDir = os.ReadDir

	// mock osutil.CopySpecialFile
	osutilCopySpecialFile = mockCopySpecialFile
	t.Cleanup(func() {
		osutilCopySpecialFile = osutil.CopySpecialFile
	})
	err = stateMachine.populateClassicRootfsContents()
	asserter.AssertErrContains(err, "Error copying rootfs")
	osutilCopySpecialFile = osutil.CopySpecialFile

	// mock os.WriteFile
	osWriteFile = mockWriteFile
	t.Cleanup(func() {
		osWriteFile = os.WriteFile
	})
	err = stateMachine.populateClassicRootfsContents()
	asserter.AssertErrContains(err, "Error writing to fstab")
	osWriteFile = os.WriteFile

	// mock os.ReadFile
	osReadFile = mockReadFile
	t.Cleanup(func() {
		osReadFile = os.ReadFile
	})
	err = stateMachine.populateClassicRootfsContents()
	asserter.AssertErrContains(err, "Error reading fstab")
	osReadFile = os.ReadFile

	// return when existing fstab contains LABEL=writable
	//nolint:gosec,G306
	err = os.WriteFile(filepath.Join(stateMachine.tempDirs.chroot, "etc", "fstab"),
		[]byte("LABEL=writable\n"),
		0644)
	asserter.AssertErrNil(err, true)
	err = stateMachine.populateClassicRootfsContents()
	asserter.AssertErrNil(err, true)

	// create an /etc/resolv.conf.tmp in the chroot
	err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.chroot, "etc"), 0755)
	asserter.AssertErrNil(err, true)
	_, err = os.Create(filepath.Join(stateMachine.tempDirs.chroot, "etc", "resolv.conf.tmp"))
	asserter.AssertErrNil(err, true)

	// mock helper.RestoreResolvConf
	helperRestoreResolvConf = mockRestoreResolvConf
	t.Cleanup(func() {
		helperRestoreResolvConf = helper.RestoreResolvConf
	})
	err = stateMachine.populateClassicRootfsContents()
	asserter.AssertErrContains(err, "Error restoring /etc/resolv.conf")
	helperRestoreResolvConf = helper.RestoreResolvConf
}

// TestSateMachine_customizeSourcesList tests functionality of the customizeSourcesList state function
func TestSateMachine_customizeSourcesList(t *testing.T) {
	series := getHostSuite()
	testCases := []struct {
		name                      string
		deb822Format              bool
		existingSourcesList       string
		existingDeb822SourcesList string
		customization             *imagedefinition.Customization
		mockFuncs                 func() func()
		expectedErr               string
		expectedSourcesList       string
		expectedDeb822SourcesList string
	}{
		{
			name:                "set default sources.list",
			deb822Format:        false,
			existingSourcesList: "deb http://ports.ubuntu.com/ubuntu-ports jammy main restricted",
			customization:       &imagedefinition.Customization{},
			expectedSourcesList: fmt.Sprintf(`# See http://help.ubuntu.com/community/UpgradeNotes for how to upgrade to
# newer versions of the distribution.
deb http://archive.ubuntu.com/ubuntu/ %s main restricted universe
`, series),
		},
		{
			name:                "set less components sources.list",
			deb822Format:        false,
			existingSourcesList: "deb http://ports.ubuntu.com/ubuntu-ports jammy main restricted",
			customization: &imagedefinition.Customization{
				Components: []string{"main"},
			},
			expectedSourcesList: fmt.Sprintf(`# See http://help.ubuntu.com/community/UpgradeNotes for how to upgrade to
# newer versions of the distribution.
deb http://archive.ubuntu.com/ubuntu/ %s main
`, series),
		},
		{
			name:                "set components and pocket sources.list",
			deb822Format:        false,
			existingSourcesList: "deb http://ports.ubuntu.com/ubuntu-ports jammy main restricted",
			customization: &imagedefinition.Customization{
				Components: []string{"main"},
				Pocket:     "security",
			},
			expectedSourcesList: fmt.Sprintf(`# See http://help.ubuntu.com/community/UpgradeNotes for how to upgrade to
# newer versions of the distribution.
deb http://archive.ubuntu.com/ubuntu/ %[1]s main
deb http://security.ubuntu.com/ubuntu/ %[1]s-security main
`, series),
		},
		{
			name:                "fail to write sources.list",
			deb822Format:        false,
			existingSourcesList: "deb http://ports.ubuntu.com/ubuntu-ports jammy main restricted",
			customization: &imagedefinition.Customization{
				Components: []string{"main"},
				Pocket:     "security",
			},
			expectedSourcesList: "deb http://ports.ubuntu.com/ubuntu-ports jammy main restricted",
			expectedErr:         "unable to open sources.list file",
			mockFuncs: func() func() {
				mock := testhelper.NewOSMock(
					&testhelper.OSMockConf{
						OpenFileThreshold: 0,
					},
				)

				osOpenFile = mock.OpenFile
				return func() { osOpenFile = os.OpenFile }
			},
		},
		{
			name:                "set default ubuntu.sources and commented sources.list",
			deb822Format:        true,
			existingSourcesList: "deb http://ports.ubuntu.com/ubuntu-ports jammy main restricted",
			existingDeb822SourcesList: fmt.Sprintf(`Types: deb
URIs: http://archive.ubuntu.com/
Suites: %s
Components: main universe restricted multiverse
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg
`, series),
			customization:       &imagedefinition.Customization{},
			expectedSourcesList: imagedefinition.LegacySourcesListComment,
			expectedDeb822SourcesList: fmt.Sprintf(`## Ubuntu distribution repository
##
## The following settings can be adjusted to configure which packages to use from Ubuntu.
## Mirror your choices (except for URIs and Suites) in the security section below to
## ensure timely security updates.
##
## Types: Append deb-src to enable the fetching of source package.
## URIs: A URL to the repository (you may add multiple URLs)
## Suites: The following additional suites can be configured
##   <name>-updates   - Major bug fix updates produced after the final release of the
##                      distribution.
##   <name>-backports - software from this repository may not have been tested as
##                      extensively as that contained in the main release, although it includes
##                      newer versions of some applications which may provide useful features.
##                      Also, please note that software in backports WILL NOT receive any review
##                      or updates from the Ubuntu security team.
## Components: Aside from main, the following components can be added to the list
##   restricted  - Software that may not be under a free license, or protected by patents.
##   universe    - Community maintained packages. Software in this repository receives maintenance
##                 from volunteers in the Ubuntu community, or a 10 year security maintenance
##                 commitment from Canonical when an Ubuntu Pro subscription is attached.
##   multiverse  - Community maintained of restricted. Software from this repository is
##                 ENTIRELY UNSUPPORTED by the Ubuntu team, and may not be under a free
##                 licence. Please satisfy yourself as to your rights to use the software.
##                 Also, please note that software in multiverse WILL NOT receive any
##                 review or updates from the Ubuntu security team.
##
## See the sources.list(5) manual page for further settings.
Types: deb
URIs: http://archive.ubuntu.com/ubuntu/
Suites: %[1]s
Components: main restricted
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg

## Ubuntu security updates. Aside from URIs and Suites,
## this should mirror your choices in the previous section.
Types: deb
URIs: http://security.ubuntu.com/ubuntu/
Suites: %[1]s-security
Components: main restricted
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg

`, series),
		},
		{
			name:                "fail to write ubuntu.sources and commented sources.list",
			deb822Format:        true,
			existingSourcesList: "deb http://ports.ubuntu.com/ubuntu-ports jammy main restricted",
			existingDeb822SourcesList: fmt.Sprintf(`Types: deb
URIs: http://archive.ubuntu.com/
Suites: %s
Components: main universe restricted multiverse
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg
`, series),
			customization:       &imagedefinition.Customization{},
			expectedSourcesList: "deb http://ports.ubuntu.com/ubuntu-ports jammy main restricted",
			expectedDeb822SourcesList: fmt.Sprintf(`Types: deb
URIs: http://archive.ubuntu.com/
Suites: %s
Components: main universe restricted multiverse
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg
`, series),
			expectedErr: "unable to open ubuntu.sources file",
			mockFuncs: func() func() {
				mock := testhelper.NewOSMock(
					&testhelper.OSMockConf{
						OpenFileThreshold: 0,
					},
				)

				osOpenFile = mock.OpenFile
				return func() { osOpenFile = os.OpenFile }
			},
		},
		{
			name:                "fail to create sources.list.d",
			deb822Format:        true,
			existingSourcesList: "deb http://ports.ubuntu.com/ubuntu-ports jammy main restricted",
			existingDeb822SourcesList: fmt.Sprintf(`Types: deb
URIs: http://archive.ubuntu.com/
Suites: %s
Components: main universe restricted multiverse
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg
`, series),
			customization:       &imagedefinition.Customization{},
			expectedSourcesList: "deb http://ports.ubuntu.com/ubuntu-ports jammy main restricted",
			expectedDeb822SourcesList: fmt.Sprintf(`Types: deb
URIs: http://archive.ubuntu.com/
Suites: %s
Components: main universe restricted multiverse
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg
`, series),
			expectedErr: "Error /etc/apt/sources.list.d directory",
			mockFuncs: func() func() {
				mock := testhelper.NewOSMock(
					&testhelper.OSMockConf{
						MkdirAllThreshold: 0,
					},
				)

				osMkdirAll = mock.MkdirAll
				return func() { osMkdirAll = os.MkdirAll }
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			restoreCWD := testhelper.SaveCWD()
			defer restoreCWD()

			var stateMachine ClassicStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine
			stateMachine.ImageDef = imagedefinition.ImageDefinition{
				Architecture: getHostArch(),
				Series:       series,
				Rootfs: &imagedefinition.Rootfs{
					SourcesListDeb822: helper.BoolPtr(tc.deb822Format),
				},
				Customization: tc.customization,
			}

			err := helper.SetDefaults(&stateMachine.ImageDef)
			asserter.AssertErrNil(err, true)

			err = stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)

			t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

			err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.chroot, "etc", "apt", "sources.list.d"), 0644)
			asserter.AssertErrNil(err, true)

			sourcesListPath := filepath.Join(stateMachine.tempDirs.chroot, "etc", "apt", "sources.list")
			deb822SourcesListPath := filepath.Join(stateMachine.tempDirs.chroot, "etc", "apt", "sources.list.d", "ubuntu.sources")

			err = osWriteFile(sourcesListPath, []byte(tc.existingSourcesList), 0644)
			asserter.AssertErrNil(err, true)

			err = osWriteFile(deb822SourcesListPath, []byte(tc.existingDeb822SourcesList), 0644)
			asserter.AssertErrNil(err, true)

			if tc.mockFuncs != nil {
				restoreMock := tc.mockFuncs()
				t.Cleanup(restoreMock)
			}

			err = stateMachine.customizeSourcesList()
			if err != nil || len(tc.expectedErr) != 0 {
				asserter.AssertErrContains(err, tc.expectedErr)
			}

			sourcesListBytes, err := os.ReadFile(sourcesListPath)
			asserter.AssertErrNil(err, true)

			asserter.AssertEqual(tc.expectedSourcesList, string(sourcesListBytes))

			deb822SourcesListBytes, err := os.ReadFile(deb822SourcesListPath)
			asserter.AssertErrNil(err, true)

			asserter.AssertEqual(tc.expectedDeb822SourcesList, string(deb822SourcesListBytes))

		})
	}
}

// TestSateMachine_fixFstab tests functionality of the fixFstab function
func TestSateMachine_fixFstab(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name          string
		existingFstab string
		expectedFstab string
	}{
		{
			name:          "add entry to an existing but empty fstab",
			existingFstab: "# UNCONFIGURED FSTAB",
			expectedFstab: `LABEL=writable	/	ext4	discard,errors=remount-ro	0	1
`,
		},
		{
			name: "fix existing entry amongst several others",
			existingFstab: `# /etc/fstab: static file system information.
UUID=1565-1398	/	ext4	defaults	0	0
#Here is another comment that should be left in place
/dev/mapper/vgubuntu-swap_1	none	swap	sw	0	0
`,
			expectedFstab: `# /etc/fstab: static file system information.
LABEL=writable	/	ext4	discard,errors=remount-ro	0	1
#Here is another comment that should be left in place
/dev/mapper/vgubuntu-swap_1	none	swap	sw	0	0
`,
		},
		{
			name: "fix existing entry amongst several others (with spaces)",
			existingFstab: `# /etc/fstab: static file system information.
UUID=1565-1398	/	ext4	defaults	0	0
/dev/mapper/vgubuntu-swap_1	none  swap sw      0   0
`,
			expectedFstab: `# /etc/fstab: static file system information.
LABEL=writable	/	ext4	discard,errors=remount-ro	0	1
/dev/mapper/vgubuntu-swap_1	none	swap	sw	0	0
`,
		},
		{
			name: "fix only one root mount point",
			existingFstab: `# /etc/fstab: static file system information.
UUID=1565-1398	/	ext4	defaults	0	0
UUID=1234-5678	/	ext4	defaults	0	0
`,
			expectedFstab: `# /etc/fstab: static file system information.
LABEL=writable	/	ext4	discard,errors=remount-ro	0	1
UUID=1234-5678	/	ext4	defaults	0	0
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			restoreCWD := testhelper.SaveCWD()
			defer restoreCWD()

			var stateMachine ClassicStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine
			stateMachine.ImageDef = imagedefinition.ImageDefinition{
				Architecture:  getHostArch(),
				Series:        getHostSuite(),
				Rootfs:        &imagedefinition.Rootfs{},
				Customization: &imagedefinition.Customization{},
			}

			// set the defaults for the imageDef
			err := helper.SetDefaults(&stateMachine.ImageDef)
			asserter.AssertErrNil(err, true)

			err = stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)

			t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

			// create the <chroot>/etc directory
			err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.rootfs, "etc"), 0644)
			asserter.AssertErrNil(err, true)

			fstabPath := filepath.Join(stateMachine.tempDirs.rootfs, "etc", "fstab")

			// simulate an already existing fstab file
			if len(tc.existingFstab) != 0 {
				err = osWriteFile(fstabPath, []byte(tc.existingFstab), 0644)
				asserter.AssertErrNil(err, true)
			}

			err = stateMachine.fixFstab()
			asserter.AssertErrNil(err, true)

			fstabBytes, err := os.ReadFile(fstabPath)
			asserter.AssertErrNil(err, true)

			if string(fstabBytes) != tc.expectedFstab {
				t.Errorf("Expected fstab content \"%s\", but got \"%s\"",
					tc.expectedFstab, string(fstabBytes))
			}
		})
	}
}

// TestGeneratePackageManifest tests if classic image manifest generation works
func TestGeneratePackageManifest(t *testing.T) {
	asserter := helper.Asserter{T: t}

	// Setup the exec.Command mock
	testCaseName = "TestGeneratePackageManifest"
	execCommand = fakeExecCommand
	t.Cleanup(func() {
		execCommand = exec.Command
	})
	// We need the output directory set for this
	outputDir, err := os.MkdirTemp("/tmp", "ubuntu-image-")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(outputDir) })

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.commonFlags.OutputDir = outputDir
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Rootfs: &imagedefinition.Rootfs{
			Archive: "ubuntu",
		},
		Customization: &imagedefinition.Customization{},
		Artifacts: &imagedefinition.Artifact{
			Manifest: &imagedefinition.Manifest{
				ManifestName: "filesystem.manifest",
			},
		},
	}
	err = osMkdirAll(stateMachine.commonFlags.OutputDir, 0755)
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(stateMachine.commonFlags.OutputDir) })

	err = stateMachine.generatePackageManifest()
	asserter.AssertErrNil(err, true)

	os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	// Check if manifest file got generated and if it has expected contents
	manifestPath := filepath.Join(stateMachine.commonFlags.OutputDir, "filesystem.manifest")
	manifestBytes, err := os.ReadFile(manifestPath)
	asserter.AssertErrNil(err, true)
	// The order of packages shouldn't matter
	examplePackages := []string{"foo 1.2", "bar 1.4-1ubuntu4.1", "libbaz 0.1.3ubuntu2"}
	for _, pkg := range examplePackages {
		if !strings.Contains(string(manifestBytes), pkg) {
			t.Errorf("filesystem.manifest does not contain expected package: %s", pkg)
		}
	}
}

// TestFailedGeneratePackageManifest tests if classic manifest generation failures are reported
func TestFailedGeneratePackageManifest(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Rootfs: &imagedefinition.Rootfs{
			Archive: "ubuntu",
		},
		Customization: &imagedefinition.Customization{},
		Artifacts: &imagedefinition.Artifact{
			Manifest: &imagedefinition.Manifest{
				ManifestName: "filesystem.manifest",
			},
		},
	}

	// We need the output directory set for this
	outputDir, err := os.MkdirTemp("/tmp", "ubuntu-image-")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(outputDir) })
	stateMachine.commonFlags.OutputDir = outputDir

	// Setup the exec.Command mock - version from the success test
	testCaseName = "TestGeneratePackageManifest"
	execCommand = fakeExecCommand
	t.Cleanup(func() {
		execCommand = exec.Command
	})

	// Setup the mock for os.Create, making those fail
	osCreate = mockCreate
	t.Cleanup(func() {
		osCreate = os.Create
	})

	err = stateMachine.generatePackageManifest()
	asserter.AssertErrContains(err, "Error creating manifest file")
	osCreate = os.Create

	// Setup the exec.Command mock - version from the fail test
	testCaseName = "TestFailedGeneratePackageManifest"
	execCommand = fakeExecCommand
	t.Cleanup(func() {
		execCommand = exec.Command
	})
	err = stateMachine.generatePackageManifest()
	asserter.AssertErrContains(err, "Error generating package manifest with command")
}

// TestGenerateFilelist tests if classic image filelist generation works
func TestGenerateFilelist(t *testing.T) {
	asserter := helper.Asserter{T: t}

	// Setup the exec.Command mock
	testCaseName = "TestGenerateFilelist"
	execCommand = fakeExecCommand
	t.Cleanup(func() {
		execCommand = exec.Command
	})
	// We need the output directory set for this
	outputDir, err := os.MkdirTemp("/tmp", "ubuntu-image-")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(outputDir) })

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.commonFlags.OutputDir = outputDir
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Rootfs: &imagedefinition.Rootfs{
			Archive: "ubuntu",
		},
		Customization: &imagedefinition.Customization{},
		Artifacts: &imagedefinition.Artifact{
			Filelist: &imagedefinition.Filelist{
				FilelistName: "filesystem.filelist",
			},
		},
	}
	err = osMkdirAll(stateMachine.commonFlags.OutputDir, 0755)
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(stateMachine.commonFlags.OutputDir) })

	err = stateMachine.generateFilelist()
	asserter.AssertErrNil(err, true)

	os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	// Check if filelist file got generated
	filelistPath := filepath.Join(stateMachine.commonFlags.OutputDir, "filesystem.filelist")
	_, err = os.Stat(filelistPath)
	asserter.AssertErrNil(err, true)
}

// TestFailedGenerateFilelist tests if classic filelist generation failures are reported
func TestFailedGenerateFilelist(t *testing.T) {
	asserter := helper.Asserter{T: t}

	// Setup the exec.Command mock - version from the success test
	testCaseName = "TestGenerateFilelist"
	execCommand = fakeExecCommand
	defer func() {
		execCommand = exec.Command
	}()
	// Setup the mock for os.Create, making those fail
	osCreate = mockCreate
	defer func() {
		osCreate = os.Create
	}()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Rootfs: &imagedefinition.Rootfs{
			Archive: "ubuntu",
		},
		Customization: &imagedefinition.Customization{},
		Artifacts: &imagedefinition.Artifact{
			Filelist: &imagedefinition.Filelist{
				FilelistName: "filesystem.filelist",
			},
		},
	}

	// We need the output directory set for this
	outputDir, err := os.MkdirTemp("/tmp", "ubuntu-image-")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(outputDir) })
	stateMachine.commonFlags.OutputDir = outputDir

	// Setup the exec.Command mock - version from the success test
	testCaseName = "TestGenerateFilelist"
	execCommand = fakeExecCommand
	defer func() {
		execCommand = exec.Command
	}()

	// Setup the mock for os.Create, making those fail
	osCreate = mockCreate
	defer func() {
		osCreate = os.Create
	}()

	err = stateMachine.generateFilelist()
	asserter.AssertErrContains(err, "Error creating filelist")
	osCreate = os.Create

	// Setup the exec.Command mock - version from the fail test
	testCaseName = "TestFailedGenerateFilelist"
	execCommand = fakeExecCommand
	defer func() {
		execCommand = exec.Command
	}()
	err = stateMachine.generateFilelist()
	asserter.AssertErrContains(err, "Error generating file list with command")
}

// TestSuccessfulClassicRun runs through a full classic state machine run and ensures
// it is successful. It creates a .img and a .qcow2 file, ensures they are the
// correct file types, it mounts the resulting .img and ensures grub was updated
func TestSuccessfulClassicRun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	t.Cleanup(restoreCWD)

	// We need the output directory set for this
	outputDir, err := os.MkdirTemp("/tmp", "ubuntu-image-")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(outputDir) })

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.commonFlags.Debug = true
	stateMachine.commonFlags.Size = "5G"
	stateMachine.commonFlags.OutputDir = outputDir
	stateMachine.Args.ImageDefinition = filepath.Join("testdata", "image_definitions",
		"test_amd64.yaml")

	err = stateMachine.Setup()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	err = stateMachine.Run()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() {
		err = stateMachine.Teardown()
		asserter.AssertErrNil(err, true)
	})

	testHelperCheckPPAInstalled(t, &asserter, stateMachine.tempDirs.chroot)
	testHelperCheckSnapInstalled(t, &asserter, stateMachine.tempDirs.chroot)

	artifacts := map[string]string{
		"pc-amd64.img":            "DOS/MBR boot sector",
		"pc-amd64.qcow2":          `QEMU QCOW2? Image \(v3\)`,
		"filesystem-manifest.txt": "text",
		"filesystem-filelist.txt": "text",
	}
	testHelperCheckArtifacts(t, &asserter, stateMachine.commonFlags.OutputDir, artifacts)

	// create a directory in which to mount the rootfs
	mountDir := filepath.Join(stateMachine.tempDirs.scratch, "loopback")
	var setupImageCmds []*exec.Cmd
	var teardownImageCmds []*exec.Cmd

	t.Cleanup(func() {
		for _, teardownCmd := range teardownImageCmds {
			if tmpErr := teardownCmd.Run(); tmpErr != nil {
				if err != nil {
					err = fmt.Errorf("%s after previous error: %w", tmpErr, err)
				} else {
					err = tmpErr
				}
			}
		}
	})

	imgPath := filepath.Join(stateMachine.commonFlags.OutputDir, "pc-amd64.img")

	// set up the loopback device
	loopUsed, losetupDetachCmd, err := associateLoopDevice(imgPath, stateMachine.SectorSize)
	asserter.AssertErrNil(err, true)

	teardownImageCmds = append(teardownImageCmds, losetupDetachCmd)

	setupImageCmds = append(setupImageCmds,
		//nolint:gosec,G204
		exec.Command("mount", fmt.Sprintf("%sp3", loopUsed), mountDir), // with this example the rootfs is partition 3 mountDir
	)

	teardownImageCmds = append([]*exec.Cmd{exec.Command("umount", "--recursive", mountDir)}, teardownImageCmds...)

	// set up the mountpoints
	mountPoints := []mountPoint{
		{
			src:      "devtmpfs-build",
			basePath: mountDir,
			relpath:  "/dev",
			typ:      "devtmpfs",
		},
		{
			src:      "devpts-build",
			basePath: mountDir,
			relpath:  "/dev/pts",
			typ:      "devpts",
			opts:     []string{"nodev", "nosuid"},
		},
		{
			src:      "proc-build",
			basePath: mountDir,
			relpath:  "/proc",
			typ:      "proc",
		},
		{
			src:      "sysfs-build",
			basePath: mountDir,
			relpath:  "/sys",
			typ:      "sysfs",
		},
	}
	for _, mp := range mountPoints {
		mountCmds, umountCmds, err := mp.getMountCmd()
		if err != nil {
			t.Errorf("Error preparing mountpoint \"%s\": \"%s\"",
				mp.relpath,
				err.Error(),
			)
		}
		setupImageCmds = append(setupImageCmds, mountCmds...)
		teardownImageCmds = append(umountCmds, teardownImageCmds...)
	}

	teardownImageCmds = append([]*exec.Cmd{execCommand("udevadm", "settle")}, teardownImageCmds...)

	// now run all the commands to mount the image
	for _, cmd := range setupImageCmds {
		outPut := helper.SetCommandOutput(cmd, true)
		err := cmd.Run()
		if err != nil {
			t.Errorf("Error running command \"%s\". Error is \"%s\". Output is: \n%s",
				cmd.String(), err.Error(), outPut.String())
		}
	}

	testHelperCheckMakeDirs(t, mountDir)
	testHelperCheckAddUser(t, &asserter, mountDir)
	testHelperCheckGrubConfig(t, mountDir)
	testHelperCheckCleanedFiles(t, mountDir)
	testHelperCheckLocaleFile(t, &asserter, mountDir)
	testHelperCheckSourcesList(t, &asserter, mountDir)
}

func testHelperCheckPPAInstalled(t *testing.T, asserter *helper.Asserter, chroot string) {
	t.Helper()
	files := []string{
		filepath.Join(chroot, "usr", "bin", "hello-ubuntu-image-public"),
		filepath.Join(chroot, "usr", "bin", "hello-ubuntu-image-private"),
	}
	for _, file := range files {
		_, err := os.Stat(file)
		asserter.AssertErrNil(err, true)
	}
}

func testHelperCheckSnapInstalled(t *testing.T, asserter *helper.Asserter, chroot string) {
	t.Helper()
	type snapList struct {
		Snaps []struct {
			Name    string `yaml:"name"`
			Channel string `yaml:"channel"`
		} `yaml:"snaps"`
	}

	seedYaml := filepath.Join(chroot,
		"var", "lib", "snapd", "seed", "seed.yaml")

	seedFile, err := os.Open(seedYaml)
	asserter.AssertErrNil(err, true)
	defer seedFile.Close()

	var seededSnaps snapList
	err = yaml.NewDecoder(seedFile).Decode(&seededSnaps)
	asserter.AssertErrNil(err, true)

	expectedSnapChannels := map[string]string{
		"hello":  "candidate",
		"core20": "stable",
	}

	for _, seededSnap := range seededSnaps.Snaps {
		channel, found := expectedSnapChannels[seededSnap.Name]
		if found {
			if channel != seededSnap.Channel {
				t.Errorf("Expected snap %s to be pre-seeded with channel %s, but got %s",
					seededSnap.Name, channel, seededSnap.Channel)
			}
		}
	}
}

func testHelperCheckArtifacts(t *testing.T, asserter *helper.Asserter, outputDir string, artifacts map[string]string) {
	t.Helper()
	for artifact, fileType := range artifacts {
		fullPath := filepath.Join(outputDir, artifact)
		_, err := os.Stat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				t.Errorf("File \"%s\" should exist, but does not", fullPath)
			}
		}

		// check it is the expected file type
		fileCommand := *exec.Command("file", fullPath)
		cmdOutput, err := fileCommand.CombinedOutput()
		asserter.AssertErrNil(err, true)

		fileTypeRegex := regexp.MustCompile(fileType)

		if !fileTypeRegex.Match([]byte(cmdOutput)) {
			t.Errorf("File \"%s\" is the wrong file type. Expected \"%s\" but got \"%s\"",
				fullPath, fileType, string(cmdOutput))
		}
	}
}

func testHelperCheckMakeDirs(t *testing.T, mountDir string) {
	t.Helper()
	addedDir := filepath.Join(mountDir, "etc", "foo", "bar")
	_, err := os.Stat(addedDir)
	if err != nil {
		if os.IsNotExist(err) {
			t.Errorf("Directory \"%s\" should exist, but does not", addedDir)
		}
	}
}

func testHelperCheckAddUser(t *testing.T, asserter *helper.Asserter, mountDir string) {
	t.Helper()
	shadowPath := filepath.Join(mountDir, "etc", "shadow")
	shadowFile, err := os.Open(shadowPath)
	asserter.AssertErrNil(err, true)
	defer shadowFile.Close()
	ubuntu2Found := false
	ubuntu2Line := ""

	scanner := bufio.NewScanner(shadowFile)

	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "ubuntu2") {
			ubuntu2Line = scanner.Text()
			ubuntu2Found = true
			break
		}
	}

	if !ubuntu2Found {
		t.Error("ubuntu2 user not created")
	}

	expire := strings.Split(ubuntu2Line, ":")[2]

	if expire != "0" {
		t.Error("ubuntu2 user password should be expired")
	}
}

func testHelperCheckGrubConfig(t *testing.T, mountDir string) {
	t.Helper()
	grubCfg := filepath.Join(mountDir, "boot", "grub", "grub.cfg")
	_, err := os.Stat(grubCfg)
	if err != nil {
		if os.IsNotExist(err) {
			t.Errorf("File \"%s\" should exist, but does not", grubCfg)
		}
	}
}

func testHelperCheckCleanedFiles(t *testing.T, mountDir string) {
	t.Helper()
	cleaned := []string{
		filepath.Join(mountDir, "var", "lib", "dbus", "machine-id"),
		filepath.Join(mountDir, "etc", "ssh", "ssh_host_rsa_key"),
		filepath.Join(mountDir, "etc", "ssh", "ssh_host_rsa_key.pub"),
		filepath.Join(mountDir, "etc", "ssh", "ssh_host_ecdsa_key"),
		filepath.Join(mountDir, "etc", "ssh", "ssh_host_ecdsa_key.pub"),
		filepath.Join(mountDir, "usr", "sbin", "policy-rc.d"),
		filepath.Join(mountDir, "sbin", "start-stop-daemon.REAL"),
		filepath.Join(mountDir, "sbin", "initctl.REAL"),
	}
	for _, file := range cleaned {
		_, err := os.Stat(file)
		if !os.IsNotExist(err) {
			t.Errorf("File %s should not exist, but does", file)
		}
	}

	truncated := []string{
		filepath.Join(mountDir, "etc", "machine-id"),
	}
	for _, file := range truncated {
		fileInfo, err := os.Stat(file)
		if os.IsNotExist(err) {
			t.Errorf("File %s should exist, but does not", file)
		}

		if fileInfo.Size() != 0 {
			t.Errorf("File %s should be empty, but it is not. Size: %v", file, fileInfo.Size())
		}
	}
}

func testHelperCheckLocaleFile(t *testing.T, asserter *helper.Asserter, mountDir string) {
	t.Helper()
	localeFile := filepath.Join(mountDir, "etc", "default", "locale")
	localeBytes, err := os.ReadFile(localeFile)
	asserter.AssertErrNil(err, true)
	if !strings.Contains(string(localeBytes), "LANG=C.UTF-8") {
		t.Errorf("Expected LANG=C.UTF-8 in %s, but got %s", localeFile, string(localeBytes))
	}
}

// testHelperCheckSourcesList checks if components and pocket correctly setup in /etc/apt/sources.list.d/ubuntu.sources
func testHelperCheckSourcesList(t *testing.T, asserter *helper.Asserter, mountDir string) {
	t.Helper()
	aptDeb822SourcesListBytes, err := os.ReadFile(filepath.Join(mountDir, "etc", "apt", "sources.list.d", "ubuntu.sources"))
	asserter.AssertErrNil(err, true)
	wantAptDeb822SourcesList := `## Ubuntu distribution repository
##
## The following settings can be adjusted to configure which packages to use from Ubuntu.
## Mirror your choices (except for URIs and Suites) in the security section below to
## ensure timely security updates.
##
## Types: Append deb-src to enable the fetching of source package.
## URIs: A URL to the repository (you may add multiple URLs)
## Suites: The following additional suites can be configured
##   <name>-updates   - Major bug fix updates produced after the final release of the
##                      distribution.
##   <name>-backports - software from this repository may not have been tested as
##                      extensively as that contained in the main release, although it includes
##                      newer versions of some applications which may provide useful features.
##                      Also, please note that software in backports WILL NOT receive any review
##                      or updates from the Ubuntu security team.
## Components: Aside from main, the following components can be added to the list
##   restricted  - Software that may not be under a free license, or protected by patents.
##   universe    - Community maintained packages. Software in this repository receives maintenance
##                 from volunteers in the Ubuntu community, or a 10 year security maintenance
##                 commitment from Canonical when an Ubuntu Pro subscription is attached.
##   multiverse  - Community maintained of restricted. Software from this repository is
##                 ENTIRELY UNSUPPORTED by the Ubuntu team, and may not be under a free
##                 licence. Please satisfy yourself as to your rights to use the software.
##                 Also, please note that software in multiverse WILL NOT receive any
##                 review or updates from the Ubuntu security team.
##
## See the sources.list(5) manual page for further settings.
Types: deb
URIs: http://archive.ubuntu.com/ubuntu/
Suites: jammy jammy-updates jammy-proposed
Components: main universe restricted
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg

## Ubuntu security updates. Aside from URIs and Suites,
## this should mirror your choices in the previous section.
Types: deb
URIs: http://security.ubuntu.com/ubuntu/
Suites: jammy-security
Components: main universe restricted
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg

`
	asserter.AssertEqual(wantAptDeb822SourcesList, string(aptDeb822SourcesListBytes))

	// check if components and pocket correctly setup in /etc/apt/sources.list
	aptSourcesListBytes, err := os.ReadFile(filepath.Join(mountDir, "etc", "apt", "sources.list"))
	asserter.AssertErrNil(err, true)
	asserter.AssertEqual(imagedefinition.LegacySourcesListComment, string(aptSourcesListBytes))
}

// TestSuccessfulClassicRunNoArtifact runs through a full classic state machine run without artifact
func TestSuccessfulClassicRunNoArtifact(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	t.Cleanup(restoreCWD)

	// We need the output directory set for this
	outputDir, err := os.MkdirTemp("/tmp", "ubuntu-image-")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(outputDir) })

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.commonFlags.Debug = true
	stateMachine.commonFlags.Size = "5G"
	stateMachine.commonFlags.OutputDir = outputDir
	stateMachine.Args.ImageDefinition = filepath.Join("testdata", "image_definitions",
		"test_no_artifact.yaml")

	err = stateMachine.Setup()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	err = stateMachine.Run()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() {
		err = stateMachine.Teardown()
		asserter.AssertErrNil(err, true)
	})

	// make sure packages were successfully installed from public and private ppas
	testHelperCheckPPAInstalled(t, &asserter, stateMachine.tempDirs.chroot)

	// make sure snaps from the correct channel were installed
	testHelperCheckSnapInstalled(t, &asserter, stateMachine.tempDirs.chroot)
}

func TestSuccessfulRootfsGeneration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	t.Cleanup(restoreCWD)

	// We need the output directory set for this
	outputDir, err := os.MkdirTemp("/tmp", "ubuntu-image-")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(outputDir) })

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.commonFlags.Debug = true
	stateMachine.commonFlags.Size = "5G"
	stateMachine.commonFlags.OutputDir = outputDir
	stateMachine.Args.ImageDefinition = filepath.Join("testdata", "image_definitions",
		"test_rootfs_tarball.yaml")

	err = stateMachine.Setup()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	err = stateMachine.Run()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() {
		err = stateMachine.Teardown()
		asserter.AssertErrNil(err, true)
	})

	// make sure all the artifacts were created and are the correct file types
	artifacts := map[string]string{
		"rootfs.tar": "tar archive",
	}
	for artifact, fileType := range artifacts {
		fullPath := filepath.Join(stateMachine.commonFlags.OutputDir, artifact)
		_, err := os.Stat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				t.Errorf("File \"%s\" should exist, but does not", fullPath)
			}
		}

		// check it is the expected file type
		fileCommand := *exec.Command("file", fullPath)
		cmdOutput, err := fileCommand.CombinedOutput()
		asserter.AssertErrNil(err, true)
		if !strings.Contains(string(cmdOutput), fileType) {
			t.Errorf("File \"%s\" is the wrong file type. Expected \"%s\" but got \"%s\"",
				fullPath, fileType, string(cmdOutput))
		}
	}
}

// TestGerminate tests the germinate state and ensures some necessary packages are included
func TestGerminate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	testCases := []struct {
		name             string
		flavor           string
		seedURLs         []string
		seedNames        []string
		expectedPackages []string
		expectedSnaps    []string
		vcs              bool
	}{
		{
			"git",
			"ubuntu",
			[]string{"git://git.launchpad.net/~ubuntu-core-dev/ubuntu-seeds/+git/"},
			[]string{"server", "minimal", "standard", "cloud-image"},
			[]string{"python3", "sudo", "cloud-init", "ubuntu-server"},
			[]string{},
			true,
		},
		{
			"http",
			"ubuntu",
			[]string{"https://people.canonical.com/~ubuntu-archive/seeds/"},
			[]string{"server", "minimal", "standard", "cloud-image", "desktop"},
			[]string{"python3", "sudo", "cloud-init", "ubuntu-server"},
			[]string{"thunderbird"},
			false,
		},
		{
			"bzr+git",
			"ubuntu",
			[]string{"http://bazaar.launchpad.net/~ubuntu-mate-dev/ubuntu-seeds/",
				"git://git.launchpad.net/~ubuntu-core-dev/ubuntu-seeds/+git/",
				"https://people.canonical.com/~ubuntu-archive/seeds/",
			},
			[]string{"desktop", "desktop-common", "standard", "minimal"},
			[]string{"xorg", "wget", "ubuntu-minimal"},
			[]string{"thunderbird"},
			true,
		},
	}
	for _, tc := range testCases {
		t.Run("test_germinate_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			restoreCWD := testhelper.SaveCWD()
			defer restoreCWD()

			var stateMachine ClassicStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine

			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)

			t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

			hostArch := getHostArch()
			series := "noble"
			imageDef := imagedefinition.ImageDefinition{
				Architecture: hostArch,
				Series:       series,
				Rootfs: &imagedefinition.Rootfs{
					Flavor: tc.flavor,
					Mirror: "http://archive.ubuntu.com/ubuntu/",
					Seed: &imagedefinition.Seed{
						SeedURLs:   tc.seedURLs,
						SeedBranch: series,
						Names:      tc.seedNames,
						Vcs:        helper.BoolPtr(tc.vcs),
					},
				},
			}

			stateMachine.ImageDef = imageDef

			err = stateMachine.germinate()
			asserter.AssertErrNil(err, true)

			// spot check some packages that should remain seeded for a long time
			testHelperCheckGerminatedPackages(t, tc.expectedPackages, stateMachine.Packages)
			// spot check some snaps that should remain seeded for a long time
			testHelperCheckGerminatedSnaps(t, tc.expectedSnaps, stateMachine.Snaps)
		})
	}
}

func testHelperCheckGerminatedPackages(t *testing.T, expectedPackages []string, gotPackages []string) {
	for _, expectedPackage := range expectedPackages {
		found := false
		for _, seedPackage := range gotPackages {
			if expectedPackage == seedPackage {
				found = true
			}
		}
		if !found {
			t.Errorf("Expected to find %s in list of packages: %v",
				expectedPackage, gotPackages)
		}
	}
}

func testHelperCheckGerminatedSnaps(t *testing.T, expectedSnaps []string, gotSnaps []string) {
	for _, expectedSnap := range expectedSnaps {
		found := false
		for _, seedSnap := range gotSnaps {
			snapName := strings.Split(seedSnap, "=")[0]
			if expectedSnap == snapName {
				found = true
			}
		}
		if !found {
			t.Errorf("Expected to find %s in list of snaps: %v",
				expectedSnap, gotSnaps)
		}
	}
}

// TestFailedGerminate mocks function calls to test
// failure cases in the germinate state
func TestFailedGerminate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	// create a valid imageDefinition
	hostArch := getHostArch()
	hostSuite := getHostSuite()
	imageDef := imagedefinition.ImageDefinition{
		Architecture: hostArch,
		Series:       hostSuite,
		Rootfs: &imagedefinition.Rootfs{
			Flavor: "ubuntu",
			Mirror: "http://archive.ubuntu.com/ubuntu/",
			Seed: &imagedefinition.Seed{
				SeedURLs:   []string{"git://git.launchpad.net/~ubuntu-core-dev/ubuntu-seeds/+git/"},
				SeedBranch: hostSuite,
				Names:      []string{"server", "minimal", "standard", "cloud-image"},
				Vcs:        helper.BoolPtr(true),
			},
		},
	}
	stateMachine.ImageDef = imageDef

	// mock os.Mkdir
	osMkdir = mockMkdir
	t.Cleanup(func() {
		osMkdir = os.Mkdir
	})
	err = stateMachine.germinate()
	asserter.AssertErrContains(err, "Error creating germinate directory")
	osMkdir = os.Mkdir

	// Setup the exec.Command mock
	testCaseName = "TestFailedGerminate"
	execCommand = fakeExecCommand
	t.Cleanup(func() {
		execCommand = exec.Command
	})
	err = stateMachine.germinate()
	asserter.AssertErrContains(err, "Error running germinate command")
	execCommand = exec.Command

	// mock os.Open
	osOpen = mockOpen
	t.Cleanup(func() {
		osOpen = os.Open
	})
	err = stateMachine.germinate()
	asserter.AssertErrContains(err, "Error opening seed file")
	osOpen = os.Open

	os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
}

// TestBuildGadgetTreeGit tests the successful build of a gadget tree
func TestBuildGadgetTreeGit(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	// test the directory method
	d, err := os.Getwd()
	asserter.AssertErrNil(err, true)
	sourcePath := filepath.Join(d, "testdata", "gadget_source")
	sourcePath = "file://" + sourcePath
	imageDef := imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Gadget: &imagedefinition.Gadget{
			GadgetURL:  sourcePath,
			GadgetType: "directory",
		},
	}

	stateMachine.ImageDef = imageDef

	err = stateMachine.buildGadgetTree()
	asserter.AssertErrNil(err, true)

	// test the git method
	imageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Gadget: &imagedefinition.Gadget{
			GadgetURL:    "https://github.com/snapcore/pc-gadget",
			GadgetType:   "git",
			GadgetBranch: "classic",
		},
	}

	stateMachine.ImageDef = imageDef

	err = stateMachine.buildGadgetTree()
	asserter.AssertErrNil(err, true)
}

// TestBuildGadgetTreeDirectory tests the successful build of a gadget tree
func TestBuildGadgetTreeDirectory(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	asserter := helper.Asserter{T: t}
	saveCWD := testhelper.SaveCWD()
	defer saveCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine

	// need workdir set up for this
	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	// git clone the gadget into a /tmp dir
	gadgetDir, err := os.MkdirTemp("", "pc-gadget-")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(gadgetDir) })
	gitCloneCommand := *exec.Command(
		"git",
		"clone",
		"--depth",
		"1",
		"--branch",
		"classic",
		"https://github.com/snapcore/pc-gadget",
		gadgetDir,
	)
	err = gitCloneCommand.Run()
	asserter.AssertErrNil(err, true)

	// now set up the image definition to build from this directory
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Gadget: &imagedefinition.Gadget{
			GadgetURL:  fmt.Sprintf("file://%s", gadgetDir),
			GadgetType: "directory",
		},
	}

	err = stateMachine.buildGadgetTree()
	asserter.AssertErrNil(err, true)

	// now make sure the gadget.yaml is in the expected location
	// this was a bug reported by the CPC team
	err = stateMachine.prepareGadgetTree()
	asserter.AssertErrNil(err, true)
	err = stateMachine.loadGadgetYaml()
	asserter.AssertErrNil(err, true)
}

func TestStateMachine_buildGadgetTree_paths(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	asserter := helper.Asserter{T: t}
	// git clone the gadget into a /tmp dir
	originGadgetDir, err := os.MkdirTemp("", "pc-gadget-")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() {
		err = os.RemoveAll(originGadgetDir)
		if err != nil {
			t.Error(err)
		}
	})
	gitCloneCommand := *exec.Command(
		"git",
		"clone",
		"--depth",
		"1",
		"--branch",
		"classic",
		"https://github.com/snapcore/pc-gadget",
		originGadgetDir,
	)
	err = gitCloneCommand.Run()
	asserter.AssertErrNil(err, true)

	tmpDir, err := os.MkdirTemp("", "")
	t.Cleanup(func() {
		err := osRemoveAll(tmpDir)
		if err != nil {
			t.Error(err)
		}
	})

	testCases := []struct {
		name      string
		gadgetDir string
	}{
		{
			name:      "gadget URL poiting to an absolute dir",
			gadgetDir: originGadgetDir,
		},
		{
			name:      "gadget URL pointing to an absolute sub dir",
			gadgetDir: filepath.Join(tmpDir, "a", "b"),
		},
		{
			name:      "gadget URL pointing to a relative sub dir",
			gadgetDir: filepath.Join("a", "b"),
		},
	}

	for _, tc := range testCases {
		t.Run("test_build_gadget_tree_paths_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			restoreCWD := testhelper.SaveCWD()
			defer restoreCWD()

			var stateMachine ClassicStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine

			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)
			t.Cleanup(func() {
				err := os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
				if err != nil {
					t.Error(err)
				}
			})

			// move the original gadget dir to the desire location to test it will be found
			if originGadgetDir != tc.gadgetDir {
				fullGadgetDir := tc.gadgetDir
				if !filepath.IsAbs(tc.gadgetDir) {
					fullGadgetDir = filepath.Join(tmpDir, tc.gadgetDir)
				}

				err = os.MkdirAll(filepath.Dir(fullGadgetDir), 0777)
				asserter.AssertErrNil(err, true)

				err = os.Rename(originGadgetDir, fullGadgetDir)
				asserter.AssertErrNil(err, true)
				// move it back once the test is done
				t.Cleanup(func() {
					err := os.Rename(fullGadgetDir, originGadgetDir)
					if err != nil {
						t.Error(err)
					}
				})
			}

			// now set up the image definition to build from this directory
			stateMachine.ImageDef = imagedefinition.ImageDefinition{
				Architecture: getHostArch(),
				Series:       getHostSuite(),
				Gadget: &imagedefinition.Gadget{
					GadgetURL:  fmt.Sprintf("file://%s", tc.gadgetDir),
					GadgetType: "directory",
				},
			}

			err = stateMachine.setConfDefDir(filepath.Join(tmpDir, "image_definition.yaml"))
			asserter.AssertErrNil(err, true)

			err = stateMachine.buildGadgetTree()
			asserter.AssertErrNil(err, true)

			// now make sure the gadget.yaml is in the expected location
			// this was a bug reported by the CPC team
			err = stateMachine.prepareGadgetTree()
			asserter.AssertErrNil(err, true)
			err = stateMachine.loadGadgetYaml()
			asserter.AssertErrNil(err, true)
		})
	}
}

// TestGadgetGadgetTargets tests using alternate make targets with gadget builds
func TestGadgetGadgetTargets(t *testing.T) {
	testCases := []struct {
		name           string
		target         string
		expectedOutput string
	}{
		{
			"default",
			"",
			"make target test1",
		},
		{
			"test2",
			"test2",
			"make target test2",
		},
	}
	for _, tc := range testCases {
		t.Run("test_gadget_make_targets_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			restoreCWD := testhelper.SaveCWD()
			defer restoreCWD()

			var stateMachine ClassicStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine
			stateMachine.commonFlags.Debug = true

			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)

			wd, err := os.Getwd()
			asserter.AssertErrNil(err, true)
			gadgetSrc := filepath.Join(wd, "testdata", "gadget_source")
			imageDef := imagedefinition.ImageDefinition{
				Architecture: getHostArch(),
				Series:       getHostSuite(),
				Gadget: &imagedefinition.Gadget{
					GadgetURL:    fmt.Sprintf("file://%s", gadgetSrc),
					GadgetType:   "directory",
					GadgetTarget: tc.target,
				},
			}
			stateMachine.ImageDef = imageDef

			// capture stdout, build the gadget tree, and make
			// sure the expected output matches the make target
			stdout, restoreStdout, err := helper.CaptureStd(&os.Stdout)
			defer restoreStdout()
			asserter.AssertErrNil(err, true)

			err = stateMachine.buildGadgetTree()
			asserter.AssertErrNil(err, true)

			// restore stdout and examine what was printed
			restoreStdout()
			readStdout, err := io.ReadAll(stdout)
			asserter.AssertErrNil(err, true)
			if !strings.Contains(string(readStdout), tc.expectedOutput) {
				t.Errorf("Expected make output\n\"%s\"\nto contain the string \"%s\"",
					string(readStdout),
					tc.expectedOutput,
				)
			}
		})
	}
}

// TestFailedBuildGadgetTree tests failures in the  buildGadgetTree function
func TestFailedBuildGadgetTree(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	// mock os.MkdirAll
	osMkdir = mockMkdir
	t.Cleanup(func() {
		osMkdir = os.Mkdir
	})
	err = stateMachine.buildGadgetTree()
	asserter.AssertErrContains(err, "Error creating scratch/gadget")
	osMkdir = os.Mkdir

	// try to clone a repo that doesn't exist
	imageDef := imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Gadget: &imagedefinition.Gadget{
			GadgetURL:  "http://fakerepo.git",
			GadgetType: "git",
		},
	}
	stateMachine.ImageDef = imageDef

	err = stateMachine.buildGadgetTree()
	asserter.AssertErrContains(err, "Error cloning gadget repository")

	// try to copy a file that doesn't exist
	imageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Gadget: &imagedefinition.Gadget{
			GadgetURL:  "file:///fake/file/that/does/not/exist",
			GadgetType: "directory",
		},
	}
	stateMachine.ImageDef = imageDef

	err = stateMachine.buildGadgetTree()
	asserter.AssertErrContains(err, "Error reading gadget tree")

	// mock osutil.CopySpecialFile and run with /tmp as the gadget source
	imageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Gadget: &imagedefinition.Gadget{
			GadgetURL:  "file:///tmp",
			GadgetType: "directory",
		},
	}
	stateMachine.ImageDef = imageDef

	// mock osutil.CopySpecialFile
	osutilCopySpecialFile = mockCopySpecialFile
	t.Cleanup(func() {
		osutilCopySpecialFile = osutil.CopySpecialFile
	})
	err = stateMachine.buildGadgetTree()
	asserter.AssertErrContains(err, "Error copying gadget source")
	osutilCopySpecialFile = osutil.CopySpecialFile

	// run a "make" command that will fail by mocking exec.Command
	testCaseName = "TestFailedBuildGadgetTree"
	execCommand = fakeExecCommand
	t.Cleanup(func() {
		execCommand = exec.Command
	})
	wd, err := os.Getwd()
	asserter.AssertErrNil(err, true)
	sourcePath := filepath.Join(wd, "testdata", "gadget_source")
	sourcePath = "file://" + sourcePath
	imageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Gadget: &imagedefinition.Gadget{
			GadgetURL:  sourcePath,
			GadgetType: "directory",
		},
	}
	stateMachine.ImageDef = imageDef

	err = stateMachine.buildGadgetTree()
	asserter.AssertErrContains(err, "Error running \"make\" in gadget source")

	os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
}

// TestCreateChroot runs the createChroot step and spot checks that some
// expected files in the chroot exist
func TestCreateChroot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Rootfs: &imagedefinition.Rootfs{
			Pocket:            "proposed",
			SourcesListDeb822: helper.BoolPtr(true),
		},
	}

	err := helper.SetDefaults(&stateMachine.ImageDef)
	asserter.AssertErrNil(err, true)

	err = stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	err = stateMachine.createChroot()
	asserter.AssertErrNil(err, true)

	expectedFiles := []string{
		"etc",
		"home",
		"boot",
		"var",
	}
	for _, expectedFile := range expectedFiles {
		fullPath := filepath.Join(stateMachine.tempDirs.chroot, expectedFile)
		_, err := os.Stat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				t.Errorf("File \"%s\" should exist, but does not", fullPath)
			}
		}
	}

	// check that the hostname is set correctly
	hostnameFile := filepath.Join(stateMachine.tempDirs.chroot, "etc", "hostname")
	hostnameData, err := os.ReadFile(hostnameFile)
	asserter.AssertErrNil(err, true)
	if string(hostnameData) != "ubuntu\n" {
		t.Errorf("Expected hostname to be \"ubuntu\", but is \"%s\"", string(hostnameData))
	}

	// check that the resolv.conf file was truncated
	resolvConfFile := filepath.Join(stateMachine.tempDirs.chroot, "etc", "resolv.conf")
	resolvConfData, err := os.ReadFile(resolvConfFile)
	asserter.AssertErrNil(err, true)
	if string(resolvConfData) != "" {
		t.Errorf("Expected resolv.conf to be empty, but is \"%s\"", string(resolvConfData))
	}

	// check if components and pocket correctly setup in /etc/apt/sources.list.d/ubuntu.sources
	aptDeb822SourcesListBytes, err := os.ReadFile(filepath.Join(stateMachine.tempDirs.chroot, "etc", "apt", "sources.list.d", "ubuntu.sources"))
	asserter.AssertErrNil(err, true)
	wantAptDeb822SourcesList := `## Ubuntu distribution repository
##
## The following settings can be adjusted to configure which packages to use from Ubuntu.
## Mirror your choices (except for URIs and Suites) in the security section below to
## ensure timely security updates.
##
## Types: Append deb-src to enable the fetching of source package.
## URIs: A URL to the repository (you may add multiple URLs)
## Suites: The following additional suites can be configured
##   <name>-updates   - Major bug fix updates produced after the final release of the
##                      distribution.
##   <name>-backports - software from this repository may not have been tested as
##                      extensively as that contained in the main release, although it includes
##                      newer versions of some applications which may provide useful features.
##                      Also, please note that software in backports WILL NOT receive any review
##                      or updates from the Ubuntu security team.
## Components: Aside from main, the following components can be added to the list
##   restricted  - Software that may not be under a free license, or protected by patents.
##   universe    - Community maintained packages. Software in this repository receives maintenance
##                 from volunteers in the Ubuntu community, or a 10 year security maintenance
##                 commitment from Canonical when an Ubuntu Pro subscription is attached.
##   multiverse  - Community maintained of restricted. Software from this repository is
##                 ENTIRELY UNSUPPORTED by the Ubuntu team, and may not be under a free
##                 licence. Please satisfy yourself as to your rights to use the software.
##                 Also, please note that software in multiverse WILL NOT receive any
##                 review or updates from the Ubuntu security team.
##
## See the sources.list(5) manual page for further settings.
Types: deb
URIs: http://archive.ubuntu.com/ubuntu/
Suites: jammy jammy-updates jammy-proposed
Components: main restricted
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg

## Ubuntu security updates. Aside from URIs and Suites,
## this should mirror your choices in the previous section.
Types: deb
URIs: http://security.ubuntu.com/ubuntu/
Suites: jammy-security
Components: main restricted
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg

`
	asserter.AssertEqual(wantAptDeb822SourcesList, string(aptDeb822SourcesListBytes))

}

// TestFailedCreateChroot tests failure cases in createChroot
func TestFailedCreateChroot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Rootfs: &imagedefinition.Rootfs{
			SourcesListDeb822: helper.BoolPtr(false),
		},
	}

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	// mock os.Mkdir
	osMkdir = mockMkdir
	t.Cleanup(func() {
		osMkdir = os.Mkdir
	})
	err = stateMachine.createChroot()
	asserter.AssertErrContains(err, "Failed to create chroot")
	osMkdir = os.Mkdir

	// Setup the exec.Command mock
	testCaseName = "TestFailedCreateChroot"
	execCommand = fakeExecCommand
	t.Cleanup(func() {
		execCommand = exec.Command
	})
	err = stateMachine.createChroot()
	asserter.AssertErrContains(err, "Error running debootstrap command")
	execCommand = exec.Command

	// Check if failure of open hostname file is detected

	os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	err = stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	// Prepare a fallthrough debootstrap
	testCaseName = "TestFailedCreateChrootNoHostname"
	execCommand = fakeExecCommand
	t.Cleanup(func() {
		execCommand = exec.Command
	})
	osOpenFile = mockOpenFile
	t.Cleanup(func() {
		osOpenFile = os.OpenFile
	})

	err = stateMachine.createChroot()
	asserter.AssertErrContains(err, "unable to open hostname file")

	osOpenFile = os.OpenFile
	execCommand = exec.Command

	// Check if failure of truncation is detected

	// Clean the work directory
	os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	err = stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	// Prepare a fallthrough debootstrap
	testCaseName = "TestFailedCreateChrootSkip"
	osTruncate = mockTruncate
	t.Cleanup(func() {
		osTruncate = os.Truncate
	})
	err = stateMachine.createChroot()
	asserter.AssertErrContains(err, "Error truncating resolv.conf")
	osTruncate = os.Truncate
	execCommand = exec.Command

	os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
}

// TestStateMachine_installPackages_checkcmds checks commands to install packages order is ok
func TestStateMachine_installPackages_checkcmds(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.commonFlags.Debug = true
	stateMachine.parent = &stateMachine
	stateMachine.commonFlags.OutputDir = "/tmp"

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)
	err = os.MkdirAll(stateMachine.tempDirs.chroot, 0755)
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	// create an /usr/sbin/policy-rc.d in the chroot
	err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.chroot, "usr", "sbin"), 0755)
	asserter.AssertErrNil(err, true)
	_, err = os.Create(filepath.Join(stateMachine.tempDirs.chroot, "usr", "sbin", "policy-rc.d"))
	asserter.AssertErrNil(err, true)

	// create an /sbin/start-stop-daemon in the chroot
	err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.chroot, "sbin"), 0755)
	asserter.AssertErrNil(err, true)
	_, err = os.Create(filepath.Join(stateMachine.tempDirs.chroot, "sbin", "start-stop-daemon"))
	asserter.AssertErrNil(err, true)

	// create an /sbin/initctl in the chroot
	_, err = os.Create(filepath.Join(stateMachine.tempDirs.chroot, "sbin", "initctl"))
	asserter.AssertErrNil(err, true)

	mockCmder := NewMockExecCommand()

	execCommand = mockCmder.Command
	t.Cleanup(func() { execCommand = exec.Command })

	stdout, restoreStdout, err := helper.CaptureStd(&os.Stdout)
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { restoreStdout() })

	helperBackupAndCopyResolvConf = mockBackupAndCopyResolvConfSuccess
	t.Cleanup(func() {
		helperBackupAndCopyResolvConf = helper.BackupAndCopyResolvConf
	})

	err = stateMachine.installPackages()
	asserter.AssertErrNil(err, true)

	restoreStdout()
	readStdout, err := io.ReadAll(stdout)
	asserter.AssertErrNil(err, true)

	expectedCmds := []*regexp.Regexp{
		regexp.MustCompile("^mount -t devtmpfs devtmpfs-build /tmp.*/chroot/dev$"),
		regexp.MustCompile("^mount -t devpts devpts-build -o nodev,nosuid /tmp.*/chroot/dev/pts$"),
		regexp.MustCompile("^mount -t proc proc-build /tmp.*/chroot/proc$"),
		regexp.MustCompile("^mount -t sysfs sysfs-build /tmp.*/chroot/sys$"),
		regexp.MustCompile("^mount --bind .*/scratch/run.* .*/chroot/run$"),
		regexp.MustCompile("^chroot /tmp.*/chroot dpkg-divert"),
		regexp.MustCompile("^chroot /tmp.*/chroot apt update$"),
		regexp.MustCompile("^chroot /tmp.*/chroot apt install --assume-yes --quiet --option=Dpkg::options::=--force-unsafe-io --option=Dpkg::Options::=--force-confold$"),
		regexp.MustCompile("^chroot /tmp.*/chroot dpkg-divert --remove"),
		regexp.MustCompile("^udevadm settle$"),
		regexp.MustCompile("^mount --make-rprivate /tmp.*/chroot/run$"),
		regexp.MustCompile("^umount --recursive /tmp.*/chroot/run$"),
		regexp.MustCompile("^mount --make-rprivate /tmp.*/chroot/sys$"),
		regexp.MustCompile("^umount --recursive /tmp.*/chroot/sys$"),
		regexp.MustCompile("^mount --make-rprivate /tmp.*/chroot/proc$"),
		regexp.MustCompile("^umount --recursive /tmp.*/chroot/proc$"),
		regexp.MustCompile("^mount --make-rprivate /tmp.*/chroot/dev/pts$"),
		regexp.MustCompile("^umount --recursive /tmp.*/chroot/dev/pts$"),
		regexp.MustCompile("^mount --make-rprivate /tmp.*/chroot/dev$"),
		regexp.MustCompile("^umount --recursive /tmp.*/chroot/dev$"),
	}

	gotCmds := strings.Split(strings.TrimSpace(string(readStdout)), "\n")
	if len(expectedCmds) != len(gotCmds) {
		t.Fatalf("%v commands to be executed, expected %v commands. Got: %v", len(gotCmds), len(expectedCmds), gotCmds)
	}

	for i, gotCmd := range gotCmds {
		expected := expectedCmds[i]

		if !expected.Match([]byte(gotCmd)) {
			t.Errorf("Cmd \"%v\" not matching. Expected %v\n", gotCmd, expected.String())
		}
	}
}

// TestStateMachine_installPackages_checkcmds checks commands to install packages order is ok when failing
func TestStateMachine_installPackages_checkcmds_failing(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.commonFlags.Debug = true
	stateMachine.parent = &stateMachine
	stateMachine.commonFlags.OutputDir = "/tmp"

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	mockCmder := NewMockExecCommand()

	execCommand = mockCmder.Command
	t.Cleanup(func() { execCommand = exec.Command })

	stdout, restoreStdout, err := helper.CaptureStd(&os.Stdout)
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { restoreStdout() })

	helperBackupAndCopyResolvConf = mockBackupAndCopyResolvConfSuccess
	t.Cleanup(func() {
		helperBackupAndCopyResolvConf = helper.BackupAndCopyResolvConf
	})

	osMkdirTemp = mockMkdirTemp
	t.Cleanup(func() {
		osMkdirTemp = os.MkdirTemp
	})

	err = stateMachine.installPackages()
	asserter.AssertErrContains(err, "Test error")

	restoreStdout()
	readStdout, err := io.ReadAll(stdout)
	asserter.AssertErrNil(err, true)

	gotCmds := strings.Split(strings.TrimSpace(string(readStdout)), "\n")
	// Clean empty commands
	for i, cmd := range gotCmds {
		if len(cmd) == 0 {
			copy(gotCmds[i:], gotCmds[i+1:])
			gotCmds[len(gotCmds)-1] = ""
			gotCmds = gotCmds[:len(gotCmds)-1]
		}
	}

	if len(gotCmds) != 0 {
		t.Fatalf("%v commands to be executed, expected no commands. Got: %v", len(gotCmds), gotCmds)
	}
}

// TestStateMachine_installPackages_fail tests failure cases in installPackages
func TestStateMachine_installPackages_fail(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Rootfs:       &imagedefinition.Rootfs{},
		Customization: &imagedefinition.Customization{
			ExtraPackages: []*imagedefinition.Package{
				{
					PackageName: "test1",
				},
			},
		},
	}

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	// create an /etc/resolv.conf in the chroot
	err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.chroot, "etc"), 0755)
	asserter.AssertErrNil(err, true)
	_, err = os.Create(filepath.Join(stateMachine.tempDirs.chroot, "etc", "resolv.conf"))
	asserter.AssertErrNil(err, true)

	osMkdirTemp = mockMkdirTemp
	t.Cleanup(func() {
		osMkdirTemp = os.MkdirTemp
	})
	err = stateMachine.installPackages()
	asserter.AssertErrContains(err, "Error making temporary directory for mountpoint")
	osMkdirTemp = os.MkdirTemp

	// Setup the exec.Command mock
	testCaseName = "TestStateMachine_installPackages_fail"
	execCommand = fakeExecCommand
	t.Cleanup(func() {
		execCommand = exec.Command
	})
	err = stateMachine.installPackages()
	asserter.AssertErrContains(err, "Error running command")
	execCommand = exec.Command

	// delete the backed up resolv.conf to trigger another backup
	err = os.Remove(filepath.Join(stateMachine.tempDirs.chroot, "etc", "resolv.conf.tmp"))
	asserter.AssertErrNil(err, true)
	// mock helper.BackupAndCopyResolvConf
	helperBackupAndCopyResolvConf = mockBackupAndCopyResolvConfFail
	t.Cleanup(func() {
		helperBackupAndCopyResolvConf = helper.BackupAndCopyResolvConf
	})
	err = stateMachine.installPackages()
	asserter.AssertErrContains(err, "Error setting up /etc/resolv.conf")
	helperBackupAndCopyResolvConf = helper.BackupAndCopyResolvConf

	osMkdirAll = mockMkdirAll
	t.Cleanup(func() {
		osMkdirAll = os.MkdirAll
	})
	err = stateMachine.installPackages()
	asserter.AssertErrContains(err, "Error creating policy-rc.d dir")
	osMkdirAll = os.MkdirAll

	osWriteFile = mockWriteFile
	t.Cleanup(func() {
		osWriteFile = os.WriteFile
	})
	err = stateMachine.installPackages()
	asserter.AssertErrContains(err, "Error writing to policy-rc.d")
	osWriteFile = os.WriteFile

	osRename = mockRename
	t.Cleanup(func() {
		osRename = os.Rename
	})
	err = stateMachine.installPackages()
	asserter.AssertErrContains(err, "Error moving file ")
	osRename = os.Rename

}

// Test_generateMountPointCmds_fail tests when generateMountPointCmds fails
func Test_generateMountPointCmds_fail(t *testing.T) {
	asserter := helper.Asserter{T: t}

	tmpDirPath := filepath.Join("/tmp", "test_failed_set_conf_dir")
	err := os.Mkdir(tmpDirPath, 0755)
	t.Cleanup(func() {
		os.RemoveAll(tmpDirPath)
	})
	asserter.AssertErrNil(err, true)

	mountPoints := []*mountPoint{
		{
			src:      "devtmpfs-build",
			basePath: tmpDirPath,
			relpath:  "/dev",
			typ:      "devtmpfs",
		},
		{
			src:      "doesnotexists",
			basePath: "/doesnotexists",
			relpath:  "/doesnotexists",
			typ:      "devpts",
			bind:     true,
			opts:     []string{"nodev", "nosuid"},
		},
	}

	gotAllMountCmds, gotAllUmountCmds, err := generateMountPointCmds(mountPoints, tmpDirPath)
	asserter.AssertErrContains(err, "Error preparing mountpoint")
	asserter.AssertEqual(nil, gotAllMountCmds)
	asserter.AssertEqual(nil, gotAllUmountCmds)

}

// TestCustomizeFstab tests functionality of the customizeFstab function
func TestCustomizeFstab(t *testing.T) {
	testCases := []struct {
		name          string
		fstab         []*imagedefinition.Fstab
		expectedFstab string
		existingFstab string
	}{
		{
			name: "one entry to an empty fstab",
			fstab: []*imagedefinition.Fstab{
				{
					Label:        "writable",
					Mountpoint:   "/",
					FSType:       "ext4",
					MountOptions: "defaults",
					Dump:         true,
					FsckOrder:    1,
				},
			},
			expectedFstab: `LABEL=writable	/	ext4	defaults	1	1
`,
		},
		{
			name: "one entry to a non-empty fstab",
			fstab: []*imagedefinition.Fstab{
				{
					Label:        "writable",
					Mountpoint:   "/",
					FSType:       "ext4",
					MountOptions: "defaults",
					Dump:         true,
					FsckOrder:    1,
				},
			},
			expectedFstab: `LABEL=writable	/	ext4	defaults	1	1
`,
			existingFstab: `LABEL=xxx / ext4 discard,errors=remount-ro 0 1`,
		},
		{
			name: "two entries",
			fstab: []*imagedefinition.Fstab{
				{
					Label:        "writable",
					Mountpoint:   "/",
					FSType:       "ext4",
					MountOptions: "defaults",
					Dump:         false,
					FsckOrder:    1,
				},
				{
					Label:        "system-boot",
					Mountpoint:   "/boot/firmware",
					FSType:       "vfat",
					MountOptions: "defaults",
					Dump:         false,
					FsckOrder:    1,
				},
			},
			expectedFstab: `LABEL=writable	/	ext4	defaults	0	1
LABEL=system-boot	/boot/firmware	vfat	defaults	0	1
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			restoreCWD := testhelper.SaveCWD()
			defer restoreCWD()

			var stateMachine ClassicStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine
			stateMachine.ImageDef = imagedefinition.ImageDefinition{
				Architecture: getHostArch(),
				Series:       getHostSuite(),
				Rootfs:       &imagedefinition.Rootfs{},
				Customization: &imagedefinition.Customization{
					Fstab: tc.fstab,
				},
			}

			// set the defaults for the imageDef
			err := helper.SetDefaults(&stateMachine.ImageDef)
			asserter.AssertErrNil(err, true)

			err = stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)

			// create the <chroot>/etc directory
			err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.chroot, "etc"), 0644)
			asserter.AssertErrNil(err, true)

			fstabPath := filepath.Join(stateMachine.tempDirs.chroot, "etc", "fstab")

			// simulate an already existing fstab file
			if len(tc.existingFstab) != 0 {
				err = osWriteFile(fstabPath, []byte(tc.existingFstab), 0644)
				asserter.AssertErrNil(err, true)
			}

			// customize the fstab, ensure no errors, and check the contents
			err = stateMachine.customizeFstab()
			asserter.AssertErrNil(err, true)

			fstabBytes, err := os.ReadFile(fstabPath)
			asserter.AssertErrNil(err, true)

			if string(fstabBytes) != tc.expectedFstab {
				t.Errorf("Expected fstab contents \"%s\", but got \"%s\"",
					tc.expectedFstab, string(fstabBytes))
			}
		})
	}
}

// TestStateMachine_customizeFstab_fail tests failures in the customizeFstab function
func TestStateMachine_customizeFstab_fail(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Rootfs:       &imagedefinition.Rootfs{},
		Customization: &imagedefinition.Customization{
			Fstab: []*imagedefinition.Fstab{
				{
					Label:        "writable",
					Mountpoint:   "/",
					FSType:       "ext4",
					MountOptions: "defaults",
					Dump:         false,
					FsckOrder:    1,
				},
			},
		},
	}

	osOpenFile = mockOpenFile
	t.Cleanup(func() {
		osOpenFile = os.OpenFile
	})
	err := stateMachine.customizeFstab()
	asserter.AssertErrContains(err, "Error opening fstab")
}

// TestGenerateRootfsTarball tests that a rootfs tarball is generated
// when appropriate
func TestGenerateRootfsTarball(t *testing.T) {
	testCases := []struct {
		name     string // the name will double as the compression type
		tarPath  string
		fileType string
		// Define an interval since we cannot predict the exact
		// size of the resulting archive due to changing atime/ctime
		minArchiveSize int64
		maxArchiveSize int64
	}{
		{
			name:           "uncompressed",
			tarPath:        "test_generate_rootfs_tarball.tar",
			fileType:       "tar archive",
			minArchiveSize: 61440,
			maxArchiveSize: 61440, // 92160 without --sparse option
		},
		{
			name:           "bzip2",
			tarPath:        "test_generate_rootfs_tarball.tar.bz2",
			fileType:       "bzip2 compressed data",
			minArchiveSize: 32100,
			maxArchiveSize: 32300,
		},
		{
			name:           "gzip",
			tarPath:        "test_generate_rootfs_tarball.tar.gz",
			fileType:       "gzip compressed data",
			minArchiveSize: 31600,
			maxArchiveSize: 31900,
		},
		{
			name:           "xz",
			tarPath:        "test_generate_rootfs_tarball.tar.xz",
			fileType:       "XZ compressed data",
			minArchiveSize: 31700,
			maxArchiveSize: 31910,
		},
		{
			name:           "zstd",
			tarPath:        "test_generate_rootfs_tarball.tar.zst",
			fileType:       "Zstandard compressed data",
			minArchiveSize: 31400,
			maxArchiveSize: 31700,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			restoreCWD := testhelper.SaveCWD()
			defer restoreCWD()

			var stateMachine ClassicStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine
			stateMachine.ImageDef = imagedefinition.ImageDefinition{
				Architecture: getHostArch(),
				Series:       getHostSuite(),
				Rootfs:       &imagedefinition.Rootfs{},
				Artifacts: &imagedefinition.Artifact{
					RootfsTar: &imagedefinition.RootfsTar{
						RootfsTarName: tc.tarPath,
						Compression:   tc.name,
					},
				},
			}

			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)
			stateMachine.commonFlags.OutputDir = stateMachine.stateMachineFlags.WorkDir

			t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

			// Copy a rootfs so the tar is not empty
			rootfsSource := filepath.Join("testdata", "rootfs", "root")
			err = osutilCopyFile(rootfsSource, stateMachine.stateMachineFlags.WorkDir, osutil.CopyFlagPreserveAll)
			asserter.AssertErrNil(err, true)

			// Make sure the root dir contains a sparse file
			sparseFilePath := filepath.Join(stateMachine.tempDirs.rootfs, "bin", "sparseablefile")
			sparseFilePathTemp := filepath.Join(stateMachine.tempDirs.rootfs, "bin", "sparseablefiletmp")
			sparsifyOutputStep1, err := exec.Command("cp", "--sparse=always", sparseFilePath, sparseFilePathTemp).CombinedOutput()
			if err != nil {
				t.Error(string(sparsifyOutputStep1))
				asserter.AssertErrNil(err, true)
			}
			sparsifyOutputStep2, err := exec.Command("mv", sparseFilePathTemp, sparseFilePath).CombinedOutput()
			if err != nil {
				t.Error(string(sparsifyOutputStep2))
				asserter.AssertErrNil(err, true)
			}

			err = stateMachine.generateRootfsTarball()
			asserter.AssertErrNil(err, true)

			// make sure tar archive exists and is the correct compression type
			_, err = os.Stat(filepath.Join(stateMachine.stateMachineFlags.WorkDir, tc.tarPath))
			if err != nil {
				t.Errorf("File %s should be in workdir, but is missing", tc.tarPath)
			}

			fullPath := filepath.Join(stateMachine.commonFlags.OutputDir, tc.tarPath)
			fileCommand := exec.Command("file", fullPath)
			cmdOutput, err := fileCommand.CombinedOutput()
			asserter.AssertErrNil(err, true)
			if !strings.Contains(string(cmdOutput), tc.fileType) {
				t.Errorf("File \"%s\" is the wrong file type. Expected \"%s\" but got \"%s\"",
					fullPath, tc.fileType, string(cmdOutput))
			}

			fileInfo, err := os.Stat(fullPath)
			asserter.AssertErrNil(err, true)

			if fileInfo.Size() < tc.minArchiveSize {
				asserter.Errorf("Archive too small.\ngot: %d\nwant a minimum of: %d", fileInfo.Size(), tc.minArchiveSize)
			}

			if fileInfo.Size() > tc.maxArchiveSize {
				asserter.Errorf("Archive too big.\ngot: %d\nwant a maximum of: %d", fileInfo.Size(), tc.maxArchiveSize)
			}
		})
	}
}

// TestFailedMakeQcow2Img tests failures in the makeQcow2Img function
func TestFailedMakeQcow2Img(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Artifacts: &imagedefinition.Artifact{
			Qcow2: &[]imagedefinition.Qcow2{
				{
					Qcow2Name: "test.qcow2",
				},
			},
		},
	}

	// Setup the exec.Command mock
	testCaseName = "TestFailedMakeQcow2Image"
	execCommand = fakeExecCommand
	defer func() {
		execCommand = exec.Command
	}()

	err := stateMachine.makeQcow2Img()
	asserter.AssertErrContains(err, "Error running command")
	asserter.AssertErrContains(err, "qemu-img convert")
}

// TestPreseedResetChroot tests that calling prepareClassicImage on a
// preseeded chroot correctly resets the chroot and preseeds over it
func TestPreseedResetChroot(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.Snaps = []string{"lxd"}
	stateMachine.commonFlags.Channel = "stable"
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Rootfs: &imagedefinition.Rootfs{
			Archive: "ubuntu",
		},
		Customization: &imagedefinition.Customization{
			ExtraPackages: []*imagedefinition.Package{
				{
					PackageName: "squashfs-tools",
				},
				{
					PackageName: "snapd",
				},
			},
			ExtraSnaps: []*imagedefinition.Snap{
				{
					SnapName: "hello",
				},
				{
					SnapName: "core",
				},
				{
					SnapName: "core20",
				},
			},
		},
	}

	err := helper.SetDefaults(&stateMachine.ImageDef)
	asserter.AssertErrNil(err, true)

	err = stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	err = getBasicChroot(stateMachine.StateMachine)
	asserter.AssertErrNil(err, true)

	// install the packages that snap-preseed needs
	err = stateMachine.installPackages()
	asserter.AssertErrNil(err, true)

	// first call prepareClassicImage to eventually preseed it
	err = stateMachine.prepareClassicImage()
	asserter.AssertErrNil(err, true)

	// now preseed the chroot
	err = stateMachine.preseedClassicImage()
	asserter.AssertErrNil(err, true)

	// set up a new set of snaps to be installed
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Customization: &imagedefinition.Customization{
			ExtraSnaps: []*imagedefinition.Snap{
				{
					SnapName: "ubuntu-image",
				},
			},
		},
	}

	// call prepareClassicImage again to trigger the reset
	err = stateMachine.prepareClassicImage()
	asserter.AssertErrNil(err, true)

	// make sure the snaps from both prepares are present
	expectedSnaps := []string{"lxd", "hello", "ubuntu-image"}
	for _, expectedSnap := range expectedSnaps {
		snapGlobs, err := filepath.Glob(filepath.Join(stateMachine.tempDirs.chroot,
			"var", "lib", "snapd", "seed", "snaps", fmt.Sprintf("%s*.snap", expectedSnap)))
		asserter.AssertErrNil(err, true)
		if len(snapGlobs) == 0 {
			t.Errorf("expected snap %s to exist in the chroot but it does not", expectedSnap)
		}
	}
}

// TestFailedUpdateBootloader tests failures in the updateBootloader function
func TestFailedUpdateBootloader(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Gadget:       &imagedefinition.Gadget{},
	}

	// set up work dir
	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	// first, test that updateBootloader fails when the rootfs partition
	// has not been found in earlier steps
	stateMachine.RootfsPartNum = -1
	stateMachine.RootfsVolName = ""
	err = stateMachine.updateBootloader()
	asserter.AssertErrContains(err, "Error: could not determine partition number of the root filesystem")

	// place a test gadget tree in the scratch directory so we don't
	// have to build one
	gadgetDir := filepath.Join(stateMachine.tempDirs.scratch, "gadget")
	err = os.MkdirAll(gadgetDir, 0755)
	asserter.AssertErrNil(err, true)

	gadgetSource := filepath.Join("testdata", "gadget_tree")
	gadgetDest := filepath.Join(gadgetDir, "install")
	err = osutil.CopySpecialFile(gadgetSource, gadgetDest)
	asserter.AssertErrNil(err, true)
	// also copy gadget.yaml to the root of the scratch/gadget dir
	err = osutil.CopyFile(
		filepath.Join(gadgetDest, "meta", "gadget.yaml"),
		filepath.Join(gadgetDest, "gadget.yaml"),
		osutil.CopyFlagDefault,
	)
	asserter.AssertErrNil(err, true)

	// prepare state in such a way that the rootfs partition was found in
	// earlier steps
	stateMachine.RootfsPartNum = 3
	stateMachine.RootfsVolName = "pc"

	// parse gadget.yaml and run updateBootloader with the mocked os.Mkdir
	err = stateMachine.prepareGadgetTree()
	asserter.AssertErrNil(err, true)
	err = stateMachine.loadGadgetYaml()
	asserter.AssertErrNil(err, true)
	osMkdir = mockMkdir
	t.Cleanup(func() {
		osMkdir = os.Mkdir
	})

	err = stateMachine.updateBootloader()
	asserter.AssertErrContains(err, "Error creating scratch/loopback directory")
}

// TestUnsupportedBootloader tests that a warning is thrown if the
// bootloader specified in gadget.yaml is not supported
func TestUnsupportedBootloader(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Gadget:       &imagedefinition.Gadget{},
	}

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	// place a test gadget tree in the scratch directory so we don't
	// have to build one
	gadgetDir := filepath.Join(stateMachine.tempDirs.scratch, "gadget")
	err = os.MkdirAll(gadgetDir, 0755)
	asserter.AssertErrNil(err, true)

	gadgetSource := filepath.Join("testdata", "gadget_tree")
	gadgetDest := filepath.Join(gadgetDir, "install")
	err = osutil.CopySpecialFile(gadgetSource, gadgetDest)
	asserter.AssertErrNil(err, true)
	// also copy gadget.yaml to the root of the scratch/gadget dir
	err = osutil.CopyFile(
		filepath.Join(gadgetDest, "meta", "gadget.yaml"),
		filepath.Join(gadgetDest, "gadget.yaml"),
		osutil.CopyFlagDefault,
	)
	asserter.AssertErrNil(err, true)
	// parse gadget.yaml
	err = stateMachine.prepareGadgetTree()
	asserter.AssertErrNil(err, true)
	err = stateMachine.loadGadgetYaml()
	asserter.AssertErrNil(err, true)

	// prepare state in such a way that the rootfs partition was found in
	// earlier steps
	stateMachine.RootfsPartNum = 3
	stateMachine.RootfsVolName = "pc"

	// set the bootloader for the volume to "test"
	stateMachine.GadgetInfo.Volumes["pc"].Bootloader = "test"

	// capture stdout, run updateBootloader and make sure the states were printed
	stdout, restoreStdout, err := helper.CaptureStd(&os.Stdout)
	defer restoreStdout()
	asserter.AssertErrNil(err, true)

	err = stateMachine.updateBootloader()
	asserter.AssertErrNil(err, true)

	// restore stdout and examine what was printed
	restoreStdout()
	readStdout, err := io.ReadAll(stdout)
	asserter.AssertErrNil(err, true)
	if !strings.Contains(string(readStdout), "WARNING: updating bootloader test not yet supported") {
		t.Error("Warning for unsupported bootloader not printed")
	}
}

// TestPreseedClassicImage unit tests the prepareClassicImage function
func TestPreseedClassicImage(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.Snaps = []string{"lxd"}
	stateMachine.commonFlags.Channel = "stable"
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Rootfs: &imagedefinition.Rootfs{
			Archive: "ubuntu",
		},
		Customization: &imagedefinition.Customization{
			ExtraPackages: []*imagedefinition.Package{
				{
					PackageName: "squashfs-tools",
				},
				{
					PackageName: "snapd",
				},
			},
			ExtraSnaps: []*imagedefinition.Snap{
				{
					SnapName: "hello",
				},
				{
					SnapName: "core",
				},
				{
					SnapName: "core20",
				},
			},
		},
	}

	err := helper.SetDefaults(&stateMachine.ImageDef)
	asserter.AssertErrNil(err, true)

	err = stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	err = getBasicChroot(stateMachine.StateMachine)
	asserter.AssertErrNil(err, true)

	// install the packages that snap-preseed needs
	err = stateMachine.installPackages()
	asserter.AssertErrNil(err, true)

	// first call prepareClassicImage
	err = stateMachine.prepareClassicImage()
	asserter.AssertErrNil(err, true)

	// now preseed the chroot
	err = stateMachine.preseedClassicImage()
	asserter.AssertErrNil(err, true)

	// make sure the snaps are fully preseeded
	expectedSnaps := []string{"lxc", "lxd", "hello"}
	for _, expectedSnap := range expectedSnaps {
		snapPath := filepath.Join(stateMachine.tempDirs.chroot, "snap", "bin", expectedSnap)
		_, err := os.Stat(snapPath)
		if err != nil {
			t.Errorf("File %s should be in chroot, but is missing", snapPath)
		}
	}
}

// TestFailedPreseedClassicImage tests failures in the preseedClassicImage function
func TestFailedPreseedClassicImage(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	// mock os.MkdirAll
	osMkdirAll = mockMkdirAll
	t.Cleanup(func() {
		osMkdirAll = os.MkdirAll
	})
	err = stateMachine.preseedClassicImage()
	asserter.AssertErrContains(err, "Error creating mountpoint")
	osMkdirAll = os.MkdirAll

	testCaseName = "TestFailedPreseedClassicImage"
	execCommand = fakeExecCommand
	t.Cleanup(func() {
		execCommand = exec.Command
	})
	err = stateMachine.preseedClassicImage()
	asserter.AssertErrContains(err, "Error running command")
	execCommand = exec.Command
}

// TestStateMachine_defaultLocale tests that the default locale is set
func TestStateMachine_defaultLocale(t *testing.T) {
	testCases := []struct {
		name           string
		localeContents string
		localeExpected string
	}{
		{
			"no_locale",
			"",
			"# Default Ubuntu locale\nLANG=C.UTF-8\n",
		},
		{
			"locale_set",
			"LANG=en_US.UTF-8\n",
			"LANG=en_US.UTF-8\n",
		},
		{
			"locale_set_non_lang",
			"LC_ALL=en_US.UTF-8\n",
			"LC_ALL=en_US.UTF-8\n",
		},
		{
			"locale_set_with_comment",
			"# some comment\nLANG=en_US.UTF-8\n",
			"# some comment\nLANG=en_US.UTF-8\n",
		},
		{
			"no_locale_with_comment",
			"# some comment\n",
			"# Default Ubuntu locale\nLANG=C.UTF-8\n",
		},
		{
			"no_locale_with_comment_locale",
			"# LANG=en_US.UTF-8",
			"# Default Ubuntu locale\nLANG=C.UTF-8\n",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			var stateMachine ClassicStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine

			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)

			t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

			// create the <chroot>/etc/default directory
			defaultPath := filepath.Join(stateMachine.tempDirs.chroot, "etc", "default")
			err = os.MkdirAll(defaultPath, 0744)
			asserter.AssertErrNil(err, true)

			// create the <chroot>/etc/default/locale file
			localePath := filepath.Join(defaultPath, "locale")
			err = os.WriteFile(localePath, []byte(tc.localeContents), 0600)
			asserter.AssertErrNil(err, true)

			// call the function under test
			err = stateMachine.setDefaultLocale()
			asserter.AssertErrNil(err, true)

			// read the locale file and make sure it matches the expected contents
			localeBytes, err := os.ReadFile(localePath)
			asserter.AssertErrNil(err, true)

			if string(localeBytes) != tc.localeExpected {
				t.Errorf("Expected locale contents \"%s\", but got \"%s\"",
					tc.localeExpected, string(localeBytes))
			}
		})
	}
}

// TestStateMachine_defaultLocaleFailures tests failures in the setDefaultLocale function
func TestStateMachine_defaultLocaleFailures(t *testing.T) {
	asserter := helper.Asserter{T: t}

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	// check failure in MkDirAll
	osMkdirAll = mockMkdirAll
	t.Cleanup(func() {
		osMkdirAll = os.MkdirAll
	})
	err = stateMachine.setDefaultLocale()
	asserter.AssertErrContains(err, "Error creating default directory")
	osMkdirAll = os.MkdirAll

	// check failure in WriteFile
	osWriteFile = mockWriteFile
	t.Cleanup(func() {
		osWriteFile = os.WriteFile
	})
	err = stateMachine.setDefaultLocale()
	asserter.AssertErrContains(err, "Error writing to locale file")
	osWriteFile = os.WriteFile
}

func TestClassicStateMachine_cleanRootfs_real_rootfs(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	t.Cleanup(restoreCWD)

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.Snaps = []string{"lxd"}
	stateMachine.commonFlags.Channel = "stable"
	stateMachine.commonFlags.Debug = true
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: getHostArch(),
		Series:       getHostSuite(),
		Rootfs: &imagedefinition.Rootfs{
			Archive: "ubuntu",
		},
		Customization: &imagedefinition.Customization{
			ExtraPackages: []*imagedefinition.Package{
				{
					PackageName: "squashfs-tools",
				},
				{
					PackageName: "snapd",
				},
			},
		},
	}

	err := helper.SetDefaults(&stateMachine.ImageDef)
	asserter.AssertErrNil(err, true)

	err = stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	err = getBasicChroot(stateMachine.StateMachine)
	asserter.AssertErrNil(err, true)

	// install the packages that snap-preseed needs
	err = stateMachine.installPackages()
	asserter.AssertErrNil(err, true)

	err = stateMachine.cleanRootfs()
	asserter.AssertErrNil(err, true)

	// Check cleaned files were removed
	cleaned := []string{
		filepath.Join(stateMachine.tempDirs.chroot, "var", "lib", "dbus", "machine-id"),
		filepath.Join(stateMachine.tempDirs.chroot, "etc", "ssh", "ssh_host_rsa_key"),
		filepath.Join(stateMachine.tempDirs.chroot, "etc", "ssh", "ssh_host_rsa_key.pub"),
		filepath.Join(stateMachine.tempDirs.chroot, "etc", "ssh", "ssh_host_ecdsa_key"),
		filepath.Join(stateMachine.tempDirs.chroot, "etc", "ssh", "ssh_host_ecdsa_key.pub"),
		filepath.Join(stateMachine.tempDirs.chroot, "dev", "stderr"),
		filepath.Join(stateMachine.tempDirs.chroot, "dev", "stdin"),
		filepath.Join(stateMachine.tempDirs.chroot, "dev", "stdout"),
		filepath.Join(stateMachine.tempDirs.chroot, "dev", "fd"),
		filepath.Join(stateMachine.tempDirs.chroot, "sys", "kernel", "security"),
		filepath.Join(stateMachine.tempDirs.chroot, "sys", "fs", "cgroup"),
		filepath.Join(stateMachine.tempDirs.chroot, "run", "mount", "utab.lock"),
		filepath.Join(stateMachine.tempDirs.chroot, "run", "lock"),
	}
	for _, file := range cleaned {
		_, err := os.Stat(file)
		if !os.IsNotExist(err) {
			t.Errorf("File %s should not exist, but does", file)
		}
	}

	truncated := []string{
		filepath.Join(stateMachine.tempDirs.chroot, "etc", "machine-id"),
	}
	for _, file := range truncated {
		fileInfo, err := os.Stat(file)
		if os.IsNotExist(err) {
			t.Errorf("File %s should exist, but does not", file)
		}

		if fileInfo.Size() != 0 {
			t.Errorf("File %s should be empty, but it is not. Size: %v", file, fileInfo.Size())
		}
	}
}

func TestClassicStateMachine_cleanRootfs(t *testing.T) {
	sampleContent := "test"
	sampleSize := int64(len(sampleContent))

	testCases := []struct {
		name                 string
		mockFuncs            func() func()
		expectedErr          string
		initialRootfsContent []string
		wantRootfsContent    map[string]int64 // name: size
	}{
		{
			name: "success",
			initialRootfsContent: []string{
				filepath.Join("etc", "machine-id"),
				filepath.Join("var", "lib", "dbus", "machine-id"),
				filepath.Join("etc", "udev", "rules.d", "test-persistent-net.rules"),
				filepath.Join("etc", "udev", "rules.d", "test2-persistent-net.rules"),
				filepath.Join("var", "cache", "debconf", "test-old"),
				filepath.Join("var", "lib", "dpkg", "testdpkg-old"),
				filepath.Join("dev", "stderr"),
				filepath.Join("dev", "stdin"),
				filepath.Join("dev", "stdout"),
				filepath.Join("sys", "kernel", "security"),
				filepath.Join("run", "mount", "utab.lock"),
			},
			wantRootfsContent: map[string]int64{
				filepath.Join("etc", "machine-id"):                                    0,
				filepath.Join("etc", "udev", "rules.d", "test-persistent-net.rules"):  0,
				filepath.Join("etc", "udev", "rules.d", "test2-persistent-net.rules"): 0,
			},
		},
		{
			name: "fail to clean files",
			mockFuncs: func() func() {
				mock := testhelper.NewOSMock(
					&testhelper.OSMockConf{},
				)

				osRemoveAll = mock.RemoveAll
				return func() { osRemoveAll = os.RemoveAll }
			},
			expectedErr: "Error removing",
			initialRootfsContent: []string{
				filepath.Join("etc", "machine-id"),
				filepath.Join("var", "lib", "dbus", "machine-id"),
				filepath.Join("etc", "udev", "rules.d", "test-persistent-net.rules"),
				filepath.Join("var", "cache", "debconf", "test-old"),
				filepath.Join("var", "lib", "dpkg", "testdpkg-old"),
				filepath.Join("dev", "stderr"),
				filepath.Join("dev", "stdin"),
				filepath.Join("dev", "stdout"),
				filepath.Join("sys", "kernel", "security"),
				filepath.Join("run", "mount", "utab.lock"),
			},
			wantRootfsContent: map[string]int64{
				filepath.Join("etc", "machine-id"):                                   sampleSize,
				filepath.Join("var", "lib", "dbus", "machine-id"):                    sampleSize,
				filepath.Join("etc", "udev", "rules.d", "test-persistent-net.rules"): sampleSize,
				filepath.Join("var", "cache", "debconf", "test-old"):                 sampleSize,
				filepath.Join("var", "lib", "dpkg", "testdpkg-old"):                  sampleSize,
				filepath.Join("dev", "stderr"):                                       sampleSize,
				filepath.Join("dev", "stdin"):                                        sampleSize,
				filepath.Join("dev", "stdout"):                                       sampleSize,
				filepath.Join("sys", "kernel", "security"):                           sampleSize,
				filepath.Join("run", "mount", "utab.lock"):                           sampleSize,
			},
		},
		{
			name: "fail to truncate files",
			mockFuncs: func() func() {
				mock := testhelper.NewOSMock(
					&testhelper.OSMockConf{},
				)

				osTruncate = mock.Truncate
				return func() { osTruncate = os.Truncate }
			},
			expectedErr: "Error truncating",
			initialRootfsContent: []string{
				filepath.Join("etc", "machine-id"),
				filepath.Join("var", "lib", "dbus", "machine-id"),
				filepath.Join("etc", "udev", "rules.d", "test-persistent-net.rules"),
			},
			wantRootfsContent: map[string]int64{
				filepath.Join("etc", "machine-id"):                                   sampleSize,
				filepath.Join("etc", "udev", "rules.d", "test-persistent-net.rules"): sampleSize,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			stateMachine := &ClassicStateMachine{}
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = stateMachine

			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)

			t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

			if tc.mockFuncs != nil {
				restoreMock := tc.mockFuncs()
				t.Cleanup(restoreMock)
			}

			for _, path := range tc.initialRootfsContent {
				// create dir if necessary
				fullPath := filepath.Join(stateMachine.tempDirs.chroot, path)
				err = os.MkdirAll(filepath.Dir(fullPath), 0777)
				asserter.AssertErrNil(err, true)

				err := os.WriteFile(fullPath, []byte(sampleContent), 0600)
				asserter.AssertErrNil(err, true)
			}

			err = stateMachine.cleanRootfs()
			if err != nil || len(tc.expectedErr) != 0 {
				asserter.AssertErrContains(err, tc.expectedErr)
			}

			for path, size := range tc.wantRootfsContent {
				fullPath := filepath.Join(stateMachine.tempDirs.chroot, path)
				s, err := os.Stat(fullPath)
				if os.IsNotExist(err) {
					t.Errorf("File %s should exist, but does not", path)
				}

				if s.Size() != size {
					t.Errorf("File size of %s is not matching: want %d, got %d", path, size, s.Size())
				}
			}
		})
	}
}

func Test_addUniqueSnaps(t *testing.T) {
	type args struct {
		currentSnaps []string
		newSnaps     []string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "no duplicate",
			args: args{
				currentSnaps: []string{
					"a",
					"b",
				},
				newSnaps: []string{
					"c",
					"d",
				},
			},
			want: []string{
				"a",
				"b",
				"c",
				"d",
			},
		},
		{
			name: "current empty",
			args: args{
				currentSnaps: nil,
				newSnaps: []string{
					"c",
					"d",
				},
			},
			want: []string{

				"c",
				"d",
			},
		},
		{
			name: "new empty",
			args: args{
				currentSnaps: []string{
					"a",
					"b",
				},
				newSnaps: nil,
			},
			want: []string{
				"a",
				"b",
			},
		},
		{
			name: "with duplicates, conserve order",
			args: args{
				currentSnaps: []string{
					"a",
					"b",
					"e",
					"f",
				},
				newSnaps: []string{
					"c",
					"d",
					"e",
					"f",
				},
			},
			want: []string{
				"a",
				"b",
				"e",
				"f",
				"c",
				"d",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			got := addUniqueSnaps(tt.args.currentSnaps, tt.args.newSnaps)
			asserter.AssertEqual(tt.want, got)
		})
	}
}
