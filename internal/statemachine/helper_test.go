package statemachine

import (
	"bytes"
	"crypto/rand"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/google/uuid"
	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/gadget/quantity"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/osutil/mkfs"
	"github.com/snapcore/snapd/seed"
)

// TestSetupCrossArch tests that the lb commands are set up correctly for cross arch compilation
func TestSetupCrossArch(t *testing.T) {
	t.Run("test_setup_cross_arch", func(t *testing.T) {
		if runtime.GOARCH == "s390x" || runtime.GOARCH == "ppc64le" {
			t.Skipf("No qemu-user-static available on %s", runtime.GOARCH)
		}
		asserter := helper.Asserter{T: t}
		// set up a temp dir for this
		os.MkdirAll(testDir, 0755)
		defer os.RemoveAll(testDir)

		// make sure we always call with a different arch than we are currently running tests on
		var arch string
		if getHostArch() != "arm64" {
			arch = "arm64"
		} else {
			arch = "armhf"
		}

		lbConfig, _, err := setupLiveBuildCommands(testDir, arch, []string{}, true)
		asserter.AssertErrNil(err, true)

		// make sure the qemu args were appended to "lb config"
		qemuArgFound := false
		for _, arg := range lbConfig.Args {
			if arg == "--bootstrap-qemu-arch" {
				qemuArgFound = true
			}
		}
		if !qemuArgFound {
			t.Errorf("lb config command \"%s\" is missing qemu arguments",
				lbConfig.String())
		}
	})
}

// TestFailedSetupLiveBuildCommands tests failures in the setupLiveBuildCommands helper function
func TestFailedSetupLiveBuildCommands(t *testing.T) {
	t.Run("test_failed_setup_live_build_commands", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		// set up a temp dir for this
		os.MkdirAll(testDir, 0755)
		defer os.RemoveAll(testDir)

		// first test a failure in the dpkg command
		// Setup the exec.Command mock
		testCaseName = "TestFailedSetupLiveBuildCommands"
		execCommand = fakeExecCommand
		defer func() {
			execCommand = exec.Command
		}()
		_, _, err := setupLiveBuildCommands(testDir, "amd64", []string{}, true)
		asserter.AssertErrContains(err, "exit status 1")
		execCommand = exec.Command

		// mock osutil.CopySpecialFile
		osutilCopySpecialFile = mockCopySpecialFile
		defer func() {
			osutilCopySpecialFile = osutil.CopySpecialFile
		}()
		_, _, err = setupLiveBuildCommands(testDir, "amd64", []string{}, true)
		asserter.AssertErrContains(err, "Error copying livecd-rootfs/auto")
		osutilCopySpecialFile = osutil.CopySpecialFile

		// use an arch with no qemu-static binary
		os.Unsetenv("UBUNTU_IMAGE_QEMU_USER_STATIC_PATH")
		_, _, err = setupLiveBuildCommands(testDir, "fake64", []string{}, true)
		asserter.AssertErrContains(err, "in case of non-standard archs or custom paths")
	})
}

// TestMaxOffset tests the functionality of the maxOffset function
func TestMaxOffset(t *testing.T) {
	t.Run("test_max_offset", func(t *testing.T) {
		lesser := quantity.Offset(0)
		greater := quantity.Offset(1)

		if maxOffset(lesser, greater) != greater {
			t.Errorf("maxOffset returned the lower number")
		}

		// reverse argument order
		if maxOffset(greater, lesser) != greater {
			t.Errorf("maxOffset returned the lower number")
		}
	})
}

