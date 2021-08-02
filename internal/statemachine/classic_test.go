// This test file tests a successful classic run and success/error scenarios for all states
// that are specific to the classic builds
package statemachine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/canonical/ubuntu-image/internal/helper"
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

		if err := stateMachine.Setup(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}

		if err := stateMachine.Run(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}

		if err := stateMachine.Teardown(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}
	})
}

// TestFailedPrepareGadgetTree tests failures in os, osutil, and ioutil libraries
func TestFailedPrepareGadgetTree(t *testing.T) {
	t.Run("test_failed_prepare_gadget_tree", func(t *testing.T) {
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine

		failFuncs := []interface{}{osMkdirAll, ioutilReadDir, osutilCopySpecialFile}
		for _, failFunc := range failFuncs {
			// replace the function with a new function that just returns an error
			origFunc := failFunc
			failFunc = func() error { return fmt.Errorf("Test Error") }

			if err := stateMachine.prepareGadgetTree(); err == nil {
				t.Error("Expected an error, but got none")
			}
			// restore the function
			failFunc = origFunc
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

		if err := stateMachine.Teardown(); err != nil {
			t.Errorf("Did not expect an error, got %s\n", err.Error())
		}
	})
}

// TestFailedRunLiveBuild tests the scenario where calls to live build fail.
// this is accomplished by passing invalid arguments to live-build
func TestFailedRunLiveBuild(t *testing.T) {
	t.Run("test_successful_classic_run", func(t *testing.T) {
		saveCWD := helper.SaveCWD()
		defer saveCWD()

		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine
		stateMachine.Opts.Project = "ubuntu-cpc"
		stateMachine.Opts.Suite = "fakesuite"
		stateMachine.Opts.Arch = "fake"
		stateMachine.Opts.Subproject = "fakeproject"
		stateMachine.Opts.Subarch = "fakearch"
		stateMachine.Opts.WithProposed = true
		stateMachine.Opts.ExtraPPAs = []string{"ppa:fake_user/fakeppa"}
		stateMachine.Args.GadgetTree = filepath.Join("testdata", "gadget_tree")

		if err := stateMachine.runLiveBuild(); err == nil {
			t.Error("Expected an error but there was none")
		}

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestFailedPopulateClassicRootfsContents tests failures in the PopulateRootfsContents state
func TestFailedPopulateClassicRootfsContents(t *testing.T) {
	t.Run("test_failed_populate_classic_rootfs_contents", func(t *testing.T) {
		var stateMachine ClassicStateMachine
		stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
		stateMachine.parent = &stateMachine

		failFuncs := []interface{}{ioutilReadDir, osutilCopySpecialFile, ioutilReadFile, regexpCompile}
		for _, failFunc := range failFuncs {
			// replace the function with a new function that just returns an error
			origFunc := failFunc
			failFunc = func() error { return fmt.Errorf("Test Error") }

			if err := stateMachine.prepareGadgetTree(); err == nil {
				t.Error("Expected an error, but got none")
			}
			// restore the function
			failFunc = origFunc
		}

		os.RemoveAll(stateMachine.stateMachineFlags.WorkDir)
	})
}

// TestPopulateClassicRootfsContents runs the state machine through populate_rootfs_contents and examines
// the rootfs to ensure at least some of the correct file are in place
func TestPopulateClassicRootfsContents(t *testing.T) {
	testCases := []struct {
		name     string
		suite    string
		fileList []string
	}{
		{"focal", "focal", []string{filepath.Join("etc", "shadow"), filepath.Join("boot", "vmlinuz"), filepath.Join("var", "lib")}},
		{"impish", "impish", []string{filepath.Join("etc", "systemd"), filepath.Join("boot", "grub"), filepath.Join("usr", "lib")}},
	}
	for _, tc := range testCases {
		t.Run("test "+tc.name, func(t *testing.T) {
			saveCWD := helper.SaveCWD()
			defer saveCWD()

			var stateMachine ClassicStateMachine
			stateMachine.commonFlags, stateMachine.stateMachineFlags = helper.InitCommonOpts()
			stateMachine.parent = &stateMachine
			stateMachine.Opts.Project = "ubuntu-cpc"
			stateMachine.Opts.Suite = tc.suite
			stateMachine.Args.GadgetTree = filepath.Join("testdata", "gadget_tree")
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
}
