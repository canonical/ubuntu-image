// This test file tests a successful snap run and success/error scenarios for all states
// that are specific to the snap builds
package statemachine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/snapcore/snapd/asserts"
	"github.com/snapcore/snapd/image"
	"github.com/snapcore/snapd/store"

	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/canonical/ubuntu-image/internal/testhelper"
)

// TestSnapStateMachine_Setup_Fail_setConfDefDir tests a failure in the Setup() function when setting the configuration definition directory
func TestSnapStateMachine_Setup_Fail_setConfDefDir(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine SnapStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine

	tmpDirPath := filepath.Join("/tmp", "test_failed_set_conf_dir")
	err := os.Mkdir(tmpDirPath, 0755)
	t.Cleanup(func() {
		os.RemoveAll(tmpDirPath)
	})
	asserter.AssertErrNil(err, true)

	err = os.Chdir(tmpDirPath)
	asserter.AssertErrNil(err, true)

	_ = os.RemoveAll(tmpDirPath)

	err = stateMachine.Setup()
	asserter.AssertErrContains(err, "unable to determine the configuration definition directory")
	os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
}

// TestSnapStateMachine_Setup_Fail_SetSeries tests a failure in the Setup() function when setting the Series.
func TestSnapStateMachine_Setup_Fail_SetSeries(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine SnapStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.Args.ModelAssertion = "/non-existent-path"

	err := stateMachine.Setup()
	asserter.AssertErrContains(err, "cannot read model assertion")
	os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
}

// TestFailedValidateInputSnap tests a failure in the Setup() function when validating common input
func TestFailedSnapSetup(t *testing.T) {
	testCases := []struct {
		name   string
		until  string
		thru   string
		errMsg string
	}{
		{"invalid_until_name", "fake step", "", "not a valid state name"},
		{"invalid_thru_name", "", "fake step", "not a valid state name"},
		{"both_until_and_thru", "make_temporary_directories", "calculate_rootfs_size", "cannot specify both --until and --thru"},
	}
	for _, tc := range testCases {
		t.Run("test_failed_snap_setup_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			restoreCWD := testhelper.SaveCWD()
			defer restoreCWD()

			// use both --until and --thru to trigger this failure
			var stateMachine SnapStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine
			stateMachine.stateMachineFlags.Until = tc.until
			stateMachine.stateMachineFlags.Thru = tc.thru

			err := stateMachine.Setup()
			asserter.AssertErrContains(err, tc.errMsg)
		})
	}
}

// TestFailedReadMetadataSnap tests a failed metadata read by passing --resume with no previous partial state machine run
func TestFailedReadMetadataSnap(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	// start a --resume with no previous SM run
	var stateMachine SnapStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.stateMachineFlags.Resume = true
	stateMachine.stateMachineFlags.WorkDir = testDir

	err := stateMachine.Setup()
	asserter.AssertErrContains(err, "error reading metadata file")
}

// TestSnapStateMachine_Setup_Fail_makeTemporaryDirectories tests the Setup function
// with makeTemporaryDirectories failing
func TestSnapStateMachine_Setup_Fail_makeTemporaryDirectories(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine SnapStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.Args.ModelAssertion = filepath.Join("testdata", "modelAssertion20")

	stateMachine.stateMachineFlags.WorkDir = testDir

	// mock os.MkdirAll
	osMkdirAll = mockMkdirAll
	t.Cleanup(func() {
		osMkdirAll = os.MkdirAll
	})
	err := stateMachine.Setup()
	asserter.AssertErrContains(err, "Error creating work directory")
}

// TestSnapStateMachine_Setup_Fail_determineOutputDirectory tests the Setup function
// with determineOutputDirectory failing
func TestSnapStateMachine_Setup_Fail_determineOutputDirectory(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine SnapStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.Args.ModelAssertion = filepath.Join("testdata", "modelAssertion20")
	stateMachine.commonFlags.OutputDir = "/tmp/test"

	// mock os.MkdirAll
	osMkdirAll = mockMkdirAll
	t.Cleanup(func() {
		osMkdirAll = os.MkdirAll
	})
	err := stateMachine.Setup()
	asserter.AssertErrContains(err, "Error creating OutputDir")
}