// TestFailedRunHooks tests failures in the runHooks function. This is accomplished by mocking
// functions and calling hook scripts that intentionally return errors
func TestFailedRunHooks(t *testing.T) {
	t.Run("test_failed_run_hooks", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.commonFlags.Debug = true // for coverage!

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// first set a good hooks directory
		stateMachine.commonFlags.HooksDirectories = []string{filepath.Join(
			"testdata", "good_hookscript")}
		// mock ioutil.ReadDir
		ioutilReadDir = mockReadDir
		defer func() {
			ioutilReadDir = ioutil.ReadDir
		}()
		err = stateMachine.runHooks("post-populate-rootfs",
			"UBUNTU_IMAGE_HOOK_ROOTFS", stateMachine.tempDirs.rootfs)
		asserter.AssertErrContains(err, "Error reading hooks directory")
		ioutilReadDir = ioutil.ReadDir

		// now set a hooks directory that will fail
		stateMachine.commonFlags.HooksDirectories = []string{filepath.Join(
			"testdata", "hooks_return_error")}
		err = stateMachine.runHooks("post-populate-rootfs",
			"UBUNTU_IMAGE_HOOK_ROOTFS", stateMachine.tempDirs.rootfs)
		asserter.AssertErrContains(err, "Error running hook")
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedHandleSecureBoot tests failures in the handleSecureBoot function by mocking functions
func TestFailedHandleSecureBoot(t *testing.T) {
	t.Run("test_failed_handle_secure_boot", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		// need workdir for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		// create a volume
		volume := new(gadget.Volume)
		volume.Bootloader = "u-boot"
		// make the u-boot directory and add a file
		bootDir := filepath.Join(stateMachine.tempDirs.unpack,
			"image", "boot", "uboot")
		os.MkdirAll(bootDir, 0755)
		osutil.CopySpecialFile(filepath.Join("testdata", "grubenv"), bootDir)

		// mock os.Mkdir
		osMkdirAll = mockMkdirAll
		defer func() {
			osMkdirAll = os.MkdirAll
		}()
		err := stateMachine.handleSecureBoot(volume, stateMachine.tempDirs.rootfs)
		asserter.AssertErrContains(err, "Error creating ubuntu dir")
		osMkdirAll = os.MkdirAll

		// mock ioutil.ReadDir
		ioutilReadDir = mockReadDir
		defer func() {
			ioutilReadDir = ioutil.ReadDir
		}()
		err = stateMachine.handleSecureBoot(volume, stateMachine.tempDirs.rootfs)
		asserter.AssertErrContains(err, "Error reading boot dir")
		ioutilReadDir = ioutil.ReadDir

		// mock os.Rename
		osRename = mockRename
		defer func() {
			osRename = os.Rename
		}()
		err = stateMachine.handleSecureBoot(volume, stateMachine.tempDirs.rootfs)
		asserter.AssertErrContains(err, "Error copying boot dir")
		osRename = os.Rename
	})
}

// TestFailedHandleSecureBootPiboot tests failures in the handleSecureBoot
// function by mocking functions, for piboot
func TestFailedHandleSecureBootPiboot(t *testing.T) {
	t.Run("test_failed_handle_secure_boot_piboot", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		// need workdir for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		// create a volume
		volume := new(gadget.Volume)
		volume.Bootloader = "piboot"
		// make the piboot directory and add a file
		bootDir := filepath.Join(stateMachine.tempDirs.unpack,
			"image", "boot", "piboot")
		os.MkdirAll(bootDir, 0755)
		osutil.CopySpecialFile(filepath.Join("testdata", "gadget_tree_piboot",
			"piboot.conf"), bootDir)

		// mock os.Mkdir
		osMkdirAll = mockMkdirAll
		defer func() {
			osMkdirAll = os.MkdirAll
		}()
		err := stateMachine.handleSecureBoot(volume, stateMachine.tempDirs.rootfs)
		asserter.AssertErrContains(err, "Error creating ubuntu dir")
		osMkdirAll = os.MkdirAll

		// mock ioutil.ReadDir
		ioutilReadDir = mockReadDir
		defer func() {
			ioutilReadDir = ioutil.ReadDir
		}()
		err = stateMachine.handleSecureBoot(volume, stateMachine.tempDirs.rootfs)
		asserter.AssertErrContains(err, "Error reading boot dir")
		ioutilReadDir = ioutil.ReadDir

		// mock os.Rename
		osRename = mockRename
		defer func() {
			osRename = os.Rename
		}()
		err = stateMachine.handleSecureBoot(volume, stateMachine.tempDirs.rootfs)
		asserter.AssertErrContains(err, "Error copying boot dir")
		osRename = os.Rename
	})
}

