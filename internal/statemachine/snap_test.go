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

// TestGenerateSnapManifest tests if snap-based image manifest generation works
func TestGenerateSnapManifest(t *testing.T) {
	testCases := []struct {
		name   string
		seeded bool
	}{
		{"snap_manifest_regular", false},
		{"snap_manifest_seeded", true},
	}
	for _, tc := range testCases {
		t.Run("test_generate_"+tc.name, func(t *testing.T) {
			saveCWD := helper.SaveCWD()
			defer saveCWD()

			workDir, err := ioutil.TempDir("/tmp", "ubuntu-image-")
			if err != nil {
				t.Errorf("Failed to create work directory")
			}
			defer os.RemoveAll(workDir)
			var stateMachine SnapStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.stateMachineFlags.WorkDir = workDir
			stateMachine.tempDirs.rootfs = filepath.Join(workDir, "rootfs")
			stateMachine.isSeeded = tc.seeded
			stateMachine.commonFlags.OutputDir = filepath.Join(workDir, "output")
			osMkdirAll(stateMachine.commonFlags.OutputDir, 0755)

			// Prepare direcory structure for installed and seeded snaps
			snapsDir := filepath.Join(stateMachine.tempDirs.rootfs, "system-data", "var", "lib", "snapd", "snaps")
			seedDir := filepath.Join(stateMachine.tempDirs.rootfs, "system-data", "var", "lib", "snapd", "seed", "snaps")
			uc20Dir := filepath.Join(stateMachine.tempDirs.rootfs, "snaps")
			osMkdirAll(snapsDir, 0755)
			osMkdirAll(seedDir, 0755)
			osMkdirAll(uc20Dir, 0755)
			var testEnvMap map[string][]string
			if tc.seeded {
				testEnvMap = map[string][]string{
					uc20Dir: {"foo_1.23.snap", "uc20specific_345.snap"},
				}
			} else {
				testEnvMap = map[string][]string{
					snapsDir: {"foo_1.23.snap", "bar_1.23_version.snap", "baz_234.snap", "dummy_file"},
					seedDir:  {"foo_1.23.snap", "dummy_file_2.txt", "test_1234.snap"},
				}
			}
			for dir, fileList := range testEnvMap {
				for _, file := range fileList {
					fp, err := os.Create(filepath.Join(dir, file))
					if err != nil {
						t.Error("Failed to create necessary dummy files")
					}
					fp.Close()
				}
			}

			if err := stateMachine.generateSnapManifest(); err != nil {
				t.Errorf("Did not expect an error, but got %s", err.Error())
			}

			// Check if manifests got generated and if they have expected contents
			// For both UC20+ and regular images
			var testResultMap map[string][]string
			if tc.seeded {
				testResultMap = map[string][]string{
					"seed.manifest": {"foo 1.23", "uc20specific 345"},
				}
			} else {
				testResultMap = map[string][]string{
					"snaps.manifest": {"foo 1.23", "bar 1.23_version", "baz 234"},
					"seed.manifest":  {"foo 1.23", "test 1234"},
				}
			}
			for manifest, snapList := range testResultMap {
				manifestPath := filepath.Join(stateMachine.commonFlags.OutputDir, manifest)
				manifestBytes, err := ioutil.ReadFile(manifestPath)
				if err != nil {
					t.Errorf("Failed to read manifest file %s: %s", manifestPath, err.Error())
				}
				// The order of snaps shouldn't matter
				for _, snap := range snapList {
					if !strings.Contains(string(manifestBytes), snap) {
						t.Errorf("%s does not contain expected snap: %s", manifest, snap)
					}
				}
			}
		})
	}
}

// TestFailedGenerateSnapManifest tests if snap-based image manifest generation failures are catched
func TestFailedGenerateSnapManifest(t *testing.T) {
	t.Run("test_failed_generate_snap_manifest", func(t *testing.T) {
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		ioutilReadDir = func(string) ([]os.FileInfo, error) {
			return []os.FileInfo{}, nil
		}
		defer func() {
			ioutilReadDir = ioutil.ReadDir
		}()
		// Setup the mock for os.Create, making those fail
		osCreate = mockCreate
		defer func() {
			osCreate = os.Create
		}()

		var stateMachine SnapStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.stateMachineFlags.WorkDir = "/dummy/path"
		stateMachine.tempDirs.rootfs = "/dummy/path"
		stateMachine.isSeeded = false
		stateMachine.commonFlags.OutputDir = "/dummy/path"

		if err := stateMachine.generateSnapManifest(); err == nil {
			t.Error("Expected an error, but got none")
		}
	})
}
