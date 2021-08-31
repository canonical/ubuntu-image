// This file contains unit tests for all of the common state functions
package statemachine

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/google/uuid"
	"github.com/snapcore/snapd/gadget"
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

// TestLoadGadgetYaml tests a successful load of gadget.yaml. It also tests that the unpack
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
		os.Unsetenv("UBUNTU_IMAGE_PRESERVE_UNPACK")

		// set an invalid --image-size argument to cause a failure
		stateMachine.commonFlags.Size = "test"
		if err := stateMachine.loadGadgetYaml(); err == nil {
			t.Error("Expected an error, but got none")
		}

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

// TestPopulateBootfsContents tests a successful run of the populateBootfsContents state
// and ensures that the appropriate files are placed in the bootfs
func TestPopulateBootfsContents(t *testing.T) {
	t.Run("test_populate_bootfs_contents", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		// need workdir set up for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}
		defer os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)

		// set a valid yaml file and load it in
		stateMachine.yamlFilePath = filepath.Join("testdata",
			"gadget_tree", "meta", "gadget.yaml")
		// ensure unpack exists
		os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
		if err := stateMachine.loadGadgetYaml(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		// populate unpack
		files, _ := ioutil.ReadDir(filepath.Join("testdata", "gadget_tree"))
		for _, srcFile := range files {
			srcFile := filepath.Join("testdata", "gadget_tree", srcFile.Name())
			osutilCopySpecialFile(srcFile, filepath.Join(stateMachine.tempDirs.unpack, "gadget"))
		}

		// ensure volumes exists
		os.MkdirAll(stateMachine.tempDirs.volumes, 0755)
		if err := stateMachine.populateBootfsContents(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		bootFiles := []string{"boot", "ubuntu"}
		for _, file := range bootFiles {
			fullPath := filepath.Join(stateMachine.tempDirs.volumes,
				"pc", "part2", "EFI", file)
			if _, err := os.Stat(fullPath); err != nil {
				t.Errorf("Expected %s to exist, but it does not", fullPath)
			}
		}
	})
}

// TestFailedPopulateBootfsContents tests failures in the populateBootfsContents state
func TestFailedPopulateBootfsContents(t *testing.T) {
	t.Run("test_failed_populate_bootfs_contents", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		// need workdir set up for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}
		defer os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)

		// set a valid yaml file and load it in
		stateMachine.yamlFilePath = filepath.Join("testdata", "gadget-seed.yaml")
		// ensure unpack exists
		os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
		if err := stateMachine.loadGadgetYaml(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		// ensure volumes exists
		os.MkdirAll(stateMachine.tempDirs.volumes, 0755)

		// populate unpack
		files, _ := ioutil.ReadDir(filepath.Join("testdata", "gadget_tree"))
		for _, srcFile := range files {
			srcFile := filepath.Join("testdata", "gadget_tree", srcFile.Name())
			osutilCopySpecialFile(srcFile, filepath.Join(stateMachine.tempDirs.unpack, "gadget"))
		}

		// mock gadget.LayoutVolume
		gadgetLayoutVolume = mockLayoutVolume
		defer func() {
			gadgetLayoutVolume = gadget.LayoutVolume
		}()
		if err := stateMachine.populateBootfsContents(); err == nil {
			t.Errorf("Expected an error, but got none")
		}
		gadgetLayoutVolume = gadget.LayoutVolume

		// mock gadget.NewMountedFilesystemWriter
		gadgetNewMountedFilesystemWriter = mockNewMountedFilesystemWriter
		defer func() {
			gadgetNewMountedFilesystemWriter = gadget.NewMountedFilesystemWriter
		}()
		if err := stateMachine.populateBootfsContents(); err == nil {
			t.Errorf("Expected an error, but got none")
		}
		gadgetNewMountedFilesystemWriter = gadget.NewMountedFilesystemWriter

		// set rootfs to an empty string in order to trigger a failure in Write()
		stateMachine.tempDirs.rootfs = ""
		if err := stateMachine.populateBootfsContents(); err == nil {
			t.Errorf("Expected an error, but got none")
		}

		// cause a failure in handleSecureBoot. First change to un-seeded yaml file and load it in
		stateMachine.yamlFilePath = filepath.Join("testdata",
			"gadget_tree", "meta", "gadget.yaml")
		// ensure unpack exists
		if err := stateMachine.loadGadgetYaml(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}
		stateMachine.isSeeded = false
		// now ensure grub dir exists
		os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack,
			"image", "boot", "grub"), 0755)
		// mock os.MkdirAll
		osMkdirAll = mockMkdirAll
		defer func() {
			osMkdirAll = os.MkdirAll
		}()
		if err := stateMachine.populateBootfsContents(); err == nil {
			t.Error("Expected an error, but got none")
		}
		osMkdirAll = os.MkdirAll
	})
}