// TestSnapStateMachine_DryRun tests a successful dry-run execution
func TestSnapStateMachine_DryRun(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	workDir := "ubuntu-image-test-dry-run"
	err := os.Mkdir(workDir, 0755)
	asserter.AssertErrNil(err, true)

	t.Cleanup(func() { os.RemoveAll(workDir) })

	var stateMachine SnapStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.Args.ModelAssertion = filepath.Join("testdata", "modelAssertion20")
	stateMachine.stateMachineFlags.WorkDir = workDir
	stateMachine.commonFlags.DryRun = true

	err = stateMachine.Setup()
	asserter.AssertErrNil(err, true)

	files, err := osReadDir(workDir)
	asserter.AssertErrNil(err, true)

	if len(files) != 0 {
		t.Errorf("Some files were created in the workdir but should not. Created files: %s", files)
	}

	err = stateMachine.Run()
	asserter.AssertErrNil(err, true)

	err = stateMachine.Teardown()
	asserter.AssertErrNil(err, true)
}

// TestSuccessfulSnapCore20 builds a core 20 image and makes sure the factory boot flag is set
func TestSuccessfulSnapCore20(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine SnapStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.Args.ModelAssertion = filepath.Join("testdata", "modelAssertion20")
	stateMachine.Opts.FactoryImage = true
	workDir, err := os.MkdirTemp("/tmp", "ubuntu-image-")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(workDir) })
	stateMachine.stateMachineFlags.WorkDir = workDir

	err = stateMachine.Setup()
	asserter.AssertErrNil(err, true)

	err = stateMachine.Run()
	asserter.AssertErrNil(err, true)

	// make sure the "factory" boot flag was set
	grubenvFile := filepath.Join(stateMachine.tempDirs.rootfs,
		"EFI", "ubuntu", "grubenv")
	grubenvBytes, err := os.ReadFile(grubenvFile)
	asserter.AssertErrNil(err, true)

	if !strings.Contains(string(grubenvBytes), "snapd_boot_flags=factory") {
		t.Errorf("grubenv file does not have factory boot flag set")
	}

	err = stateMachine.Teardown()
	asserter.AssertErrNil(err, true)
}

func TestSuccessfulSnapCore20WithComponents(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine SnapStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	// note that we must use a dangerous model here, since we're testing
	// explicitly adding components
	stateMachine.Args.ModelAssertion = filepath.Join("testdata", "modelAssertion20Dangerous")
	stateMachine.Opts.FactoryImage = true
	stateMachine.Opts.Snaps = []string{"core24", "test-snap-with-components"}
	stateMachine.Opts.Components = []string{"test-snap-with-components+one", "test-snap-with-components+two"}
	workDir, err := os.MkdirTemp("/tmp", "ubuntu-image-")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(workDir) })
	stateMachine.stateMachineFlags.WorkDir = workDir

	err = stateMachine.Setup()
	asserter.AssertErrNil(err, true)

	err = stateMachine.Run()
	asserter.AssertErrNil(err, true)

	for _, glob := range []string{"test-snap-with-components+one_*.comp", "test-snap-with-components+two_*.comp", "test-snap-with-components_*.snap"} {
		matches, err := filepath.Glob(filepath.Join(
			stateMachine.tempDirs.rootfs,
			"systems/*/snaps/",
			glob,
		))
		asserter.AssertErrNil(err, true)

		if len(matches) != 1 {
			t.Errorf("Expected exactly one match for %s, got %d", glob, len(matches))
		}
	}

	// make sure the "factory" boot flag was set
	grubenvFile := filepath.Join(stateMachine.tempDirs.rootfs,
		"EFI", "ubuntu", "grubenv")
	grubenvBytes, err := os.ReadFile(grubenvFile)
	asserter.AssertErrNil(err, true)

	if !strings.Contains(string(grubenvBytes), "snapd_boot_flags=factory") {
		t.Errorf("grubenv file does not have factory boot flag set")
	}

	err = stateMachine.Teardown()
	asserter.AssertErrNil(err, true)
}

