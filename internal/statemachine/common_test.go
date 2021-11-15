// This file contains unit tests for all of the common state functions
package statemachine

import (
	"bytes"
	"crypto/rand"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/canonical/ubuntu-image/internal/helper"
	diskfs "github.com/diskfs/go-diskfs"
	"github.com/google/uuid"
	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/gadget/quantity"
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
	t.Run("test_failed_mkdir", func(t *testing.T) {
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
	})
}

// TestLoadGadgetYaml tests a successful load of gadget.yaml. It also tests that the unpack
// directory is preserved if the relevant environment variable is set
func TestLoadGadgetYaml(t *testing.T) {
	t.Run("test_load_gadget_yaml", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.YamlFilePath = filepath.Join("testdata", "gadget_tree", "meta", "gadget.yaml")

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		preserveDir := filepath.Join("/tmp", "ubuntu-image-"+uuid.NewString())
		os.Setenv("UBUNTU_IMAGE_PRESERVE_UNPACK", preserveDir)
		defer func() {
			os.Unsetenv("UBUNTU_IMAGE_PRESERVE_UNPACK")
		}()
		// ensure unpack exists
		os.MkdirAll(stateMachine.tempDirs.unpack, 0755)
		defer os.RemoveAll(preserveDir)
		err = stateMachine.loadGadgetYaml()
		asserter.AssertErrNil(err, true)

		// check that unpack was preserved
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
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		stateMachine.YamlFilePath = filepath.Join("testdata",
			"gadget_tree", "meta", "gadget.yaml")
		// mock osutil.CopySpecialFile
		osutilCopyFile = mockCopyFile
		defer func() {
			osutilCopyFile = osutil.CopyFile
		}()
		err = stateMachine.loadGadgetYaml()
		asserter.AssertErrContains(err, "Error copying gadget.yaml")
		osutilCopyFile = osutil.CopyFile

		// mock ioutilReadFile
		ioutilReadFile = mockReadFile
		defer func() {
			ioutilReadFile = ioutil.ReadFile
		}()
		err = stateMachine.loadGadgetYaml()
		asserter.AssertErrContains(err, "Error reading gadget.yaml bytes")
		ioutilReadFile = ioutil.ReadFile

		// now test with the invalid yaml file
		stateMachine.YamlFilePath = filepath.Join("testdata",
			"gadget_tree_invalid", "meta", "gadget.yaml")
		err = stateMachine.loadGadgetYaml()
		asserter.AssertErrContains(err, "Error running InfoFromGadgetYaml")

		// set a valid yaml file and preserveDir
		stateMachine.YamlFilePath = filepath.Join("testdata",
			"gadget_tree", "meta", "gadget.yaml")

		// mock os.MkdirAll
		osMkdirAll = mockMkdirAll
		defer func() {
			osMkdirAll = os.MkdirAll
		}()
		// run with and without the environment variable set
		err = stateMachine.loadGadgetYaml()
		asserter.AssertErrContains(err, "Error creating volume dir")

		preserveDir := filepath.Join("/tmp", "ubuntu-image-"+uuid.NewString())
		os.Setenv("UBUNTU_IMAGE_PRESERVE_UNPACK", preserveDir)
		defer func() {
			os.Unsetenv("UBUNTU_IMAGE_PRESERVE_UNPACK")
		}()
		defer os.RemoveAll(preserveDir)
		err = stateMachine.loadGadgetYaml()
		asserter.AssertErrContains(err, "Error creating preserve_unpack directory")
		osMkdirAll = os.MkdirAll

		// mock osutil.CopySpecialFile
		osutilCopySpecialFile = mockCopySpecialFile
		defer func() {
			osutilCopySpecialFile = osutil.CopySpecialFile
		}()
		err = stateMachine.loadGadgetYaml()
		asserter.AssertErrContains(err, "Error preserving unpack dir")
		osutilCopySpecialFile = osutil.CopySpecialFile
		os.Unsetenv("UBUNTU_IMAGE_PRESERVE_UNPACK")

		// set an invalid --image-size argument to cause a failure
		stateMachine.commonFlags.Size = "test"
		err = stateMachine.loadGadgetYaml()
		asserter.AssertErrContains(err, "Failed to parse argument to --image-size")

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
			asserter := helper.Asserter{T: t}
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.commonFlags.Debug = true
			stateMachine.commonFlags.HooksDirectories = []string{
				filepath.Join("testdata", "good_hooksd"),
				filepath.Join("testdata", "good_hookscript"),
			}
			stateMachine.IsSeeded = tc.isSeeded

			// need workdir set up for this
			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)

			err = stateMachine.populateRootfsContentsHooks()
			asserter.AssertErrNil(err, false)

			// the hook scripts used for testing simply touches some files.
			// make sure they were successfully created
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
			asserter := helper.Asserter{T: t}
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.commonFlags.HooksDirectories = tc.hooksDirs
			stateMachine.IsSeeded = false

			// need workdir set up for this
			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)

			err = stateMachine.populateRootfsContentsHooks()
			asserter.AssertErrContains(err, "Error running hook")
			os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
		})
	}
}

