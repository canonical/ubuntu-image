// This file contains unit tests for all of the common state functions
package statemachine

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	diskfs "github.com/diskfs/go-diskfs"
	diskutils "github.com/diskfs/go-diskfs/disk"
	partutils "github.com/diskfs/go-diskfs/partition"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/gadget/quantity"
	"github.com/snapcore/snapd/osutil"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/canonical/ubuntu-image/internal/testhelper"
)

// TestLoadGadgetYaml tests a successful load of gadget.yaml. It also tests that the unpack
// directory is preserved if the relevant environment variable is set
func TestLoadGadgetYaml(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine StateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.YamlFilePath = filepath.Join("testdata", "gadget_tree", "meta", "gadget.yaml")

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	preserveDir := filepath.Join("/tmp", "ubuntu-image-"+uuid.NewString())
	os.Setenv("UBUNTU_IMAGE_PRESERVE_UNPACK", preserveDir)
	defer func() {
		os.Unsetenv("UBUNTU_IMAGE_PRESERVE_UNPACK")
	}()
	// ensure unpack exists
	err = os.MkdirAll(stateMachine.tempDirs.unpack, 0755)
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(preserveDir) })
	err = stateMachine.loadGadgetYaml()
	asserter.AssertErrNil(err, true)

	// check that unpack was preserved
	preserveUnpack := filepath.Join(preserveDir, "unpack")
	if _, err := os.Stat(preserveUnpack); err != nil {
		t.Errorf("Preserve unpack directory %s does not exist", preserveUnpack)
	}
	os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
}

// TestFailedLoadGadgetYaml tests failures in the loadGadgetYaml state
// This is achieved by providing an invalid gadget.yaml and mocking
// os.MkdirAll, iotuil.ReadFile, osutil.CopyFile, and osutil.CopySpecialFile
func TestFailedLoadGadgetYaml(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine StateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

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
	asserter.AssertErrContains(err, "\nThe gadget.yaml file is expected to be located in a \"meta\" subdirectory of the provided built gadget directory.\n")
	osutilCopyFile = osutil.CopyFile

	// mock osReadFile
	osReadFile = mockReadFile
	defer func() {
		osReadFile = os.ReadFile
	}()
	err = stateMachine.loadGadgetYaml()
	asserter.AssertErrContains(err, "Error reading gadget.yaml bytes")
	osReadFile = os.ReadFile

	// now test with the invalid yaml file
	stateMachine.YamlFilePath = filepath.Join("testdata",
		"gadget_tree_invalid", "meta", "gadget.yaml")
	err = stateMachine.loadGadgetYaml()
	asserter.AssertErrContains(err, "Error running InfoFromGadgetYaml")

	stateMachine.YamlFilePath = filepath.Join("testdata", "gadget_no_volumes.yaml")
	err = stateMachine.loadGadgetYaml()
	asserter.AssertErrContains(err, "Specify at least one volume.")

	stateMachine.YamlFilePath = filepath.Join("testdata", "gadget_two_seeded_volumes.yaml")
	err = stateMachine.loadGadgetYaml()
	asserter.AssertErrContains(err, "invalid gadget:")

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
	t.Cleanup(func() { os.RemoveAll(preserveDir) })
	err = stateMachine.loadGadgetYaml()
	asserter.AssertErrContains(err, "Error creating preserve unpack directory")
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
}

// TestGenerateDiskInfo tests that diskInfo can be generated
func TestGenerateDiskInfo(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine StateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.commonFlags.DiskInfo = filepath.Join("testdata", "disk_info")

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

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
}

// TestFailedGenerateDiskInfo tests failure scenarios in the generate_disk_info state
func TestFailedGenerateDiskInfo(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine StateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.commonFlags.DiskInfo = filepath.Join("testdata", "fake_disk_info")

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
}

// TestCalculateRootfsSizeNoImageSize tests that the rootfs size can be
// calculated by using du commands when the image size is not specified
// this is accomplished by setting the test gadget tree as rootfs and
// verifying that the size is calculated correctly
func TestCalculateRootfsSizeNoImageSize(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine StateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.tempDirs.rootfs = filepath.Join("testdata", "gadget_tree")

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	// set a valid yaml file and load it in
	stateMachine.YamlFilePath = filepath.Join("testdata",
		"gadget_tree", "meta", "gadget.yaml")
	// ensure unpack exists
	err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
	asserter.AssertErrNil(err, true)
	err = stateMachine.loadGadgetYaml()
	asserter.AssertErrNil(err, true)

	err = stateMachine.calculateRootfsSize()
	asserter.AssertErrNil(err, true)

	// rootfs size will be slightly different in different environments
	correctSizeLower, err := quantity.ParseSize("8M")
	asserter.AssertErrNil(err, true)
	correctSizeUpper := correctSizeLower + 100000 // 0.1 MB range
	if stateMachine.RootfsSize > correctSizeUpper ||
		stateMachine.RootfsSize < correctSizeLower {
		t.Errorf("expected rootfs size between %s and %s, got %s",
			correctSizeLower.IECString(),
			correctSizeUpper.IECString(),
			stateMachine.RootfsSize.IECString())
	}
}