// TestSuccessfulSnapCore18 builds a core 18 image with a few special options
func TestSuccessfulSnapCore18(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine SnapStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.Args.ModelAssertion = filepath.Join("testdata", "modelAssertion18")
	stateMachine.Opts.DisableConsoleConf = true
	stateMachine.commonFlags.Channel = "stable"
	stateMachine.Opts.CloudInit = filepath.Join("testdata", "user-data")
	stateMachine.Opts.Snaps = []string{"hello-world"}
	workDir, err := os.MkdirTemp("/tmp", "ubuntu-image-")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(workDir) })
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

	// check that the system-data partition has the name "writable"
	diskImg := filepath.Join(workDir, "pc.img")
	fdiskCommand := *exec.Command("fdisk", "-l", "-o", "Name", diskImg)

	fdiskBytes, err := fdiskCommand.CombinedOutput()
	asserter.AssertErrNil(err, true)
	if !strings.Contains(string(fdiskBytes), "writable") {
		t.Error("system-data partition is not named \"writable\"")
	}

	// check that the first 3 bytes of the resulting image file are the
	// necessary values in order to boot on Legacy BIOS
	correctLegacyBytes := []byte{0xeb, 0x63, 0x90}
	diskFile, err := os.OpenFile(diskImg, os.O_RDWR, 0755)
	asserter.AssertErrNil(err, true)
	diskImgBytes := make([]byte, 3)
	_, err = diskFile.Read(diskImgBytes)
	asserter.AssertErrNil(err, true)
	if !bytes.Equal(correctLegacyBytes, diskImgBytes) {
		t.Error("First three bytes of resulting image file are not correct")
	}

	err = stateMachine.Teardown()
	asserter.AssertErrNil(err, true)
}

// TestFailedPrepareImage tests prepareImage
func TestFailedPrepareImage(t *testing.T) {
	// Test a failure in the call to image.Prepare. This is easy to achieve
	// by attempting to use --disable-console-conf with a core20 image
	t.Run("test_failed_prepare_image_imagePrepare", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		restoreCWD := testhelper.SaveCWD()
		defer restoreCWD()

		var stateMachine SnapStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.Args.ModelAssertion = filepath.Join("testdata", "modelAssertion20")
		stateMachine.Opts.DisableConsoleConf = true

		err := stateMachine.Setup()
		asserter.AssertErrNil(err, true)

		err = stateMachine.Run()
		fmt.Print(err)
		asserter.AssertErrContains(err, "Error preparing image")

		err = stateMachine.Teardown()
		asserter.AssertErrNil(err, true)
	})

	t.Run("test_failed_prepare_image_snap_revision", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		restoreCWD := testhelper.SaveCWD()
		defer restoreCWD()

		var stateMachine SnapStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.Args.ModelAssertion = filepath.Join("testdata", "modelAssertion20")
		stateMachine.Opts.Revisions = map[string]int{
			"test": 0,
		}

		err := stateMachine.Setup()
		asserter.AssertErrNil(err, true)

		err = stateMachine.Run()
		fmt.Print(err)
		asserter.AssertErrContains(err, "error dealing with snap revision")

		err = stateMachine.Teardown()
		asserter.AssertErrNil(err, true)
	})

}