// TestGenerateDiskInfo tests that diskInfo can be generated
func TestGenerateDiskInfo(t *testing.T) {
	t.Run("test_generate_disk_info", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.commonFlags.DiskInfo = filepath.Join("testdata", "disk_info")

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		err = stateMachine.generateDiskInfo()
		asserter.AssertErrNil(err, true)

		// make sure rootfs/.disk/info exists
		_, err = os.Stat(filepath.Join(stateMachine.tempDirs.rootfs, ".disk", "info"))
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
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.commonFlags.DiskInfo = filepath.Join("testdata", "fake_disk_info")

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// mock os.Mkdir
		osMkdir = mockMkdir
		defer func() {
			osMkdir = os.Mkdir
		}()
		err = stateMachine.generateDiskInfo()
		asserter.AssertErrContains(err, "Failed to create disk info directory")
		osMkdir = os.Mkdir

		// mock osutil.CopyFile
		osutilCopyFile = mockCopyFile
		defer func() {
			osutilCopyFile = osutil.CopyFile
		}()
		err = stateMachine.generateDiskInfo()
		asserter.AssertErrContains(err, "Failed to copy Disk Info file")
		osutilCopyFile = osutil.CopyFile

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestCalculateRootfsSize tests that the rootfs size can be calculated
// this is accomplished by setting the test gadget tree as rootfs and
// verifying that the size is calculated correctly
func TestCalculateRootfsSize(t *testing.T) {
	t.Run("test_calculate_rootfs_size", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.tempDirs.rootfs = filepath.Join("testdata", "gadget_tree")

		// set a valid yaml file and load it in
		stateMachine.YamlFilePath = filepath.Join("testdata",
			"gadget_tree", "meta", "gadget.yaml")
		// ensure unpack exists
		os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
		err := stateMachine.loadGadgetYaml()
		asserter.AssertErrNil(err, true)

		err = stateMachine.calculateRootfsSize()
		asserter.AssertErrNil(err, true)

		// rootfs size will be slightly different in different environments
		correctSizeLower, _ := quantity.ParseSize("8M")
		correctSizeUpper := correctSizeLower + 100000 // 0.1 MB range
		if stateMachine.RootfsSize > correctSizeUpper ||
			stateMachine.RootfsSize < correctSizeLower {
			t.Errorf("expected rootfs size between %s and %s, got %s",
				correctSizeLower.IECString(),
				correctSizeUpper.IECString(),
				stateMachine.RootfsSize.IECString())
		}

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedCalculateRootfsSize tests a failure when calculating the rootfs size
// this is accomplished by setting rootfs to a directory that does not exist
func TestFailedCalculateRootfsSize(t *testing.T) {
	t.Run("test_failed_calculate_rootfs_size", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.tempDirs.rootfs = filepath.Join("testdata", uuid.NewString())

		err := stateMachine.calculateRootfsSize()
		asserter.AssertErrContains(err, "Error getting rootfs size")
	})
}

// TestPopulateBootfsContents tests a successful run of the populateBootfsContents state
// and ensures that the appropriate files are placed in the bootfs
func TestPopulateBootfsContents(t *testing.T) {
	t.Run("test_populate_bootfs_contents", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)
		defer os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)

		// set a valid yaml file and load it in
		stateMachine.YamlFilePath = filepath.Join("testdata",
			"gadget_tree", "meta", "gadget.yaml")
		// ensure unpack exists
		os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
		err = stateMachine.loadGadgetYaml()
		asserter.AssertErrNil(err, true)

		// populate unpack
		files, _ := ioutil.ReadDir(filepath.Join("testdata", "gadget_tree"))
		for _, srcFile := range files {
			srcFile := filepath.Join("testdata", "gadget_tree", srcFile.Name())
			osutilCopySpecialFile(srcFile, filepath.Join(stateMachine.tempDirs.unpack, "gadget"))
		}

		// ensure volumes exists
		os.MkdirAll(stateMachine.tempDirs.volumes, 0755)
		err = stateMachine.populateBootfsContents()
		asserter.AssertErrNil(err, true)

		// check that bootfs contents were actually populated
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
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)
		defer os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)

		// set a valid yaml file and load it in
		stateMachine.YamlFilePath = filepath.Join("testdata", "gadget-seed.yaml")
		// ensure unpack exists
		os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
		err = stateMachine.loadGadgetYaml()
		asserter.AssertErrNil(err, true)

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
		err = stateMachine.populateBootfsContents()
		asserter.AssertErrContains(err, "Error laying out bootfs contents")
		gadgetLayoutVolume = gadget.LayoutVolume

		// mock gadget.NewMountedFilesystemWriter
		gadgetNewMountedFilesystemWriter = mockNewMountedFilesystemWriter
		defer func() {
			gadgetNewMountedFilesystemWriter = gadget.NewMountedFilesystemWriter
		}()
		err = stateMachine.populateBootfsContents()
		asserter.AssertErrContains(err, "Error creating NewMountedFilesystemWriter")
		gadgetNewMountedFilesystemWriter = gadget.NewMountedFilesystemWriter

		// set rootfs to an empty string in order to trigger a failure in Write()
		stateMachine.tempDirs.rootfs = ""
		err = stateMachine.populateBootfsContents()
		asserter.AssertErrContains(err, "Error in mountedFilesystem.Write")

		// cause a failure in handleSecureBoot. First change to un-seeded yaml file and load it in
		stateMachine.YamlFilePath = filepath.Join("testdata",
			"gadget_tree", "meta", "gadget.yaml")
		// ensure unpack exists
		err = stateMachine.loadGadgetYaml()
		asserter.AssertErrNil(err, true)
		stateMachine.IsSeeded = false
		// now ensure grub dir exists
		os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack,
			"image", "boot", "grub"), 0755)
		// mock os.MkdirAll
		osMkdirAll = mockMkdirAll
		defer func() {
			osMkdirAll = os.MkdirAll
		}()
		err = stateMachine.populateBootfsContents()
		asserter.AssertErrContains(err, "Error creating ubuntu dir")
		osMkdirAll = os.MkdirAll
	})
}