// TestCalculateRootfsSizeBigSizeGadget tests that the rootfs size can be
// calculated by using the value given in the gadget YAML.
func TestCalculateRootfsSizeBigSizeGadget(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine StateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	// set a valid yaml file and load it in
	stateMachine.YamlFilePath = filepath.Join("testdata", "gadget-big-rootfs.yaml")
	// ensure unpack exists
	err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
	asserter.AssertErrNil(err, true)
	err = stateMachine.loadGadgetYaml()
	asserter.AssertErrNil(err, true)

	err = stateMachine.calculateRootfsSize()
	asserter.AssertErrNil(err, true)

	// rootfs size will be slightly different in different environments
	correctSizeLower, err := quantity.ParseSize("1G")
	asserter.AssertErrNil(err, true)
	correctSizeUpper := correctSizeLower + 100000 // 0.1 MB range
	if stateMachine.RootfsSize > correctSizeUpper ||
		stateMachine.RootfsSize < correctSizeLower {
		t.Errorf("expected rootfs size between %s and %s, got %s",
			correctSizeLower.IECString(),
			correctSizeUpper.IECString(),
			stateMachine.RootfsSize.IECString())
	}
}

// TestCalculateRootfsSizeImageSize tests that the rootfs size can be
// accurately calculated when the image size is specified
func TestCalculateRootfsSizeImageSize(t *testing.T) {
	testCases := []struct {
		name         string
		sizeArg      string
		expectedSize quantity.Size
	}{
		{
			name:         "one image size",
			sizeArg:      "4G",
			expectedSize: 4240407040,
		},
		{
			name:         "image size per volume",
			sizeArg:      "pc:4G",
			expectedSize: 4240407040,
		},
	}
	for _, tc := range testCases {
		t.Run("test_calculate_rootfs_size_image_size", func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.tempDirs.rootfs = filepath.Join("testdata", "gadget_tree")
			stateMachine.commonFlags.Size = tc.sizeArg

			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)

			// set a valid yaml file and load it in
			stateMachine.YamlFilePath = filepath.Join("testdata",
				"gadget_tree", "meta", "gadget.yaml")
			// ensure unpack exists
			err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
			asserter.AssertErrNil(err, true)
			err = stateMachine.loadGadgetYaml()
			asserter.AssertErrNil(err, true)

			err = stateMachine.calculateRootfsSize()
			asserter.AssertErrNil(err, true)

			if stateMachine.RootfsSize != tc.expectedSize {
				t.Errorf("Expected rootfs size %d, but got %d",
					tc.expectedSize, stateMachine.RootfsSize)
			}

			os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
		})
	}
}

// TestFailedCalculateRootfsSize tests a failure when calculating the rootfs size
// this is accomplished by setting rootfs to a directory that does not exist
func TestFailedCalculateRootfsSize(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine StateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.tempDirs.rootfs = filepath.Join("testdata", uuid.NewString())

	err := stateMachine.calculateRootfsSize()
	asserter.AssertErrContains(err, "Error getting rootfs size")

	// now set a value of --image-size that is too small to hold the rootfs
	stateMachine.commonFlags.Size = "1M"

	err = stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	// set a valid yaml file and load it in
	stateMachine.YamlFilePath = filepath.Join("testdata",
		"gadget_tree", "meta", "gadget.yaml")
	// ensure unpack exists
	err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
	asserter.AssertErrNil(err, true)
	err = stateMachine.loadGadgetYaml()
	asserter.AssertErrNil(err, true)

	// capture stdout, run copy structure content, and ensure the warning was thrown
	stdout, restoreStdout, err := helper.CaptureStd(&os.Stdout)
	defer restoreStdout()
	asserter.AssertErrNil(err, true)

	err = stateMachine.calculateRootfsSize()
	asserter.AssertErrNil(err, true)

	// restore stdout and check that the warning was printed
	restoreStdout()
	readStdout, err := io.ReadAll(stdout)
	asserter.AssertErrNil(err, true)

	if !strings.Contains(string(readStdout), "WARNING: rootfs content 8394752 is bigger than requested image size (0). Try using a larger value of --image-size") {
		t.Errorf("Warning about rootfs content size not present in stdout: \"%s\"", string(readStdout))
	}
}

// TestPopulateBootfsContents tests a successful run of the populateBootfsContents state
// and ensures that the appropriate files are placed in the bootfs
func TestPopulateBootfsContents(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine StateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	// set a valid yaml file and load it in
	stateMachine.YamlFilePath = filepath.Join("testdata",
		"gadget_tree", "meta", "gadget.yaml")
	// ensure unpack exists
	err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
	asserter.AssertErrNil(err, true)
	err = stateMachine.loadGadgetYaml()
	asserter.AssertErrNil(err, true)

	// populate unpack
	files, err := os.ReadDir(filepath.Join("testdata", "gadget_tree"))
	asserter.AssertErrNil(err, true)
	for _, srcFile := range files {
		srcFile := filepath.Join("testdata", "gadget_tree", srcFile.Name())
		err = osutilCopySpecialFile(srcFile, filepath.Join(stateMachine.tempDirs.unpack, "gadget"))
		asserter.AssertErrNil(err, true)
	}

	// ensure volumes exists
	err = os.MkdirAll(stateMachine.tempDirs.volumes, 0755)
	asserter.AssertErrNil(err, true)
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
}

