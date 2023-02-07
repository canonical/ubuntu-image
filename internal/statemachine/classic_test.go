// This test file tests a successful classic run and success/error scenarios for all states
// that are specific to the classic builds
package statemachine

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v2"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/canonical/ubuntu-image/internal/imagedefinition"
	"github.com/invopop/jsonschema"
	"github.com/pkg/xattr"
	"github.com/snapcore/snapd/image"
	"github.com/snapcore/snapd/osutil"

	//"github.com/snapcore/snapd/osutil"
	//"github.com/snapcore/snapd/seed"
	"github.com/snapcore/snapd/store"
	"github.com/xeipuuv/gojsonschema"
)

// TestClassicSetup tests a successful run of the polymorphed Setup function
func TestClassicSetup(t *testing.T) {
	t.Run("test_classic_setup", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine

		err := stateMachine.Setup()
		asserter.AssertErrNil(err, true)
	})
}

// TestYAMLSchemaParsing attempts to parse a variety of image definition files, both
// valid and invalid, and ensures the correct result/errors are returned
func TestYAMLSchemaParsing(t *testing.T) {
	testCases := []struct {
		name            string
		imageDefinition string
		shouldPass      bool
		expectedError   string
	}{
		{"valid_image_definition", "test_raspi.yaml", true, ""},
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
		{"invalid_paths_in_manual_touch_file", "test_invalid_paths_in_manual_touch_file.yaml", false, "needs to be an absolute path (../../malicious)"},
		{"invalid_paths_in_manual_touch_file_bug", "test_invalid_paths_in_manual_touch_file.yaml", false, "needs to be an absolute path (/../../malicious)"},
		{"img_specified_without_gadget", "test_image_without_gadget.yaml", false, "Key img cannot be used without key gadget:"},
	}
	for _, tc := range testCases {
		t.Run("test_yaml_schema_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			saveCWD := helper.SaveCWD()
			defer saveCWD()

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
	t.Run("test_failed_parse_image_definition", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.Args.ImageDefinition = filepath.Join("testdata", "image_definitions",
			"test_raspi.yaml")

		// mock helper.SetDefaults
		helperSetDefaults = mockSetDefaults
		defer func() {
			helperSetDefaults = helper.SetDefaults
		}()
		err := stateMachine.parseImageDefinition()
		asserter.AssertErrContains(err, "Test Error")
		helperSetDefaults = helper.SetDefaults

		// mock helper.CheckEmptyFields
		helperCheckEmptyFields = mockCheckEmptyFields
		defer func() {
			helperCheckEmptyFields = helper.CheckEmptyFields
		}()
		err = stateMachine.parseImageDefinition()
		asserter.AssertErrContains(err, "Test Error")
		helperCheckEmptyFields = helper.CheckEmptyFields

		// mock gojsonschema.Validate
		gojsonschemaValidate = mockGojsonschemaValidateError
		defer func() {
			gojsonschemaValidate = gojsonschema.Validate
		}()
		err = stateMachine.parseImageDefinition()
		asserter.AssertErrContains(err, "Schema validation returned an error")
		gojsonschemaValidate = gojsonschema.Validate

		// mock helper.CheckTags
		// the gadget must be set to nil for this test to work
		stateMachine.Args.ImageDefinition = filepath.Join("testdata", "image_definitions",
			"test_image_without_gadget.yaml")
		helperCheckTags = mockCheckTags
		defer func() {
			helperCheckTags = helper.CheckTags
		}()
		err = stateMachine.parseImageDefinition()
		asserter.AssertErrContains(err, "Test Error")
		helperCheckTags = helper.CheckTags
	})
}

// TestCalculateStates reads in a variety of yaml files and ensures
// that the correct states are added to the state machine
// TODO: manually assemble the image definitions instead of relying on the parseImageDefinition() function to make this more of a unit test
func TestCalculateStates(t *testing.T) {
	testCases := []struct {
		name            string
		imageDefinition string
		expectedStates  []string
	}{
		{"state_build_gadget", "test_build_gadget.yaml", []string{"build_gadget_tree", "load_gadget_yaml"}},
		{"state_prebuilt_gadget", "test_prebuilt_gadget.yaml", []string{"prepare_gadget_tree", "load_gadget_yaml"}},
		{"state_prebuilt_rootfs_extras", "test_prebuilt_rootfs_extras.yaml", []string{"add_extra_ppas", "install_extra_packages", "install_extra_snaps"}},
		{"extract_rootfs_tar", "test_extract_rootfs_tar.yaml", []string{"extract_rootfs_tar"}},
		{"build_rootfs_from_seed", "test_rootfs_seed.yaml", []string{"germinate"}},
		{"build_rootfs_from_tasks", "test_rootfs_tasks.yaml", []string{"build_rootfs_from_tasks"}},
		{"customization_states", "test_customization.yaml", []string{"customize_cloud_init", "perform_manual_customization"}},
		{"qcow2", "test_qcow2.yaml", []string{"make_disk", "make_qcow2_image"}},
	}
	for _, tc := range testCases {
		t.Run("test_calcluate_states_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			saveCWD := helper.SaveCWD()
			defer saveCWD()

			var stateMachine ClassicStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine
			stateMachine.Args.ImageDefinition = filepath.Join("testdata", "image_definitions", tc.imageDefinition)
			err := stateMachine.parseImageDefinition()
			asserter.AssertErrNil(err, true)

			// now calculate the states and ensure that the expected states are in the slice
			err = stateMachine.calculateStates()
			asserter.AssertErrNil(err, true)

			for _, expectedState := range tc.expectedStates {
				stateFound := false
				for _, state := range stateMachine.states {
					if expectedState == state.name {
						stateFound = true
					}
				}
				if !stateFound {
					t.Errorf("state %s should exist in %v, but does not",
						expectedState, stateMachine.states)
				}
			}
		})
	}
}

// TestFailedCalculateStates tests failure scenarios in the
// calculateStates function
func TestFailedCalculateStates(t *testing.T) {
	t.Run("test_failed_calcluate_states", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

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
		defer func() {
			helperCheckTags = helper.CheckTags
		}()
		err := stateMachine.calculateStates()
		asserter.AssertErrContains(err, "Test Error")
		helperCheckTags = helper.CheckTags

		// now set a --thru flag for a state that doesn't exist
		stateMachine.stateMachineFlags.Thru = "fake_state"

		// now calculate the states and ensure that the expected states are in the slice
		err = stateMachine.calculateStates()
		asserter.AssertErrContains(err, "not a valid state name")
	})
}

// TestPrintStates ensures the states are printed to stdout when the --debug flag is set
func TestPrintStates(t *testing.T) {
	t.Run("test_print_states", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

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

		// restore stdout and examine what was printed
		restoreStdout()
		readStdout, err := ioutil.ReadAll(stdout)
		asserter.AssertErrNil(err, true)

		expectedStates := `The calculated states are as follows:
[0] build_gadget_tree
[1] prepare_gadget_tree
[2] load_gadget_yaml
[3] verify_artifact_names
[4] germinate
[5] create_chroot
[6] install_packages
[7] preseed_image
[8] customize_fstab
[9] perform_manual_customization
[10] populate_rootfs_contents
[11] generate_disk_info
[12] calculate_rootfs_size
[13] populate_bootfs_contents
[14] populate_prepare_partitions
[15] make_disk
[16] generate_manifest
[17] finish
`
		if !strings.Contains(string(readStdout), expectedStates) {
			t.Errorf("Expected states to be printed in output:\n\"%s\"\n but got \n\"%s\"\n instead",
				expectedStates, string(readStdout))
		}
	})
}