// TestPopulatePreparePartitions tests a successful run of the populatePreparePartitions state
// and ensures that the appropriate .img files are created. It also tests that sizes smaller than
// the rootfs size are corrected
func TestPopulatePreparePartitions(t *testing.T) {
	t.Run("test_populate_prepare_partitions", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)
		defer os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)

		// set a valid yaml file and load it in
		stateMachine.YamlFilePath = filepath.Join("testdata",
			"gadget_tree", "meta", "gadget.yaml")
		// ensure unpack exists
		os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
		err = stateMachine.loadGadgetYaml()
		asserter.AssertErrNil(err, true)

		// ensure volumes exists
		os.MkdirAll(stateMachine.tempDirs.volumes, 0755)

		// populate unpack
		files, _ := ioutil.ReadDir(filepath.Join("testdata", "gadget_tree"))
		for _, srcFile := range files {
			srcFile := filepath.Join("testdata", "gadget_tree", srcFile.Name())
			osutilCopySpecialFile(srcFile, filepath.Join(stateMachine.tempDirs.unpack, "gadget"))
		}

		// populate bootfs contents to ensure no failures there
		err = stateMachine.populateBootfsContents()
		asserter.AssertErrNil(err, true)

		// calculate rootfs size so the partition sizes can be set correctly
		err = stateMachine.calculateRootfsSize()
		asserter.AssertErrNil(err, true)

		err = stateMachine.populatePreparePartitions()
		asserter.AssertErrNil(err, true)

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
		copy(dataBytes[:11], []byte{84, 69, 83, 84, 32, 70, 73, 76, 69, 10})
		if !bytes.Equal(partImgBytes, dataBytes) {
			t.Errorf("Expected part0.img to contain %v, instead got %v %d",
				dataBytes, partImgBytes, len(partImgBytes))
		}
	})
}

