// This test file tests a successful snap run and success/error scenarios for all states
// that are specific to the snap builds
package statemachine

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/snapcore/snapd/osutil"
)

// TestFailedValidateInputSnap tests a failure in the Setup() function when validating common input
func TestFailedValidateInputSnap(t *testing.T) {
	t.Run("test_failed_validate_input", func(t *testing.T) {
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		// use both --until and --thru to trigger this failure
		var stateMachine SnapStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
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
		stateMachine.parent = &stateMachine
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
		stateMachine.parent = &stateMachine
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
		stateMachine.parent = &stateMachine
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
// by attempting to use --disable-console-conf with a core20 image
func TestFailedPrepareImage(t *testing.T) {
	t.Run("test_failed_prepare_image", func(t *testing.T) {
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine SnapStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.Args.ModelAssertion = filepath.Join("testdata", "modelAssertion20")
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

// TestPopulateSnapRootfsContents runs the state machine through populate_rootfs_contents and examines
// the rootfs to ensure at least some of the correct file are in place
func TestPopulateSnapRootfsContents(t *testing.T) {
	testCases := []struct {
		name           string
		modelAssertion string
		fileList       []string
	}{
		{"core18", filepath.Join("testdata", "modelAssertion18"), []string{filepath.Join("system-data", "var", "lib", "snapd", "seed", "snaps"), filepath.Join("system-data", "var", "lib", "snapd", "seed", "assertions", "model"), filepath.Join("system-data", "var", "lib", "snapd", "seed", "seed.yaml"), filepath.Join("system-data", "var", "lib", "snapd", "seed", "snaps")}},
		{"core20", filepath.Join("testdata", "modelAssertion20"), []string{"systems", "snaps", filepath.Join("EFI", "boot"), filepath.Join("EFI", "ubuntu", "grubenv"), filepath.Join("EFI", "ubuntu", "grub.cfg")}},
	}
	for _, tc := range testCases {
		t.Run("test "+tc.name, func(t *testing.T) {
			saveCWD := helper.SaveCWD()
			defer saveCWD()

			var stateMachine SnapStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine
			stateMachine.Args.ModelAssertion = tc.modelAssertion
			stateMachine.stateMachineFlags.Thru = "populate_rootfs_contents"

			if err := stateMachine.Setup(); err != nil {
				t.Errorf("Did not expect an error, got %s\n", err.Error())
			}

			if err := stateMachine.Run(); err != nil {
				t.Errorf("Did not expect an error, got %s\n", err.Error())
			}

			// check the files before Teardown
			for _, file := range tc.fileList {
				_, err := os.Stat(filepath.Join(stateMachine.tempDirs.rootfs, file))
				if err != nil {
					if os.IsNotExist(err) {
						t.Errorf("File %s should exist, but does not", file)
					}
				}
			}

			if err := stateMachine.Teardown(); err != nil {
				t.Errorf("Did not expect an error, got %s\n", err.Error())
			}
		})
	}
}

// TestFailedPopulateSnapRootfsContents tests a failure in the PopulateRootfsContents state
// while building a snap image. This is achieved by mocking functions
func TestFailedPopulateSnapRootfsContents(t *testing.T) {
	t.Run("test_failed_populate_snap_rootfs_contents", func(t *testing.T) {
		var stateMachine SnapStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.Args.ModelAssertion = filepath.Join("testdata", "modelAssertion18")

		// need workdir and gadget.yaml set up for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}
		if err := stateMachine.prepareImage(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}
		if err := stateMachine.loadGadgetYaml(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		// mock os.MkdirAll
		osMkdirAll = mockMkdirAll
		defer func() {
			osMkdirAll = os.MkdirAll
		}()
		if err := stateMachine.populateSnapRootfsContents(); err == nil {
			t.Error("Expected an error, but got none")
		}
		osMkdirAll = os.MkdirAll

		// mock ioutil.ReadDir
		ioutilReadDir = mockReadDir
		defer func() {
			ioutilReadDir = ioutil.ReadDir
		}()
		if err := stateMachine.populateSnapRootfsContents(); err == nil {
			t.Error("Expected an error, but got none")
		}
		ioutilReadDir = ioutil.ReadDir

		// mock osutil.CopySpecialFile
		osutilCopySpecialFile = mockCopySpecialFile
		defer func() {
			osutilCopySpecialFile = osutil.CopySpecialFile
		}()
		if err := stateMachine.populateSnapRootfsContents(); err == nil {
			t.Error("Expected an error, but got none")
		}
		osutilCopySpecialFile = osutil.CopySpecialFile
	})
}