// TestPopulateSnapRootfsContents runs the state machine through populate_rootfs_contents and examines
// the rootfs to ensure at least some of the correct files are in place
func TestPopulateSnapRootfsContents(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

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
			restoreCWD := testhelper.SaveCWD()
			defer restoreCWD()

			var stateMachine SnapStateMachine
			workDir, err := os.MkdirTemp("/tmp", "ubuntu-image-")
			asserter.AssertErrNil(err, true)
			t.Cleanup(func() { os.RemoveAll(workDir) })

			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine
			stateMachine.Args.ModelAssertion = tc.modelAssertion
			stateMachine.stateMachineFlags.WorkDir = workDir
			stateMachine.stateMachineFlags.Thru = "populate_rootfs_contents"

			err = stateMachine.Setup()
			asserter.AssertErrNil(err, true)

			err = stateMachine.Run()
			asserter.AssertErrNil(err, true)

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
		{"generate_snap_manifest_regular", false},
		{"generate_snap_manifest_seeded", true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			restoreCWD := testhelper.SaveCWD()
			defer restoreCWD()

			workDir, err := os.MkdirTemp("/tmp", "ubuntu-image-")
			asserter.AssertErrNil(err, true)
			t.Cleanup(func() { os.RemoveAll(workDir) })
			var stateMachine SnapStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.stateMachineFlags.WorkDir = workDir
			stateMachine.tempDirs.rootfs = filepath.Join(workDir, "rootfs")
			stateMachine.IsSeeded = tc.seeded
			stateMachine.commonFlags.OutputDir = filepath.Join(workDir, "output")
			err = osMkdirAll(stateMachine.commonFlags.OutputDir, 0755)
			asserter.AssertErrNil(err, true)

			// Prepare direcory structure for installed and seeded snaps
			snapsDir := filepath.Join(stateMachine.tempDirs.rootfs, "system-data", "var", "lib", "snapd", "snaps")
			seedDir := filepath.Join(stateMachine.tempDirs.rootfs, "system-data", "var", "lib", "snapd", "seed", "snaps")
			uc20Dir := filepath.Join(stateMachine.tempDirs.rootfs, "snaps")
			err = osMkdirAll(snapsDir, 0755)
			asserter.AssertErrNil(err, true)
			err = osMkdirAll(seedDir, 0755)
			asserter.AssertErrNil(err, true)
			err = osMkdirAll(uc20Dir, 0755)
			asserter.AssertErrNil(err, true)
			var testEnvMap map[string][]string
			if tc.seeded {
				testEnvMap = map[string][]string{
					uc20Dir: {"foo_1.23.snap", "uc20specific_345.snap"},
				}
			} else {
				testEnvMap = map[string][]string{
					snapsDir: {"foo_1.23.snap", "bar_1.23_version.snap", "baz_234.snap", "test_file"},
					seedDir:  {"foo_1.23.snap", "test_file_2.txt", "test_1234.snap"},
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
			if !tc.seeded {
				testResultMap = map[string][]string{
					"snaps.manifest": {"foo 1.23", "bar 1.23_version", "baz 234"},
				}
			}
			for manifest, snapList := range testResultMap {
				manifestPath := filepath.Join(stateMachine.commonFlags.OutputDir, manifest)
				manifestBytes, err := os.ReadFile(manifestPath)
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
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	asserter := helper.Asserter{T: t}

	workDir, err := os.MkdirTemp("/tmp", "ubuntu-image-")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(workDir) })
	var stateMachine SnapStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.Args.ModelAssertion = filepath.Join("testdata", "modelAssertion18")
	stateMachine.stateMachineFlags.WorkDir = workDir

	// need workdir and gadget.yaml set up for this
	err = stateMachine.determineOutputDirectory()
	asserter.AssertErrNil(err, true)
	err = stateMachine.makeTemporaryDirectories()
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

	// mock os.ReadDir
	osReadDir = mockReadDir
	defer func() {
		osReadDir = os.ReadDir
	}()
	err = stateMachine.populateSnapRootfsContents()
	asserter.AssertErrContains(err, "Error reading unpack dir")
	osReadDir = os.ReadDir

	// mock osutil.CopySpecialFile
	osRename = mockRename
	defer func() {
		osRename = os.Rename
	}()
	err = stateMachine.populateSnapRootfsContents()
	asserter.AssertErrContains(err, "Error moving rootfs")
	osRename = os.Rename
}

// TestFailedGenerateSnapManifest tests if snap-based image manifest generation failures are catched
func TestFailedGenerateSnapManifest(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	osReadDir = func(string) ([]os.DirEntry, error) {
		return []os.DirEntry{}, nil
	}
	defer func() {
		osReadDir = os.ReadDir
	}()
	// Setup the mock for os.Create, making those fail
	osCreate = mockCreate
	defer func() {
		osCreate = os.Create
	}()

	var stateMachine SnapStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.stateMachineFlags.WorkDir = "/test/path"
	stateMachine.tempDirs.rootfs = "/test/path"
	stateMachine.IsSeeded = false
	stateMachine.commonFlags.OutputDir = "/test/path"

	err := stateMachine.generateSnapManifest()
	asserter.AssertErrContains(err, "Error creating manifest file")
}

// TestSnapFlagSyntax tests various syntaxes for the "--snap" argument,
// including valid and invalid syntax (LP: #1947864)
func TestSnapFlagSyntax(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	testCases := []struct {
		name     string
		snapArgs []string
		valid    bool
	}{
		{"no_channel_specified", []string{"hello", "core20"}, true},
		{"channel_specified", []string{"hello=edge", "core20"}, true},
		{"mixed_syntax", []string{"hello", "core20=candidate"}, true},
		{"invalid_syntax", []string{"hello=edge=stable", "core20"}, false},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if runtime.GOARCH != "amd64" {
				t.Skip("Test for amd64 only")
			}
			asserter := helper.Asserter{T: t}
			restoreCWD := testhelper.SaveCWD()
			defer restoreCWD()

			var stateMachine SnapStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine

			// use core18 because it builds the fastest
			stateMachine.Args.ModelAssertion = filepath.Join("testdata", "modelAssertion18")
			stateMachine.Opts.Snaps = tc.snapArgs
			workDir, err := os.MkdirTemp("/tmp", "ubuntu-image-")
			asserter.AssertErrNil(err, true)
			t.Cleanup(func() { os.RemoveAll(workDir) })
			stateMachine.stateMachineFlags.WorkDir = workDir
			stateMachine.commonFlags.OutputDir = workDir

			err = stateMachine.Setup()
			asserter.AssertErrNil(err, true)

			err = stateMachine.Run()

			if tc.valid {
				// check Run() ended without errors
				asserter.AssertErrNil(err, true)

				// make sure the correct channels were used
				for _, snapArg := range tc.snapArgs {
					var snapName string
					var snapChannel string
					if strings.Contains(snapArg, "=") {
						splitArg := strings.Split(snapArg, "=")
						snapName = splitArg[0]
						snapChannel = splitArg[1]
					} else {
						snapName = snapArg
						snapChannel = "stable"
					}
					// now reach out to the snap store to find the revision
					// of the snap for the specified channel
					snapStore := store.New(nil, nil)
					snapSpec := store.SnapSpec{Name: snapName}
					context := context.TODO() //context can be empty, just not nil
					snapInfo, err := snapStore.SnapInfo(context, snapSpec, nil)
					asserter.AssertErrNil(err, true)

					var storeRevision, seededRevision int
					storeRevision = snapInfo.Channels["latest/"+snapChannel].Revision.N

					// compile a regex used to get revision numbers from seed.manifest
					revRegex, err := regexp.Compile(fmt.Sprintf(
						"%s (.*?)\n", snapName))
					asserter.AssertErrNil(err, true)
					seedData, err := os.ReadFile(filepath.Join(
						stateMachine.stateMachineFlags.WorkDir, "seed.manifest"))
					asserter.AssertErrNil(err, true)
					revString := revRegex.FindStringSubmatch(string(seedData))
					if len(revString) != 2 {
						t.Fatal("Error finding snap revision via regex")
					}
					seededRevision, err = strconv.Atoi(revString[1])
					asserter.AssertErrNil(err, true)

					// finally check that the seeded revision matches what the store reports
					if storeRevision != seededRevision {
						t.Errorf("Error, expected snap %s to "+
							"be revision %d, but it was %d",
							snapName, storeRevision, seededRevision)
					}
				}
			} else {
				asserter.AssertErrContains(err, "Invalid syntax")
			}
		})
	}
}