// TestFailedPopulatePreparePartitions tests failures in the populatePreparePartitions state
func TestFailedPopulatePreparePartitions(t *testing.T) {
	t.Run("test_failed_populate_prepare_partitions", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)
		defer os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)

		// set a valid yaml file and load it in
		stateMachine.YamlFilePath = filepath.Join("testdata",
			"gadget_tree", "meta", "gadget.yaml")
		// ensure unpack exists
		os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
		err = stateMachine.loadGadgetYaml()
		asserter.AssertErrNil(err, true)

		// ensure volumes exists
		os.MkdirAll(stateMachine.tempDirs.volumes, 0755)

		// populate unpack
		files, _ := ioutil.ReadDir(filepath.Join("testdata", "gadget_tree"))
		for _, srcFile := range files {
			srcFile := filepath.Join("testdata", "gadget_tree", srcFile.Name())
			osutilCopySpecialFile(srcFile, filepath.Join(stateMachine.tempDirs.unpack, "gadget"))
		}

		// populate bootfs contents to ensure no failures there
		err = stateMachine.populateBootfsContents()
		asserter.AssertErrNil(err, true)

		// now mock helper.CopyBlob to cause an error in copyStructureContent
		helperCopyBlob = mockCopyBlob
		defer func() {
			helperCopyBlob = helper.CopyBlob
		}()
		err = stateMachine.populatePreparePartitions()
		asserter.AssertErrContains(err, "Error zeroing partition")
		helperCopyBlob = helper.CopyBlob

		// set a bootloader to lk and mock mkdir to cause a failure in that function
		for _, volume := range stateMachine.GadgetInfo.Volumes {
			volume.Bootloader = "lk"
		}
		osMkdir = mockMkdir
		defer func() {
			osMkdir = os.Mkdir
		}()
		err = stateMachine.populatePreparePartitions()
		asserter.AssertErrContains(err, "got lk bootloader but directory")
		osMkdir = os.Mkdir
	})
}

// TestEmptyPartPopulatePreparePartitions performs a successful run a gadget.yaml that has,
// besides regular partitions, one empty partition and makes sure that a partition image file
// has been created for it (LP: #1947863)
func TestEmptyPartPopulatePreparePartitions(t *testing.T) {
	t.Run("test_empty_part_populate_prepare_partitions", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)
		defer os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)

		// set a valid yaml file and load it in
		// we use a special gadget.yaml here, special for this testcase
		stateMachine.YamlFilePath = filepath.Join("testdata",
			"gadget-empty-part.yaml")
		// ensure unpack exists
		os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
		err = stateMachine.loadGadgetYaml()
		asserter.AssertErrNil(err, true)

		// ensure volumes exists
		os.MkdirAll(stateMachine.tempDirs.volumes, 0755)

		// populate unpack
		files, _ := ioutil.ReadDir(filepath.Join("testdata", "gadget_tree"))
		for _, srcFile := range files {
			srcFile := filepath.Join("testdata", "gadget_tree", srcFile.Name())
			osutilCopySpecialFile(srcFile, filepath.Join(stateMachine.tempDirs.unpack, "gadget"))
		}

		// populate bootfs contents to ensure no failures there
		err = stateMachine.populateBootfsContents()
		asserter.AssertErrNil(err, true)

		// calculate rootfs size so the partition sizes can be set correctly
		err = stateMachine.calculateRootfsSize()
		asserter.AssertErrNil(err, true)

		err = stateMachine.populatePreparePartitions()
		asserter.AssertErrNil(err, true)

		// ensure the .img files were created
		for ii := 0; ii < 5; ii++ {
			partImg := filepath.Join(stateMachine.tempDirs.volumes,
				"pc", "part"+strconv.Itoa(ii)+".img")
			if _, err := os.Stat(partImg); err != nil {
				t.Errorf("File %s should exist, but does not", partImg)
			}
		}

		// check part2.img, it should be empty and have a 4K size
		partImg := filepath.Join(stateMachine.tempDirs.volumes,
			"pc", "part2.img")
		partImgBytes, _ := ioutil.ReadFile(partImg)
		// these are all zeroes
		dataBytes := make([]byte, 4096)
		if !bytes.Equal(partImgBytes, dataBytes) {
			t.Errorf("Expected part2.img to contain %d zeroes, got something different (size %d)",
				len(dataBytes), len(partImgBytes))
		}
	})
}