// TestPopulateBootfsContentsPiboot tests a successful run of the
// populateBootfsContents state and ensures that the appropriate files are
// placed in the bootfs, for the piboot bootloader.
func TestPopulateBootfsContentsPiboot(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine StateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	// set a valid yaml file and load it in
	stateMachine.YamlFilePath = filepath.Join("testdata",
		"gadget_tree_piboot", "meta", "gadget.yaml")
	// ensure unpack exists
	err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
	asserter.AssertErrNil(err, true)
	err = stateMachine.loadGadgetYaml()
	asserter.AssertErrNil(err, true)

	// populate unpack
	files, err := os.ReadDir(filepath.Join("testdata", "gadget_tree_piboot"))
	asserter.AssertErrNil(err, true)
	for _, srcFile := range files {
		srcFile := filepath.Join("testdata", "gadget_tree_piboot", srcFile.Name())
		err = osutilCopySpecialFile(srcFile, filepath.Join(stateMachine.tempDirs.unpack, "gadget"))
		asserter.AssertErrNil(err, true)
	}

	// ensure volumes exists
	err = os.MkdirAll(stateMachine.tempDirs.volumes, 0755)
	asserter.AssertErrNil(err, true)
	err = stateMachine.populateBootfsContents()
	asserter.AssertErrNil(err, true)

	// check that bootfs contents were actually populated
	bootFiles := []string{"config.txt", "cmdline.txt"}
	for _, file := range bootFiles {
		fullPath := filepath.Join(stateMachine.stateMachineFlags.WorkDir,
			"root", file)
		if _, err := os.Stat(fullPath); err != nil {
			t.Errorf("Expected %s to exist, but it does not", fullPath)
		}
	}
}

// TestFailedPopulateBootfsContents tests failures in the populateBootfsContents state
func TestFailedPopulateBootfsContents(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine StateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	// set a valid yaml file and load it in
	stateMachine.YamlFilePath = filepath.Join("testdata", "gadget-seed.yaml")
	// ensure unpack exists
	err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
	asserter.AssertErrNil(err, true)
	err = stateMachine.loadGadgetYaml()
	asserter.AssertErrNil(err, true)

	// ensure volumes exists
	err = os.MkdirAll(stateMachine.tempDirs.volumes, 0755)
	asserter.AssertErrNil(err, true)

	// populate unpack
	files, err := os.ReadDir(filepath.Join("testdata", "gadget_tree"))
	asserter.AssertErrNil(err, true)
	for _, srcFile := range files {
		srcFile := filepath.Join("testdata", "gadget_tree", srcFile.Name())
		err = osutilCopySpecialFile(srcFile, filepath.Join(stateMachine.tempDirs.unpack, "gadget"))
		asserter.AssertErrNil(err, true)
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
	oldRootfs := stateMachine.tempDirs.rootfs
	stateMachine.tempDirs.rootfs = ""
	err = stateMachine.populateBootfsContents()
	asserter.AssertErrContains(err, "Error in mountedFilesystem.Write")
	// restore rootfs
	stateMachine.tempDirs.rootfs = oldRootfs

	// cause a failure in handleSecureBoot. First change to un-seeded yaml file and load it in
	stateMachine.YamlFilePath = filepath.Join("testdata",
		"gadget_tree", "meta", "gadget.yaml")
	// ensure unpack exists
	err = stateMachine.loadGadgetYaml()
	asserter.AssertErrNil(err, true)
	stateMachine.IsSeeded = false
	// now ensure grub dir exists
	err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack,
		"image", "boot", "grub"), 0755)
	asserter.AssertErrNil(err, true)
	// mock os.MkdirAll
	osMkdirAll = mockMkdirAll
	defer func() {
		osMkdirAll = os.MkdirAll
	}()
	err = stateMachine.populateBootfsContents()
	asserter.AssertErrContains(err, "Error creating ubuntu dir")
	osMkdirAll = os.MkdirAll
}

