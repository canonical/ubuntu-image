package statemachine

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/snapcore/snapd/gadget"
	"github.com/snapcore/snapd/osutil"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/canonical/ubuntu-image/internal/imagedefinition"
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

// TestStateMachine_setupGrub_checkcmds checks commands to update grub order is ok
func TestStateMachine_setupGrub_checkcmds(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.commonFlags.Debug = true
	stateMachine.commonFlags.OutputDir = "/tmp"
	stateMachine.parent = &stateMachine
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: "amd64",
		Series:       getHostSuite(),
		Rootfs: &imagedefinition.Rootfs{
			Archive: "ubuntu",
		},
	}

	err := stateMachine.SetSeries()
	asserter.AssertErrNil(err, true)

	err = stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	mockCmder := NewMockExecCommand()

	execCommand = mockCmder.Command
	t.Cleanup(func() { execCommand = exec.Command })

	stdout, restoreStdout, err := helper.CaptureStd(&os.Stdout)
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { restoreStdout() })

	helperBackupAndCopyResolvConf = mockBackupAndCopyResolvConfSuccess
	t.Cleanup(func() {
		helperBackupAndCopyResolvConf = helper.BackupAndCopyResolvConf
	})

	helperRestoreResolvConf = mockRestoreResolvConfSuccess
	t.Cleanup(func() {
		helperRestoreResolvConf = helper.RestoreResolvConf
	})

	err = stateMachine.setupGrub("", 2, 1, stateMachine.ImageDef.Architecture)
	asserter.AssertErrNil(err, true)

	restoreStdout()
	readStdout, err := io.ReadAll(stdout)
	asserter.AssertErrNil(err, true)

	expectedCmds := []*regexp.Regexp{
		regexp.MustCompile("^udevadm settle$"),
		regexp.MustCompile("^mount .*p2 .*/scratch/loopback$"),
		regexp.MustCompile("^mkdir -p .*/scratch/loopback/boot/efi$"),
		regexp.MustCompile("^mount .*p1 .*/scratch/loopback/boot/efi$"),
		regexp.MustCompile("^mount -t devtmpfs devtmpfs-build .*/scratch/loopback/dev$"),
		regexp.MustCompile("^mount -t devpts devpts-build -o nodev,nosuid .*/scratch/loopback/dev/pts$"),
		regexp.MustCompile("^mount -t proc proc-build .*/scratch/loopback/proc$"),
		regexp.MustCompile("^mount -t sysfs sysfs-build .*/scratch/loopback/sys$"),
		regexp.MustCompile("^mount --bind .*/run.*$"),
	}

	// Keep this check simple since the test suite is unlikely to be executed on a system older than jammy.
	if stateMachine.series == "jammy" {
		expectedCmds = append(expectedCmds,
			regexp.MustCompile("^chroot .*/scratch/loopback apt install --assume-yes --quiet --option=Dpkg::options::=--force-unsafe-io --option=Dpkg::Options::=--force-confold --no-install-recommends udev"),
		)
	}

	expectedCmds = append(expectedCmds, []*regexp.Regexp{
		regexp.MustCompile("^chroot .*/scratch/loopback grub-install .* --boot-directory=/boot --efi-directory=/boot/efi --target=x86_64-efi --uefi-secure-boot --no-nvram$"),
		regexp.MustCompile("^chroot .*/scratch/loopback grub-install .* --target=i386-pc$"),
		regexp.MustCompile("^chroot .*/scratch/loopback dpkg-divert"),
		regexp.MustCompile("^chroot .*/scratch/loopback update-grub$"),
		regexp.MustCompile("^chroot .*/scratch/loopback dpkg-divert --remove"),
		regexp.MustCompile("^udevadm settle$"),
		regexp.MustCompile("^mount --make-rprivate .*/scratch/loopback/run$"),
		regexp.MustCompile("^umount --recursive .*scratch/loopback/run$"),
		regexp.MustCompile("^mount --make-rprivate .*/scratch/loopback/sys$"),
		regexp.MustCompile("^umount --recursive .*scratch/loopback/sys$"),
		regexp.MustCompile("^mount --make-rprivate .*/scratch/loopback/proc$"),
		regexp.MustCompile("^umount --recursive .*scratch/loopback/proc$"),
		regexp.MustCompile("^mount --make-rprivate .*scratch/loopback/dev/pts$"),
		regexp.MustCompile("^umount --recursive .*scratch/loopback/dev/pts$"),
		regexp.MustCompile("^mount --make-rprivate .*/scratch/loopback/dev$"),
		regexp.MustCompile("^umount --recursive .*scratch/loopback/dev$"),
		regexp.MustCompile("^umount .*scratch/loopback/boot/efi$"),
		regexp.MustCompile("^umount .*scratch/loopback$"),
		regexp.MustCompile("^losetup --detach .* /tmp$"),
	}...)

	gotCmds := strings.Split(strings.TrimSpace(string(readStdout)), "\n")
	if len(expectedCmds) != len(gotCmds) {
		t.Fatalf("%v commands to be executed, expected %v commands. Got: %v", len(gotCmds), len(expectedCmds), gotCmds)
	}

	for i, gotCmd := range gotCmds {
		expected := expectedCmds[i]

		if !expected.Match([]byte(gotCmd)) {
			t.Errorf("Cmd \"%v\" not matching. Expected %v\n", gotCmd, expected.String())
		}
	}
}