// TestSnapRevisions tests the --revision flag and ensures the correct
// revisions are installed in the image
func TestSnapRevisions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	// many snaps aren't published on other architectures, so only run this on amd64
	if runtime.GOARCH != "amd64" {
		t.Skip("Test for amd64 only")
	}
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine SnapStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine

	// use core18 because it builds the fastest
	stateMachine.Args.ModelAssertion = filepath.Join("testdata", "modelAssertion18")
	stateMachine.Opts.Snaps = []string{"hello", "core", "core20"}
	stateMachine.Opts.Revisions = map[string]int{
		"hello": 38,
		"core":  14784,
	}
	workDir, err := os.MkdirTemp("/tmp", "ubuntu-image-")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(workDir) })
	stateMachine.stateMachineFlags.WorkDir = workDir
	stateMachine.commonFlags.OutputDir = workDir

	err = stateMachine.Setup()
	asserter.AssertErrNil(err, true)

	err = stateMachine.Run()
	asserter.AssertErrNil(err, true)

	for snapName, expectedRevision := range stateMachine.Opts.Revisions {
		// compile a regex used to get revision numbers from seed.manifest
		revRegex, err := regexp.Compile(fmt.Sprintf(
			"%s (.*?)\n", snapName))
		asserter.AssertErrNil(err, true)
		seedData, err := os.ReadFile(filepath.Join(
			stateMachine.stateMachineFlags.WorkDir, "seed.manifest"))
		asserter.AssertErrNil(err, true)
		revString := revRegex.FindStringSubmatch(string(seedData))
		if len(revString) != 2 {
			t.Fatal("Error finding snap revision via regex")
		}
		seededRevision, err := strconv.Atoi(revString[1])
		asserter.AssertErrNil(err, true)

		if seededRevision != expectedRevision {
			t.Errorf("Error, expected snap %s to "+
				"be revision %d, but it was %d",
				snapName, expectedRevision, seededRevision)
		}
	}
}