// TestPopulatePreparePartitions tests a successful run of the populatePreparePartitions state
// and ensures that the appropriate .img files are created. It also tests that sizes smaller than
// the rootfs size are corrected
func TestPopulatePreparePartitions(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine StateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	// set a valid yaml file and load it in
	stateMachine.YamlFilePath = filepath.Join("testdata",
		"gadget_tree", "meta", "gadget.yaml")
	// ensure unpack exists
	err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
	asserter.AssertErrNil(err, true)
	err = stateMachine.loadGadgetYaml()
	asserter.AssertErrNil(err, true)

	// ensure volumes exists
	err = os.MkdirAll(stateMachine.tempDirs.volumes, 0755)
	asserter.AssertErrNil(err, true)

	// populate unpack
	files, err := os.ReadDir(filepath.Join("testdata", "gadget_tree"))
	asserter.AssertErrNil(err, true)
	for _, srcFile := range files {
		srcFile := filepath.Join("testdata", "gadget_tree", srcFile.Name())
		err = osutilCopySpecialFile(srcFile, filepath.Join(stateMachine.tempDirs.unpack, "gadget"))
		asserter.AssertErrNil(err, true)
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
	partImgBytes, err := os.ReadFile(partImg)
	asserter.AssertErrNil(err, true)
	dataBytes := make([]byte, 440)
	// partImg should consist of these 11 bytes and 429 null bytes
	copy(dataBytes[:11], []byte{84, 69, 83, 84, 32, 70, 73, 76, 69, 10})
	if !bytes.Equal(partImgBytes, dataBytes) {
		t.Errorf("Expected part0.img to contain %v, instead got %v %d",
			dataBytes, partImgBytes, len(partImgBytes))
	}
}

// TestFailedPopulatePreparePartitions tests failures in the populatePreparePartitions state
func TestFailedPopulatePreparePartitions(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine StateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	// set a valid yaml file and load it in
	stateMachine.YamlFilePath = filepath.Join("testdata",
		"gadget_tree", "meta", "gadget.yaml")
	// ensure unpack exists
	err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
	asserter.AssertErrNil(err, true)
	err = stateMachine.loadGadgetYaml()
	asserter.AssertErrNil(err, true)

	// ensure volumes exists
	err = os.MkdirAll(stateMachine.tempDirs.volumes, 0755)
	asserter.AssertErrNil(err, true)

	// populate unpack
	files, err := os.ReadDir(filepath.Join("testdata", "gadget_tree"))
	asserter.AssertErrNil(err, true)
	for _, srcFile := range files {
		srcFile := filepath.Join("testdata", "gadget_tree", srcFile.Name())
		err = osutilCopySpecialFile(srcFile, filepath.Join(stateMachine.tempDirs.unpack, "gadget"))
		asserter.AssertErrNil(err, true)
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
}

// TestEmptyPartPopulatePreparePartitions performs a successful run with a gadget.yaml that has,
// besides regular partitions, one empty partition and makes sure that a partition image file
// has been created for it (LP: #1947863)
func TestEmptyPartPopulatePreparePartitions(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine StateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	// set a valid yaml file and load it in
	// we use a special gadget.yaml here, special for this testcase
	stateMachine.YamlFilePath = filepath.Join("testdata",
		"gadget-empty-part.yaml")
	// ensure unpack exists
	err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
	asserter.AssertErrNil(err, true)
	err = stateMachine.loadGadgetYaml()
	asserter.AssertErrNil(err, true)

	// ensure volumes exists
	err = os.MkdirAll(stateMachine.tempDirs.volumes, 0755)
	asserter.AssertErrNil(err, true)

	// populate unpack
	files, err := os.ReadDir(filepath.Join("testdata", "gadget_tree"))
	asserter.AssertErrNil(err, true)
	for _, srcFile := range files {
		srcFile := filepath.Join("testdata", "gadget_tree", srcFile.Name())
		err = osutilCopySpecialFile(srcFile, filepath.Join(stateMachine.tempDirs.unpack, "gadget"))
		asserter.AssertErrNil(err, true)
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
	partImgBytes, err := os.ReadFile(partImg)
	asserter.AssertErrNil(err, true)
	// these are all zeroes
	dataBytes := make([]byte, 4096)
	if !bytes.Equal(partImgBytes, dataBytes) {
		t.Errorf("Expected part2.img to contain %d zeroes, got something different (size %d)",
			len(dataBytes), len(partImgBytes))
	}
}

// TestMakeDiskPartitionSchemes tests that makeDisk() can successfully parse
// mbr, gpt, and hybrid schemes. It then runs "dumpe2fs" to ensure the
// resulting disk has the correct type of partition table.
// We also check various sector sizes while at it and rootfs placements
func TestMakeDiskPartitionSchemes(t *testing.T) {
	testCases := []struct {
		name              string
		tableType         string
		sectorSize        string
		rootfsVolName     string
		rootfsContentPath string
		rootfsPartNum     int
	}{
		{
			name:              "gpt",
			tableType:         "gpt",
			sectorSize:        "512",
			rootfsVolName:     "pc",
			rootfsContentPath: filepath.Join("testdata", "gadget_tree"),
			rootfsPartNum:     3,
		},
		{
			name:              "mbr",
			tableType:         "dos",
			sectorSize:        "512",
			rootfsVolName:     "pc",
			rootfsContentPath: filepath.Join("testdata", "gadget_tree"),
			rootfsPartNum:     3,
		},
		{
			name:              "hybrid",
			tableType:         "gpt",
			sectorSize:        "512",
			rootfsVolName:     "pc",
			rootfsContentPath: filepath.Join("testdata", "gadget_tree"),
			rootfsPartNum:     3,
		},
		// {
		// 	name:          "gpt4k",
		// 	tableType:     "gpt",
		// 	sectorSize:    "4096",
		// 	rootfsVolName: "pc",
		// 	rootfsPartNum: 3,
		// }, // PMBR still seems valid GPT
		{
			name:              "gpt-efi-only",
			tableType:         "gpt",
			sectorSize:        "512",
			rootfsVolName:     "pc",
			rootfsContentPath: filepath.Join("testdata", "gadget_tree"),
			rootfsPartNum:     2,
		},
		{
			name:              "small",
			tableType:         "gpt",
			sectorSize:        "512",
			rootfsVolName:     "pc",
			rootfsContentPath: filepath.Join("testdata", "gadget_tree_piboot"), // bigger than what was calculated based on the rootfs declared in the gadget.yaml
			rootfsPartNum:     1,
		},
	}
	for _, tc := range testCases {
		t.Run("test_make_disk_partition_type_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

			// set the sector size to the one needed during testing
			stateMachine.commonFlags.SectorSize = tc.sectorSize

			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)
			t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

			// also set up an output directory
			outDir, err := os.MkdirTemp("/tmp", "ubuntu-image-")
			asserter.AssertErrNil(err, true)
			t.Cleanup(func() { os.RemoveAll(outDir) })
			stateMachine.commonFlags.OutputDir = outDir

			// set up volume names
			stateMachine.VolumeNames = map[string]string{
				"pc": "pc.img",
			}

			// set a valid yaml file and load it in
			stateMachine.YamlFilePath = filepath.Join("testdata",
				"gadget-"+tc.name+".yaml")
			// ensure unpack exists
			err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
			asserter.AssertErrNil(err, true)
			err = stateMachine.loadGadgetYaml()
			asserter.AssertErrNil(err, true)

			// set up a "rootfs" that we can eventually copy into the disk
			err = os.MkdirAll(stateMachine.tempDirs.rootfs, 0755)
			asserter.AssertErrNil(err, true)
			err = osutil.CopySpecialFile(tc.rootfsContentPath, stateMachine.tempDirs.rootfs)
			asserter.AssertErrNil(err, true)

			// also need to set the rootfs size to avoid partition errors
			err = stateMachine.calculateRootfsSize()
			asserter.AssertErrNil(err, true)

			// ensure volumes exists
			err = os.MkdirAll(stateMachine.tempDirs.volumes, 0755)
			asserter.AssertErrNil(err, true)

			// populate unpack
			files, err := os.ReadDir(filepath.Join("testdata", "gadget_tree"))
			asserter.AssertErrNil(err, true)
			for _, srcFile := range files {
				srcFile := filepath.Join("testdata", "gadget_tree", srcFile.Name())
				err = osutil.CopySpecialFile(srcFile, filepath.Join(stateMachine.tempDirs.unpack, "gadget"))
				asserter.AssertErrNil(err, true)
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
			sfdiskCommand := *exec.Command("sfdisk", "--json", imgFile)

			sfdiskBytes, err := sfdiskCommand.CombinedOutput() // nolint: errcheck
			// The command will return an error because the image itself is not valid but we do
			// not care here.
			t.Logf("sfdiskBytes error: %v\n", err)
			t.Logf("sfdiskBytes stdout: %v\n", string(sfdiskBytes))

			var sfDiskRes testhelper.SfdiskOutput

			err = json.Unmarshal(sfdiskBytes, &sfDiskRes)
			t.Logf("unmarshall error: %v\n", err)
			t.Logf("sfDiskRes: %v\n", sfDiskRes)

			d, err := os.Open(imgFile)
			asserter.AssertErrNil(err, true)

			sectorSize, err := strconv.Atoi(tc.sectorSize)
			asserter.AssertErrNil(err, true)

			parTable, err := partutils.Read(d, sectorSize, sectorSize)
			asserter.AssertErrNil(err, true)
			t.Logf("partTable: %+v\n", parTable)
			t.Logf("partTable type: %+v\n", parTable.Type())

			// if !strings.Contains(string(sfdiskBytes), tc.tableType) {
			// 	t.Errorf("File %s should have partition table %s, instead got \"%s\"",
			// 		imgFile, tc.tableType, string(sfdiskBytes))
			// }

			if sfDiskRes.PartitionTable.Label != tc.tableType {
				t.Errorf("File %s should have partition table %s, instead got \"%s\"",
					imgFile, tc.tableType, sfDiskRes.PartitionTable.Label)
			}

			// ensure the resulting image file is a multiple of the block size
			diskImg, err := diskfs.Open(imgFile)
			asserter.AssertErrNil(err, true)
			defer diskImg.File.Close()
			if diskImg.Size%int64(stateMachine.SectorSize) != 0 {
				t.Errorf("Disk image size %d is not an multiple of the block size: %d",
					diskImg.Size, int64(stateMachine.SectorSize))
			}

			// while at it, ensure that the root partition has been found
			if stateMachine.RootfsPartNum != tc.rootfsPartNum || stateMachine.RootfsVolName != tc.rootfsVolName {
				t.Errorf("Root partition volume/number not detected correctly, expected %s/%d, got %s/%d",
					tc.rootfsVolName, tc.rootfsPartNum, stateMachine.RootfsVolName, stateMachine.RootfsPartNum)
			}
		})
	}
}

// testSetupMakeDiskTest setup needed stateMachine and directories to be used
// in TestFailedMakeDisk* tests
func testSetupMakeDiskTest(t *testing.T, asserter *helper.Asserter, stateMachine *StateMachine, yamlFilePath string) {
	t.Helper()
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	// also set up an output directory
	outDir, err := os.MkdirTemp("/tmp", "ubuntu-image-")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(outDir) })
	stateMachine.commonFlags.OutputDir = outDir
	err = stateMachine.determineOutputDirectory()
	asserter.AssertErrNil(err, true)

	// set up volume names
	stateMachine.VolumeNames = map[string]string{
		"pc": "pc.img",
	}

	// set a valid yaml file and load it in
	stateMachine.YamlFilePath = yamlFilePath
	// ensure unpack exists
	err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
	asserter.AssertErrNil(err, true)

	err = stateMachine.loadGadgetYaml()
	asserter.AssertErrNil(err, true)

	// ensure volumes exists
	err = os.MkdirAll(stateMachine.tempDirs.volumes, 0755)
	asserter.AssertErrNil(err, true)

	// also need to set the rootfs size to avoid partition errors
	err = stateMachine.calculateRootfsSize()
	asserter.AssertErrNil(err, true)

	// populate unpack
	files, err := os.ReadDir(filepath.Join("testdata", "gadget_tree"))
	asserter.AssertErrNil(err, true)
	for _, srcFile := range files {
		srcFile := filepath.Join("testdata", "gadget_tree", srcFile.Name())
		err = osutilCopySpecialFile(srcFile, filepath.Join(stateMachine.tempDirs.unpack, "gadget"))
		asserter.AssertErrNil(err, true)
	}

	err = stateMachine.populateBootfsContents()
	asserter.AssertErrNil(err, true)

	err = stateMachine.populatePreparePartitions()
	asserter.AssertErrNil(err, true)
}

