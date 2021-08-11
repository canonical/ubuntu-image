// This file contains unit tests for all of the common state functions
package statemachine

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/google/uuid"
	"github.com/snapcore/snapd/osutil"
)

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
		t.Run("test_"+tc.name, func(t *testing.T) {
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.stateMachineFlags.WorkDir = tc.workdir
			if err := stateMachine.makeTemporaryDirectories(); err != nil {
				t.Errorf("Did not expect an error, got %s", err.Error())
			}
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
	t.Run("test_failed_mkdir", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		// mock os.Mkdir and test with and without a WorkDir
		osMkdir = mockMkdir
		defer func() {
			osMkdir = os.Mkdir
		}()
		if err := stateMachine.makeTemporaryDirectories(); err == nil {
			t.Error("Expected an error, but got none")
		}
		stateMachine.stateMachineFlags.WorkDir = testDir
		if err := stateMachine.makeTemporaryDirectories(); err == nil {
			t.Error("Expected an error, but got none")
		}

		// mock os.MkdirAll and only test with a WorkDir
		osMkdirAll = mockMkdirAll
		defer func() {
			osMkdirAll = os.MkdirAll
		}()
		if err := stateMachine.makeTemporaryDirectories(); err == nil {
			// try adding a workdir to see if that triggers the failure
			stateMachine.stateMachineFlags.WorkDir = testDir
			if err := stateMachine.makeTemporaryDirectories(); err == nil {
				t.Error("Expected an error, but got none")
			}
		}
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestLoadGadgetYaml tests a succesful load of gadget.yaml. It also tests that the unpack
// directory is preserved if the relevant environment variable is set
func TestLoadGadgetYaml(t *testing.T) {
	t.Run("test_load_gadget_yaml", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.yamlFilePath = filepath.Join("testdata", "gadget_tree", "meta", "gadget.yaml")

		// need workdir set up for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		preserveDir := filepath.Join("/tmp", "ubuntu-image-"+uuid.NewString())
		os.Setenv("UBUNTU_IMAGE_PRESERVE_UNPACK", preserveDir)
		defer func() {
			os.Unsetenv("UBUNTU_IMAGE_PRESERVE_UNPACK")
		}()
		// ensure unpack exists
		os.MkdirAll(stateMachine.tempDirs.unpack, 0755)
		defer os.RemoveAll(preserveDir)
		if err := stateMachine.loadGadgetYaml(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}
		preserveUnpack := filepath.Join(preserveDir, "unpack")
		if _, err := os.Stat(preserveUnpack); err != nil {
			t.Errorf("Preserve unpack directory %s does not exist", preserveUnpack)
		}
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedLoadGadgetYaml tests failures in the loadGadgetYaml state
// This is achieved by providing an invalid gadget.yaml and mocking
// os.MkdirAll, iotuil.ReadFile, osutil.CopyFile, and osutil.CopySpecialFile
func TestFailedLoadGadgetYaml(t *testing.T) {
	t.Run("test_failed_load_gadget_yaml", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		// need workdir set up for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		stateMachine.yamlFilePath = filepath.Join("testdata",
			"gadget_tree", "meta", "gadget.yaml")
		// mock osutil.CopySpecialFile
		osutilCopyFile = mockCopyFile
		defer func() {
			osutilCopyFile = osutil.CopyFile
		}()
		if err := stateMachine.loadGadgetYaml(); err == nil {
			t.Error("Expected an error, but got none")
		}
		osutilCopyFile = osutil.CopyFile

		// mock ioutilReadFile
		ioutilReadFile = mockReadFile
		defer func() {
			ioutilReadFile = ioutil.ReadFile
		}()
		if err := stateMachine.loadGadgetYaml(); err == nil {
			t.Error("Expected an error, but got none")
		}
		ioutilReadFile = ioutil.ReadFile

		// now test with the invalid yaml file
		stateMachine.yamlFilePath = filepath.Join("testdata",
			"gadget_tree_invalid", "meta", "gadget.yaml")
		if err := stateMachine.loadGadgetYaml(); err == nil {
			t.Error("Expected an error, but got none")
		}

		// set a valid yaml file and preserveDir
		stateMachine.yamlFilePath = filepath.Join("testdata",
			"gadget_tree", "meta", "gadget.yaml")

		// mock os.MkdirAll
		osMkdirAll = mockMkdirAll
		defer func() {
			osMkdirAll = os.MkdirAll
		}()
		// run with and without the environment variable set
		if err := stateMachine.loadGadgetYaml(); err == nil {
			t.Error("Expected an error, but got none")
		}
		preserveDir := filepath.Join("/tmp", "ubuntu-image-"+uuid.NewString())
		os.Setenv("UBUNTU_IMAGE_PRESERVE_UNPACK", preserveDir)
		defer func() {
			os.Unsetenv("UBUNTU_IMAGE_PRESERVE_UNPACK")
		}()
		defer os.RemoveAll(preserveDir)
		if err := stateMachine.loadGadgetYaml(); err == nil {
			t.Error("Expected an error, but got none")
		}
		osMkdirAll = os.MkdirAll

		// mock osutil.CopySpecialFile
		osutilCopySpecialFile = mockCopySpecialFile
		defer func() {
			osutilCopySpecialFile = osutil.CopySpecialFile
		}()
		if err := stateMachine.loadGadgetYaml(); err == nil {
			t.Error("Expected an error, but got none")
		}
		osutilCopySpecialFile = osutil.CopySpecialFile

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedPostProcessGadgetYaml tests failues in the post processing of
// the gadget.yaml file after loading it in. This is accomplished by mocking
// os.MkdirAll
func TestFailedPostProcessGadgetYaml(t *testing.T) {
	t.Run("test_failed_post_process_gadget_yaml", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		// need workdir set up for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		// set a valid yaml file and load it in
		stateMachine.yamlFilePath = filepath.Join("testdata",
			"gadget_tree", "meta", "gadget.yaml")
		// ensure unpack exists
		os.MkdirAll(stateMachine.tempDirs.unpack, 0755)
		if err := stateMachine.loadGadgetYaml(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		// mock os.MkdirAll
		osMkdirAll = mockMkdirAll
		defer func() {
			osMkdirAll = os.MkdirAll
		}()
		if err := stateMachine.postProcessGadgetYaml(); err == nil {
			t.Error("Expected an error, but got none")
		}
		osMkdirAll = os.MkdirAll
	})
}
