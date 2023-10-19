package statemachine

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/gadget/quantity"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/osutil/mkfs"
	"github.com/snapcore/snapd/seed"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/canonical/ubuntu-image/internal/imagedefinition"
)

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
		err := os.MkdirAll(bootDir, 0755)
		asserter.AssertErrNil(err, true)
		err = osutil.CopySpecialFile(filepath.Join("testdata", "grubenv"), bootDir)
		asserter.AssertErrNil(err, true)

		// mock os.Mkdir
		osMkdirAll = mockMkdirAll
		defer func() {
			osMkdirAll = os.MkdirAll
		}()
		err = stateMachine.handleSecureBoot(volume, stateMachine.tempDirs.rootfs)
		asserter.AssertErrContains(err, "Error creating ubuntu dir")
		osMkdirAll = os.MkdirAll

		// mock os.ReadDir
		osReadDir = mockReadDir
		defer func() {
			osReadDir = os.ReadDir
		}()
		err = stateMachine.handleSecureBoot(volume, stateMachine.tempDirs.rootfs)
		asserter.AssertErrContains(err, "Error reading boot dir")
		osReadDir = os.ReadDir

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
		err := os.MkdirAll(bootDir, 0755)
		asserter.AssertErrNil(err, true)
		err = osutil.CopySpecialFile(filepath.Join("testdata", "gadget_tree_piboot",
			"piboot.conf"), bootDir)
		asserter.AssertErrNil(err, true)

		// mock os.Mkdir
		osMkdirAll = mockMkdirAll
		defer func() {
			osMkdirAll = os.MkdirAll
		}()
		err = stateMachine.handleSecureBoot(volume, stateMachine.tempDirs.rootfs)
		asserter.AssertErrContains(err, "Error creating ubuntu dir")
		osMkdirAll = os.MkdirAll

		// mock os.ReadDir
		osReadDir = mockReadDir
		defer func() {
			osReadDir = os.ReadDir
		}()
		err = stateMachine.handleSecureBoot(volume, stateMachine.tempDirs.rootfs)
		asserter.AssertErrContains(err, "Error reading boot dir")
		osReadDir = os.ReadDir

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

		// mock os.ReadDir
		osReadDir = mockReadDir
		defer func() {
			osReadDir = os.ReadDir
		}()
		err = stateMachine.handleLkBootloader(volume)
		asserter.AssertErrContains(err, "Error reading lk bootloader dir")
		osReadDir = os.ReadDir

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

		// mock os.ReadDir
		osReadDir = mockReadDir
		defer func() {
			osReadDir = os.ReadDir
		}()
		err = stateMachine.copyStructureContent(volume, rootfsStruct, 0, "",
			filepath.Join("/tmp", uuid.NewString()+".img"))
		asserter.AssertErrContains(err, "Error listing contents of volume")
		osReadDir = os.ReadDir

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
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		// create the workdir and make sure it is set to be cleaned up
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)
		stateMachine.cleanWorkDir = true
		err = stateMachine.Teardown()
		asserter.AssertErrNil(err, true)
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
		_, err = os.Create(imgPath)
		asserter.AssertErrNil(err, true)
		err = os.Truncate(imgPath, 0)
		asserter.AssertErrNil(err, true)
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
		err = os.MkdirAll(stateMachine.tempDirs.rootfs, 0755)
		asserter.AssertErrNil(err, true)
		err = osutil.CopySpecialFile(filepath.Join("testdata", "gadget_tree"), stateMachine.tempDirs.rootfs)
		asserter.AssertErrNil(err, true)

		// ensure volumes exists
		err = os.MkdirAll(stateMachine.tempDirs.volumes, 0755)
		asserter.AssertErrNil(err, true)

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
		readStdout, err := io.ReadAll(stdout)
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
	for i, tc := range testCases {
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

			randomBytes, err := generateUniqueDiskID(&testCases[i].existing)
			if tc.expectedErr {
				asserter.AssertErrContains(err, "Failed to generate unique disk ID")
			} else {
				asserter.AssertErrNil(err, true)
				if !bytes.Equal(randomBytes, tc.expected) {
					t.Errorf("Error, expected ID %v but got %v", tc.expected, randomBytes)
				}
				// check if the ID was added to the list of existing IDs
				found := false
				for _, id := range testCases[i].existing {
					if bytes.Equal(id, randomBytes) {
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

// TestGetHostArch unit tests the getHostArch function
func TestGetHostArch(t *testing.T) {
	t.Run("test_get_host_arch", func(t *testing.T) {
		hostArch := getHostArch()
		switch runtime.GOARCH {
		case "amd64":
			expected := "amd64"
			if hostArch != expected {
				t.Errorf("Wrong value of getHostArch. Expected %s, got %s", expected, hostArch)
			}
		case "arm":
			expected := "armhf"
			if hostArch != expected {
				t.Errorf("Wrong value of getHostArch. Expected %s, got %s", "amd64", hostArch)
			}
		case "arm64":
			expected := "arm64"
			if hostArch != expected {
				t.Errorf("Wrong value of getHostArch. Expected %s, got %s", "amd64", hostArch)
			}
		case "ppc64le":
			expected := "ppc64el"
			if hostArch != expected {
				t.Errorf("Wrong value of getHostArch. Expected %s, got %s", "amd64", hostArch)
			}
		case "s390x":
			expected := "s390x"
			if hostArch != expected {
				t.Errorf("Wrong value of getHostArch. Expected %s, got %s", "amd64", hostArch)
			}
		case "riscv64":
			expected := "riscv64"
			if hostArch != expected {
				t.Errorf("Wrong value of getHostArch. Expected %s, got %s", "amd64", hostArch)
			}
		default:
			t.Skipf("Test not supported on architecture %s", runtime.GOARCH)
		}
	})
}

// TestGetHostSuite unit tests the getHostSuite function to make sure
// it returns a string with length greater than zero
func TestGetHostSuite(t *testing.T) {
	t.Run("test_get_host_suite", func(t *testing.T) {
		hostSuite := getHostSuite()
		if len(hostSuite) == 0 {
			t.Error("getHostSuite could not get the host suite")
		}
	})
}

// TestGetQemuStaticForArch unit tests the getQemuStaticForArch function
func TestGetQemuStaticForArch(t *testing.T) {
	testCases := []struct {
		arch     string
		expected string
	}{
		{"amd64", ""},
		{"armhf", "qemu-arm-static"},
		{"arm64", "qemu-aarch64-static"},
		{"ppc64el", "qemu-ppc64le-static"},
		{"s390x", ""},
		{"riscv64", ""},
	}
	for _, tc := range testCases {
		t.Run("test_get_qemu_static_for_"+tc.arch, func(t *testing.T) {
			qemuStatic := getQemuStaticForArch(tc.arch)
			if qemuStatic != tc.expected {
				t.Errorf("Expected qemu static \"%s\" for arch \"%s\", instead got \"%s\"",
					tc.expected, tc.arch, qemuStatic)
			}
		})
	}
}

// TestGenerateGerminateCmd unit tests the generateGerminateCmd function
func TestGenerateGerminateCmd(t *testing.T) {
	testCases := []struct {
		name     string
		mirror   string
		seedURLs []string
		vcs      bool
	}{
		{
			name:     "amd64",
			mirror:   "http://archive.ubuntu.com/ubuntu/",
			seedURLs: []string{"git://test.git"},
			vcs:      true,
		},
		{
			name:     "armhf",
			mirror:   "http://ports.ubuntu.com/ubuntu/",
			seedURLs: []string{"git://test.git"},
			vcs:      true,
		},
		{
			name:     "arm64",
			mirror:   "http://ports.ubuntu.com/ubuntu/",
			seedURLs: []string{"git://test.git"},
			vcs:      true,
		},
		{
			name:     "arm64",
			mirror:   "http://ports.ubuntu.com/ubuntu/",
			seedURLs: []string{"https://ubuntu-archive-team.ubuntu.com/seeds/"},
			vcs:      false,
		},
	}
	for _, tc := range testCases {
		t.Run("test_generate_germinate_cmd_"+tc.name, func(t *testing.T) {
			imageDef := imagedefinition.ImageDefinition{
				Architecture: tc.name,
				Rootfs: &imagedefinition.Rootfs{
					Mirror: tc.mirror,
					Seed: &imagedefinition.Seed{
						SeedURLs:   tc.seedURLs,
						SeedBranch: "testbranch",
						Vcs:        helper.BoolPtr(tc.vcs),
					},
					Components: []string{"main", "universe"},
				},
			}
			germinateCmd := generateGerminateCmd(imageDef)
			fmt.Print(germinateCmd)

			if !strings.Contains(germinateCmd.String(), tc.mirror) {
				t.Errorf("germinate command \"%s\" has incorrect mirror. Expected \"%s\"",
					germinateCmd.String(), tc.mirror)
			}

			if !strings.Contains(germinateCmd.String(), "--components=main,universe") {
				t.Errorf("Expected germinate command \"%s\" to contain "+
					"\"--components=main,universe\"", germinateCmd.String())
			}

			if strings.Contains(germinateCmd.String(), "--vcs=auto") && !tc.vcs {
				t.Errorf("Germinate command \"%s\" should not contain "+
					"\"--vcs=auto\"", germinateCmd.String())
			}

			if !strings.Contains(germinateCmd.String(), "--vcs=auto") && tc.vcs {
				t.Errorf("Expected germinate command \"%s\" to contain "+
					"\"--vcs=auto\"", germinateCmd.String())
			}
		})
	}
}

// TestValidateInput tests that invalid state machine command line arguments result in a failure
func TestValidateInput(t *testing.T) {
	testCases := []struct {
		name    string
		until   string
		thru    string
		debug   bool
		verbose bool
		resume  bool
		errMsg  string
	}{
		{"both_until_and_thru", "make_temporary_directories", "calculate_rootfs_size", false, false, false, "cannot specify both --until and --thru"},
		{"resume_with_no_workdir", "", "", false, false, true, "must specify workdir when using --resume flag"},
		{"both_debug_and_verbose", "", "", true, true, false, "--quiet, --verbose, and --debug flags are mutually exclusive"},
	}
	for _, tc := range testCases {
		t.Run("test "+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.stateMachineFlags.Until = tc.until
			stateMachine.stateMachineFlags.Thru = tc.thru
			stateMachine.stateMachineFlags.Resume = tc.resume
			stateMachine.commonFlags.Debug = tc.debug
			stateMachine.commonFlags.Verbose = tc.verbose

			err := stateMachine.validateInput()
			asserter.AssertErrContains(err, tc.errMsg)
		})
	}
}

// TestValidateUntilThru ensures that using invalid value for --thru
// or --until returns an error
func TestValidateUntilThru(t *testing.T) {
	testCases := []struct {
		name  string
		until string
		thru  string
	}{
		{"invalid_until_name", "fake step", ""},
		{"invalid_thru_name", "", "fake step"},
	}
	for _, tc := range testCases {
		t.Run("test "+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.stateMachineFlags.Until = tc.until
			stateMachine.stateMachineFlags.Thru = tc.thru

			err := stateMachine.validateUntilThru()
			asserter.AssertErrContains(err, "not a valid state name")

		})
	}
}

// TestFailedManualCopyFile tests the fail case of the manualCopyFile function
func TestFailedManualCopyFile(t *testing.T) {
	t.Run("test_failed_manual_copy_file", func(t *testing.T) {
		asserter := helper.Asserter{T: t}

		copyFiles := []*imagedefinition.CopyFile{
			{
				Dest:   "/test/does/not/exist",
				Source: "/test/does/not/exist",
			},
		}
		err := manualCopyFile(copyFiles, "/tmp", "/fakedir", true)
		asserter.AssertErrContains(err, "Error copying file")
	})
}

// TestFailedManualTouchFile tests the fail case of the manualTouchFile function
func TestFailedManualTouchFile(t *testing.T) {
	t.Run("test_failed_manual_touch_file", func(t *testing.T) {
		asserter := helper.Asserter{T: t}

		touchFiles := []*imagedefinition.TouchFile{
			{
				TouchPath: "/test/does/not/exist",
			},
		}
		err := manualTouchFile(touchFiles, "/fakedir", true)
		asserter.AssertErrContains(err, "Error creating file")
	})
}

// TestFailedManualExecute tests the fail case of the manualExecute function
func TestFailedManualExecute(t *testing.T) {
	t.Run("test_failed_manual_execute", func(t *testing.T) {
		asserter := helper.Asserter{T: t}

		executes := []*imagedefinition.Execute{
			{
				ExecutePath: "/test/does/not/exist",
			},
		}
		err := manualExecute(executes, "fakedir", true)
		asserter.AssertErrContains(err, "Error running script")
	})
}

// TestFailedManualAddGroup tests the fail case of the manualAddGroup function
func TestFailedManualAddGroup(t *testing.T) {
	t.Run("test_failed_manual_add_group", func(t *testing.T) {
		asserter := helper.Asserter{T: t}

		addGroups := []*imagedefinition.AddGroup{
			{
				GroupName: "testgroup",
				GroupID:   "123",
			},
		}
		err := manualAddGroup(addGroups, "fakedir", true)
		asserter.AssertErrContains(err, "Error adding group")
	})
}

// TestFailedManualAddUser tests the fail case of the manualAddUser function
func TestFailedManualAddUser(t *testing.T) {
	t.Run("test_failed_manual_add_user", func(t *testing.T) {
		asserter := helper.Asserter{T: t}

		addUsers := []*imagedefinition.AddUser{
			{
				UserName: "testuser",
				UserID:   "123",
			},
		}
		err := manualAddUser(addUsers, "fakedir", true)
		asserter.AssertErrContains(err, "Error adding user")
	})
}

// TestGenerateAptCmd unit tests the generateAptCmd function
func TestGenerateAptCmds(t *testing.T) {
	testCases := []struct {
		name        string
		targetDir   string
		packageList []string
		expected    string
	}{
		{"one_package", "chroot1", []string{"test"}, "chroot chroot1 apt install --assume-yes --quiet --option=Dpkg::options::=--force-unsafe-io --option=Dpkg::Options::=--force-confold test"},
		{"many_packages", "chroot2", []string{"test1", "test2"}, "chroot chroot2 apt install --assume-yes --quiet --option=Dpkg::options::=--force-unsafe-io --option=Dpkg::Options::=--force-confold test1 test2"},
	}
	for _, tc := range testCases {
		t.Run("test_generate_apt_cmd_"+tc.name, func(t *testing.T) {
			aptCmds := generateAptCmds(tc.targetDir, tc.packageList)
			if !strings.Contains(aptCmds[1].String(), tc.expected) {
				t.Errorf("Expected apt command \"%s\" but got \"%s\"", tc.expected, aptCmds[1].String())
			}
		})
	}
}

// TestCreatePPAInfo unit tests the createPPAInfo function
/* TODO: this is the logic for deb822 sources. When other projects
(software-properties, ubuntu-release-upgrader) are ready, update
to this logic instead.
func TestCreatePPAInfo(t *testing.T) {
	testCases := []struct {
		name             string
		ppa              *imagedefinition.PPA
		series           string
		expectedName     string
		expectedContents string
	}{
		{
			"public_ppa",
			&imagedefinition.PPA{
				PPAName: "public/ppa",
			},
			"focal",
			"public-ubuntu-ppa-focal.sources",
			`X-Repolib-Name: public/ppa
Enabled: yes
Types: deb
URIS: https://ppa.launchpadcontent.net/public/ppa/ubuntu
Suites: focal
Components: main`,
		},
		{
			"private_ppa",
			&imagedefinition.PPA{
				PPAName: "private/ppa",
				Auth:    "testuser:testpass",
			},
			"jammy",
			"private-ubuntu-ppa-jammy.sources",
			`X-Repolib-Name: private/ppa
Enabled: yes
Types: deb
URIS: https://testuser:testpass@private-ppa.launchpadcontent.net/private/ppa/ubuntu
Suites: jammy
Components: main`,
		},
	}
	for _, tc := range testCases {
		t.Run("test_create_ppa_info_"+tc.name, func(t *testing.T) {
			fileName, fileContents := createPPAInfo(tc.ppa, tc.series)
			if fileName != tc.expectedName {
				t.Errorf("Expected PPA filename \"%s\" but got \"%s\"",
					tc.expectedName, fileName)
			}
			if fileContents != tc.expectedContents {
				t.Errorf("Expected PPA file contents \"%s\" but got \"%s\"",
					tc.expectedContents, fileContents)
			}
		})
	}
}
*/
// TestCreatePPAInfo unit tests the createPPAInfo function
func TestCreatePPAInfo(t *testing.T) {
	testCases := []struct {
		name             string
		ppa              *imagedefinition.PPA
		series           string
		expectedName     string
		expectedContents string
	}{
		{
			"public_ppa",
			&imagedefinition.PPA{
				PPAName: "public/ppa",
			},
			"focal",
			"public-ubuntu-ppa-focal.list",
			"deb https://ppa.launchpadcontent.net/public/ppa/ubuntu focal main",
		},
		{
			"private_ppa",
			&imagedefinition.PPA{
				PPAName: "private/ppa",
				Auth:    "testuser:testpass",
			},
			"jammy",
			"private-ubuntu-ppa-jammy.list",
			"deb https://testuser:testpass@private-ppa.launchpadcontent.net/private/ppa/ubuntu jammy main"},
	}
	for _, tc := range testCases {
		t.Run("test_create_ppa_info_"+tc.name, func(t *testing.T) {
			fileName, fileContents := createPPAInfo(tc.ppa, tc.series)
			if fileName != tc.expectedName {
				t.Errorf("Expected PPA filename \"%s\" but got \"%s\"",
					tc.expectedName, fileName)
			}
			if fileContents != tc.expectedContents {
				t.Errorf("Expected PPA file contents \"%s\" but got \"%s\"",
					tc.expectedContents, fileContents)
			}
		})
	}
}

// TestImportPPAKeys unit tests the importPPAKeys function
func TestImportPPAKeys(t *testing.T) {
	testCases := []struct {
		name        string
		ppa         *imagedefinition.PPA
		keyFileName string
	}{
		{
			"public_ppa",
			&imagedefinition.PPA{
				PPAName: "canonical-foundations/ubuntu-image",
			},
			"public-canonical-foundations-ubuntu-image.key",
		},
		{
			"private_ppa",
			&imagedefinition.PPA{
				PPAName:     "canonical-foundations/ubuntu-image-private-test",
				Auth:        "testuser:testpass",
				Fingerprint: "CDE5112BD4104F975FC8A53FD4C0B668FD4C9139",
			},
			"private-canonical-foundations-ubuntu-image-private-test.key",
		},
	}
	for _, tc := range testCases {
		t.Run("test_import_ppa_keys_"+tc.name, func(t *testing.T) {
			// a byte representation of the canonical-foundations public key
			expectedContents := []byte{152, 141, 4, 80, 190, 39, 112, 1, 4, 0,
				230, 108, 176, 170, 73, 12, 198, 74, 110, 107, 31, 246, 81, 240,
				148, 160, 52, 188, 222, 178, 67, 75, 14, 32, 247, 57, 211, 76, 26,
				31, 124, 83, 11, 75, 21, 150, 164, 28, 222, 149, 49, 8, 135, 97, 83,
				234, 138, 214, 97, 246, 15, 157, 69, 24, 112, 219, 231, 78, 36, 208,
				186, 91, 248, 177, 57, 233, 53, 154, 253, 125, 241, 173, 154, 148,
				65, 125, 20, 15, 87, 242, 144, 74, 238, 71, 133, 182, 9, 217, 23, 93,
				155, 230, 52, 19, 90, 180, 79, 70, 186, 180, 122, 167, 141, 59, 95,
				148, 196, 15, 231, 101, 80, 175, 225, 32, 163, 66, 111, 155, 167,
				76, 72, 165, 5, 190, 22, 192, 10, 241, 0, 17, 1, 0, 1, 180, 44, 76,
				97, 117, 110, 99, 104, 112, 97, 100, 32, 80, 80, 65, 32, 102, 111,
				114, 32, 67, 97, 110, 111, 110, 105, 99, 97, 108, 32, 70, 111, 117,
				110, 100, 97, 116, 105, 111, 110, 115, 32, 84, 101, 97, 109, 136,
				184, 4, 19, 1, 2, 0, 34, 5, 2, 80, 190, 39, 112, 2, 27, 3, 6, 11, 9, 8,
				7, 3, 2, 6, 21, 8, 2, 9, 10, 11, 4, 22, 2, 3, 1, 2, 30, 1, 2, 23, 128, 0,
				10, 9, 16, 212, 192, 182, 104, 253, 76, 145, 57, 187, 243, 3, 255,
				104, 246, 106, 227, 175, 235, 102, 20, 23, 4, 251, 60, 236, 165,
				143, 184, 161, 133, 147, 154, 172, 228, 130, 18, 36, 6, 138, 214,
				188, 32, 142, 251, 143, 144, 40, 43, 147, 187, 230, 224, 254, 161,
				7, 57, 165, 220, 85, 55, 99, 70, 96, 62, 81, 208, 249, 131, 8, 241,
				77, 213, 22, 252, 235, 214, 35, 182, 195, 224, 45, 231, 196, 7, 238,
				147, 115, 81, 142, 71, 216, 96, 159, 11, 146, 83, 153, 107, 194, 167,
				80, 30, 223, 236, 146, 29, 176, 101, 33, 237, 38, 7, 163, 183, 217,
				42, 32, 163, 33, 163, 92, 45, 99, 62, 217, 78, 163, 45, 55, 209, 137,
				43, 69, 78, 53, 177, 207, 209, 123, 186,
			}

			asserter := helper.Asserter{T: t}

			// create a temporary gpg keyring directory
			tmpGPGDir, err := os.MkdirTemp("/tmp", "ubuntu-image-gpg-test")
			defer os.RemoveAll(tmpGPGDir)
			asserter.AssertErrNil(err, true)

			// create a temporary trusted.gpg.d directory
			tmpTrustedDir, err := os.MkdirTemp("/tmp", "ubuntu-image-trusted.gpg.d")
			//defer os.RemoveAll(tmpTrustedDir)
			asserter.AssertErrNil(err, true)

			keyFilePath := filepath.Join(tmpTrustedDir, tc.keyFileName)
			err = importPPAKeys(tc.ppa, tmpGPGDir, keyFilePath, false)
			asserter.AssertErrNil(err, true)

			keyData, err := os.ReadFile(keyFilePath)
			asserter.AssertErrNil(err, true)

			if !reflect.DeepEqual(keyData, expectedContents) {
				t.Errorf("Expected key file to be:\n%d\n\nbut got\n%d",
					expectedContents, keyData)
			}
		})
	}
}

// TestFailedImportPPAKeys tests failures in the importPPAKeys function
func TestFailedImportPPAKeys(t *testing.T) {
	t.Run("test_failed_import_ppa_keys", func(t *testing.T) {
		asserter := helper.Asserter{T: t}

		// create a temporary gpg keyring directory
		tmpGPGDir, err := os.MkdirTemp("/tmp", "ubuntu-image-gpg-test")
		defer os.RemoveAll(tmpGPGDir)
		asserter.AssertErrNil(err, true)

		// create a temporary trusted.gpg.d directory
		tmpTrustedDir, err := os.MkdirTemp("/tmp", "ubuntu-image-trusted.gpg.d")
		defer os.RemoveAll(tmpTrustedDir)
		asserter.AssertErrNil(err, true)
		keyFilePath := filepath.Join(tmpTrustedDir, "test.key")

		// try to import an invalid gpg fingerprint
		ppa := &imagedefinition.PPA{
			PPAName:     "test-bad/fingerprint",
			Fingerprint: "testfakefingperint",
		}

		err = importPPAKeys(ppa, tmpGPGDir, keyFilePath, false)
		asserter.AssertErrContains(err, "Error running gpg command")

		// now use a valid PPA and mock some functions
		ppa = &imagedefinition.PPA{
			PPAName: "canonical-foundations/ubuntu-image",
		}

		// mock http.Get
		httpGet = mockGet
		defer func() {
			httpGet = http.Get
		}()
		err = importPPAKeys(ppa, tmpGPGDir, keyFilePath, false)
		asserter.AssertErrContains(err, "Error getting signing key")
		httpGet = http.Get

		// mock io.ReadAll
		ioReadAll = mockReadAll
		defer func() {
			ioReadAll = io.ReadAll
		}()
		err = importPPAKeys(ppa, tmpGPGDir, keyFilePath, false)
		asserter.AssertErrContains(err, "Error reading signing key")
		ioReadAll = io.ReadAll

		// mock json.Unmarshal
		jsonUnmarshal = mockUnmarshal
		defer func() {
			jsonUnmarshal = json.Unmarshal
		}()
		err = importPPAKeys(ppa, tmpGPGDir, keyFilePath, false)
		asserter.AssertErrContains(err, "Error unmarshalling launchpad API response")
		jsonUnmarshal = json.Unmarshal
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

		manifestData, err := os.ReadFile(manifestOutput)
		asserter.AssertErrNil(err, true)

		if string(manifestData) != expectedManifestData {
			t.Errorf("Expected manifest file to be:\n%s\nBut got \n%s",
				expectedManifestData, manifestData)
		}
	})
}

// TestLP1981720 tests a bug that occurred when a structure had no content specified,
// but the content was created by an earlier step of ubuntu-image
// https://bugs.launchpad.net/ubuntu-image/+bug/1981720
func TestLP1981720(t *testing.T) {
	t.Run("test_lp1981720", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.YamlFilePath = filepath.Join("testdata", "gadget-no-content.yaml")

		// need workdir and loaded gadget.yaml set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)
		err = stateMachine.loadGadgetYaml()
		asserter.AssertErrNil(err, true)

		var bootStruct gadget.VolumeStructure
		var volume *gadget.Volume = stateMachine.GadgetInfo.Volumes["pc"]
		for _, structure := range volume.Structure {
			if structure.Name == "system-boot" {
				bootStruct = structure
			}
		}

		// create a temporary file for contentRoot
		contentRoot := filepath.Join("/tmp", uuid.NewString())
		err = os.Mkdir(contentRoot, 0755)
		defer os.RemoveAll(contentRoot)
		asserter.AssertErrNil(err, true)
		testFile, err := os.Create(filepath.Join(contentRoot, "test.txt"))
		asserter.AssertErrNil(err, true)
		testData := []byte("Test string that we will search the resulting image for")
		_, err = testFile.Write(testData)
		asserter.AssertErrNil(err, true)

		// now execute copyStructureContent
		err = stateMachine.copyStructureContent(volume, bootStruct, 0, contentRoot,
			contentRoot+".img")
		asserter.AssertErrNil(err, true)

		// now check that the resulting .img file has the contents of test.txt in it
		structureContent, err := os.ReadFile(contentRoot + ".img")
		asserter.AssertErrNil(err, true)

		if !bytes.Contains(structureContent, testData) {
			t.Errorf("Test data is missing from output of copyStructureContent")
		}

		os.RemoveAll(contentRoot)
	})
}

// TestCheckCustomizationSteps unit tests the checkCustomizationSteps function
func TestCheckCustomizationSteps(t *testing.T) {
	testCases := []struct {
		name          string
		customization *imagedefinition.Customization
		expectedSteps []string
	}{
		{
			"add_extra_ppas",
			&imagedefinition.Customization{
				ExtraPPAs: []*imagedefinition.PPA{
					{
						PPAName: "test",
					},
				},
			},
			[]string{
				"add_extra_ppas",
				"clean_extra_ppas",
			},
		},
		{
			"install_extra_packages",
			&imagedefinition.Customization{
				ExtraPackages: []*imagedefinition.Package{
					{
						PackageName: "test",
					},
				},
			},
			[]string{
				"install_extra_packages",
			},
		},
		{
			"install_extra_snaps",
			&imagedefinition.Customization{
				ExtraSnaps: []*imagedefinition.Snap{
					{
						SnapName: "test",
					},
				},
			},
			[]string{
				"install_extra_snaps",
			},
		},
		{
			"all_extra_states",
			&imagedefinition.Customization{
				ExtraPPAs: []*imagedefinition.PPA{
					{
						PPAName: "test",
					},
				},
				ExtraPackages: []*imagedefinition.Package{
					{
						PackageName: "test",
					},
				},
				ExtraSnaps: []*imagedefinition.Snap{
					{
						SnapName: "test",
					},
				},
			},
			[]string{
				"add_extra_ppas",
				"install_extra_packages",
				"install_extra_snaps",
			},
		},
		{
			"no_extra_states",
			&imagedefinition.Customization{},
			[]string{},
		},
	}
	for _, tc := range testCases {
		t.Run("test_check_customization_steps_"+tc.name, func(t *testing.T) {
			extraSteps := checkCustomizationSteps(tc.customization, "extra_step_prebuilt_rootfs")
			for _, stepName := range tc.expectedSteps {
				found := false
				for _, extraStep := range extraSteps {
					if stepName == extraStep.name {
						found = true
					}
				}
				if !found {
					t.Errorf("Expected state \"%s\" to be added to the state machine but it was not",
						stepName,
					)
				}
			}
		})
	}
}

// TestFailedMountTempFS tests failures in the mountTempFS function
func TestFailedMountTempFS(t *testing.T) {
	t.Run("test_failed_mount_new_fs", func(t *testing.T) {
		asserter := helper.Asserter{T: t}

		// mock os.MkdirTemp
		osMkdirTemp = mockMkdirTemp
		defer func() {
			osMkdirTemp = os.MkdirTemp
		}()
		_, _, err := mountTempFS("", "", "")
		asserter.AssertErrContains(err, "Test error")
		osMkdirTemp = os.MkdirTemp
	})
}

// TestFailedGetPreseededSnaps tests various failure scenarios in the getPreseededSnaps function
func TestFailedGetPreseededSnaps(t *testing.T) {
	t.Run("test_failed_get_preseeded_snaps", func(t *testing.T) {
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
		//nolint:gosec,G204
		snapPrepareImage := *exec.Command("snap", "prepare-image", "--arch=amd64",
			"--classic", "--snap=core20", "--snap=core22", "--snap=snapd", "--snap=lxd",
			filepath.Join("testdata", "modelAssertionClassic"),
			stateMachine.tempDirs.rootfs)
		err = snapPrepareImage.Run()
		asserter.AssertErrNil(err, true)

		// mock seed.Open
		seedOpen = mockSeedOpen
		defer func() {
			seedOpen = seed.Open
		}()
		_, err = getPreseededSnaps(stateMachine.tempDirs.rootfs)
		asserter.AssertErrContains(err, "Test error")
		seedOpen = seed.Open

		// move the model from var/lib/snapd/seed/assertions to cause an error
		err = os.Rename(filepath.Join(seedDir, "assertions", "model"),
			filepath.Join(stateMachine.tempDirs.rootfs, "model"))
		asserter.AssertErrNil(err, true)
		_, err = getPreseededSnaps(stateMachine.tempDirs.rootfs)
		asserter.AssertErrContains(err, "seed must have a model assertion")
		err = os.Rename(filepath.Join(stateMachine.tempDirs.rootfs, "model"),
			filepath.Join(seedDir, "assertions", "model"))
		asserter.AssertErrNil(err, true)
		// move seed.yaml to cause an error in LoadMeta
		err = os.Rename(filepath.Join(seedDir, "seed.yaml"),
			filepath.Join(seedDir, "seed.yaml.bak"))
		asserter.AssertErrNil(err, true)
		_, err = getPreseededSnaps(stateMachine.tempDirs.rootfs)
		asserter.AssertErrContains(err, "no seed metadata")
		err = os.Rename(filepath.Join(seedDir, "seed.yaml.bak"),
			filepath.Join(seedDir, "seed.yaml"))
		asserter.AssertErrNil(err, true)

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedUpdateGrub tests failures in the updateGrub function
func TestFailedUpdateGrub(t *testing.T) {
	t.Run("test_failed_update_grub", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

		// mock os.Mkdir
		osMkdir = mockMkdir
		defer func() {
			osMkdir = os.Mkdir
		}()
		err := stateMachine.updateGrub("", 0)
		asserter.AssertErrContains(err, "Error creating scratch/loopback directory")
		osMkdir = os.Mkdir

		// Setup the exec.Command mock to mock losetup
		testCaseName = "TestFailedUpdateGrubLosetup"
		execCommand = fakeExecCommand
		defer func() {
			execCommand = exec.Command
		}()
		err = stateMachine.updateGrub("", 0)
		asserter.AssertErrContains(err, "Error running losetup command")

		// now test a command failure that isn't losetup
		testCaseName = "TestFailedUpdateGrubOther"
		err = stateMachine.updateGrub("", 0)
		asserter.AssertErrContains(err, "Error running command")
		execCommand = exec.Command
	})
}

func TestStateMachine_setConfDefDir(t *testing.T) {
	tests := []struct {
		name          string
		confFileArg   string
		expectedError string
		wantPath      string
		absBroken     bool
	}{
		{
			name:        "simple case",
			confFileArg: "ubuntu-server.yaml",
			wantPath:    "/tmp/simple_case",
		},
		{
			name:        "conf in subdir",
			confFileArg: "subdir/ubuntu-server.yaml",
			wantPath:    "/tmp/conf_in_subdir/subdir",
		},
		{
			name:        "conf in parent",
			confFileArg: "../ubuntu-server.yaml",
			wantPath:    "/tmp",
		},
		{
			name:        "conf at root",
			confFileArg: "../../../../../../../../../../..//ubuntu-server.yaml",
			wantPath:    "/",
		},
		{
			name:          "fail to get conf directory",
			confFileArg:   "ubuntu-server.yaml",
			wantPath:      "",
			absBroken:     true,
			expectedError: "unable to determine the configuration definition directory",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			tName := strings.ReplaceAll(tc.name, " ", "_")

			tmpDirPath := filepath.Join("/tmp", tName)
			saveCWD := helper.SaveCWD()
			defer saveCWD()

			err := os.Mkdir(tmpDirPath, 0755)
			t.Cleanup(func() {
				os.RemoveAll(tmpDirPath)
			})
			asserter.AssertErrNil(err, true)

			err = os.Chdir(tmpDirPath)
			asserter.AssertErrNil(err, true)

			if tc.absBroken {
				os.RemoveAll(tmpDirPath)
			}

			stateMachine := &StateMachine{}
			err = stateMachine.setConfDefDir(tc.confFileArg)
			if len(tc.expectedError) == 0 {
				asserter.AssertErrNil(err, true)
			} else {
				asserter.AssertErrContains(err, tc.expectedError)
			}

			if tc.wantPath != stateMachine.ConfDefPath {
				t.Errorf("Expected \"%s\" but got \"%s\"", tc.wantPath, stateMachine.ConfDefPath)
			}
		})
	}
}