// TestStateMachine_setupGrub_failed tests failures in the updateGrub function
func TestStateMachine_setupGrub_failed(t *testing.T) {
	asserter := helper.Asserter{T: t}
	var stateMachine ClassicStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.ImageDef = imagedefinition.ImageDefinition{
		Architecture: "amd64",
		Series:       getHostSuite(),
		Rootfs: &imagedefinition.Rootfs{
			Archive: "ubuntu",
		},
	}

	err := stateMachine.makeTemporaryDirectories()
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(stateMachine.stateMachineFlags.WorkDir) })

	// mock os.Mkdir
	osMkdir = mockMkdir
	t.Cleanup(func() {
		osMkdir = os.Mkdir
	})
	err = stateMachine.setupGrub("", 0, 0, stateMachine.ImageDef.Architecture)
	asserter.AssertErrContains(err, "Error creating scratch/loopback directory")
	osMkdir = os.Mkdir

	// Setup the exec.Command mock to mock losetup
	testCaseName = "TestFailedUpdateGrubLosetup"
	execCommand = fakeExecCommand
	t.Cleanup(func() {
		execCommand = exec.Command
	})
	err = stateMachine.setupGrub("", 0, 0, stateMachine.ImageDef.Architecture)
	asserter.AssertErrContains(err, "Error running losetup command")

	// now test a command failure that isn't losetup
	testCaseName = "TestFailedUpdateGrubOther"
	err = stateMachine.setupGrub("", 0, 0, stateMachine.ImageDef.Architecture)
	asserter.AssertErrContains(err, "Error running command")
	execCommand = exec.Command

	err = stateMachine.setupGrub("", 0, 0, "unknown")
	asserter.AssertErrContains(err, "no valid efi target for the provided architecture")

	// Test failing helperBackupAndCopyResolvConf
	mockCmder := NewMockExecCommand()

	execCommand = mockCmder.Command
	t.Cleanup(func() { execCommand = exec.Command })
	helperBackupAndCopyResolvConf = mockBackupAndCopyResolvConfFail
	t.Cleanup(func() {
		helperBackupAndCopyResolvConf = helper.BackupAndCopyResolvConf
	})
	err = stateMachine.setupGrub("", 0, 0, stateMachine.ImageDef.Architecture)
	asserter.AssertErrContains(err, "Error setting up /etc/resolv.conf")
	helperBackupAndCopyResolvConf = helper.BackupAndCopyResolvConf
}