// TestPopulatePreparePartitions tests a successful run of the populatePreparePartitions state
// and ensures that the appropriate .img files are created. It also tests that sizes smaller than
// the rootfs size are corrected
func TestPopulatePreparePartitions(t *testing.T) {
	t.Run("test_populate_prepare_partitions", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		// need workdir set up for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}
		defer os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)

		// set a valid yaml file and load it in
		stateMachine.yamlFilePath = filepath.Join("testdata",
			"gadget_tree", "meta", "gadget.yaml")
		// ensure unpack exists
		os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
		if err := stateMachine.loadGadgetYaml(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		// ensure volumes exists
		os.MkdirAll(stateMachine.tempDirs.volumes, 0755)

		// populate unpack
		files, _ := ioutil.ReadDir(filepath.Join("testdata", "gadget_tree"))
		for _, srcFile := range files {
			srcFile := filepath.Join("testdata", "gadget_tree", srcFile.Name())
			osutilCopySpecialFile(srcFile, filepath.Join(stateMachine.tempDirs.unpack, "gadget"))
		}

		// populate bootfs contents to ensure no failures there
		if err := stateMachine.populateBootfsContents(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		// calculate rootfs size so the partition sizes can be set correctly
		if err := stateMachine.calculateRootfsSize(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		if err := stateMachine.populatePreparePartitions(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		// ensure the .img files were created
		for ii := 0; ii < 4; ii++ {
			partImg := filepath.Join(stateMachine.tempDirs.volumes,
				"pc", "part"+strconv.Itoa(ii)+".img")
			if _, err := os.Stat(partImg); err != nil {
				t.Errorf("File %s should exist, but does not", partImg)
			}
		}

		// check the contents of part0.img
		partImg := filepath.Join(stateMachine.tempDirs.volumes,
			"pc", "part0.img")
		partImgBytes, _ := ioutil.ReadFile(partImg)
		dataBytes := make([]byte, 440)
		// partImg should consist of these 11 bytes and 429 null bytes
		copy(dataBytes[:11], []byte{68, 85, 77, 77, 89, 32, 70, 73, 76, 69, 10})
		if !bytes.Equal(partImgBytes, dataBytes) {
			t.Errorf("Expected part0.img to contain %v, instead got %v %d",
				dataBytes, partImgBytes, len(partImgBytes))
		}
	})
}

// TestFailedPopulatePreparePartitions tests failures in the populatePreparePartitions state
func TestFailedPopulatePreparePartitions(t *testing.T) {
	t.Run("test_failed_populate_prepare_partitions", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		// need workdir set up for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}
		defer os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)

		// set a valid yaml file and load it in
		stateMachine.yamlFilePath = filepath.Join("testdata",
			"gadget_tree", "meta", "gadget.yaml")
		// ensure unpack exists
		os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
		if err := stateMachine.loadGadgetYaml(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		// ensure volumes exists
		os.MkdirAll(stateMachine.tempDirs.volumes, 0755)

		// populate unpack
		files, _ := ioutil.ReadDir(filepath.Join("testdata", "gadget_tree"))
		for _, srcFile := range files {
			srcFile := filepath.Join("testdata", "gadget_tree", srcFile.Name())
			osutilCopySpecialFile(srcFile, filepath.Join(stateMachine.tempDirs.unpack, "gadget"))
		}

		// populate bootfs contents to ensure no failures there
		if err := stateMachine.populateBootfsContents(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		// now mock helper.CopyBlob to cause an error in copyStructureContent
		helperCopyBlob = mockCopyBlob
		defer func() {
			helperCopyBlob = helper.CopyBlob
		}()
		if err := stateMachine.populatePreparePartitions(); err == nil {
			t.Errorf("Expected an error, but got none")
		}
		helperCopyBlob = helper.CopyBlob

		// set a bootloader to lk and mock mkdir to cause a failure in that function
		for _, volume := range stateMachine.gadgetInfo.Volumes {
			volume.Bootloader = "lk"
		}
		osMkdir = mockMkdir
		defer func() {
			osMkdir = os.Mkdir
		}()
		if err := stateMachine.populatePreparePartitions(); err == nil {
			t.Errorf("Expected an error, but got none")
		}
		osMkdir = os.Mkdir
	})
}