// TestMakeDiskPartitionSchemes tests that makeDisk() can successfully parse
// mbr, gpt, and hybrid schemes. It then runs "dumpe2fs" to ensure the
// resulting disk has the correct type of partition table
func TestMakeDiskPartitionSchemes(t *testing.T) {
	testCases := []struct {
		name      string
		tableType string
	}{
		{"gpt", "gpt"},
		{"mbr", "dos"},
		{"hybrid", "gpt"},
	}
	for _, tc := range testCases {
		t.Run("test_make_disk_partition_type_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

			// need workdir set up for this
			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)
			defer os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)

			// also set up an output directory
			outDir, err := ioutil.TempDir("/tmp", "ubuntu-image-")
			asserter.AssertErrNil(err, true)
			defer os.RemoveAll(outDir)
			stateMachine.commonFlags.OutputDir = outDir

			// set a valid yaml file and load it in
			stateMachine.YamlFilePath = filepath.Join("testdata",
				"gadget-"+tc.name+".yaml")
			// ensure unpack exists
			os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
			err = stateMachine.loadGadgetYaml()
			asserter.AssertErrNil(err, true)

			// set up a "rootfs" that we can eventually copy into the disk
			os.MkdirAll(stateMachine.tempDirs.rootfs, 0755)
			osutil.CopySpecialFile(filepath.Join("testdata", "gadget_tree"), stateMachine.tempDirs.rootfs)

			// also need to set the rootfs size to avoid partition errors
			err = stateMachine.calculateRootfsSize()
			asserter.AssertErrNil(err, true)

			// ensure volumes exists
			os.MkdirAll(stateMachine.tempDirs.volumes, 0755)

			// populate unpack
			files, _ := ioutil.ReadDir(filepath.Join("testdata", "gadget_tree"))
			for _, srcFile := range files {
				srcFile := filepath.Join("testdata", "gadget_tree", srcFile.Name())
				osutil.CopySpecialFile(srcFile, filepath.Join(stateMachine.tempDirs.unpack, "gadget"))
			}

			// run through the rest of the states
			err = stateMachine.populateBootfsContents()
			asserter.AssertErrNil(err, true)

			err = stateMachine.populatePreparePartitions()
			asserter.AssertErrNil(err, true)

			err = stateMachine.makeDisk()
			asserter.AssertErrNil(err, true)

			// now run "dumpe2fs" to ensure the correct type of partition table exists
			imgFile := filepath.Join(stateMachine.commonFlags.OutputDir, "pc.img")
			dumpe2fsCommand := *exec.Command("dumpe2fs", imgFile)

			dumpe2fsBytes, _ := dumpe2fsCommand.CombinedOutput()
			if !strings.Contains(string(dumpe2fsBytes), tc.tableType) {
				t.Errorf("File %s should have partition table %s, instead got \"%s\"",
					imgFile, tc.tableType, string(dumpe2fsBytes))
			}

			// ensure the resulting image file is a multiple of the block size
			diskImg, err := diskfs.Open(imgFile)
			defer diskImg.File.Close()
			asserter.AssertErrNil(err, true)
			if diskImg.Size%diskImg.LogicalBlocksize != 0 {
				t.Errorf("Disk image size %d is not an multiple of the block size: %d",
					diskImg.Size, diskImg.LogicalBlocksize)
			}
		})
	}
}

