package statemachine

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/google/uuid"
	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/gadget/quantity"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/osutil/mkfs"
)

// TestSetupCrossArch tests that the lb commands are set up correctly for cross arch compilation
func TestSetupCrossArch(t *testing.T) {
	t.Run("test_setup_cross_arch", func(t *testing.T) {
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
		if err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

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
		if err == nil {
			t.Errorf("Expected an error, but got none")
		}
		execCommand = exec.Command

		// mock osutil.CopySpecialFile
		osutilCopySpecialFile = mockCopySpecialFile
		defer func() {
			osutilCopySpecialFile = osutil.CopySpecialFile
		}()
		_, _, err = setupLiveBuildCommands(testDir, "amd64", []string{}, true)
		if err == nil {
			t.Errorf("Expected an error, but got none")
		}
		osutilCopySpecialFile = osutil.CopySpecialFile

		// use an arch with no qemu-static binary
		os.Unsetenv("UBUNTU_IMAGE_QEMU_USER_STATIC_PATH")
		_, _, err = setupLiveBuildCommands(testDir, "fake64", []string{}, true)
		if err == nil {
			t.Errorf("Expected an error, but got none")
		}
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

// TestSetCommonOpts ensures that the function actually sets the correct values in the struct
func TestSetCommonOpts(t *testing.T) {
	t.Run("test_set_common_opts", func(t *testing.T) {
		commonOpts := new(commands.CommonOpts)
		stateMachineOpts := new(commands.StateMachineOpts)
		commonOpts.Debug = true
		stateMachineOpts.WorkDir = testDir

		var stateMachine testStateMachine
		stateMachine.SetCommonOpts(commonOpts, stateMachineOpts)

		if !stateMachine.commonFlags.Debug || stateMachine.stateMachineFlags.WorkDir != testDir {
			t.Error("SetCommonOpts failed to set the correct options")
		}
	})
}

// TestFailedMetadataParse tests a failure in parsing the metadata file. This is accomplished
// by giving the state machine a syntactically invalid metadata file to parse
func TestFailedMetadataParse(t *testing.T) {
	t.Run("test_failed_metadata_parse", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.stateMachineFlags.Resume = true
		stateMachine.stateMachineFlags.WorkDir = "testdata"

		if err := stateMachine.readMetadata(); err == nil {
			t.Errorf("Expected an error but there was none")
		}
	})
}

// TestFailedRunHooks tests failures in the runHooks function. This is accomplished by mocking
// functions and calling hook scripts that intentionally return errors
func TestFailedRunHooks(t *testing.T) {
	t.Run("test_failed_run_hooks", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.commonFlags.Debug = true // for coverage!

		// need workdir set up for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		// first set a good hooks directory
		stateMachine.commonFlags.HooksDirectories = []string{filepath.Join(
			"testdata", "good_hookscript")}
		// mock ioutil.ReadDir
		ioutilReadDir = mockReadDir
		defer func() {
			ioutilReadDir = ioutil.ReadDir
		}()
		err := stateMachine.runHooks("post-populate-rootfs",
			"UBUNTU_IMAGE_HOOK_ROOTFS", stateMachine.tempDirs.rootfs)
		if err == nil {
			t.Error("Expected an error, but got none")
		}
		// restore the function
		ioutilReadDir = ioutil.ReadDir

		// now set a hooks directory that will fail
		stateMachine.commonFlags.HooksDirectories = []string{filepath.Join(
			"testdata", "hooks_return_error")}
		err = stateMachine.runHooks("post-populate-rootfs",
			"UBUNTU_IMAGE_HOOK_ROOTFS", stateMachine.tempDirs.rootfs)
		if err == nil {
			t.Error("Expected an error, but got none")
		}
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
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
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.yamlFilePath = filepath.Join("testdata", "gadget-multi.yaml")
			stateMachine.commonFlags.Size = tc.size

			// need workdir and loaded gadget.yaml set up for this
			if err := stateMachine.makeTemporaryDirectories(); err != nil {
				t.Errorf("Did not expect an error, got %s", err.Error())
			}
			if err := stateMachine.loadGadgetYaml(); err != nil {
				t.Errorf("Did not expect an error, got %s", err.Error())
			}

			if err := stateMachine.parseImageSizes(); err != nil {
				t.Errorf("Did not expect an error, got %s", err.Error())
			}
			// ensure the correct size was set
			for volumeName := range stateMachine.gadgetInfo.Volumes {
				setSize := stateMachine.imageSizes[volumeName]
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
		name string
		size string
	}{
		{"invalid_size", "4test"},
		{"too_many_args", "first:1G:2G"},
		{"multiple_invalid", "first:1test"},
		{"volume_not_exist", "fifth:1G"},
		{"index_out_of_range", "9:1G"},
	}
	for _, tc := range testCases {
		t.Run("test_failed_parse_image_sizes_"+tc.name, func(t *testing.T) {
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.yamlFilePath = filepath.Join("testdata", "gadget-multi.yaml")

			// need workdir and loaded gadget.yaml set up for this
			if err := stateMachine.makeTemporaryDirectories(); err != nil {
				t.Errorf("Did not expect an error, got %s", err.Error())
			}
			if err := stateMachine.loadGadgetYaml(); err != nil {
				t.Errorf("Did not expect an error, got %s", err.Error())
			}

			// run parseImage size and make sure it failed
			stateMachine.commonFlags.Size = tc.size
			if err := stateMachine.parseImageSizes(); err == nil {
				t.Errorf("Expected an error, but got none")
			}
		})
	}
}

// TestFailedHandleSecureBoot tests failures in the handleSecureBoot function by mocking functions
func TestFailedHandleSecureBoot(t *testing.T) {
	t.Run("test_failed_handle_secure_boot", func(t *testing.T) {
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
		if err := stateMachine.handleSecureBoot(volume, stateMachine.tempDirs.rootfs); err == nil {
			t.Errorf("Expected an error, but got none")
		}
		osMkdirAll = os.MkdirAll

		// mock ioutil.ReadDir
		ioutilReadDir = mockReadDir
		defer func() {
			ioutilReadDir = ioutil.ReadDir
		}()
		if err := stateMachine.handleSecureBoot(volume, stateMachine.tempDirs.rootfs); err == nil {
			t.Errorf("Expected an error, but got none")
		}
		ioutilReadDir = ioutil.ReadDir

		// mock osutil.CopySpecialFile
		osutilCopySpecialFile = mockCopySpecialFile
		defer func() {
			osutilCopySpecialFile = osutil.CopySpecialFile
		}()
		if err := stateMachine.handleSecureBoot(volume, stateMachine.tempDirs.rootfs); err == nil {
			t.Errorf("Expected an error, but got none")
		}
		osutilCopySpecialFile = osutil.CopySpecialFile
	})
}

// TestHandleLkBootloader tests that the handleLkBootloader function runs successfully
func TestHandleLkBootloader(t *testing.T) {
	t.Run("test_handle_lk_bootloader", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.yamlFilePath = filepath.Join("testdata", "gadget_tree",
			"meta", "gadget.yaml")

		// need workdir set up for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}
		// create image/boot/lk and place a test file there
		bootDir := filepath.Join(stateMachine.tempDirs.unpack, "image", "boot", "lk")
		if err := os.MkdirAll(bootDir, 0755); err != nil {
			t.Errorf("Error setting up lk boot dir: %s", err.Error())
		}
		if err := osutil.CopyFile(filepath.Join("testdata", "disk_info"),
			filepath.Join(bootDir, "disk_info"), 0); err != nil {
			t.Errorf("Error setting up lk boot dir: %s", err.Error())
		}

		// set up the volume
		volume := new(gadget.Volume)
		volume.Bootloader = "lk"

		if err := stateMachine.handleLkBootloader(volume); err != nil {
			t.Errorf("Did not expect an error in handleLkBootloader, got %s", err.Error())
		}

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
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.yamlFilePath = filepath.Join("testdata", "gadget_tree",
			"meta", "gadget.yaml")

		// need workdir set up for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}
		// create image/boot/lk and place a test file there
		bootDir := filepath.Join(stateMachine.tempDirs.unpack, "image", "boot", "lk")
		if err := os.MkdirAll(bootDir, 0755); err != nil {
			t.Errorf("Error setting up lk boot dir: %s", err.Error())
		}
		if err := osutil.CopyFile(filepath.Join("testdata", "disk_info"),
			filepath.Join(bootDir, "disk_info"), 0); err != nil {
			t.Errorf("Error setting up lk boot dir: %s", err.Error())
		}

		// set up the volume
		volume := new(gadget.Volume)
		volume.Bootloader = "lk"

		// mock os.Mkdir
		osMkdir = mockMkdir
		defer func() {
			osMkdir = os.Mkdir
		}()
		if err := stateMachine.handleLkBootloader(volume); err == nil {
			t.Errorf("Expected an error, but got none")
		}
		osMkdir = os.Mkdir

		// mock ioutil.ReadDir
		ioutilReadDir = mockReadDir
		defer func() {
			ioutilReadDir = ioutil.ReadDir
		}()
		if err := stateMachine.handleLkBootloader(volume); err == nil {
			t.Errorf("Expected an error, but got none")
		}
		ioutilReadDir = ioutil.ReadDir

		// mock osutil.CopySpecialFile
		osutilCopySpecialFile = mockCopySpecialFile
		defer func() {
			osutilCopySpecialFile = osutil.CopySpecialFile
		}()
		if err := stateMachine.handleLkBootloader(volume); err == nil {
			t.Errorf("Expected an error, but got none")
		}
		osutilCopySpecialFile = osutil.CopySpecialFile
	})
}

// TestHandleContentSizes ensures that using --image-size with a few different values
// results in the correct sizes in stateMachine.imageSizes
func TestHandleContentSizes(t *testing.T) {
	testCases := []struct {
		name   string
		size   string
		result map[string]quantity.Size
	}{
		{"size_not_specified", "", map[string]quantity.Size{"pc": 17825792}},
		{"size_smaller_than_content", "pc:123", map[string]quantity.Size{"pc": 17825792}},
		{"size_bigger_than_content", "pc:4G", map[string]quantity.Size{"pc": 4 * quantity.SizeGiB}},
	}
	for _, tc := range testCases {
		t.Run("test_handle_content_sizes_"+tc.name, func(t *testing.T) {
			var stateMachine StateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.commonFlags.Size = tc.size
			stateMachine.yamlFilePath = filepath.Join("testdata", "gadget_tree",
				"meta", "gadget.yaml")

			// need workdir and loaded gadget.yaml set up for this
			if err := stateMachine.makeTemporaryDirectories(); err != nil {
				t.Errorf("Did not expect an error, got %s", err.Error())
			}
			if err := stateMachine.loadGadgetYaml(); err != nil {
				t.Errorf("Did not expect an error, got %s", err.Error())
			}

			stateMachine.handleContentSizes(0, "pc")
			// ensure the correct size was set
			for volumeName := range stateMachine.gadgetInfo.Volumes {
				setSize := stateMachine.imageSizes[volumeName]
				if setSize != tc.result[volumeName] {
					t.Errorf("Volume %s has the wrong size set: %d. "+
						"Should be %d", volumeName, setSize, tc.result[volumeName])
				}
			}
		})
	}
}

// TestFailedCopyStructureContent tests failures in the copyStructureContent function by mocking
// functions and setting invalid bs= arguments in dd
func TestFailedCopyStructureContent(t *testing.T) {
	t.Run("test_failed_copy_structure_content", func(t *testing.T) {
		var stateMachine StateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.yamlFilePath = filepath.Join("testdata", "gadget_tree",
			"meta", "gadget.yaml")

		// need workdir and loaded gadget.yaml set up for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}
		if err := stateMachine.loadGadgetYaml(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		// separate out the volumeStructures to test different scenarios
		var mbrStruct gadget.VolumeStructure
		var rootfsStruct gadget.VolumeStructure
		for _, volume := range stateMachine.gadgetInfo.Volumes {
			for _, structure := range volume.Structure {
				if structure.Name == "mbr" {
					mbrStruct = structure
				} else if structure.Name == "EFI System" {
					rootfsStruct = structure
				}
			}
		}

		// mock helper.CopyBlob and test with no filesystem specified
		helperCopyBlob = mockCopyBlob
		defer func() {
			helperCopyBlob = helper.CopyBlob
		}()
		if err := stateMachine.copyStructureContent(mbrStruct, "",
			filepath.Join("/tmp", uuid.NewString()+".img")); err == nil {
			t.Errorf("Expected an error, but got none")
		}
		helperCopyBlob = helper.CopyBlob

		// set an invalid blocksize to mock the binary copy blob
		mockableBlockSize = "0"
		defer func() {
			mockableBlockSize = "1"
		}()
		if err := stateMachine.copyStructureContent(mbrStruct, "",
			filepath.Join("/tmp", uuid.NewString()+".img")); err == nil {
			t.Errorf("Expected an error, but got none")
		}
		mockableBlockSize = "1"

		// mock helper.CopyBlob and test with filesystem: vfat
		helperCopyBlob = mockCopyBlob
		defer func() {
			helperCopyBlob = helper.CopyBlob
		}()
		if err := stateMachine.copyStructureContent(rootfsStruct, "",
			filepath.Join("/tmp", uuid.NewString()+".img")); err == nil {
			t.Errorf("Expected an error, but got none")
		}
		helperCopyBlob = helper.CopyBlob

		// mock gadget.MkfsWithContent
		mkfsMakeWithContent = mockMkfsWithContent
		defer func() {
			mkfsMakeWithContent = mkfs.MakeWithContent
		}()
		if err := stateMachine.copyStructureContent(rootfsStruct, "",
			filepath.Join("/tmp", uuid.NewString()+".img")); err == nil {
			t.Errorf("Expected an error, but got none")
		}
		mkfsMakeWithContent = mkfs.MakeWithContent
	})
}