// TestValidationFlag ensures that the the validation flag is passed through to image.Prepare
// correctly. This is accomplished by enabling the flag and ensuring the correct version
// of a snap is installed as a result
func TestValidationFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine SnapStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.Args.ModelAssertion = filepath.Join("testdata", "modelAssertionValidation")
	workDir, err := os.MkdirTemp("/tmp", "ubuntu-image-")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(workDir) })
	stateMachine.stateMachineFlags.WorkDir = workDir
	stateMachine.stateMachineFlags.Thru = "prepare_image"
	stateMachine.commonFlags.Validation = "enforce"

	err = stateMachine.Setup()
	asserter.AssertErrNil(err, true)

	err = stateMachine.Run()
	asserter.AssertErrNil(err, true)

	// make sure the correct revision of the snap exists
	gatedRevision := "2"
	_, err = os.Stat(filepath.Join(stateMachine.tempDirs.unpack,
		"system-seed", "snaps", "test-snapd-gated_"+gatedRevision+".snap"))
	asserter.AssertErrNil(err, true)

	err = stateMachine.Teardown()
	asserter.AssertErrNil(err, true)
}

// TestGadgetEdgeCases tests a few edge cases with odd structures in gadget.yaml
// LP: #1968205
func TestGadgetEdgeCases(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var stateMachine SnapStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.Args.ModelAssertion = filepath.Join("testdata", "modelAssertion20Dangerous")
	stateMachine.Opts.FactoryImage = true
	workDir, err := os.MkdirTemp("/tmp", "ubuntu-image-")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(workDir) })
	stateMachine.stateMachineFlags.WorkDir = workDir
	// use the custom snap with a complicated gadget.yaml
	customSnap := filepath.Join("testdata", "pc_20-gadget-edge-cases.snap")
	stateMachine.Opts.Snaps = []string{customSnap}

	err = stateMachine.Setup()
	asserter.AssertErrNil(err, true)

	err = stateMachine.Run()
	asserter.AssertErrNil(err, true)

	err = stateMachine.Teardown()
	asserter.AssertErrNil(err, true)
}

