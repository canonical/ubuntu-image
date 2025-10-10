package statemachine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/osutil"

	"github.com/canonical/ubuntu-image/internal/helper"
)

// TestFailedHandleSecureBoot tests failures in the handleSecureBoot function by mocking functions
func TestFailedHandleSecureBoot(t *testing.T) {
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
	t.Cleanup(func() {
		osMkdirAll = os.MkdirAll
	})
	err = stateMachine.handleSecureBoot(volume, stateMachine.tempDirs.rootfs)
	asserter.AssertErrContains(err, "Error creating ubuntu dir")
	osMkdirAll = os.MkdirAll

	// mock os.ReadDir
	osReadDir = mockReadDir
	t.Cleanup(func() {
		osReadDir = os.ReadDir
	})
	err = stateMachine.handleSecureBoot(volume, stateMachine.tempDirs.rootfs)
	asserter.AssertErrContains(err, "Error reading boot dir")
	osReadDir = os.ReadDir

	// mock os.Rename
	osRename = mockRename
	t.Cleanup(func() {
		osRename = os.Rename
	})
	err = stateMachine.handleSecureBoot(volume, stateMachine.tempDirs.rootfs)
	asserter.AssertErrContains(err, "Error copying boot dir")
	osRename = os.Rename
}

// TestFailedHandleSecureBootPiboot tests failures in the handleSecureBoot
// function by mocking functions, for piboot
func TestFailedHandleSecureBootPiboot(t *testing.T) {
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
	t.Cleanup(func() {
		osMkdirAll = os.MkdirAll
	})
	err = stateMachine.handleSecureBoot(volume, stateMachine.tempDirs.rootfs)
	asserter.AssertErrContains(err, "Error creating ubuntu dir")
	osMkdirAll = os.MkdirAll

	// mock os.ReadDir
	osReadDir = mockReadDir
	t.Cleanup(func() {
		osReadDir = os.ReadDir
	})
	err = stateMachine.handleSecureBoot(volume, stateMachine.tempDirs.rootfs)
	asserter.AssertErrContains(err, "Error reading boot dir")
	osReadDir = os.ReadDir

	// mock os.Rename
	osRename = mockRename
	t.Cleanup(func() {
		osRename = os.Rename
	})
	err = stateMachine.handleSecureBoot(volume, stateMachine.tempDirs.rootfs)
	asserter.AssertErrContains(err, "Error copying boot dir")
	osRename = os.Rename
}

// TestHandleLkBootloader tests that the handleLkBootloader function runs successfully
func TestHandleLkBootloader(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine StateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.YamlFilePath = filepath.Join("testdata", "gadget_tree",
		"meta", "gadget.yaml")

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
}

// TestFailedHandleLkBootloader tests failures in handleLkBootloader by mocking functions
func TestFailedHandleLkBootloader(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine StateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.YamlFilePath = filepath.Join("testdata", "gadget_tree",
		"meta", "gadget.yaml")

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
	t.Cleanup(func() {
		osMkdir = os.Mkdir
	})
	err = stateMachine.handleLkBootloader(volume)
	asserter.AssertErrContains(err, "Failed to create gadget dir")
	osMkdir = os.Mkdir

	// mock os.ReadDir
	osReadDir = mockReadDir
	t.Cleanup(func() {
		osReadDir = os.ReadDir
	})
	err = stateMachine.handleLkBootloader(volume)
	asserter.AssertErrContains(err, "Error reading lk bootloader dir")
	osReadDir = os.ReadDir

	// mock osutil.CopySpecialFile
	osutilCopySpecialFile = mockCopySpecialFile
	t.Cleanup(func() {
		osutilCopySpecialFile = osutil.CopySpecialFile
	})
	err = stateMachine.handleLkBootloader(volume)
	asserter.AssertErrContains(err, "Error copying lk bootloader dir")
	osutilCopySpecialFile = osutil.CopySpecialFile
}
