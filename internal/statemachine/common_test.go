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

// TestPopulateRootfsContentsHooks ensures that the PopulateSnapRootfsContentsHooks
// function can successfully run hook scripts and that core20 skips them
func TestPopulateRootfsContentsHooks(t *testing.T) {
	testCases := []struct {
		name         string
		isSeeded     bool
		hooksCreated []string
	}{
		{"hooks_succeed", false, []string{"post-populate-rootfs-hookfile", "post-populate-rootfs-hookfile.d1", "post-populate-rootfs-hookfile.d2"}},
		{"hooks_not_allowed", true, []string{}},
	}
	for _, tc := range testCases {
		t.Run("test_"+tc.name, func(t *testing.T) {
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.commonFlags.Debug = true
			stateMachine.commonFlags.HooksDirectories = []string{
				filepath.Join("testdata", "good_hooksd"),
				filepath.Join("testdata", "good_hookscript"),
			}
			stateMachine.isSeeded = tc.isSeeded

			// need workdir set up for this
			if err := stateMachine.makeTemporaryDirectories(); err != nil {
				t.Errorf("Did not expect an error, got %s", err.Error())
			}

			if err := stateMachine.populateRootfsContentsHooks(); err != nil {
				t.Errorf("Did not expect an error, got %s", err.Error())
			}

			// the hook scripts used for testing simply touch some files
			for _, file := range tc.hooksCreated {
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
}

// TestFailedPopulateRootfsContentsHooks tests a variety of failures in running the hooks
func TestFailedPopulateRootfsContentsHooks(t *testing.T) {
	testCases := []struct {
		name      string
		hooksDirs []string
	}{
		{"hooks_not_executable", []string{filepath.Join("testdata", "hooks_not_executable")}},
		{"hooks_return_error", []string{filepath.Join("testdata", "hooks_return_error")}},
	}
	for _, tc := range testCases {
		t.Run("test_"+tc.name, func(t *testing.T) {
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.commonFlags.HooksDirectories = tc.hooksDirs
			stateMachine.isSeeded = false

			// need workdir set up for this
			if err := stateMachine.makeTemporaryDirectories(); err != nil {
				t.Errorf("Did not expect an error, got %s", err.Error())
			}

			if err := stateMachine.populateRootfsContentsHooks(); err == nil {
				t.Errorf("Expected an error, but got none")
			}
			os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
		})
	}
}

// TestGenerateDiskInfo tests that diskInfo can be generated
func TestGenerateDiskInfo(t *testing.T) {
	t.Run("test_generate_disk_info", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.commonFlags.DiskInfo = filepath.Join("testdata", "disk_info")

		// need workdir set up for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		if err := stateMachine.generateDiskInfo(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		// make sure rootfs/.disk/info exists
		_, err := os.Stat(filepath.Join(stateMachine.tempDirs.rootfs, ".disk", "info"))
		if err != nil {
			if os.IsNotExist(err) {
				t.Errorf("Disk Info file should exist, but does not")
			}
		}

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedGenerateDiskInfo tests failure scenarios in the generate_disk_info state
func TestFailedGenerateDiskInfo(t *testing.T) {
	t.Run("test_failed_generate_disk_info", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.commonFlags.DiskInfo = filepath.Join("testdata", "fake_disk_info")

		// need workdir set up for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		// mock os.Mkdir
		osMkdir = mockMkdir
		defer func() {
			osMkdir = os.Mkdir
		}()
		if err := stateMachine.generateDiskInfo(); err == nil {
			t.Error("Expected an error, but got none")
		}
		osMkdir = os.Mkdir

		// mock osutil.CopyFile
		osutilCopyFile = mockCopyFile
		defer func() {
			osutilCopyFile = osutil.CopyFile
		}()
		if err := stateMachine.generateDiskInfo(); err == nil {
			t.Error("Expected an error, but got none")
		}
		osutilCopyFile = osutil.CopyFile

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
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
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

// TestCalculateRootfsSize tests that the rootfs size can be calculated
// this is accomplished by setting the test gadget tree as rootfs and
// verifying that the size is calculated correctly
func TestCalculateRootfsSize(t *testing.T) {
	t.Run("test_calculate_rootfs_size", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.tempDirs.rootfs = filepath.Join("testdata", "gadget_tree")

		if err := stateMachine.calculateRootfsSize(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		correctSize := "12.01 MiB"
		if stateMachine.rootfsSize.IECString() != correctSize {
			t.Errorf("expected rootfsSize = %s, got %s", correctSize,
				stateMachine.rootfsSize.IECString())
		}

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedCalculateRootfsSize tests a failure when calculating the rootfs size
// this is accomplished by setting rootfs to a directory that does not exist
func TestFailedCalculateRootfsSize(t *testing.T) {
	t.Run("test_failed_calculate_rootfs_size", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.tempDirs.rootfs = filepath.Join("testdata", uuid.NewString())

		if err := stateMachine.calculateRootfsSize(); err == nil {
			t.Errorf("Expected an error, but got none")
		}

	})
}
