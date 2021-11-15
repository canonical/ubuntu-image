// This test file tests a successful classic run and success/error scenarios for all states
// that are specific to the classic builds
package statemachine

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/snapcore/snapd/osutil"
)

// TestInvalidCommandLineClassic tests invalid command line input for classic images
func TestInvalidCommandLineClassic(t *testing.T) {
	testCases := []struct {
		name       string
		project    string
		filesystem string
		errMsg     string
	}{
		{"neither_project_nor_filesystem", "", "", "project or filesystem is required"},
		{"both_project_and_filesystem", "ubuntu-cpc", "/tmp", "project and filesystem are mutually exclusive"},
	}
	for _, tc := range testCases {
		t.Run("test "+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			saveCWD := helper.SaveCWD()
			defer saveCWD()

			var stateMachine ClassicStateMachine
			stateMachine.Opts.Project = tc.project
			stateMachine.Opts.Filesystem = tc.filesystem
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

			err := stateMachine.Setup()
			asserter.AssertErrContains(err, tc.errMsg)
			os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
		})
	}
}

// TestFailedValidateInputClassic tests a failure in the Setup() function when validating common input
func TestFailedValidateInputClassic(t *testing.T) {
	t.Run("test_failed_validate_input", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		// use both --until and --thru to trigger this failure
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.stateMachineFlags.Until = "until-test"
		stateMachine.stateMachineFlags.Thru = "thru-test"

		err := stateMachine.Setup()
		asserter.AssertErrContains(err, "cannot specify both --until and --thru")
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedReadMetadataClassic tests a failed metadata read by passing --resume with no previous partial state machine run
func TestFailedReadMetadataClassic(t *testing.T) {
	t.Run("test_failed_read_metadata", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		// start a --resume with no previous SM run
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.stateMachineFlags.Resume = true
		stateMachine.stateMachineFlags.WorkDir = testDir

		err := stateMachine.Setup()
		asserter.AssertErrContains(err, "error reading metadata file")
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestPrepareGadgetTree runs prepareGadgetTree() and ensures the gadget_tree files
// are placed in the correct locations
func TestPrepareGadgetTree(t *testing.T) {
	t.Run("test_prepare_gadget_tree", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.Args.GadgetTree = filepath.Join("testdata", "gadget_tree")
		stateMachine.parent = &stateMachine

		err := stateMachine.prepareGadgetTree()
		asserter.AssertErrNil(err, true)

		gadgetTreeFiles := []string{"grub.conf", "pc-boot.img", "meta/gadget.yaml"}
		for _, file := range gadgetTreeFiles {
			_, err := os.Stat(filepath.Join(stateMachine.tempDirs.unpack, "gadget", file))
			if err != nil {
				t.Errorf("File %s should be in unpack, but is missing", file)
			}
		}
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedPrepareGadgetTree tests failures in os, osutil, and ioutil libraries
func TestFailedPrepareGadgetTree(t *testing.T) {
	t.Run("test_failed_prepare_gadget_tree", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.Args.GadgetTree = filepath.Join("testdata", "gadget_tree")
		stateMachine.parent = &stateMachine

		// mock os.Mkdir
		osMkdirAll = mockMkdirAll
		defer func() {
			osMkdirAll = os.MkdirAll
		}()
		err := stateMachine.prepareGadgetTree()
		asserter.AssertErrContains(err, "Error creating unpack directory")
		osMkdirAll = os.MkdirAll

		// mock ioutil.ReadDir
		ioutilReadDir = mockReadDir
		defer func() {
			ioutilReadDir = ioutil.ReadDir
		}()
		err = stateMachine.prepareGadgetTree()
		asserter.AssertErrContains(err, "Error reading gadget tree")
		ioutilReadDir = ioutil.ReadDir

		// mock osutil.CopySpecialFile
		osutilCopySpecialFile = mockCopySpecialFile
		defer func() {
			osutilCopySpecialFile = osutil.CopySpecialFile
		}()
		err = stateMachine.prepareGadgetTree()
		asserter.AssertErrContains(err, "Error copying gadget tree")
		osutilCopySpecialFile = osutil.CopySpecialFile

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TODO replace this with fakeExecCommand that sil2100 wrote
// TestFailedLiveBuildCommands tests the scenario where calls to `lb` fail
// this is accomplished by temporarily replacing lb on disk with a test script
func TestFailedLiveBuildCommands(t *testing.T) {
	testCases := []struct {
		name       string
		testScript string
	}{
		{"failed_lb_config", "lb_config_fail"},
		{"failed_lb_build", "lb_build_fail"},
	}
	for _, tc := range testCases {
		t.Run("test_"+tc.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			saveCWD := helper.SaveCWD()
			defer saveCWD()

			var stateMachine ClassicStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.Opts.Project = "ubuntu-cpc"
			stateMachine.Opts.Subproject = "fakeproject"
			stateMachine.Opts.Subarch = "fakearch"
			stateMachine.Opts.WithProposed = true
			stateMachine.Opts.ExtraPPAs = []string{"ppa:fake_user/fakeppa"}
			stateMachine.Args.GadgetTree = filepath.Join("testdata", "gadget_tree")
			stateMachine.parent = &stateMachine

			scriptPath := filepath.Join("testscripts", tc.testScript)
			// save the original lb
			whichLb := *exec.Command("which", "lb")
			lbLocationBytes, _ := whichLb.Output()
			lbLocation := strings.TrimSpace(string(lbLocationBytes))
			// ensure the backup doesn't exist
			os.Remove(lbLocation + ".bak")
			err := os.Rename(lbLocation, lbLocation+".bak")
			asserter.AssertErrNil(err, true)

			err = osutil.CopyFile(scriptPath, lbLocation, 0)
			asserter.AssertErrNil(err, true)
			defer func() {
				os.Remove(lbLocation)
				os.Rename(lbLocation+".bak", lbLocation)
			}()

			// need workdir set up for this
			err = stateMachine.makeTemporaryDirectories()
			asserter.AssertErrNil(err, true)

			// also need unpack set up
			err = os.Mkdir(stateMachine.tempDirs.unpack, 0755)
			asserter.AssertErrNil(err, true)

			err = stateMachine.runLiveBuild()
			asserter.AssertErrContains(err, "Error running command")
			os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
		})
	}
}

// TestNoStatic tests that the helper function to prepare lb commands
// returns an error if the qemu-static binary is missing. This is accomplished
// by passing an architecture for which there is no qemu-static binary
func TestNoStatic(t *testing.T) {
	t.Run("test_no_qemu_static", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.Opts.Project = "ubuntu-cpc"
		stateMachine.Opts.Arch = "fakearch"
		stateMachine.Args.GadgetTree = filepath.Join("testdata", "gadget_tree")
		stateMachine.parent = &stateMachine

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// also need unpack set up
		err = os.Mkdir(stateMachine.tempDirs.unpack, 0755)
		asserter.AssertErrNil(err, true)

		err = stateMachine.runLiveBuild()
		asserter.AssertErrContains(err, "in case of non-standard archs or custom paths")
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestPopulateClassicRootfsContents runs the state machine through populate_rootfs_contents and examines
// the rootfs to ensure at least some of the correct file are in place
func TestPopulateClassicRootfsContents(t *testing.T) {
	t.Run("test_populate_classic_rootfs_contents", func(t *testing.T) {
		if runtime.GOARCH != "amd64" {
			t.Skip("Test for amd64 only")
		}
		asserter := helper.Asserter{T: t}
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.Opts.Project = "ubuntu-cpc"
		stateMachine.Opts.Suite = "focal"
		stateMachine.Args.GadgetTree = filepath.Join("testdata", "gadget_tree")
		stateMachine.commonFlags.Snaps = []string{"hello", "ubuntu-image/classic"}
		stateMachine.stateMachineFlags.Thru = "populate_rootfs_contents"

		err := stateMachine.Setup()
		asserter.AssertErrNil(err, true)

		err = stateMachine.Run()
		asserter.AssertErrNil(err, true)

		// check the files before Teardown
		fileList := []string{filepath.Join("etc", "shadow"),
			filepath.Join("etc", "systemd"),
			filepath.Join("boot", "vmlinuz"),
			filepath.Join("boot", "grub"),
			filepath.Join("usr", "lib")}
		for _, file := range fileList {
			_, err := os.Stat(filepath.Join(stateMachine.tempDirs.rootfs, file))
			if err != nil {
				if os.IsNotExist(err) {
					t.Errorf("File %s should exist, but does not", file)
				}
			}
		}

		// check /etc/fstab contents to test the regex part
		fstab, err := ioutilReadFile(filepath.Join(stateMachine.tempDirs.rootfs,
			"etc", "fstab"))
		if err != nil {
			t.Errorf("Error reading fstab to check regex")
		}
		correctLabel := "LABEL=writable   /    ext4   defaults    0 0"
		if !strings.Contains(string(fstab), correctLabel) {
			t.Errorf("Expected fstab contents %s to contain %s",
				string(fstab), correctLabel)
		}

		// check that extra snaps were added to the rootfs
		for _, snap := range stateMachine.commonFlags.Snaps {
			if strings.Contains(snap, "/") {
				snap = strings.Split(snap, "/")[0]
			}
			filePath := filepath.Join(stateMachine.tempDirs.unpack,
				"chroot", "var", "snap", snap)
			if !osutil.FileExists(filePath) {
				t.Errorf("File %s should exist but it does not", filePath)
			}
		}

		err = stateMachine.Teardown()
		asserter.AssertErrNil(err, false)
	})
}

// TestFailedPopulateClassicRootfsContents tests failed scenarios in populateClassicRootfsContents
// this is accomplished by mocking functions
func TestFailedPopulateClassicRootfsContents(t *testing.T) {
	t.Run("test_failed_populate_classic_rootfs_contents", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.Opts.Filesystem = filepath.Join("testdata", "filesystem")
		stateMachine.commonFlags.CloudInit = filepath.Join("testdata", "user-data")

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		// mock ioutil.ReadDir
		ioutilReadDir = mockReadDir
		defer func() {
			ioutilReadDir = ioutil.ReadDir
		}()
		err = stateMachine.populateClassicRootfsContents()
		asserter.AssertErrContains(err, "Error reading unpack/chroot dir")
		ioutilReadDir = ioutil.ReadDir

		// mock osutil.CopySpecialFile
		osutilCopySpecialFile = mockCopySpecialFile
		defer func() {
			osutilCopySpecialFile = osutil.CopySpecialFile
		}()
		err = stateMachine.populateClassicRootfsContents()
		asserter.AssertErrContains(err, "Error copying rootfs")
		osutilCopySpecialFile = osutil.CopySpecialFile

		// mock ioutil.ReadFile
		ioutilReadFile = mockReadFile
		defer func() {
			ioutilReadFile = ioutil.ReadFile
		}()
		err = stateMachine.populateClassicRootfsContents()
		asserter.AssertErrContains(err, "Error opening fstab")
		ioutilReadFile = ioutil.ReadFile

		// mock ioutil.WriteFile
		ioutilWriteFile = mockWriteFile
		defer func() {
			ioutilWriteFile = ioutil.WriteFile
		}()
		err = stateMachine.populateClassicRootfsContents()
		asserter.AssertErrContains(err, "Error writing to fstab")
		ioutilWriteFile = ioutil.WriteFile

		// mock os.MkdirAll
		osMkdirAll = mockMkdirAll
		defer func() {
			osMkdirAll = os.MkdirAll
		}()
		err = stateMachine.populateClassicRootfsContents()
		asserter.AssertErrContains(err, "Error creating cloud-init dir")
		osMkdirAll = os.MkdirAll

		// mock os.OpenFile
		osOpenFile = mockOpenFile
		defer func() {
			osOpenFile = os.OpenFile
		}()
		err = stateMachine.populateClassicRootfsContents()
		asserter.AssertErrContains(err, "Error opening cloud-init meta-data file")
		osOpenFile = os.OpenFile

		// mock osutil.CopyFile
		osutilCopyFile = mockCopyFile
		defer func() {
			osutilCopyFile = osutil.CopyFile
		}()
		err = stateMachine.populateClassicRootfsContents()
		asserter.AssertErrContains(err, "Error copying cloud-init")
		osutilCopyFile = osutil.CopyFile
	})
}

// TestFilesystemFlag makes sure that with the --filesystem flag the specified filesystem is copied
// to the rootfs directory
func TestFilesystemFlag(t *testing.T) {
	t.Run("test_filesystem_flag", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.Opts.Filesystem = filepath.Join("testdata", "filesystem")

		// need workdir set up for this
		err := stateMachine.makeTemporaryDirectories()
		asserter.AssertErrNil(err, true)

		err = stateMachine.populateClassicRootfsContents()
		asserter.AssertErrNil(err, true)

		// check that the specified filesystem was copied over
		if _, err := os.Stat(filepath.Join(stateMachine.tempDirs.rootfs, "testfile")); err != nil {
			t.Errorf("Failed to copy --filesystem to rootfs")
		}
	})
}

// TestGeneratePackageManifest tests if classic image manifest generation works
func TestGeneratePackageManifest(t *testing.T) {
	t.Run("test_generate_package_manifest", func(t *testing.T) {
		asserter := helper.Asserter{T: t}

		// Setup the exec.Command mock
		testCaseName = "TestGeneratePackageManifest"
		execCommand = fakeExecCommand
		defer func() {
			execCommand = exec.Command
		}()
		// We need the output directory set for this
		outputDir, err := ioutil.TempDir("/tmp", "ubuntu-image-")
		asserter.AssertErrNil(err, true)
		defer os.RemoveAll(outputDir)

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.commonFlags.OutputDir = outputDir
		osMkdirAll(stateMachine.commonFlags.OutputDir, 0755)

		err = stateMachine.generatePackageManifest()
		asserter.AssertErrNil(err, true)

		// Check if manifest file got generated and if it has expected contents
		manifestPath := filepath.Join(stateMachine.commonFlags.OutputDir, "filesystem.manifest")
		manifestBytes, err := ioutil.ReadFile(manifestPath)
		asserter.AssertErrNil(err, true)
		// The order of packages shouldn't matter
		examplePackages := []string{"foo 1.2", "bar 1.4-1ubuntu4.1", "libbaz 0.1.3ubuntu2"}
		for _, pkg := range examplePackages {
			if !strings.Contains(string(manifestBytes), pkg) {
				t.Errorf("filesystem.manifest does not contain expected package: %s", pkg)
			}
		}
	})
}

// TestFailedGeneratePackageManifest tests if classic manifest generation failures are reported
func TestFailedGeneratePackageManifest(t *testing.T) {
	t.Run("test_failed_generate_package_manifest", func(t *testing.T) {
		asserter := helper.Asserter{T: t}

		// Setup the exec.Command mock - version from the success test
		testCaseName = "TestGeneratePackageManifest"
		execCommand = fakeExecCommand
		defer func() {
			execCommand = exec.Command
		}()
		// Setup the mock for os.Create, making those fail
		osCreate = mockCreate
		defer func() {
			osCreate = os.Create
		}()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.commonFlags.OutputDir = "/dummy/path"

		err := stateMachine.generatePackageManifest()
		asserter.AssertErrContains(err, "Error creating manifest file")
	})
}

// TestFailedRunLiveBuild tests some error scenarios in the runLiveBuild state that are not
// caused by actual failures in the `lb` commands
func TestFailedRunLiveBuild(t *testing.T) {
	t.Run("test_failed_run_live_build", func(t *testing.T) {
		asserter := helper.Asserter{T: t}

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.Opts.Project = "ubuntu-cpc"
		stateMachine.Opts.Suite = "focal"
		stateMachine.Args.GadgetTree = filepath.Join("testdata", "gadget_tree")
		stateMachine.commonFlags.Snaps = []string{"hello", "ubuntu-image/classic"}
		stateMachine.stateMachineFlags.Thru = "run_live_build"

		// replace the lb commands with a script that will simply pass
		testCaseName = "TestFailedRunLiveBuild"
		execCommand = fakeExecCommand
		defer func() {
			execCommand = exec.Command
		}()
		// since we have mocked exec.Command, running dpkg -L to find the livecd-rootfs
		// filepath will fail. We can use an environment variable instead
		dpkgArgs := "dpkg -L livecd-rootfs | grep \"auto$\""
		dpkgCommand := *exec.Command("bash", "-c", dpkgArgs)
		dpkgBytes, err := dpkgCommand.Output()
		asserter.AssertErrNil(err, true)
		autoSrc := strings.TrimSpace(string(dpkgBytes))
		os.Setenv("UBUNTU_IMAGE_LIVECD_ROOTFS_AUTO_PATH", autoSrc)

		// mock os.OpenFile
		osOpenFile = mockOpenFile
		defer func() {
			osOpenFile = os.OpenFile
		}()
		err = stateMachine.Setup()
		asserter.AssertErrNil(err, true)

		err = stateMachine.Run()
		asserter.AssertErrContains(err, "Error opening seeded-snaps")
		osOpenFile = os.OpenFile
		os.RemoveAll(testDir)

		// mock os.OpenFile
		osOpenFile = mockOpenFileBadPerms
		defer func() {
			osOpenFile = os.OpenFile
		}()
		err = stateMachine.Setup()
		asserter.AssertErrNil(err, true)

		err = stateMachine.Run()
		asserter.AssertErrContains(err, "Error writing snap hello=stable to seeded-snaps")
		osOpenFile = os.OpenFile
		os.RemoveAll(testDir)
		os.Unsetenv("UBUNTU_IMAGE_LIVECD_ROOTFS_AUTO_PATH")
	})
}