// TestHandleLkBootloader tests that the handleLkBootloader function runs successfully
func TestHandleLkBootloader(t *testing.T) {
	t.Run("test_handle_lk_bootloader", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.YamlFilePath = filepath.Join("testdata", "gadget_tree",
			"meta", "gadget.yaml")

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// create image/boot/lk and place a test file there
		bootDir := filepath.Join(stateMachine.tempDirs.unpack, "image", "boot", "lk")
		err = os.MkdirAll(bootDir, 0755)
		asserter.AssertErrNil(err, true)

		err = osutil.CopyFile(filepath.Join("testdata", "disk_info"),
			filepath.Join(bootDir, "disk_info"), 0)
		asserter.AssertErrNil(err, true)

		// set up the volume
		volume := new(gadget.Volume)
		volume.Bootloader = "lk"

		err = stateMachine.handleLkBootloader(volume)
		asserter.AssertErrNil(err, true)

		// ensure the test file was moved
		movedFile := filepath.Join(stateMachine.tempDirs.unpack, "gadget", "disk_info")
		if _, err := os.Stat(movedFile); err != nil {
			t.Errorf("File %s should exist but it does not", movedFile)
		}
	})
}

// TestFailedHandleLkBootloader tests failures in handleLkBootloader by mocking functions
func TestFailedHandleLkBootloader(t *testing.T) {
	t.Run("test_failed_handle_lk_bootloader", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.YamlFilePath = filepath.Join("testdata", "gadget_tree",
			"meta", "gadget.yaml")

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)
		// create image/boot/lk and place a test file there
		bootDir := filepath.Join(stateMachine.tempDirs.unpack, "image", "boot", "lk")
		err = os.MkdirAll(bootDir, 0755)
		asserter.AssertErrNil(err, true)

		err = osutil.CopyFile(filepath.Join("testdata", "disk_info"),
			filepath.Join(bootDir, "disk_info"), 0)
		asserter.AssertErrNil(err, true)

		// set up the volume
		volume := new(gadget.Volume)
		volume.Bootloader = "lk"

		// mock os.Mkdir
		osMkdir = mockMkdir
		defer func() {
			osMkdir = os.Mkdir
		}()
		err = stateMachine.handleLkBootloader(volume)
		asserter.AssertErrContains(err, "Failed to create gadget dir")
		osMkdir = os.Mkdir

		// mock ioutil.ReadDir
		ioutilReadDir = mockReadDir
		defer func() {
			ioutilReadDir = ioutil.ReadDir
		}()
		err = stateMachine.handleLkBootloader(volume)
		asserter.AssertErrContains(err, "Error reading lk bootloader dir")
		ioutilReadDir = ioutil.ReadDir

		// mock osutil.CopySpecialFile
		osutilCopySpecialFile = mockCopySpecialFile
		defer func() {
			osutilCopySpecialFile = osutil.CopySpecialFile
		}()
		err = stateMachine.handleLkBootloader(volume)
		asserter.AssertErrContains(err, "Error copying lk bootloader dir")
		osutilCopySpecialFile = osutil.CopySpecialFile
	})
}

