package statemachine

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canonical/ubuntu-image/internal/helper"
)

// TestFailedValidateInputSnap tests a failure in the Setup() function when validating common input
func TestFailedValidateInputSnap(t *testing.T) {
	t.Run("test_failed_validate_input", func(t *testing.T) {
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		// use both --until and --thru to trigger this failure
		var stateMachine SnapStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.stateMachineFlags.Until = "until-test"
		stateMachine.stateMachineFlags.Thru = "thru-test"

		if err := stateMachine.Setup(); err == nil {
			t.Error("Expected an error but there was none")
		}
	})
}

// TestFailedReadMetadataSnap tests a failed metadata read by passing --resume with no previous partial state machine run
func TestFailedReadMetadataSnap(t *testing.T) {
	t.Run("test_failed_read_metadata", func(t *testing.T) {
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		// start a --resume with no previous SM run
		var stateMachine SnapStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.stateMachineFlags.Resume = true
		stateMachine.stateMachineFlags.WorkDir = testDir

		if err := stateMachine.Setup(); err == nil {
			t.Error("Expected an error but there was none")
		}
	})
}

// TestSuccessfulSnapCore20 builds a core 20 image and makes sure the factory boot flag is set
func TestSuccessfulSnapCore20(t *testing.T) {
	t.Run("test_successful_snap_run", func(t *testing.T) {
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine SnapStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.Args.ModelAssertion = filepath.Join("testdata", "modelAssertion20")
		stateMachine.Opts.FactoryImage = true

		if err := stateMachine.Setup(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}

		if err := stateMachine.Run(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}

		// make sure the "factory" boot flag was set
		grubenvFile := filepath.Join(stateMachine.tempDirs.unpack,
			"system-seed", "EFI", "ubuntu", "grubenv")
		grubenvBytes, err := ioutil.ReadFile(grubenvFile)
		if err != nil {
			t.Errorf("Failed to read file %s: %s", grubenvFile, err.Error())
		}

		if !strings.Contains(string(grubenvBytes), "snapd_boot_flags=factory") {
			t.Errorf("grubenv file does not have factory boot flag set")
		}

		if err := stateMachine.Teardown(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}
	})
}

// TestSuccessfulSnapCore18 builds a core 18 image with a few special options
func TestSuccessfulSnapCore18(t *testing.T) {
	t.Run("test_successful_snap_options", func(t *testing.T) {
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine SnapStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.Args.ModelAssertion = filepath.Join("testdata", "modelAssertion18")
		stateMachine.Opts.Channel = "stable"
		stateMachine.Opts.Snaps = []string{"hello-world"}
		stateMachine.Opts.DisableConsoleConf = true
		stateMachine.commonFlags.CloudInit = filepath.Join("testdata", "user-data")

		if err := stateMachine.Setup(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}

		if err := stateMachine.Run(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}

		// make sure cloud-init user-data was placed correctly
		userDataPath := filepath.Join(stateMachine.tempDirs.unpack,
			"image", "var", "lib", "cloud", "seed", "nocloud-net", "user-data")
		_, err := os.Stat(userDataPath)
		if err != nil {
			t.Errorf("cloud-init user-data file %s does not exist", userDataPath)
		}

		if err := stateMachine.Teardown(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}
	})
}

// TestFailedPrepareImage tests a failure in the call to image.Prepare. This is easy to achieve
// attempting to use --disable-console-conf with a core20 image
func TestFailedPrepareImage(t *testing.T) {
	t.Run("test_failed_prepare_image", func(t *testing.T) {
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine SnapStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.Args.ModelAssertion = "testdata/modelAssertion20"
		stateMachine.Opts.DisableConsoleConf = true

		if err := stateMachine.Setup(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}

		if err := stateMachine.Run(); err == nil {
			t.Errorf("Expected an error, but got none")
		}

		if err := stateMachine.Teardown(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}
	})
}