// TestFailedMakeDisk tests failures in the MakeDisk state
func TestFailedMakeDisk(t *testing.T) {
	testCases := []struct {
		name         string
		mockFuncs    func() func()
		expectedErr  string
		yamlFilePath string
	}{
		{
			name: "MBR - failing removing old image",
			mockFuncs: func() func() {
				osRemoveAll = mockRemoveAll
				return func() { osRemoveAll = os.RemoveAll }
			},
			expectedErr:  "Error removing old disk image",
			yamlFilePath: filepath.Join("testdata", "gadget-mbr.yaml"),
		},
		{
			name: "MBR - failing creating disk",
			mockFuncs: func() func() {
				diskfsCreate = mockDiskfsCreate
				return func() { diskfsCreate = diskfs.Create }
			},
			expectedErr:  "Error creating disk image",
			yamlFilePath: filepath.Join("testdata", "gadget-mbr.yaml"),
		},
		{
			name: "MBR - failing when creating a read only disk",
			mockFuncs: func() func() {
				diskfsCreate = readOnlyDiskfsCreate
				return func() { diskfsCreate = diskfs.Create }
			},
			expectedErr:  "Error partitioning image file",
			yamlFilePath: filepath.Join("testdata", "gadget-mbr.yaml"),
		},
		{
			name: "MBR - failing opening disk",
			mockFuncs: func() func() {
				osOpenFile = mockOpenFile
				return func() { osOpenFile = os.OpenFile }
			},
			expectedErr:  "Error opening disk to write MBR disk identifier",
			yamlFilePath: filepath.Join("testdata", "gadget-mbr.yaml"),
		},
		{
			name: "MBR - failing generating a disk ID",
			mockFuncs: func() func() {
				randRead = mockRandRead
				return func() { randRead = rand.Read }
			},
			expectedErr:  "Error generating disk ID",
			yamlFilePath: filepath.Join("testdata", "gadget-mbr.yaml"),
		},
		{
			name: "MBR - failing writing to disk",
			mockFuncs: func() func() {
				osOpenFile = mockOpenFileAppend
				return func() { osOpenFile = os.OpenFile }
			},
			expectedErr:  "Error writing MBR disk identifier",
			yamlFilePath: filepath.Join("testdata", "gadget-mbr.yaml"),
		},
		{
			name: "MBR - failing copying blob to disk",
			mockFuncs: func() func() {
				helperCopyBlob = mockCopyBlob
				return func() { helperCopyBlob = helper.CopyBlob }
			},
			expectedErr:  "Error writing disk image",
			yamlFilePath: filepath.Join("testdata", "gadget-mbr.yaml"),
		},
		{
			name: "GPT - failing writing offset",
			mockFuncs: func() func() {
				osOpenFile = mockOpenFile
				helperCopyBlob = mockCopyBlobSuccess
				return func() {
					osOpenFile = os.OpenFile
					helperCopyBlob = helper.CopyBlob
				}
			},
			expectedErr:  "Error opening image file",
			yamlFilePath: filepath.Join("testdata", "gadget-gpt.yaml"),
		},
		{
			name: "GPT - failing writing to disk image",
			mockFuncs: func() func() {
				helperCopyBlob = mockCopyBlob
				return func() {
					helperCopyBlob = helper.CopyBlob
				}
			},
			expectedErr:  "Error writing disk image",
			yamlFilePath: filepath.Join("testdata", "gadget-gpt.yaml"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			var stateMachine StateMachine

			testSetupMakeDiskTest(t, &asserter, &stateMachine, tc.yamlFilePath)

			if tc.mockFuncs != nil {
				restoreMock := tc.mockFuncs()
				t.Cleanup(restoreMock)
			}
			err := stateMachine.makeDisk()
			defer os.Remove("pc.img")
			if err != nil || len(tc.expectedErr) != 0 {
				asserter.AssertErrContains(err, tc.expectedErr)
			}
		})
	}
}