func TestPreseedFlag(t *testing.T) {
	asserter := helper.Asserter{T: t}
	restoreCWD := testhelper.SaveCWD()
	defer restoreCWD()

	var calledOpts *image.Options
	imagePrepare = func(opts *image.Options) error {
		calledOpts = opts
		return nil
	}
	defer func() {
		imagePrepare = image.Prepare
	}()

	var stateMachine SnapStateMachine
	stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
	stateMachine.parent = &stateMachine
	stateMachine.Args.ModelAssertion = filepath.Join("testdata", "modelAssertionValidation")
	workDir, err := os.MkdirTemp("/tmp", "ubuntu-image-")
	asserter.AssertErrNil(err, true)
	t.Cleanup(func() { os.RemoveAll(workDir) })
	stateMachine.stateMachineFlags.WorkDir = workDir
	stateMachine.stateMachineFlags.Thru = "prepare_image"
	stateMachine.Opts.Preseed = true
	stateMachine.Opts.AppArmorKernelFeaturesDir = "/some/path"
	stateMachine.Opts.PreseedSignKey = "akey"

	err = stateMachine.Setup()
	asserter.AssertErrNil(err, true)

	err = stateMachine.Run()
	asserter.AssertErrNil(err, true)

	if calledOpts == nil {
		t.Errorf("options passed to image.Prepare are nil")
	}

	if calledOpts.Preseed == false {
		t.Error("Expected preseed flag to be set")
	}

	expectedKey := "akey"
	if calledOpts.PreseedSignKey != expectedKey {
		t.Errorf("Expected preseed sign key to be %q, but it's %q", expectedKey, calledOpts.PreseedSignKey)
	}

	expectedAAPath := "/some/path"
	if calledOpts.AppArmorKernelFeaturesDir != expectedAAPath {
		t.Errorf("Expected apparmor kernel features dir to be %q, but it's %q", expectedAAPath, calledOpts.AppArmorKernelFeaturesDir)
	}

	err = stateMachine.Teardown()
	asserter.AssertErrNil(err, true)
}

func TestSnapStateMachine_decodeModelAssertion(t *testing.T) {
	type fields struct {
		Args commands.SnapArgs
	}
	tests := []struct {
		name           string
		fields         fields
		modelAssertion string
		want           *asserts.Model
		expectedError  string
		shouldPass     bool
	}{
		{
			name:           "fail when missing file",
			modelAssertion: "inexistent_file",
			shouldPass:     false,
			expectedError:  "cannot read model assertion",
		},
		{
			name:           "fail to decode empty model assertion",
			modelAssertion: filepath.Join("testdata", "modelAssertionEmpty"),
			shouldPass:     false,
			expectedError:  "cannot decode model assertion",
		},
		{
			name:           "fail to decode invalid model assertion",
			modelAssertion: filepath.Join("testdata", "modelAssertionNotOne"),
			shouldPass:     false,
			expectedError:  "is not a model assertion",
		},
		{
			name:           "fail to decode model assertion with reserved",
			modelAssertion: filepath.Join("testdata", "modelAssertionReserverdHeader"),
			shouldPass:     false,
			expectedError:  "model assertion cannot have reserved/unsupported header",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}

			snapStateMachine := &SnapStateMachine{
				Args: commands.SnapArgs{
					ModelAssertion: tt.modelAssertion,
				},
			}
			got, err := snapStateMachine.decodeModelAssertion()
			if tt.shouldPass {
				asserter.AssertEqual(tt.want, got)
			} else {
				asserter.AssertErrContains(err, tt.expectedError)
			}
		})
	}
}