// TestFailedCopyStructureContent tests failures in the copyStructureContent function by mocking
// functions and setting invalid bs= arguments in dd
func TestFailedCopyStructureContent(t *testing.T) {
	t.Run("test_failed_copy_structure_content", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.YamlFilePath = filepath.Join("testdata", "gadget_tree",
			"meta", "gadget.yaml")

		// need workdir and loaded gadget.yaml set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)
		err = stateMachine.loadGadgetYaml()
		asserter.AssertErrNil(err, true)

		// separate out the volumeStructures to test different scenarios
		var mbrStruct gadget.VolumeStructure
		var rootfsStruct gadget.VolumeStructure
		var volume *gadget.Volume = stateMachine.GadgetInfo.Volumes["pc"]
		for _, structure := range volume.Structure {
			if structure.Name == "mbr" {
				mbrStruct = structure
			} else if structure.Name == "EFI System" {
				rootfsStruct = structure
			}
		}

		// mock helper.CopyBlob and test with no filesystem specified
		helperCopyBlob = mockCopyBlob
		defer func() {
			helperCopyBlob = helper.CopyBlob
		}()
		err = stateMachine.copyStructureContent(volume, mbrStruct, 0, "",
			filepath.Join("/tmp", uuid.NewString()+".img"))
		asserter.AssertErrContains(err, "Error zeroing partition")
		helperCopyBlob = helper.CopyBlob

		// set an invalid blocksize to mock the binary copy blob
		mockableBlockSize = "0"
		defer func() {
			mockableBlockSize = "1"
		}()
		err = stateMachine.copyStructureContent(volume, mbrStruct, 0, "",
			filepath.Join("/tmp", uuid.NewString()+".img"))
		asserter.AssertErrContains(err, "Error copying image blob")
		mockableBlockSize = "1"

		// mock helper.CopyBlob and test with filesystem: vfat
		helperCopyBlob = mockCopyBlob
		defer func() {
			helperCopyBlob = helper.CopyBlob
		}()
		err = stateMachine.copyStructureContent(volume, rootfsStruct, 0, "",
			filepath.Join("/tmp", uuid.NewString()+".img"))
		asserter.AssertErrContains(err, "Error zeroing image file")
		helperCopyBlob = helper.CopyBlob

		// mock gadget.MkfsWithContent
		mkfsMakeWithContent = mockMkfsWithContent
		defer func() {
			mkfsMakeWithContent = mkfs.MakeWithContent
		}()
		err = stateMachine.copyStructureContent(volume, rootfsStruct, 0, "",
			filepath.Join("/tmp", uuid.NewString()+".img"))
		asserter.AssertErrContains(err, "Error running mkfs with content")
		mkfsMakeWithContent = mkfs.MakeWithContent

		// mock mkfs.Mkfs
		rootfsStruct.Content = nil // to trigger the "empty partition" case
		mkfsMake = mockMkfs
		defer func() {
			mkfsMake = mkfs.Make
		}()
		err = stateMachine.copyStructureContent(volume, rootfsStruct, 0, "",
			filepath.Join("/tmp", uuid.NewString()+".img"))
		asserter.AssertErrContains(err, "Error running mkfs")
		mkfsMake = mkfs.Make
	})
}

// TestCleanup ensures that the temporary workdir is cleaned up after the
// state machine has finished running
func TestCleanup(t *testing.T) {
	t.Run("test_cleanup", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.Run()
		stateMachine.Teardown()
		if _, err := os.Stat(stateMachine.stateMachineFlags.WorkDir); err == nil {
			t.Errorf("Error: temporary workdir %s was not cleaned up\n",
				stateMachine.stateMachineFlags.WorkDir)
		}
	})
}

// TestFailedCleanup tests a failure in os.RemoveAll while deleting the temporary directory
func TestFailedCleanup(t *testing.T) {
	t.Run("test_failed_cleanup", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.cleanWorkDir = true

		osRemoveAll = mockRemoveAll
		defer func() {
			osRemoveAll = os.RemoveAll
		}()
		err := stateMachine.cleanup()
		asserter.AssertErrContains(err, "Error cleaning up workDir")
	})
}