// TestFailedMakeDisk_NoOutputDir tests failures in the MakeDisk state
func TestFailedMakeDisk_NoOutputDir(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine StateMachine

	testSetupMakeDiskTest(t, &asserter, &stateMachine, filepath.Join("testdata", "gadget-gpt.yaml"))

	helperCopyBlob = mockCopyBlob
	defer func() {
		helperCopyBlob = helper.CopyBlob
	}()
	stateMachine.cleanWorkDir = true // for coverage!
	stateMachine.commonFlags.OutputDir = ""
	defer os.Remove("pc.img")
	err := stateMachine.makeDisk()
	asserter.AssertErrContains(err, "Error writing disk image")
	helperCopyBlob = helper.CopyBlob

	// make sure with no OutputDir the image was created in the cwd
	_, err = os.Stat("pc.img")
	asserter.AssertErrNil(err, true)
}

// TestImageSizeFlag performs a successful call to StateMachine.MakeDisk with the
// --image-size flag, and ensures that the resulting image is the size specified
// with the flag (LP: #1947867)
func TestImageSizeFlag(t *testing.T) {
	testCases := []struct {
		name           string
		sizeArg        string
		gadgetTree     string
		volNames       map[string]string
		wantImageSizes map[string]quantity.Size
	}{
		{
			name:       "one volume",
			sizeArg:    "4G",
			gadgetTree: filepath.Join("testdata", "gadget_tree"),
			volNames: map[string]string{
				"pc": "pc.img",
			},
			wantImageSizes: map[string]quantity.Size{
				"pc": 4*quantity.SizeGiB + 67*quantity.Size(512), // For now add the 67 sectors of the GPT header until we understand why they are added.
			},
		},
		{
			name:       "one volume with requested size smaller than needed size",
			sizeArg:    "4G",
			gadgetTree: filepath.Join("testdata", "gadget_tree"),
			volNames: map[string]string{
				"pc": "pc.img",
			},
			wantImageSizes: map[string]quantity.Size{
				"pc": 4*quantity.SizeGiB + 67*quantity.Size(512), // For now add the 67 sectors of the GPT header until we understand why they are added.
			},
		},
		{
			name:       "multi volume",
			sizeArg:    "first:4G,second:1G",
			gadgetTree: filepath.Join("testdata", "gadget_tree_multi"),
			volNames: map[string]string{
				"first":  "first.img",
				"second": "second.img",
			},
			wantImageSizes: map[string]quantity.Size{
				"first":  4*quantity.SizeGiB + 67*quantity.Size(512), // For now add the 67 sectors of the GPT header until we understand why they are added.
				"second": 1*quantity.SizeGiB + 67*quantity.Size(512), // For now add the 67 sectors of the GPT header until we understand why they are added.
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.IsSeeded = true
			stateMachine.commonFlags.Size = tc.sizeArg

			err := stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)
			t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

			// also set up an output directory
			outDir, err := os.MkdirTemp("/tmp", "ubuntu-image-")
			asserter.AssertErrNil(err, true)
			t.Cleanup(func() { os.RemoveAll(outDir) })
			stateMachine.commonFlags.OutputDir = outDir

			// set up volume names
			stateMachine.VolumeNames = tc.volNames

			// set up a "rootfs" that we can eventually copy into the disk
			err = os.MkdirAll(stateMachine.tempDirs.rootfs, 0755)
			asserter.AssertErrNil(err, true)
			err = osutil.CopySpecialFile(tc.gadgetTree, stateMachine.tempDirs.rootfs)
			asserter.AssertErrNil(err, true)

			// set a valid yaml file and load it in
			stateMachine.YamlFilePath = filepath.Join(tc.gadgetTree, "meta", "gadget.yaml")
			// ensure unpack exists
			err = os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "gadget"), 0755)
			asserter.AssertErrNil(err, true)
			err = stateMachine.loadGadgetYaml()
			asserter.AssertErrNil(err, true)

			// ensure volumes exists
			err = os.MkdirAll(stateMachine.tempDirs.volumes, 0755)
			asserter.AssertErrNil(err, true)
			// populate unpack
			files, err := os.ReadDir(tc.gadgetTree)
			asserter.AssertErrNil(err, true)
			for _, srcFile := range files {
				srcFile := filepath.Join(tc.gadgetTree, srcFile.Name())
				err = osutil.CopySpecialFile(srcFile, filepath.Join(stateMachine.tempDirs.unpack, "gadget"))
				asserter.AssertErrNil(err, true)
			}

			err = stateMachine.parseImageSizes()
			asserter.AssertErrNil(err, true)

			// also need to set the rootfs size to avoid partition errors
			err = stateMachine.calculateRootfsSize()
			asserter.AssertErrNil(err, true)

			// run through the rest of the states
			err = stateMachine.populateBootfsContents()
			asserter.AssertErrNil(err, true)

			err = stateMachine.populatePreparePartitions()
			asserter.AssertErrNil(err, true)

			err = stateMachine.makeDisk()
			asserter.AssertErrNil(err, true)

			// check the size of the disk(s)
			for volume, wantSize := range tc.wantImageSizes {
				imgFile := filepath.Join(stateMachine.commonFlags.OutputDir, volume+".img")
				diskImg, err := os.Stat(imgFile)
				asserter.AssertErrNil(err, true)
				if diskImg.Size() != int64(wantSize) {
					t.Errorf("--image-size %d was specified, but resulting image is %d bytes",
						wantSize, diskImg.Size())
				}
			}
		})

	}
}