// TestFailedMakeDisk tests failures in the MakeDisk state
func TestFailedMakeDisk(t *testing.T) {
	t.Run("test_failed_make_disk", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)
		defer os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)

		// also set up an output directory
		outDir, err := ioutil.TempDir("/tmp", "ubuntu-image-")
		asserter.AssertErrNil(err, true)
		defer os.RemoveAll(outDir)
		stateMachine.commonFlags.OutputDir = outDir

		// set a valid yaml file and load it in
		stateMachine.YamlFilePath = filepath.Join("testdata", "gadget-mbr.yaml")
		// ensure unpack exists
		os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
		err = stateMachine.loadGadgetYaml()
		asserter.AssertErrNil(err, true)

		// also need to set the rootfs size to avoid partition errors
		err = stateMachine.calculateRootfsSize()
		asserter.AssertErrNil(err, true)

		// ensure volumes exists
		os.MkdirAll(stateMachine.tempDirs.volumes, 0755)

		// populate unpack
		files, _ := ioutil.ReadDir(filepath.Join("testdata", "gadget_tree"))
		for _, srcFile := range files {
			srcFile := filepath.Join("testdata", "gadget_tree", srcFile.Name())
			osutilCopySpecialFile(srcFile, filepath.Join(stateMachine.tempDirs.unpack, "gadget"))
		}

		// mock os.MkdirAll
		osMkdirAll = mockMkdirAll
		defer func() {
			osMkdirAll = os.MkdirAll
		}()
		err = stateMachine.makeDisk()
		asserter.AssertErrContains(err, "Error creating OutputDir")
		osMkdirAll = os.MkdirAll

		// mock os.RemoveAll
		osRemoveAll = mockRemoveAll
		defer func() {
			osRemoveAll = os.RemoveAll
		}()
		err = stateMachine.makeDisk()
		asserter.AssertErrContains(err, "Error removing old disk image")
		osRemoveAll = os.RemoveAll

		// mock diskfs.Create
		diskfsCreate = mockDiskfsCreate
		defer func() {
			diskfsCreate = diskfs.Create
		}()
		err = stateMachine.makeDisk()
		asserter.AssertErrContains(err, "Error creating disk image")
		diskfsCreate = diskfs.Create

		// mock os.Truncate
		osTruncate = mockTruncate
		defer func() {
			osTruncate = os.Truncate
		}()
		err = stateMachine.makeDisk()
		asserter.AssertErrContains(err, "Error resizing disk image")
		osTruncate = os.Truncate

		// mock diskfs.Create to create a read only disk
		diskfsCreate = readOnlyDiskfsCreate
		defer func() {
			diskfsCreate = diskfs.Create
		}()
		err = stateMachine.makeDisk()
		asserter.AssertErrContains(err, "Error partitioning image file")
		diskfsCreate = diskfs.Create

		// mock os.OpenFile
		// errors in file.WriteAt()
		osOpenFile = mockOpenFile
		defer func() {
			osOpenFile = os.OpenFile
		}()
		err = stateMachine.makeDisk()
		asserter.AssertErrContains(err, "Error opening disk to write MBR disk identifier")
		osOpenFile = os.OpenFile

		// mock rand.Read
		// errors in generateUniqueDiskID()
		randRead = mockRandRead
		defer func() {
			randRead = rand.Read
		}()
		err = stateMachine.makeDisk()
		asserter.AssertErrContains(err, "Error generating disk ID")
		randRead = rand.Read

		// mock os.OpenFile to force it to use os.O_APPEND, which causes
		// errors in file.WriteAt()
		osOpenFile = mockOpenFileAppend
		defer func() {
			osOpenFile = os.OpenFile
		}()
		err = stateMachine.makeDisk()
		asserter.AssertErrContains(err, "Error writing MBR disk identifier")
		osOpenFile = os.OpenFile

		// mock helper.CopyBlob to simulate a failure in copyDataToImage
		helperCopyBlob = mockCopyBlob
		defer func() {
			helperCopyBlob = helper.CopyBlob
		}()
		err = stateMachine.makeDisk()
		asserter.AssertErrContains(err, "Error writing disk image")
		helperCopyBlob = helper.CopyBlob

		// Change to GPT for these next tests
		stateMachine.YamlFilePath = filepath.Join("testdata", "gadget-gpt.yaml")
		err = stateMachine.loadGadgetYaml()
		asserter.AssertErrNil(err, true)

		err = stateMachine.populateBootfsContents()
		asserter.AssertErrNil(err, true)

		err = stateMachine.populatePreparePartitions()
		asserter.AssertErrNil(err, true)

		// mock os.OpenFile to simulate a failure in writeOffsetValues
		osOpenFile = mockOpenFile
		defer func() {
			osOpenFile = os.OpenFile
		}()
		// also mock helperCopyBlob to ignore missing files and return success
		helperCopyBlob = mockCopyBlobSuccess
		defer func() {
			helperCopyBlob = helper.CopyBlob
		}()
		err = stateMachine.makeDisk()
		asserter.AssertErrContains(err, "Error opening image file")
		osOpenFile = os.OpenFile
		helperCopyBlob = helper.CopyBlob

		helperCopyBlob = mockCopyBlob
		defer func() {
			helperCopyBlob = helper.CopyBlob
		}()
		stateMachine.cleanWorkDir = true // for coverage!
		stateMachine.commonFlags.OutputDir = ""
		err = stateMachine.makeDisk()
		asserter.AssertErrContains(err, "Error writing disk image")
		helperCopyBlob = helper.CopyBlob
	})
}

