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
)

// TestFailedValidateInputSnap tests a failure in the Setup() function when validating common input
func TestFailedValidateInputSnap(t *testing.T) {
	t.Run("test_failed_validate_input", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		// use both --until and --thru to trigger this failure
		var stateMachine SnapStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.stateMachineFlags.Until = "until-test"
		stateMachine.stateMachineFlags.Thru = "thru-test"

		err := stateMachine.Setup()
		asserter.AssertErrContains(err, "cannot specify both --until and --thru")
	})
}

// TestFailedReadMetadataSnap tests a failed metadata read by passing --resume with no previous partial state machine run
func TestFailedReadMetadataSnap(t *testing.T) {
	t.Run("test_failed_read_metadata", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		// start a --resume with no previous SM run
		var stateMachine SnapStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.stateMachineFlags.Resume = true
		stateMachine.stateMachineFlags.WorkDir = testDir

		err := stateMachine.Setup()
		asserter.AssertErrContains(err, "error reading metadata file")
	})
}

// TestSuccessfulSnapCore20 builds a core 20 image and makes sure the factory boot flag is set
func TestSuccessfulSnapCore20(t *testing.T) {
	t.Run("test_successful_snap_run", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine SnapStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.Args.ModelAssertion = filepath.Join("testdata", "modelAssertion20")
		stateMachine.Opts.FactoryImage = true
		workDir, err := ioutil.TempDir("/tmp", "ubuntu-image-")
		asserter.AssertErrNil(err, true)
		defer os.RemoveAll(workDir)
		stateMachine.stateMachineFlags.WorkDir = workDir

		err = stateMachine.Setup()
		asserter.AssertErrNil(err, true)

		err = stateMachine.Run()
		asserter.AssertErrNil(err, true)

		// make sure the "factory" boot flag was set
		grubenvFile := filepath.Join(stateMachine.tempDirs.rootfs,
			"EFI", "ubuntu", "grubenv")
		grubenvBytes, err := ioutil.ReadFile(grubenvFile)
		asserter.AssertErrNil(err, true)

		if !strings.Contains(string(grubenvBytes), "snapd_boot_flags=factory") {
			t.Errorf("grubenv file does not have factory boot flag set")
		}

		err = stateMachine.Teardown()
		asserter.AssertErrNil(err, true)
	})
}

// TestSuccessfulSnapCore18 builds a core 18 image with a few special options
func TestSuccessfulSnapCore18(t *testing.T) {
	t.Run("test_successful_snap_options", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
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
		workDir, err := ioutil.TempDir("/tmp", "ubuntu-image-")
		asserter.AssertErrNil(err, true)
		defer os.RemoveAll(workDir)
		stateMachine.stateMachineFlags.WorkDir = workDir

		err = stateMachine.Setup()
		asserter.AssertErrNil(err, true)

		err = stateMachine.Run()
		asserter.AssertErrNil(err, true)

		// make sure cloud-init user-data was placed correctly
		userDataPath := filepath.Join(stateMachine.tempDirs.rootfs,
			"system-data", "var", "lib", "cloud", "seed", "nocloud-net", "user-data")
		_, err = os.Stat(userDataPath)
		if err != nil {
			t.Errorf("cloud-init user-data file %s does not exist", userDataPath)
		}

		// check that the grubenv file is in EFI/ubuntu
		grubenvFile := filepath.Join(stateMachine.tempDirs.volumes,
			"pc", "part2", "EFI", "ubuntu", "grubenv")
		_, err = os.Stat(grubenvFile)
		if err != nil {
			t.Errorf("Expected file %s to exist, but it does not", grubenvFile)
		}

		err = stateMachine.Teardown()
		asserter.AssertErrNil(err, true)
	})
}