// TestFailedValidateInputClassic tests a failure in the Setup() function when validating common input
func TestFailedValidateInputClassic(t *testing.T) {
	t.Run("test_failed_validate_input", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		// use both --until and --thru to trigger this failure
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.stateMachineFlags.Until = "until-test"
		stateMachine.stateMachineFlags.Thru = "thru-test"

		err := stateMachine.Setup()
		asserter.AssertErrContains(err, "cannot specify both --until and --thru")
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedReadMetadataClassic tests a failed metadata read by passing --resume with no previous partial state machine run
func TestFailedReadMetadataClassic(t *testing.T) {
	t.Run("test_failed_read_metadata", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		// start a --resume with no previous SM run
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.stateMachineFlags.Resume = true
		stateMachine.stateMachineFlags.WorkDir = testDir

		err := stateMachine.Setup()
		asserter.AssertErrContains(err, "error reading metadata file")
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestPrepareGadgetTree runs prepareGadgetTree() and ensures the gadget_tree files
// are placed in the correct locations
func TestPrepareGadgetTree(t *testing.T) {
	t.Run("test_prepare_gadget_tree", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.ImageDef = imagedefinition.ImageDefinition{
			Architecture: getHostArch(),
			Series:       getHostSuite(),
			Gadget:       &imagedefinition.Gadget{},
		}

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// place a test gadget tree in the  scratch directory so we don't have to build one
		gadgetSource := filepath.Join("testdata", "gadget_tree")
		gadgetDest := filepath.Join(stateMachine.tempDirs.scratch, "gadget")
		err = osutil.CopySpecialFile(gadgetSource, gadgetDest)
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
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestPrepareGadgetTreePrebuilt tests the prepareGadgetTree function with prebuilt gadgets
func TestPrepareGadgetTreePrebuilt(t *testing.T) {
	t.Run("test_prepare_gadget_tree_prebuilt", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

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

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
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
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedPrepareGadgetTree tests failures in os, osutil, and ioutil libraries
func TestFailedPrepareGadgetTree(t *testing.T) {
	t.Run("test_failed_prepare_gadget_tree", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.ImageDef = imagedefinition.ImageDefinition{
			Architecture: getHostArch(),
			Series:       getHostSuite(),
			Gadget:       &imagedefinition.Gadget{},
		}

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// place a test gadget tree in the  scratch directory so we don't have to build one
		gadgetSource := filepath.Join("testdata", "gadget_tree")
		gadgetDest := filepath.Join(stateMachine.tempDirs.scratch, "gadget")
		err = osutil.CopySpecialFile(gadgetSource, gadgetDest)
		asserter.AssertErrNil(err, true)

		// mock os.Mkdir
		osMkdirAll = mockMkdirAll
		defer func() {
			osMkdirAll = os.MkdirAll
		}()
		err = stateMachine.prepareGadgetTree()
		asserter.AssertErrContains(err, "Error creating unpack directory")
		osMkdirAll = os.MkdirAll

		// mock ioutil.ReadDir
		ioutilReadDir = mockReadDir
		defer func() {
			ioutilReadDir = ioutil.ReadDir
		}()
		err = stateMachine.prepareGadgetTree()
		asserter.AssertErrContains(err, "Error reading gadget tree")
		ioutilReadDir = ioutil.ReadDir

		// mock osutil.CopySpecialFile
		osutilCopySpecialFile = mockCopySpecialFile
		defer func() {
			osutilCopySpecialFile = osutil.CopySpecialFile
		}()
		err = stateMachine.prepareGadgetTree()
		asserter.AssertErrContains(err, "Error copying gadget tree")
		osutilCopySpecialFile = osutil.CopySpecialFile

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestVerifyArtifactNames unit tests the verifyArtifactNames function
func TestVerifyArtifactNames(t *testing.T) {
	testCases := []struct {
		name             string
		gadgetYAML       string
		img              *[]imagedefinition.Img
		qcow2            *[]imagedefinition.Qcow2
		expectedVolNames map[string]string
		shouldPass       bool
	}{
		{
			"single_volume_specified",
			"gadget_tree/meta/gadget.yaml",
			&[]imagedefinition.Img{
				{
					ImgName:   "test1.img",
					ImgVolume: "pc",
				},
			},
			nil,
			map[string]string{
				"pc": "test1.img",
			},
			true,
		},
		{
			"single_volume_not_specified",
			"gadget_tree/meta/gadget.yaml",
			&[]imagedefinition.Img{
				{
					ImgName: "test-single.img",
				},
			},
			nil,
			map[string]string{
				"pc": "test-single.img",
			},
			true,
		},
		{
			"mutli_volume_specified",
			"gadget-multi.yaml",
			&[]imagedefinition.Img{
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
			nil,
			map[string]string{
				"first":  "test1.img",
				"second": "test2.img",
				"third":  "test3.img",
				"fourth": "test4.img",
			},
			true,
		},
		{
			"mutli_volume_not_specified",
			"gadget-multi.yaml",
			&[]imagedefinition.Img{
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
			nil,
			map[string]string{},
			false,
		},
		{
			"mutli_volume_some_specified",
			"gadget-multi.yaml",
			&[]imagedefinition.Img{
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
			nil,
			map[string]string{},
			false,
		},
		{
			"mutli_volume_only_create_some_images",
			"gadget-multi.yaml",
			&[]imagedefinition.Img{
				{
					ImgName:   "test1.img",
					ImgVolume: "first",
				},
				{
					ImgName:   "test2.img",
					ImgVolume: "second",
				},
			},
			nil,
			map[string]string{
				"first":  "test1.img",
				"second": "test2.img",
			},
			true,
		},
		{
			"qcow2_single_volume_no_img",
			"gadget_tree/meta/gadget.yaml",
			nil,
			&[]imagedefinition.Qcow2{
				{
					Qcow2Name:   "test1.qcow2",
					Qcow2Volume: "pc",
				},
			},
			map[string]string{
				"pc": "test1.qcow2.img",
			},
			true,
		},
		{
			"qcow2_single_volume_not_specified_no_img",
			"gadget_tree/meta/gadget.yaml",
			nil,
			&[]imagedefinition.Qcow2{
				{
					Qcow2Name: "test1.qcow2",
				},
			},
			map[string]string{
				"pc": "test1.qcow2.img",
			},
			true,
		},
		{
			"qcow2_single_volume_yes_img",
			"gadget_tree/meta/gadget.yaml",
			&[]imagedefinition.Img{
				{
					ImgName:   "test1.img",
					ImgVolume: "pc",
				},
			},
			&[]imagedefinition.Qcow2{
				{
					Qcow2Name:   "test1.img",
					Qcow2Volume: "pc",
				},
			},
			map[string]string{
				"pc": "test1.img",
			},
			true,
		},
		{
			"qcow2_mutli_volume_not_specified",
			"gadget-multi.yaml",
			nil,
			&[]imagedefinition.Qcow2{
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
			map[string]string{},
			false,
		},
		{
			"qcow2_mutli_volume_no_img",
			"gadget-multi.yaml",
			nil,
			&[]imagedefinition.Qcow2{
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
			map[string]string{
				"first":  "test1.qcow2.img",
				"second": "test2.qcow2.img",
				"third":  "test3.qcow2.img",
				"fourth": "test4.qcow2.img",
			},
			true,
		},
		{
			"qcow2_mutli_volume_yes_img",
			"gadget-multi.yaml",
			&[]imagedefinition.Img{
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
			&[]imagedefinition.Qcow2{
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
			map[string]string{
				"first":  "test1.img",
				"second": "test2.img",
				"third":  "test3.img",
				"fourth": "test4.img",
			},
			true,
		},
		{
			"qcow2_mutli_volume_img_for_different_volume",
			"gadget-multi.yaml",
			&[]imagedefinition.Img{
				{
					ImgName:   "test1.img",
					ImgVolume: "first",
				},
				{
					ImgName:   "test2.img",
					ImgVolume: "second",
				},
			},
			&[]imagedefinition.Qcow2{
				{
					Qcow2Name:   "test3.qcow2",
					Qcow2Volume: "third",
				},
				{
					Qcow2Name:   "test4.qcow2",
					Qcow2Volume: "fourth",
				},
			},
			map[string]string{
				"first":  "test1.img",
				"second": "test2.img",
				"third":  "test3.qcow2.img",
				"fourth": "test4.qcow2.img",
			},
			true,
		},
	}
	for _, tc := range testCases {
		t.Run("test_verify_artifact_names_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			saveCWD := helper.SaveCWD()
			defer saveCWD()

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
				Artifacts: &imagedefinition.Artifact{
					Img:   tc.img,
					Qcow2: tc.qcow2,
				},
			}

			// need workdir set up for this
			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)

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
	t.Run("test_build_rootfs_from_tasks", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		err := stateMachine.buildRootfsFromTasks()
		asserter.AssertErrNil(err, true)

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestExtractRootfsTar unit tests the extractRootfsTar function
func TestExtractRootfsTar(t *testing.T) {
	wd, _ := os.Getwd()
	testCases := []struct {
		name          string
		rootfsTar     string
		SHA256sum     string
		expectedFiles []string
	}{
		{
			"vanilla_tar",
			filepath.Join(wd, "testdata", "rootfs_tarballs", "rootfs.tar"),
			"ec01fd8488b0f35d2ca69e6f82edfaecef5725da70913bab61240419ce574918",
			[]string{
				"test_tar1",
				"test_tar2",
			},
		},
		{
			"vanilla_tar_relative_path",
			filepath.Join("testdata", "rootfs_tarballs", "rootfs.tar"),
			"ec01fd8488b0f35d2ca69e6f82edfaecef5725da70913bab61240419ce574918",
			[]string{
				"test_tar1",
				"test_tar2",
			},
		},
		{
			"gz",
			filepath.Join(wd, "testdata", "rootfs_tarballs", "rootfs.tar.gz"),
			"29152fd9cadbc92f174815ec642ab3aea98f08f902a4f317ec037f8fe60e40c3",
			[]string{
				"test_tar_gz1",
				"test_tar_gz2",
			},
		},
		{
			"xz",
			filepath.Join(wd, "testdata", "rootfs_tarballs", "rootfs.tar.xz"),
			"e3708f1d98ccea0e0c36843d9576580505ee36d523bfcf78b0f73a035ae9a14e",
			[]string{
				"test_tar_xz1",
				"test_tar_xz2",
			},
		},
		{
			"bz2",
			filepath.Join(wd, "testdata", "rootfs_tarballs", "rootfs.tar.bz2"),
			"a1180a73b652d85d7330ef21d433b095363664f2f808363e67f798fae15abf0c",
			[]string{
				"test_tar_bz1",
				"test_tar_bz2",
			},
		},
		{
			"zst",
			filepath.Join(wd, "testdata", "rootfs_tarballs", "rootfs.tar.zst"),
			"5fb00513f84e28225a3155fd78c59a6a923b222e1c125aab35bbfd4091281829",
			[]string{
				"test_tar_zstd1",
				"test_tar_zstd2",
			},
		},
	}
	for _, tc := range testCases {
		t.Run("test_extract_rootfs_tar_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			saveCWD := helper.SaveCWD()
			defer saveCWD()

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

			// need workdir set up for this
			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)

			err = stateMachine.extractRootfsTar()
			asserter.AssertErrNil(err, true)

			for _, testFile := range tc.expectedFiles {
				_, err := os.Stat(filepath.Join(stateMachine.tempDirs.chroot, testFile))
				if err != nil {
					t.Errorf("File %s should be in chroot, but is missing", testFile)
				}
			}
			os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
		})
	}
}

// TestFailedExtractRootfsTar tests failures in the extractRootfsTar function
func TestFailedExtractRootfsTar(t *testing.T) {
	t.Run("test_failed_extract_rootfs_tar", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		absTarPath, err := filepath.Abs(filepath.Join("testdata", "rootfs_tarballs", "rootfs.tar"))
		asserter.AssertErrNil(err, true)
		stateMachine.ImageDef = imagedefinition.ImageDefinition{
			Architecture: getHostArch(),
			Series:       getHostSuite(),
			Rootfs: &imagedefinition.Rootfs{
				Tarball: &imagedefinition.Tarball{
					TarballURL: fmt.Sprintf("file://%s", absTarPath),
					SHA256sum:  "fail",
				},
			},
		}

		// need workdir set up for this
		err = stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// mock os.Mkdir
		osMkdir = mockMkdir
		defer func() {
			osMkdir = os.Mkdir
		}()
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
	})
}

// TestCustomizeCloudInit unit tests the customizeCloudInit function
func TestCustomizeCloudInit(t *testing.T) {
	cloudInitConfigs := []imagedefinition.CloudInit{
		{
			MetaData:      "foo: bar",
			NetworkConfig: "foobar: foobar",
			UserData:      "bar: baz",
		},
		{
			MetaData:      "foo: bar",
			NetworkConfig: "foobar: foobar",
			UserData:      "",
		},
		{
			NetworkConfig: "foobar: foobar",
			UserData:      "",
		},
		{
			UserData: `chpasswd:
  expire: true
  users:
    - name: ubuntu
      password: ubuntu
      type: text
`,
		},
	}

	for _, cloudInitConfig := range cloudInitConfigs {
		t.Run("test_customize_cloud_init", func(t *testing.T) {
			// Test setup
			asserter := helper.Asserter{T: t}
			saveCWD := helper.SaveCWD()
			defer saveCWD()

			var stateMachine ClassicStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine
			tmpDir, err := os.MkdirTemp("", "")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(tmpDir)
			stateMachine.tempDirs.chroot = tmpDir

			// this directory is expected to be present as it is installed by cloud-init
			os.MkdirAll(path.Join(tmpDir, "etc/cloud/cloud.cfg.d"), 0777)

			stateMachine.ImageDef.Customization = &imagedefinition.Customization{
				CloudInit: &cloudInitConfig,
			}

			// Running function to test
			err = stateMachine.customizeCloudInit()
			asserter.AssertErrNil(err, true)

			// Validation
			seedPath := path.Join(tmpDir, "var/lib/cloud/seed/nocloud")

			metaDataFile, err := os.Open(path.Join(seedPath, "meta-data"))
			if cloudInitConfig.MetaData != "" {
				asserter.AssertErrNil(err, false)

				metaDataFileContent, err := ioutil.ReadAll(metaDataFile)
				asserter.AssertErrNil(err, false)

				if string(metaDataFileContent[:]) != cloudInitConfig.MetaData {
					t.Errorf("un-expected meta-data content found: expected:\n%v\ngot:%v", cloudInitConfig.MetaData, string(metaDataFileContent[:]))
				}
			} else {
				asserter.AssertErrContains(err, "no such file or directory")
			}

			networkConfigFile, err := os.Open(path.Join(seedPath, "network-config"))
			if cloudInitConfig.NetworkConfig != "" {
				asserter.AssertErrNil(err, false)

				networkConfigFileContent, err := ioutil.ReadAll(networkConfigFile)
				asserter.AssertErrNil(err, false)
				if string(networkConfigFileContent[:]) != cloudInitConfig.NetworkConfig {
					t.Errorf("un-expected network-config found: expected:\n%v\ngot:%v", cloudInitConfig.NetworkConfig, string(networkConfigFileContent[:]))
				}
			} else {
				asserter.AssertErrContains(err, "no such file or directory")
			}

			userDataFile, err := os.Open(path.Join(seedPath, "user-data"))
			if cloudInitConfig.UserData != "" {
				asserter.AssertErrNil(err, false)

				userDataFileContent, err := ioutil.ReadAll(userDataFile)
				asserter.AssertErrNil(err, false)

				if string(userDataFileContent[:]) != cloudInitConfig.UserData {
					t.Errorf("un-expected user-data content found: expected:\n%v\ngot:%v", cloudInitConfig.UserData, string(userDataFileContent[:]))
				}
			} else {
				asserter.AssertErrContains(err, "no such file or directory")
			}

			os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
		})
	}
}

func TestFailedCustomizeCloudInit(t *testing.T) {
	// Test setup
	asserter := helper.Asserter{T: t}
	saveCWD := helper.SaveCWD()
	defer saveCWD()

	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	tmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	stateMachine.tempDirs.chroot = tmpDir

	stateMachine.ImageDef.Customization = &imagedefinition.Customization{
		CloudInit: &imagedefinition.CloudInit{
			MetaData:      "foo: bar",
			NetworkConfig: "foobar: foobar",
			UserData: `chpasswd:
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
			os.MkdirAll(cloudInitConfigDirPath, 0777)
			defer os.RemoveAll(cloudInitConfigDirPath)

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
			os.MkdirAll(cloudInitConfigDirPath, 0777)
			defer os.RemoveAll(cloudInitConfigDirPath)

			osCreate = func(name string) (*os.File, error) {
				if strings.Contains(name, file) {
					fileReadWrite, _ := os.Create(name)
					fileReadWrite.Close()
					fileReadOnly, _ := os.Open(name)
					return fileReadOnly, nil
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
		os.MkdirAll(cloudInitConfigDirPath, 0777)
		defer os.RemoveAll(cloudInitConfigDirPath)

		osMkdirAll = mockMkdirAll
		defer func() {
			osMkdirAll = os.MkdirAll
		}()

		err := stateMachine.customizeCloudInit()
		if err == nil {
			t.Error()
		}
	})

	// Test if yaml.Marshal fails
	t.Run("Test_failed_customize_cloud_init_yaml_marshal", func(t *testing.T) {
		// this directory is expected to be present as it is installed by cloud-init
		cloudInitConfigDirPath := path.Join(tmpDir, "etc/cloud/cloud.cfg.d")
		os.MkdirAll(cloudInitConfigDirPath, 0777)
		defer os.RemoveAll(cloudInitConfigDirPath)

		yamlMarshal = mockMarshal
		defer func() {
			yamlMarshal = yaml.Marshal
		}()

		err := stateMachine.customizeCloudInit()
		if err == nil {
			t.Error()
		}
	})
}

// TestManualCustomization unit tests the manualCustomization function
func TestManualCustomization(t *testing.T) {
	t.Run("test_manual_customization", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

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

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// also create chroot
		err = stateMachine.createChroot()
		asserter.AssertErrNil(err, true)

		err = stateMachine.manualCustomization()
		asserter.AssertErrNil(err, true)

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
		passwdContents, err := ioutil.ReadFile(passwdFile)
		asserter.AssertErrNil(err, true)
		if !strings.Contains(string(passwdContents), "testuser:x:123456") {
			t.Errorf("Test user was not created in the chroot")
		}

		// Check that the test group exists with the correct gid
		groupFile := filepath.Join(stateMachine.tempDirs.chroot, "etc", "group")
		groupContents, err := ioutil.ReadFile(groupFile)
		asserter.AssertErrNil(err, true)
		if !strings.Contains(string(groupContents), "testgroup:x:456789") {
			t.Errorf("Test group was not created in the chroot")
		}

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedManualCustomization tests failures in the manualCustomization function
func TestFailedManualCustomization(t *testing.T) {
	t.Run("test_failed_manual_customization", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine

		stateMachine.ImageDef = imagedefinition.ImageDefinition{
			Customization: &imagedefinition.Customization{
				Manual: &imagedefinition.Manual{
					TouchFile: []*imagedefinition.TouchFile{
						{
							TouchPath: filepath.Join("this", "path", "does", "not", "exist"),
						},
					},
				},
			},
		}

		err := stateMachine.manualCustomization()
		asserter.AssertErrContains(err, "no such file or directory")
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestPreseedClassicImage unit tests the preseedClassicImage function
func TestPreseedClassicImage(t *testing.T) {
	t.Run("test_preseed_classic_image", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

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
						SnapName: "hello",
						Channel:  "candidate",
					},
				},
			},
		}

		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		err = stateMachine.preseedClassicImage()
		asserter.AssertErrNil(err, true)

		// check that the lxd and hello snaps, as well as lxd's base, core20
		// were preseeded in the correct location
		snaps := map[string]string{"lxd": "stable", "hello": "candidate", "core20": "stable"}
		for snapName, snapChannel := range snaps {
			// reach out to the snap store to find the revision
			// of the snap for the specified channel
			snapStore := store.New(nil, nil)
			snapSpec := store.SnapSpec{Name: snapName}
			context := context.TODO() //context can be empty, just not nil
			snapInfo, err := snapStore.SnapInfo(context, snapSpec, nil)
			asserter.AssertErrNil(err, true)

			var storeRevision int
			storeRevision = snapInfo.Channels["latest/"+snapChannel].Revision.N
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
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedPreseedClassicImage tests failures in the preseedClassicImage function
func TestFailedPreseedClassicImage(t *testing.T) {
	t.Run("test_failed_preseed_classic_image", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.ImageDef = imagedefinition.ImageDefinition{
			Architecture: getHostArch(),
			Customization: &imagedefinition.Customization{
				ExtraSnaps: []*imagedefinition.Snap{},
			},
		}

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// include an invalid snap snap name to trigger a failure in
		// parseSnapsAndChannels
		stateMachine.Snaps = []string{"lxd=test=invalid=name"}
		err = stateMachine.preseedClassicImage()
		asserter.AssertErrContains(err, "Invalid syntax")

		// try to include a nonexistent snap to trigger a failure
		// in snapStore.SnapInfo
		stateMachine.Snaps = []string{"test-this-snap-name-should-never-exist"}
		err = stateMachine.preseedClassicImage()
		asserter.AssertErrContains(err, "Error getting info for snap")

		// mock image.Prepare
		stateMachine.Snaps = []string{"hello"}
		imagePrepare = mockImagePrepare
		defer func() {
			imagePrepare = image.Prepare
		}()
		err = stateMachine.preseedClassicImage()
		asserter.AssertErrContains(err, "Error preparing image")
		imagePrepare = image.Prepare

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestPopulateClassicRootfsContents runs the state machine through populate_rootfs_contents and examines
// the rootfs to ensure at least some of the correct file are in place
func TestPopulateClassicRootfsContents(t *testing.T) {
	t.Run("test_populate_classic_rootfs_contents", func(t *testing.T) {
		if runtime.GOARCH != "amd64" {
			t.Skip("Test for amd64 only")
		}
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

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

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// also create chroot
		err = stateMachine.createChroot()
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

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedPopulateClassicRootfsContents tests failed scenarios in populateClassicRootfsContents
// this is accomplished by mocking functions
func TestFailedPopulateClassicRootfsContents(t *testing.T) {
	t.Run("test_failed_populate_classic_rootfs_contents", func(t *testing.T) {
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

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// also create chroot
		err = stateMachine.createChroot()
		asserter.AssertErrNil(err, true)

		// mock ioutil.ReadDir
		ioutilReadDir = mockReadDir
		defer func() {
			ioutilReadDir = ioutil.ReadDir
		}()
		err = stateMachine.populateClassicRootfsContents()
		asserter.AssertErrContains(err, "Error reading unpack/chroot dir")
		ioutilReadDir = ioutil.ReadDir

		// mock osutil.CopySpecialFile
		osutilCopySpecialFile = mockCopySpecialFile
		defer func() {
			osutilCopySpecialFile = osutil.CopySpecialFile
		}()
		err = stateMachine.populateClassicRootfsContents()
		asserter.AssertErrContains(err, "Error copying rootfs")
		osutilCopySpecialFile = osutil.CopySpecialFile

		// mock ioutil.WriteFile
		ioutilWriteFile = mockWriteFile
		defer func() {
			ioutilWriteFile = ioutil.WriteFile
		}()
		err = stateMachine.populateClassicRootfsContents()
		asserter.AssertErrContains(err, "Error writing to fstab")
		ioutilWriteFile = ioutil.WriteFile

		// create an /etc/resolv.conf.tmp in the chroot
		err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.chroot, "etc"), 0755)
		asserter.AssertErrNil(err, true)
		_, err = os.Create(filepath.Join(stateMachine.tempDirs.chroot, "etc", "resolv.conf.tmp"))
		asserter.AssertErrNil(err, true)

		// mock os.Rename
		osRename = mockRename
		defer func() {
			osRename = os.Rename
		}()
		err = stateMachine.populateClassicRootfsContents()
		asserter.AssertErrContains(err, "Error moving file")
		osRename = os.Rename
	})
}

// TestGeneratePackageManifest tests if classic image manifest generation works
func TestGeneratePackageManifest(t *testing.T) {
	t.Run("test_generate_package_manifest", func(t *testing.T) {
		asserter := helper.Asserter{T: t}

		// Setup the exec.Command mock
		testCaseName = "TestGeneratePackageManifest"
		execCommand = fakeExecCommand
		defer func() {
			execCommand = exec.Command
		}()
		// We need the output directory set for this
		outputDir, err := ioutil.TempDir("/tmp", "ubuntu-image-")
		asserter.AssertErrNil(err, true)
		defer os.RemoveAll(outputDir)

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
		osMkdirAll(stateMachine.commonFlags.OutputDir, 0755)
		defer os.RemoveAll(stateMachine.commonFlags.OutputDir)

		err = stateMachine.generatePackageManifest()
		asserter.AssertErrNil(err, true)

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
		// Check if manifest file got generated and if it has expected contents
		manifestPath := filepath.Join(stateMachine.commonFlags.OutputDir, "filesystem.manifest")
		manifestBytes, err := ioutil.ReadFile(manifestPath)
		asserter.AssertErrNil(err, true)
		// The order of packages shouldn't matter
		examplePackages := []string{"foo 1.2", "bar 1.4-1ubuntu4.1", "libbaz 0.1.3ubuntu2"}
		for _, pkg := range examplePackages {
			if !strings.Contains(string(manifestBytes), pkg) {
				t.Errorf("filesystem.manifest does not contain expected package: %s", pkg)
			}
		}
	})
}

// TestFailedGeneratePackageManifest tests if classic manifest generation failures are reported
func TestFailedGeneratePackageManifest(t *testing.T) {
	t.Run("test_failed_generate_package_manifest", func(t *testing.T) {
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
		outputDir, err := ioutil.TempDir("/tmp", "ubuntu-image-")
		asserter.AssertErrNil(err, true)
		defer os.RemoveAll(outputDir)
		stateMachine.commonFlags.OutputDir = outputDir

		// Setup the exec.Command mock - version from the success test
		testCaseName = "TestGeneratePackageManifest"
		execCommand = fakeExecCommand
		defer func() {
			execCommand = exec.Command
		}()

		// Setup the mock for os.Create, making those fail
		osCreate = mockCreate
		defer func() {
			osCreate = os.Create
		}()

		err = stateMachine.generatePackageManifest()
		asserter.AssertErrContains(err, "Error creating manifest file")
		osCreate = os.Create

		// Setup the exec.Command mock - version from the fail test
		testCaseName = "TestFailedGeneratePackageManifest"
		execCommand = fakeExecCommand
		defer func() {
			execCommand = exec.Command
		}()
		err = stateMachine.generatePackageManifest()
		asserter.AssertErrContains(err, "Error generating package manifest with command")
	})
}

// TestGenerateFilelist tests if classic image filelist generation works
func TestGenerateFilelist(t *testing.T) {
	t.Run("test_generate_filelist", func(t *testing.T) {
		asserter := helper.Asserter{T: t}

		// Setup the exec.Command mock
		testCaseName = "TestGenerateFilelist"
		execCommand = fakeExecCommand
		defer func() {
			execCommand = exec.Command
		}()
		// We need the output directory set for this
		outputDir, err := ioutil.TempDir("/tmp", "ubuntu-image-")
		asserter.AssertErrNil(err, true)
		defer os.RemoveAll(outputDir)

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
		osMkdirAll(stateMachine.commonFlags.OutputDir, 0755)
		defer os.RemoveAll(stateMachine.commonFlags.OutputDir)

		err = stateMachine.generateFilelist()
		asserter.AssertErrNil(err, true)

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
		// Check if filelist file got generated
		filelistPath := filepath.Join(stateMachine.commonFlags.OutputDir, "filesystem.filelist")
		_, err = os.Stat(filelistPath)
		asserter.AssertErrNil(err, true)
	})
}

// TestFailedGenerateFilelist tests if classic filelist generation failures are reported
func TestFailedGenerateFilelist(t *testing.T) {
	t.Run("test_failed_generate_filelist", func(t *testing.T) {
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
		outputDir, err := ioutil.TempDir("/tmp", "ubuntu-image-")
		asserter.AssertErrNil(err, true)
		defer os.RemoveAll(outputDir)
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
	})
}

// TestSuccessfulClassicRun runs through a full classic state machine run and ensures
// it is successful. It creates a .img and a .qcow2 file and ensures they are the
// correct file types
func TestSuccessfulClassicRun(t *testing.T) {
	t.Run("test_successful_classic_run", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		// We need the output directory set for this
		outputDir, err := ioutil.TempDir("/tmp", "ubuntu-image-")
		asserter.AssertErrNil(err, true)
		defer os.RemoveAll(outputDir)

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.commonFlags.Debug = true
		stateMachine.commonFlags.Size = "4G"
		stateMachine.commonFlags.OutputDir = outputDir
		stateMachine.Args.ImageDefinition = filepath.Join("testdata", "image_definitions",
			"test_amd64.yaml")

		err = stateMachine.Setup()
		asserter.AssertErrNil(err, true)

		err = stateMachine.Run()
		asserter.AssertErrNil(err, true)

		// make sure packages were successfully installed from public and private ppas
		files := []string{
			filepath.Join(stateMachine.tempDirs.chroot, "usr", "bin", "hello-ubuntu-image-public"),
			filepath.Join(stateMachine.tempDirs.chroot, "usr", "bin", "hello-ubuntu-image-private"),
		}
		for _, file := range files {
			_, err = os.Stat(file)
			asserter.AssertErrNil(err, true)
		}

		// make sure snaps from the correct channel were installed
		type snapList struct {
			Snaps []struct {
				Name    string `yaml:"name"`
				Channel string `yaml:"channel"`
			} `yaml:"snaps"`
		}

		seedYaml := filepath.Join(stateMachine.tempDirs.chroot,
			"var", "lib", "snapd", "seed", "seed.yaml")

		seedFile, err := os.Open(seedYaml)
		defer seedFile.Close()
		asserter.AssertErrNil(err, true)

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

		// make sure all the artifacts were created and are the correct file types
		artifacts := map[string]string{
			"pc-amd64.img":            "DOS/MBR boot sector",
			"pc-amd64.qcow2":          "QEMU QCOW",
			"filesystem-manifest.txt": "text",
			"filesystem-filelist.txt": "text",
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

		err = stateMachine.Teardown()
		asserter.AssertErrNil(err, true)

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestCheckEmptyFields unit tests the helper.CheckEmptyFields function
func TestCheckEmptyFields(t *testing.T) {
	// define the struct we will use to test
	type testStruct struct {
		A string `yaml:"a" json:"fieldA,required"`
		B string `yaml:"b" json:"fieldB"`
		C string `yaml:"c" json:"fieldC,omitempty"`
	}

	// generate the schema for our testStruct
	var jsonReflector jsonschema.Reflector
	schema := jsonReflector.Reflect(&testStruct{})

	// now run CheckEmptyFields with a variety of test data
	// to ensure the correct return values
	testCases := []struct {
		name       string
		structData testStruct
		shouldPass bool
	}{
		{"success", testStruct{A: "foo", B: "bar", C: "baz"}, true},
		{"missing_explicitly_required", testStruct{B: "bar", C: "baz"}, false},
		{"missing_implicitly_required", testStruct{A: "foo", C: "baz"}, false},
		{"missing_omitempty", testStruct{A: "foo", B: "bar"}, true},
	}
	for _, tc := range testCases {
		t.Run("test_check_empty_fields_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}

			result := new(gojsonschema.Result)
			err := helper.CheckEmptyFields(&tc.structData, result, schema)
			asserter.AssertErrNil(err, false)
			schema.Required = append(schema.Required, "fieldA")

			// make sure validation will fail only when expected
			if tc.shouldPass && !result.Valid() {
				t.Error("CheckEmptyFields added errors when it should not have")
			}
			if !tc.shouldPass && result.Valid() {
				t.Error("CheckEmptyFields did NOT add errors when it should have")
			}

		})
	}
}

// TestGerminate tests the germinate state and ensures some necessary packages are included
func TestGerminate(t *testing.T) {
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
			[]string{"lxd"},
			true,
		},
		{
			"http",
			"ubuntu",
			[]string{"https://people.canonical.com/~ubuntu-archive/seeds/"},
			[]string{"server", "minimal", "standard", "cloud-image"},
			[]string{"python3", "sudo", "cloud-init", "ubuntu-server"},
			[]string{"lxd"},
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
			[]string{},
			true,
		},
	}
	for _, tc := range testCases {
		t.Run("test_germinate_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			saveCWD := helper.SaveCWD()
			defer saveCWD()

			var stateMachine ClassicStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine

			// need workdir set up for this
			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)

			hostArch := getHostArch()
			hostSuite := getHostSuite()
			imageDef := imagedefinition.ImageDefinition{
				Architecture: hostArch,
				Series:       hostSuite,
				Rootfs: &imagedefinition.Rootfs{
					Flavor: tc.flavor,
					Mirror: "http://archive.ubuntu.com/ubuntu/",
					Seed: &imagedefinition.Seed{
						SeedURLs:   tc.seedURLs,
						SeedBranch: hostSuite,
						Names:      tc.seedNames,
						Vcs:        tc.vcs,
					},
				},
			}

			stateMachine.ImageDef = imageDef

			err = stateMachine.germinate()
			asserter.AssertErrNil(err, true)

			// spot check some packages that should remain seeded for a long time
			for _, expectedPackage := range tc.expectedPackages {
				found := false
				for _, seedPackage := range stateMachine.Packages {
					if expectedPackage == seedPackage {
						found = true
					}
				}
				if !found {
					t.Errorf("Expected to find %s in list of packages: %v",
						expectedPackage, stateMachine.Packages)
				}
			}
			// spot check some snaps that should remain seeded for a long time
			for _, expectedSnap := range tc.expectedSnaps {
				found := false
				for _, seedSnap := range stateMachine.Snaps {
					snapName := strings.Split(seedSnap, "=")[0]
					if expectedSnap == snapName {
						found = true
					}
				}
				if !found {
					t.Errorf("Expected to find %s in list of snaps: %v",
						expectedSnap, stateMachine.Snaps)
				}
			}

			os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
		})
	}
}

// TestFailedGerminate mocks function calls to test
// failure cases in the germinate state
func TestFailedGerminate(t *testing.T) {
	t.Run("test_failed_germinate", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine

		// need workdir set up for this
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
					Vcs:        true,
				},
			},
		}
		stateMachine.ImageDef = imageDef

		// mock os.Mkdir
		osMkdir = mockMkdir
		defer func() {
			osMkdir = os.Mkdir
		}()
		err = stateMachine.germinate()
		asserter.AssertErrContains(err, "Error creating germinate directory")
		osMkdir = os.Mkdir

		// Setup the exec.Command mock
		testCaseName = "TestFailedGerminate"
		execCommand = fakeExecCommand
		defer func() {
			execCommand = exec.Command
		}()
		err = stateMachine.germinate()
		asserter.AssertErrContains(err, "Error running germinate command")
		execCommand = exec.Command

		// mock os.Open
		osOpen = mockOpen
		defer func() {
			osOpen = os.Open
		}()
		err = stateMachine.germinate()
		asserter.AssertErrContains(err, "Error opening seed file")
		osOpen = os.Open

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestBuildGadgetTree tests the successful build of a gadget tree
func TestBuildGadgetTree(t *testing.T) {
	t.Run("test_build_gadget_tree", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// test the directory method
		wd, _ := os.Getwd()
		sourcePath := filepath.Join(wd, "testdata", "gadget_source")
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
				GadgetURL:    "https://github.com/snapcore/pc-amd64-gadget",
				GadgetType:   "git",
				GadgetBranch: "classic",
			},
		}

		stateMachine.ImageDef = imageDef

		err = stateMachine.buildGadgetTree()
		asserter.AssertErrNil(err, true)

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedBuildGadgetTree tests failures in the  buildGadgetTree function
func TestFailedBuildGadgetTree(t *testing.T) {
	t.Run("test_failed_build_gadget_tree", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// mock os.MkdirAll
		osMkdir = mockMkdir
		defer func() {
			osMkdir = os.Mkdir
		}()
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
		asserter.AssertErrContains(err, "Error copying gadget source")

		// run a "make" command that will fail by mocking exec.Command
		testCaseName = "TestFailedBuildGadgetTree"
		execCommand = fakeExecCommand
		defer func() {
			execCommand = exec.Command
		}()
		wd, _ := os.Getwd()
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
	})
}

// TestCreateChroot runs the createChroot step and spot checks that some
// expected files in the chroot exist
func TestCreateChroot(t *testing.T) {
	t.Run("test_create_chroot", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.ImageDef = imagedefinition.ImageDefinition{
			Architecture: getHostArch(),
			Series:       getHostSuite(),
			Rootfs: &imagedefinition.Rootfs{
				Pocket: "proposed",
			},
		}

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

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

		// check that security, updates, and proposed were added to /etc/apt/sources.list
		sourcesList := filepath.Join(stateMachine.tempDirs.chroot, "etc", "apt", "sources.list")
		sourcesListData, err := os.ReadFile(sourcesList)
		asserter.AssertErrNil(err, true)

		pockets := []string{
			fmt.Sprintf("%s-updates", stateMachine.ImageDef.Series),
			fmt.Sprintf("%s-security", stateMachine.ImageDef.Series),
			fmt.Sprintf("%s-proposed", stateMachine.ImageDef.Series),
		}

		for _, pocket := range pockets {
			if !strings.Contains(string(sourcesListData), pocket) {
				t.Errorf("%s is not present in /etc/apt/sources.list", pocket)
			}
		}
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedCreateChroot tests failure cases in createChroot
func TestFailedCreateChroot(t *testing.T) {
	t.Run("test_failed_create_chroot", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.ImageDef = imagedefinition.ImageDefinition{
			Architecture: getHostArch(),
			Series:       getHostSuite(),
			Rootfs:       &imagedefinition.Rootfs{},
		}

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// mock os.Mkdir
		osMkdir = mockMkdir
		defer func() {
			osMkdir = os.Mkdir
		}()
		err = stateMachine.createChroot()
		asserter.AssertErrContains(err, "Failed to create chroot")
		osMkdir = os.Mkdir

		// Setup the exec.Command mock
		testCaseName = "TestFailedCreateChroot"
		execCommand = fakeExecCommand
		defer func() {
			execCommand = exec.Command
		}()
		err = stateMachine.createChroot()
		asserter.AssertErrContains(err, "Error running debootstrap command")
		execCommand = exec.Command

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedInstallPackages tests failure cases in installPackages
func TestFailedInstallPackages(t *testing.T) {
	t.Run("test_failed_install_packages", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

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

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// create an /etc/resolv.conf in the chroot
		err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.chroot, "etc"), 0755)
		asserter.AssertErrNil(err, true)
		_, err = os.Create(filepath.Join(stateMachine.tempDirs.chroot, "etc", "resolv.conf"))
		asserter.AssertErrNil(err, true)

		// mock os.MkdirTemp to cause a failure in mountTempFS
		osMkdirTemp = mockMkdirTemp
		defer func() {
			osMkdirTemp = os.MkdirTemp
		}()
		err = stateMachine.installPackages()
		asserter.AssertErrContains(err, "Error mounting temporary directory for mountpoint")
		osMkdirTemp = os.MkdirTemp

		// Setup the exec.Command mock
		testCaseName = "TestFailedInstallPackages"
		execCommand = fakeExecCommand
		defer func() {
			execCommand = exec.Command
		}()
		err = stateMachine.installPackages()
		asserter.AssertErrContains(err, "Error running command")
		execCommand = exec.Command

		// mock os.Rename
		osRename = mockRename
		defer func() {
			osRename = os.Rename
		}()
		err = stateMachine.installPackages()
		asserter.AssertErrContains(err, "Error moving file")
		osRename = os.Rename

		// mock osutil.CopyFile
		osutilCopyFile = mockCopyFile
		defer func() {
			osutilCopyFile = osutil.CopyFile
		}()
		err = stateMachine.installPackages()
		asserter.AssertErrContains(err, "Error copying file")
		osutilCopyFile = osutil.CopyFile

		// clean up
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedAddExtraPPAs tests failure cases in addExtraPPAs
func TestFailedAddExtraPPAs(t *testing.T) {
	t.Run("test_failed_add_extra_ppas", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		validPPA := &imagedefinition.PPA{
			PPAName: "canonical-foundations/ubuntu-image",
		}
		invalidPPA := &imagedefinition.PPA{
			PPAName:     "canonical-foundations/ubuntu-image",
			Fingerprint: "TEST FINGERPRINT",
		}
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.ImageDef = imagedefinition.ImageDefinition{
			Architecture: getHostArch(),
			Series:       getHostSuite(),
			Rootfs:       &imagedefinition.Rootfs{},
			Customization: &imagedefinition.Customization{
				ExtraPPAs: []*imagedefinition.PPA{
					validPPA,
				},
			},
		}

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// create the /etc/apt/ dir in workdir
		os.MkdirAll(filepath.Join(stateMachine.tempDirs.chroot, "etc", "apt", "trusted.gpg.d"), 0755)

		// mock os.Mkdir
		osMkdir = mockMkdir
		defer func() {
			osMkdir = os.Mkdir
		}()
		err = stateMachine.addExtraPPAs()
		asserter.AssertErrContains(err, "Failed to create apt sources.list.d")
		osMkdir = os.Mkdir

		// mock os.MkdirTemp
		osMkdirTemp = mockMkdirTemp
		defer func() {
			osMkdirTemp = os.MkdirTemp
		}()
		err = stateMachine.addExtraPPAs()
		asserter.AssertErrContains(err, "Error creating temp dir for gpg")
		osMkdirTemp = os.MkdirTemp

		// mock os.OpenFile
		osOpenFile = mockOpenFile
		defer func() {
			osOpenFile = os.OpenFile
		}()
		err = stateMachine.addExtraPPAs()
		asserter.AssertErrContains(err, "Error creating")
		osOpenFile = os.OpenFile

		// Use an invalid PPA to trigger a failure in importPPAKeys
		stateMachine.ImageDef.Customization.ExtraPPAs = []*imagedefinition.PPA{invalidPPA}
		err = stateMachine.addExtraPPAs()
		asserter.AssertErrContains(err, "Error retrieving signing key")
		stateMachine.ImageDef.Customization.ExtraPPAs = []*imagedefinition.PPA{validPPA}

		// mock os.RemoveAll
		osRemoveAll = mockRemoveAll
		defer func() {
			osRemoveAll = os.RemoveAll
		}()
		err = stateMachine.addExtraPPAs()
		asserter.AssertErrContains(err, "Error removing temporary gpg directory")
		osRemoveAll = os.RemoveAll

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestCustomizeFstab tests functionality of the customizeFstab function
func TestCustomizeFstab(t *testing.T) {
	testCases := []struct {
		name          string
		fstab         []*imagedefinition.Fstab
		expectedFstab string
	}{
		{
			"one_entry",
			[]*imagedefinition.Fstab{
				{
					Label:        "writable",
					Mountpoint:   "/",
					FSType:       "ext4",
					MountOptions: "defaults",
					Dump:         true,
					FsckOrder:    1,
				},
			},
			`LABEL=writable	/	ext4	defaults	1	1`,
		},
		{
			"two_entries",
			[]*imagedefinition.Fstab{
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
			`LABEL=writable	/	ext4	defaults	0	1
LABEL=system-boot	/boot/firmware	vfat	defaults	0	1`,
		},
		{
			"defaults_assumed",
			[]*imagedefinition.Fstab{
				{
					Label:      "writable",
					Mountpoint: "/",
					FSType:     "ext4",
					FsckOrder:  1,
				},
			},
			`LABEL=writable	/	ext4	defaults	0	1`,
		},
	}

	for _, tc := range testCases {
		t.Run("test_customize_fstab_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			saveCWD := helper.SaveCWD()
			defer saveCWD()

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

			// need workdir set up for this
			err = stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)

			// create the <chroot>/etc directory
			err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.chroot, "etc"), 0644)
			asserter.AssertErrNil(err, true)

			// customize the fstab, ensure no errors, and check the contents
			err = stateMachine.customizeFstab()
			asserter.AssertErrNil(err, true)

			fstabBytes, err := ioutil.ReadFile(
				filepath.Join(stateMachine.tempDirs.chroot, "etc", "fstab"),
			)
			asserter.AssertErrNil(err, true)

			if string(fstabBytes) != tc.expectedFstab {
				t.Errorf("Expected fstab contents \"%s\", but got \"%s\"",
					tc.expectedFstab, string(fstabBytes))
			}
		})
	}
}

// TestFailedCustomizeFstab tests failures in the customizeFstab function
func TestFailedCustomizeFstab(t *testing.T) {
	t.Run("test_failed_customize_fstab", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

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

		// mock os.OpenFile
		osOpenFile = mockOpenFile
		defer func() {
			osOpenFile = os.OpenFile
		}()
		err := stateMachine.customizeFstab()
		asserter.AssertErrContains(err, "Error opening fstab")
		osOpenFile = os.OpenFile
	})
}

// TestGenerateRootfsTarball tests that a rootfs tarball is generated
// when appropriate and that it contains the correct files
func TestGenerateRootfsTarball(t *testing.T) {
	testCases := []struct {
		name     string // the name will double as the compression type
		tarPath  string
		fileType string
	}{
		{
			"uncompressed",
			"test_generate_rootfs_tarball.tar",
			"tar archive",
		},
		{
			"bzip2",
			"test_generate_rootfs_tarball.tar.bz2",
			"bzip2 compressed data",
		},
		{
			"gzip",
			"test_generate_rootfs_tarball.tar.gz",
			"gzip compressed data",
		},
		{
			"xz",
			"test_generate_rootfs_tarball.tar.xz",
			"XZ compressed data",
		},
		{
			"zstd",
			"test_generate_rootfs_tarball.tar.zst",
			"Zstandard compressed data",
		},
	}
	for _, tc := range testCases {
		t.Run("test_generate_rootfs_tarball_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			saveCWD := helper.SaveCWD()
			defer saveCWD()

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

			// need workdir set up for this
			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)
			stateMachine.commonFlags.OutputDir = stateMachine.stateMachineFlags.WorkDir

			err = stateMachine.generateRootfsTarball()
			asserter.AssertErrNil(err, true)

			// make sure tar archive exists and is the correct compression type
			_, err = os.Stat(filepath.Join(stateMachine.stateMachineFlags.WorkDir, tc.tarPath))
			if err != nil {
				t.Errorf("File %s should be in workdir, but is missing", tc.tarPath)
			}

			fullPath := filepath.Join(stateMachine.commonFlags.OutputDir, tc.tarPath)
			fileCommand := *exec.Command("file", fullPath)
			cmdOutput, err := fileCommand.CombinedOutput()
			asserter.AssertErrNil(err, true)
			if !strings.Contains(string(cmdOutput), tc.fileType) {
				t.Errorf("File \"%s\" is the wrong file type. Expected \"%s\" but got \"%s\"",
					fullPath, tc.fileType, string(cmdOutput))
			}
		})
	}
}

// TestTarXattrs sets an xattr on a file, puts it in a tar archive,
// extracts the tar archive and ensures the xattr is still present
func TestTarXattrs(t *testing.T) {
	t.Run("test_tar_xattrs", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine

		// create a file with xattrs in a temporary directory
		xattrBytes := []byte("ui-test")
		testDir, err := os.MkdirTemp("/tmp", "ubuntu-image-xattr-test")
		asserter.AssertErrNil(err, true)
		extractDir, err := os.MkdirTemp("/tmp", "ubuntu-image-xattr-test")
		asserter.AssertErrNil(err, true)
		testFile, err := os.CreateTemp(testDir, "test-xattrs-")
		asserter.AssertErrNil(err, true)
		testFileName := filepath.Base(testFile.Name())
		defer os.RemoveAll(testDir)
		defer os.RemoveAll(extractDir)

		err = xattr.FSet(testFile, "user.test", xattrBytes)
		asserter.AssertErrNil(err, true)

		// now run the helper tar creation and extraction functions
		tarPath := filepath.Join(testDir, "test-xattrs.tar")
		err = helper.CreateTarArchive(testDir, tarPath, "uncompressed", false, false)
		asserter.AssertErrNil(err, true)

		err = helper.ExtractTarArchive(tarPath, extractDir, false, false)
		asserter.AssertErrNil(err, true)

		// now read the extracted file's extended attributes
		finalXattrs, err := xattr.List(filepath.Join(extractDir, testFileName))
		asserter.AssertErrNil(err, true)

		if !reflect.DeepEqual(finalXattrs, []string{"user.test"}) {
			t.Errorf("test file \"%s\" does not have correct xattrs set", testFile.Name())
		}
	})
}

// TestPingXattrs runs the ExtractTarArchive file on a pre-made test file that contains /bin/ping
// and ensures that the security.capability extended attribute is still present
func TestPingXattrs(t *testing.T) {
	t.Run("test_tar_xattrs", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine

		testDir, err := os.MkdirTemp("/tmp", "ubuntu-image-ping-xattr-test")
		asserter.AssertErrNil(err, true)
		//defer os.RemoveAll(testDir)
		testFile := filepath.Join("testdata", "rootfs_tarballs", "ping.tar")

		err = helper.ExtractTarArchive(testFile, testDir, true, true)
		asserter.AssertErrNil(err, true)

		binPing := filepath.Join(testDir, "bin", "ping")
		pingXattrs, err := xattr.List(binPing)
		asserter.AssertErrNil(err, true)
		if !reflect.DeepEqual(pingXattrs, []string{"security.capability"}) {
			t.Error("ping has lost the security.capability xattr after tar extraction")
		}
	})
}

// TestFailedMakeQcow2Img tests failures in the makeQcow2Img function
func TestFailedMakeQcow2Img(t *testing.T) {
	t.Run("test_failed_make_qcow2_image", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

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
		asserter.AssertErrContains(err, "Error creating qcow2 artifact")
	})
}