// TestImageSizeFlag performs a successful call to StateMachine.MakeDisk with the
// --image-size flag, and ensures that the resulting image is the size specified
// with the flag (LP: #1947867)
func TestImageSizeFlag(t *testing.T) {
	testCases := []struct {
		name       string
		sizeArg    string
		gadgetTree string
		imageSize  map[string]quantity.Size
	}{
		{"one_volume", "4G", filepath.Join("testdata", "gadget_tree"),
			map[string]quantity.Size{"pc": 4 * quantity.SizeGiB}},
		{"multi-volume", "first:4G,second:1G",
			filepath.Join("testdata", "gadget_tree_multi"),
			map[string]quantity.Size{
				"first":  4 * quantity.SizeGiB,
				"second": 1 * quantity.SizeGiB}},
	}
	for _, tc := range testCases {

		t.Run("test_image_size_flag_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.IsSeeded = true
			stateMachine.commonFlags.Size = tc.sizeArg

			// need workdir set up for this
			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)
			//defer os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)

			// also set up an output directory
			outDir, err := ioutil.TempDir("/tmp", "ubuntu-image-")
			asserter.AssertErrNil(err, true)
			//defer os.RemoveAll(outDir)
			stateMachine.commonFlags.OutputDir = outDir

			// set a valid yaml file and load it in
			stateMachine.YamlFilePath = filepath.Join(tc.gadgetTree, "meta", "gadget.yaml")
			// ensure unpack exists
			os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
			err = stateMachine.loadGadgetYaml()
			asserter.AssertErrNil(err, true)

			// set up a "rootfs" that we can eventually copy into the disk
			os.MkdirAll(stateMachine.tempDirs.rootfs, 0755)
			osutil.CopySpecialFile(tc.gadgetTree, stateMachine.tempDirs.rootfs)

			// also need to set the rootfs size to avoid partition errors
			err = stateMachine.calculateRootfsSize()
			asserter.AssertErrNil(err, true)

			// ensure volumes exists
			os.MkdirAll(stateMachine.tempDirs.volumes, 0755)

			// populate unpack
			files, _ := ioutil.ReadDir(tc.gadgetTree)
			for _, srcFile := range files {
				srcFile := filepath.Join(tc.gadgetTree, srcFile.Name())
				osutil.CopySpecialFile(srcFile, filepath.Join(stateMachine.tempDirs.unpack, "gadget"))
			}

			// run through the rest of the states
			err = stateMachine.populateBootfsContents()
			asserter.AssertErrNil(err, true)

			err = stateMachine.populatePreparePartitions()
			asserter.AssertErrNil(err, true)

			err = stateMachine.makeDisk()
			asserter.AssertErrNil(err, true)

			// check the size of the disk(s)
			for volume, expectedSize := range tc.imageSize {
				imgFile := filepath.Join(stateMachine.commonFlags.OutputDir, volume+".img")
				diskImg, err := os.Stat(imgFile)
				asserter.AssertErrNil(err, true)
				if diskImg.Size() != int64(expectedSize) {
					t.Errorf("--image-size %d was specified, but resulting image is %d bytes",
						expectedSize, diskImg.Size())
				}
			}
		})

	}
}