// TestFailedCalculateImageSize tests a scenario when calculateImageSize() is called
// with a nil pointer to stateMachine.GadgetInfo
func TestFailedCalculateImageSize(t *testing.T) {
	t.Run("test_failed_calculate_image_size", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		_, err := stateMachine.calculateImageSize()
		asserter.AssertErrContains(err, "Cannot calculate image size before initializing GadgetInfo")
	})
}

// TestFailedWriteOffsetValues tests various error scenarios for writeOffsetValues
func TestFailedWriteOffsetValues(t *testing.T) {
	t.Run("test_failed_write_offset_values", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.YamlFilePath = filepath.Join("testdata", "gadget_tree",
			"meta", "gadget.yaml")

		// need workdir and loaded gadget.yaml set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)
		err = stateMachine.loadGadgetYaml()
		asserter.AssertErrNil(err, true)

		// create an empty pc.img
		imgPath := filepath.Join(stateMachine.stateMachineFlags.WorkDir, "pc.img")
		os.Create(imgPath)
		os.Truncate(imgPath, 0)

		volume, found := stateMachine.GadgetInfo.Volumes["pc"]
		if !found {
			t.Fatalf("Failed to find gadget volume")
		}
		// pass an image size that's too small
		err = writeOffsetValues(volume, imgPath, 512, 4)
		asserter.AssertErrContains(err, "write offset beyond end of file")

		// mock os.Open file to force it to use os.O_APPEND, which causes
		// errors in file.WriteAt()
		osOpenFile = mockOpenFileAppend
		defer func() {
			osOpenFile = os.OpenFile
		}()
		err = writeOffsetValues(volume, imgPath, 512, 0)
		asserter.AssertErrContains(err, "Failed to write offset to disk")
		osOpenFile = os.OpenFile
	})
}

// TestWarningRootfsSizeTooSmall tests that a warning is thrown if the structure size
// for the rootfs specified in gadget.yaml is smaller than the calculated rootfs size.
// It also ensures that the size is corrected in the structure struct
func TestWarningRootfsSizeTooSmall(t *testing.T) {
	t.Run("test_warning_rootfs_size_too_small", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		stateMachine.YamlFilePath = filepath.Join("testdata", "gadget_tree",
			"meta", "gadget.yaml")

		// need workdir and loaded gadget.yaml set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)
		err = stateMachine.loadGadgetYaml()
		asserter.AssertErrNil(err, true)

		// set up a "rootfs" that we can calculate the size of
		os.MkdirAll(stateMachine.tempDirs.rootfs, 0755)
		osutil.CopySpecialFile(filepath.Join("testdata", "gadget_tree"), stateMachine.tempDirs.rootfs)

		// ensure volumes exists
		os.MkdirAll(stateMachine.tempDirs.volumes, 0755)

		// calculate the size of the rootfs
		err = stateMachine.calculateRootfsSize()
		asserter.AssertErrNil(err, true)

		// manually set the size of the rootfs structure to 0
		var volume *gadget.Volume = stateMachine.GadgetInfo.Volumes["pc"]
		var rootfsStructure gadget.VolumeStructure
		var rootfsStructureNumber int
		for structureNumber, structure := range volume.Structure {
			if structure.Role == gadget.SystemData {
				structure.Size = 0
				rootfsStructure = structure
				rootfsStructureNumber = structureNumber
			}
		}

		// capture stdout, run copy structure content, and ensure the warning was thrown
		stdout, restoreStdout, err := helper.CaptureStd(&os.Stdout)
		defer restoreStdout()
		asserter.AssertErrNil(err, true)

		err = stateMachine.copyStructureContent(volume,
			rootfsStructure,
			rootfsStructureNumber,
			stateMachine.tempDirs.rootfs,
			filepath.Join(stateMachine.tempDirs.volumes, "part0.img"))
		asserter.AssertErrNil(err, true)

		// restore stdout and check that the warning was printed
		restoreStdout()
		readStdout, err := ioutil.ReadAll(stdout)
		asserter.AssertErrNil(err, true)

		if !strings.Contains(string(readStdout), "WARNING: rootfs structure size 0 B smaller than actual rootfs contents") {
			t.Errorf("Warning about structure size to small not present in stdout: \"%s\"", string(readStdout))
		}

		// check that the size was correctly updated in the volume
		for _, structure := range volume.Structure {
			if structure.Role == gadget.SystemData {
				if structure.Size != stateMachine.RootfsSize {
					t.Errorf("rootfs structure size %s is not equal to calculated size %s",
						structure.Size.IECString(),
						stateMachine.RootfsSize.IECString())
				}
			}
		}
	})
}