// TestFailedPrepareImage tests a failure in the call to image.Prepare. This is easy to achieve
// by attempting to use --disable-console-conf with a core20 image
func TestFailedPrepareImage(t *testing.T) {
	t.Run("test_failed_prepare_image", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine SnapStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.Args.ModelAssertion = filepath.Join("testdata", "modelAssertion20")
		stateMachine.Opts.DisableConsoleConf = true

		err := stateMachine.Setup()
		asserter.AssertErrNil(err, true)

		err = stateMachine.Run()
		asserter.AssertErrContains(err, "Error preparing image")

		err = stateMachine.Teardown()
		asserter.AssertErrNil(err, true)
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
			asserter := helper.Asserter{T: t}
			saveCWD := helper.SaveCWD()
			defer saveCWD()

			var stateMachine SnapStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine
			stateMachine.Args.ModelAssertion = tc.modelAssertion
			stateMachine.stateMachineFlags.Thru = "populate_rootfs_contents"

			err := stateMachine.Setup()
			asserter.AssertErrNil(err, true)

			err = stateMachine.Run()
			asserter.AssertErrNil(err, true)

			// check the files before Teardown
			for _, file := range tc.fileList {
				_, err := os.Stat(filepath.Join(stateMachine.tempDirs.rootfs, file))
				if err != nil {
					if os.IsNotExist(err) {
						t.Errorf("File %s should exist, but does not", file)
					}
				}
			}

			err = stateMachine.Teardown()
			asserter.AssertErrNil(err, true)
		})
	}
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
			asserter := helper.Asserter{T: t}
			saveCWD := helper.SaveCWD()
			defer saveCWD()

			workDir, err := ioutil.TempDir("/tmp", "ubuntu-image-")
			asserter.AssertErrNil(err, true)
			defer os.RemoveAll(workDir)
			var stateMachine SnapStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.stateMachineFlags.WorkDir = workDir
			stateMachine.tempDirs.rootfs = filepath.Join(workDir, "rootfs")
			stateMachine.IsSeeded = tc.seeded
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
					asserter.AssertErrNil(err, false)
					fp.Close()
				}
			}

			err = stateMachine.generateSnapManifest()
			asserter.AssertErrNil(err, false)

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
				asserter.AssertErrNil(err, false)
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

// TestFailedPopulateSnapRootfsContents tests a failure in the PopulateRootfsContents state
// while building a snap image. This is achieved by mocking functions
func TestFailedPopulateSnapRootfsContents(t *testing.T) {
	t.Run("test_failed_populate_snap_rootfs_contents", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine SnapStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.Args.ModelAssertion = filepath.Join("testdata", "modelAssertion18")

		// need workdir and gadget.yaml set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		err = stateMachine.prepareImage()
		asserter.AssertErrNil(err, true)

		err = stateMachine.loadGadgetYaml()
		asserter.AssertErrNil(err, true)

		// mock os.MkdirAll
		osMkdirAll = mockMkdirAll
		defer func() {
			osMkdirAll = os.MkdirAll
		}()
		err = stateMachine.populateSnapRootfsContents()
		asserter.AssertErrContains(err, "Error creating boot dir")
		osMkdirAll = os.MkdirAll

		// mock ioutil.ReadDir
		ioutilReadDir = mockReadDir
		defer func() {
			ioutilReadDir = ioutil.ReadDir
		}()
		err = stateMachine.populateSnapRootfsContents()
		asserter.AssertErrContains(err, "Error reading unpack dir")
		ioutilReadDir = ioutil.ReadDir

		// mock osutil.CopySpecialFile
		osRename = mockRename
		defer func() {
			osRename = os.Rename
		}()
		err = stateMachine.populateSnapRootfsContents()
		asserter.AssertErrContains(err, "Error moving rootfs")
		osRename = os.Rename
	})
}

// TestFailedGenerateSnapManifest tests if snap-based image manifest generation failures are catched
func TestFailedGenerateSnapManifest(t *testing.T) {
	t.Run("test_failed_generate_snap_manifest", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
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
		stateMachine.IsSeeded = false
		stateMachine.commonFlags.OutputDir = "/dummy/path"

		err := stateMachine.generateSnapManifest()
		asserter.AssertErrContains(err, "Error creating manifest file")
	})
}