var volume1 = &gadget.Volume{
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
			Content:    []gadget.VolumeContent{},
			Update:     gadget.VolumeUpdate{Edition: 2},
			YamlIndex:  1,
		},
		{
			VolumeName: "pc",
			Name:       "ubuntu-data",
			Label:      "writable",
			Offset:     createOffsetPointer(1260388352),
			MinSize:    786432000,
			Size:       786432000,
			Type:       "83,0FC63DAF-8483-4772-8E79-3D69D8477DE4",
			Role:       "system-data",
			Filesystem: "ext4",
			Content:    []gadget.VolumeContent{},
			YamlIndex:  2,
		},
	},
	Name: "pc",
}

var volume2Unaligned = &gadget.Volume{
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
			Offset:     createOffsetPointer(1048476),
			OffsetWrite: &gadget.RelativeOffset{
				RelativeTo: "mbr",
				Offset:     quantity.Offset(92),
			},
			MinSize: 1048476,
			Size:    1048476,
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
			Offset:     createOffsetPointer(2096952),
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
			VolumeName: "pc",
			Name:       "ubuntu-data",
			Label:      "writable",
			Offset:     createOffsetPointer(1260388152),
			MinSize:    786432000,
			Size:       786432000,
			Type:       "83,0FC63DAF-8483-4772-8E79-3D69D8477DE4",
			Role:       "system-data",
			Filesystem: "ext4",
			Content:    []gadget.VolumeContent{},
			YamlIndex:  2,
		},
	},
	Name: "pc",
}

