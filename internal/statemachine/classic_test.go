// This test file tests a successful classic run and success/error scenarios for all states
// that are specific to the classic builds
package statemachine

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/snapcore/snapd/osutil"
)

// This is a helper function that we can use to examine the lb commands without running them
func (stateMachine *StateMachine) examineLiveBuild() error {
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)
	if classicStateMachine.Opts.Filesystem == "" {
		// --filesystem was not provided, so we use live-build to create one
		var env []string
		var arch string
		env = append(env, "PROJECT="+classicStateMachine.Opts.Project)
		if classicStateMachine.Opts.Suite != "" {
			env = append(env, "SUITE="+classicStateMachine.Opts.Suite)
		} else {
			env = append(env, "SUITE="+helper.GetHostSuite())
		}
		if classicStateMachine.Opts.Arch == "" {
			arch = helper.GetHostArch()
		} else {
			arch = classicStateMachine.Opts.Arch
		}
		env = append(env, "ARCH="+arch)
		if classicStateMachine.Opts.Subproject != "" {
			env = append(env, "SUBPROJECT="+classicStateMachine.Opts.Subproject)
		}
		if classicStateMachine.Opts.Subarch != "" {
			env = append(env, "SUBARCH="+classicStateMachine.Opts.Subarch)
		}
		if classicStateMachine.Opts.WithProposed {
			env = append(env, "PROPOSED=1")
		}
		if len(classicStateMachine.Opts.ExtraPPAs) > 0 {
			env = append(env, "EXTRA_PPAS="+strings.Join(classicStateMachine.Opts.ExtraPPAs, " "))
		}
		env = append(env, "IMAGEFORMAT=none")
		lbConfig, _, err := helper.SetupLiveBuildCommands(classicStateMachine.tempDirs.rootfs,
			arch, env, true)
		if err != nil {
			return fmt.Errorf("error setting up live_build: %s", err.Error())
		}

		for _, arg := range lbConfig.Args {
			if arg == "--bootstrap-qemu-arch" {
				return nil
			}
		}
		// bootstrap-qemu-arch not found, fail test
		return fmt.Errorf("Error: --bootstramp-qemu-arch not found in lb config args")
	}
	return nil
}

// TestInvalidCommandLineClassic tests invalid command line input for classic images
func TestInvalidCommandLineClassic(t *testing.T) {
	testCases := []struct {
		name       string
		project    string
		filesystem string
	}{
		{"neither_project_nor_filesystem", "", ""},
		{"both_project_and_filesystem", "ubuntu-cpc", "/tmp"},
	}
	for _, tc := range testCases {
		t.Run("test "+tc.name, func(t *testing.T) {
			saveCWD := helper.SaveCWD()
			defer saveCWD()

			var stateMachine ClassicStateMachine
			stateMachine.Opts.Project = tc.project
			stateMachine.Opts.Filesystem = tc.filesystem
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()

			if err := stateMachine.Setup(); err == nil {
				t.Error("Expected an error but there was none")
			}
			os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
		})
	}
}