// TestGetStructureOffset ensures structure offset safely dereferences structure.Offset
func TestGetStructureOffset(t *testing.T) {
	var testOffset quantity.Offset = 1
	testCases := []struct {
		name      string
		structure gadget.VolumeStructure
		expected  quantity.Offset
	}{
		{"nil", gadget.VolumeStructure{Offset: nil}, 0},
		{"with_value", gadget.VolumeStructure{Offset: &testOffset}, 1},
	}
	for _, tc := range testCases {
		t.Run("test_get_structure_offset_"+tc.name, func(t *testing.T) {
			offset := getStructureOffset(tc.structure)
			if offset != tc.expected {
				t.Errorf("Error, expected offset %d but got %d", tc.expected, offset)
			}
		})
	}
}

// TestGenerateUniqueDiskID ensures that we generate unique disk IDs
func TestGenerateUniqueDiskID(t *testing.T) {
	testCases := []struct {
		name        string
		existing    [][]byte
		randomBytes [][]byte
		expected    []byte
		expectedErr bool
	}{
		{"one_time", [][]byte{{4, 5, 6, 7}}, [][]byte{{0, 1, 2, 3}}, []byte{0, 1, 2, 3}, false},
		{"collision", [][]byte{{0, 1, 2, 3}}, [][]byte{{0, 1, 2, 3}, {4, 5, 6, 7}}, []byte{4, 5, 6, 7}, false},
		{"broken", [][]byte{{0, 0, 0, 0}}, nil, []byte{0, 0, 0, 0}, true},
	}
	for _, tc := range testCases {
		t.Run("test_generate_unique_diskid_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			// create a test rng reader, using data from our testcase
			ithRead := 0
			randRead = func(output []byte) (int, error) {
				var randomBytes []byte
				if tc.randomBytes == nil || ithRead > (len(tc.randomBytes)-1) {
					randomBytes = []byte{0, 0, 0, 0}
				} else {
					randomBytes = tc.randomBytes[ithRead]
				}
				copy(output, randomBytes)
				ithRead++
				return 0, nil
			}
			defer func() {
				randRead = rand.Read
			}()

			randomBytes, err := generateUniqueDiskID(&tc.existing)
			if tc.expectedErr {
				asserter.AssertErrContains(err, "Failed to generate unique disk ID")
			} else {
				asserter.AssertErrNil(err, true)
				if bytes.Compare(randomBytes, tc.expected) != 0 {
					t.Errorf("Error, expected ID %v but got %v", tc.expected, randomBytes)
				}
				// check if the ID was added to the list of existing IDs
				found := false
				for _, id := range tc.existing {
					if bytes.Compare(id, randomBytes) == 0 {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Error, disk ID not added to the existing list")
				}
			}
		})
	}
}