func TestStateMachine_createDiskImage(t *testing.T) {
	cmpOpts := []cmp.Option{
		cmpopts.IgnoreUnexported(
			gadget.Volume{},
			os.File{},
		),
		cmpopts.IgnoreFields(
			diskutils.Disk{},
			"File",
			"Info",
		),
	}
	type args struct {
		volumeName string
		volume     *gadget.Volume
		imgName    string
	}
	tests := []struct {
		name        string
		imageSizes  map[string]quantity.Size
		sectorSize  quantity.Size
		args        args
		wantDiskImg *diskutils.Disk
		wantImgSize quantity.Size
		expectedErr string
	}{
		{
			name:       "basic case",
			sectorSize: quantity.Size(512),
			args: args{
				volumeName: "pc",
				volume:     volume1,
			},
			wantDiskImg: &diskutils.Disk{
				DefaultBlocks:     true,
				Writable:          true,
				PhysicalBlocksize: 512,
				LogicalBlocksize:  512,
				Size:              2046854656,
			},
			wantImgSize: quantity.Size(2046854656),
		},
		{
			name:       "basic case sector size 4k",
			sectorSize: quantity.Size(4096),
			args: args{
				volumeName: "pc",
				volume:     volume1,
			},
			wantDiskImg: &diskutils.Disk{
				DefaultBlocks:     true,
				Writable:          true,
				PhysicalBlocksize: 4096,
				LogicalBlocksize:  4096,
				Size:              2046865408,
			},
			wantImgSize: quantity.Size(2046865408),
		},
		{
			name:       "size to align to sector size 4k",
			sectorSize: quantity.Size(4096),
			args: args{
				volumeName: "pc",
				volume:     volume2Unaligned,
			},
			wantDiskImg: &diskutils.Disk{
				DefaultBlocks:     true,
				Writable:          true,
				PhysicalBlocksize: 4096,
				LogicalBlocksize:  4096,
				Size:              2046865408, // would be 2046820152 if unaligned
			},
			wantImgSize: quantity.Size(2046865408),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}

			stateMachine := &StateMachine{
				SectorSize: tc.sectorSize,
				ImageSizes: tc.imageSizes,
			}

			outDir, err := os.MkdirTemp("/tmp", "ubuntu-image-")
			asserter.AssertErrNil(err, true)
			t.Cleanup(func() { os.RemoveAll(outDir) })

			imgPath := filepath.Join(outDir, tc.args.imgName)

			gotDiskImg, err := stateMachine.createDiskImage(tc.args.volumeName, tc.args.volume, imgPath)

			if err != nil || len(tc.expectedErr) != 0 {
				asserter.AssertErrContains(err, tc.expectedErr)
			}

			asserter.AssertEqual(tc.wantDiskImg, gotDiskImg, cmpOpts...)
			asserter.AssertEqual(tc.wantImgSize, quantity.Size(gotDiskImg.Size))
		})
	}
}
