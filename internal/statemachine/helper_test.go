package statemachine

import (
	"os"
	"os/exec"
	"testing"

	"github.com/snapcore/snapd/gadget/quantity"
	"github.com/snapcore/snapd/osutil"
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