// TestFailedRemovePreseeding tests various failure scenarios in the removePreseeding function
func TestFailedRemovePreseeding(t *testing.T) {
	t.Run("test_failed_remove_preseeding", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		seedDir := filepath.Join(stateMachine.tempDirs.rootfs, "var", "lib", "snapd", "seed")
		err = os.MkdirAll(seedDir, 0755)
		asserter.AssertErrNil(err, true)

		// call "snap prepare image" to preseed the filesystem.
		// Doing the preseed at the time of the test allows it to
		// run on each architecture and keeps the github repository
		// free of large .snap files
		snapPrepareImage := *exec.Command("snap", "prepare-image", "--arch=amd64",
			"--classic", "--snap=core20", "--snap=snapd", "--snap=lxd",
			filepath.Join("testdata", "modelAssertionClassic"),
			stateMachine.tempDirs.rootfs)
		err = snapPrepareImage.Run()
		asserter.AssertErrNil(err, true)

		// mock os.RemoveAll so the directory isn't cleared out every time
		osRemoveAll = mockRemoveAll
		defer func() {
			osRemoveAll = os.RemoveAll
		}()

		// mock seed.Open
		seedOpen = mockSeedOpen
		defer func() {
			seedOpen = seed.Open
		}()
		_, err = removePreseeding(stateMachine.tempDirs.rootfs)
		asserter.AssertErrContains(err, "Test error")
		seedOpen = seed.Open

		// move the model from var/lib/snapd/seed/assertions to cause an error
		err = os.Rename(filepath.Join(seedDir, "assertions", "model"),
			filepath.Join(stateMachine.tempDirs.rootfs, "model"))
		asserter.AssertErrNil(err, true)
		_, err = removePreseeding(stateMachine.tempDirs.rootfs)
		asserter.AssertErrContains(err, "seed must have a model assertion")
		err = os.Rename(filepath.Join(stateMachine.tempDirs.rootfs, "model"),
			filepath.Join(seedDir, "assertions", "model"))
		asserter.AssertErrNil(err, true)

		// move seed.yaml to cause an error in LoadMeta
		err = os.Rename(filepath.Join(seedDir, "seed.yaml"),
			filepath.Join(seedDir, "seed.yaml.bak"))
		asserter.AssertErrNil(err, true)
		_, err = removePreseeding(stateMachine.tempDirs.rootfs)
		asserter.AssertErrContains(err, "no seed metadata")
		err = os.Rename(filepath.Join(seedDir, "seed.yaml.bak"),
			filepath.Join(seedDir, "seed.yaml"))
		asserter.AssertErrNil(err, true)

		// the files have been restored, just test the failure in os.RemoveAll
		_, err = removePreseeding(stateMachine.tempDirs.rootfs)
		asserter.AssertErrContains(err, "Test error")
		osRemoveAll = os.RemoveAll

		//os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// We had a bug where the snap manifest would contain ".snap" in the
// revision field. This test ensures that bug stays fixed
func TestManifestRevisionFormat(t *testing.T) {
	t.Run("test_manifest_revision_format", func(t *testing.T) {
		asserter := helper.Asserter{T: t}

		// generate temporary directory
		tempDir := filepath.Join("/tmp", "manifest-revision-format-"+uuid.NewString())
		err := os.Mkdir(tempDir, 0755)
		asserter.AssertErrNil(err, true)
		defer os.RemoveAll(tempDir)

		fakeSnaps := []string{"test1_123.snap", "test2_456.snap", "test3_789.snap"}
		for _, fakeSnap := range fakeSnaps {
			fullPath := filepath.Join(tempDir, fakeSnap)
			_, err := os.Create(fullPath)
			asserter.AssertErrNil(err, true)
		}

		manifestOutput := filepath.Join(tempDir, "test.manifest")
		err = WriteSnapManifest(tempDir, manifestOutput)
		asserter.AssertErrNil(err, true)

		expectedManifestData := "test1 123\ntest2 456\ntest3 789\n"

		manifestData, err := ioutil.ReadFile(manifestOutput)
		asserter.AssertErrNil(err, true)

		if string(manifestData) != expectedManifestData {
			t.Errorf("Expected manifest file to be:\n%s\nBut got \n%s",
				expectedManifestData, manifestData)
		}
	})
}