// TestFailedValidateInputClassic tests a failure in the Setup() function when validating common input
func TestFailedValidateInputClassic(t *testing.T) {
	t.Run("test_failed_validate_input", func(t *testing.T) {
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		// use both --until and --thru to trigger this failure
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.stateMachineFlags.Until = "until-test"
		stateMachine.stateMachineFlags.Thru = "thru-test"

		if err := stateMachine.Setup(); err == nil {
			t.Error("Expected an error but there was none")
		}
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedReadMetadataClassic tests a failed metadata read by passing --resume with no previous partial state machine run
func TestFailedReadMetadataClassic(t *testing.T) {
	t.Run("test_failed_read_metadata", func(t *testing.T) {
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		// start a --resume with no previous SM run
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.stateMachineFlags.Resume = true
		stateMachine.stateMachineFlags.WorkDir = testDir

		if err := stateMachine.Setup(); err == nil {
			t.Error("Expected an error but there was none")
		}
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestPrepareGadgetTree runs prepareGadgetTree() and ensures the gadget_tree files
// are placed in the correct locations
func TestPrepareGadgetTree(t *testing.T) {
	t.Run("test_prepare_gadget_tree", func(t *testing.T) {
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.Args.GadgetTree = filepath.Join("testdata", "gadget_tree")
		stateMachine.parent = &stateMachine

		if err := stateMachine.prepareGadgetTree(); err != nil {
			t.Errorf("Did not expect an error, but got %s", err.Error())
		}
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
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.Args.GadgetTree = filepath.Join("testdata", "gadget_tree")
		stateMachine.parent = &stateMachine

		// mock os.Mkdir and test with and without a WorkDir
		osMkdirAll = mockMkdirAll
		defer func() {
			osMkdirAll = os.MkdirAll
		}()
		if err := stateMachine.prepareGadgetTree(); err == nil {
			t.Error("Expected an error, but got none")
		}
		// restore the function
		osMkdirAll = os.MkdirAll

		// mock ioutil.ReadDir
		ioutilReadDir = mockReadDir
		defer func() {
			ioutilReadDir = ioutil.ReadDir
		}()
		if err := stateMachine.prepareGadgetTree(); err == nil {
			t.Error("Expected an error, but got none")
		}
		// restore the function
		ioutilReadDir = ioutil.ReadDir

		// mock osutil.CopySpecialFile
		osutilCopySpecialFile = mockCopySpecialFile
		defer func() {
			osutilCopySpecialFile = osutil.CopySpecialFile
		}()
		if err := stateMachine.prepareGadgetTree(); err == nil {
			t.Error("Expected an error, but got none")
		}
		// restore the function
		osutilCopySpecialFile = osutil.CopySpecialFile

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestSuccessfulClassicRun runs through all states ensuring none failed
func TestSuccessfulClassicRun(t *testing.T) {
	t.Run("test_successful_classic_run", func(t *testing.T) {
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.Opts.Project = "ubuntu-cpc"
		stateMachine.Opts.Suite = "focal"
		stateMachine.Args.GadgetTree = filepath.Join("testdata", "gadget_tree")
		stateMachine.parent = &stateMachine

		if err := stateMachine.Setup(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}

		if err := stateMachine.Run(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}

		if err := stateMachine.Teardown(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestCrossArch uses a different arch than the host arch and ensures lb commands are set up correctly.
// These tend to be flakey or fail in different environments, so we don't actually run lb commands
func TestSuccessfulClassicCrossArch(t *testing.T) {
	t.Run("test_successful_classic_cross_arch", func(t *testing.T) {
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.Opts.Project = "ubuntu-cpc"
		stateMachine.stateMachineFlags.Thru = "run_live_build"
		if helper.GetHostArch() != "arm64" {
			stateMachine.Opts.Arch = "arm64"
		} else {
			stateMachine.Opts.Arch = "armhf"
		}
		stateMachine.Args.GadgetTree = "testdata/gadget_tree"
		stateMachine.stateMachineFlags.Thru = "run_live_build"
		stateMachine.parent = &stateMachine

		if err := stateMachine.Setup(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}

		// change the runLiveBuild function to not run the live build commands but inspect their args
		stateNum := stateMachine.getStateNumberByName("run_live_build")
		oldFunc := stateMachine.states[stateNum]
		defer func() {
			stateMachine.states[stateNum] = oldFunc
		}()
		stateMachine.states[stateNum] = stateFunc{"run_live_build", (*StateMachine).examineLiveBuild}

		if err := stateMachine.Run(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

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

			// TODO: write a helper function for "mv"
			scriptPath := filepath.Join("testscripts", tc.testScript)
			// save the original lb
			whichLb := *exec.Command("which", "lb")
			lbLocationBytes, _ := whichLb.Output()
			lbLocation := strings.TrimSpace(string(lbLocationBytes))
			// ensure the backup doesn't exist
			os.Remove(lbLocation + ".bak")
			err := osutil.CopyFile(lbLocation, lbLocation+".bak", 0)
			if err != nil {
				t.Errorf("Failed back up lb: %s", err.Error())
			}

			// copy testscript to lb
			os.Remove(lbLocation)
			err = osutil.CopyFile(scriptPath, lbLocation, 0)
			if err != nil {
				t.Errorf("Failed to copy testscript %s: %s", tc.testScript, err.Error())
			}
			defer func() {
				os.Remove(lbLocation)
				osutil.CopyFile(lbLocation+".bak", lbLocation, 0)
			}()

			// need workdir set up for this
			if err := stateMachine.makeTemporaryDirectories(); err != nil {
				t.Errorf("Did not expect an error, got %s", err.Error())
			}

			// also need unpack set up
			if err := os.Mkdir(stateMachine.tempDirs.unpack, 0755); err != nil {
				t.Error("Failed to create unpack directory")
			}
			if err := stateMachine.runLiveBuild(); err == nil {
				t.Error("Expected an error but there was none")
			}
			os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
		})
	}
}

// TestNoStatic tests that the helper function to prepare lb commands
// returns an error if the qemu-static binary is missing. This is accomplished
// by passing an architecture for which there is no qemu-static binary
func TestNoStatic(t *testing.T) {
	t.Run("test_no_qemu_static", func(t *testing.T) {
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.Opts.Project = "ubuntu-cpc"
		stateMachine.Opts.Arch = "fakearch"
		stateMachine.Args.GadgetTree = filepath.Join("testdata", "gadget_tree")
		stateMachine.parent = &stateMachine

		// need workdir set up for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		// also need unpack set up
		if err := os.Mkdir(stateMachine.tempDirs.unpack, 0755); err != nil {
			t.Error("Failed to create unpack directory")
		}
		if err := stateMachine.runLiveBuild(); err == nil {
			t.Error("Expected an error but there was none")
		}
		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestPopulateClassicRootfsContents runs the state machine through populate_rootfs_contents and examines
// the rootfs to ensure at least some of the correct file are in place
func TestPopulateClassicRootfsContents(t *testing.T) {
	t.Run("test_populate_classic_rootfs_contents", func(t *testing.T) {
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.Opts.Project = "ubuntu-cpc"
		stateMachine.Opts.Suite = "focal"
		stateMachine.Args.GadgetTree = filepath.Join("testdata", "gadget_tree")
		stateMachine.stateMachineFlags.Thru = "populate_rootfs_contents"

		if err := stateMachine.Setup(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}

		if err := stateMachine.Run(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}

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

		if err := stateMachine.Teardown(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}
	})
}

// TestFailedPopulateClassicRootfsContents tests failed scenarios in populateClassicRootfsContents
// this is accomplished by mocking functions
func TestFailedPopulateClassicRootfsContents(t *testing.T) {
	t.Run("test_failed_populate_classic_rootfs_contents", func(t *testing.T) {
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.Opts.Filesystem = filepath.Join("testdata", "filesystem")
		stateMachine.commonFlags.CloudInit = filepath.Join("testdata", "user-data")

		// need workdir set up for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		// mock ioutil.ReadDir
		ioutilReadDir = mockReadDir
		defer func() {
			ioutilReadDir = ioutil.ReadDir
		}()
		if err := stateMachine.populateClassicRootfsContents(); err == nil {
			t.Error("Expected an error, but got none")
		}
		ioutilReadDir = ioutil.ReadDir

		// mock osutil.CopySpecialFile
		osutilCopySpecialFile = mockCopySpecialFile
		defer func() {
			osutilCopySpecialFile = osutil.CopySpecialFile
		}()
		if err := stateMachine.populateClassicRootfsContents(); err == nil {
			t.Error("Expected an error, but got none")
		}
		osutilCopySpecialFile = osutil.CopySpecialFile

		// mock ioutil.ReadFile
		ioutilReadFile = mockReadFile
		defer func() {
			ioutilReadFile = ioutil.ReadFile
		}()
		if err := stateMachine.populateClassicRootfsContents(); err == nil {
			t.Error("Expected an error, but got none")
		}
		ioutilReadFile = ioutil.ReadFile

		// mock ioutil.WriteFile
		ioutilWriteFile = mockWriteFile
		defer func() {
			ioutilWriteFile = ioutil.WriteFile
		}()
		if err := stateMachine.populateClassicRootfsContents(); err == nil {
			t.Error("Expected an error, but got none")
		}
		ioutilWriteFile = ioutil.WriteFile

		// mock os.MkdirAll
		osMkdirAll = mockMkdirAll
		defer func() {
			osMkdirAll = os.MkdirAll
		}()
		if err := stateMachine.populateClassicRootfsContents(); err == nil {
			t.Error("Expected an error, but got none")
		}
		osMkdirAll = os.MkdirAll

		// mock os.OpenFile
		osOpenFile = mockOpenFile
		defer func() {
			osOpenFile = os.OpenFile
		}()
		if err := stateMachine.populateClassicRootfsContents(); err == nil {
			t.Error("Expected an error, but got none")
		}
		osOpenFile = os.OpenFile

		// mock osutil.CopyFile
		osutilCopyFile = mockCopyFile
		defer func() {
			osutilCopyFile = osutil.CopyFile
		}()
		if err := stateMachine.populateClassicRootfsContents(); err == nil {
			t.Error("Expected an error, but got none")
		}
		osutilCopyFile = osutil.CopyFile
	})
}

// TestFilesystemFlag makes sure that with the --filesystem flag the specified filesystem is copied
// to the rootfs directory
func TestFilesystemFlag(t *testing.T) {
	t.Run("test_filesystem_flag", func(t *testing.T) {
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.Opts.Filesystem = filepath.Join("testdata", "filesystem")

		// need workdir set up for this
		if err := stateMachine.makeTemporaryDirectories(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		if err := stateMachine.populateClassicRootfsContents(); err != nil {
			t.Errorf("Did not expect an error, got %s", err.Error())
		}

		// check that the specified filesystem was copied over
		if _, err := os.Stat(filepath.Join(stateMachine.tempDirs.rootfs, "testfile")); err != nil {
			t.Errorf("Failed to copy --filesystem to rootfs")
		}
	})
}
