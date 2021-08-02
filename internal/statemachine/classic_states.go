package statemachine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/canonical/ubuntu-image/internal/helper"
)

// Prepare the gadget tree
func (stateMachine *StateMachine) prepareGadgetTree() error {
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)
	gadgetDir := filepath.Join(classicStateMachine.tempDirs.unpack, "gadget")
	err := osMkdirAll(gadgetDir, 0755)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("Error creating unpack directory: %s", err.Error())
	}
	// recursively copy the gadget tree to unpack/gadget
	files, err := ioutilReadDir(classicStateMachine.Args.GadgetTree)
	if err != nil {
		return fmt.Errorf("Error reading gadget tree: %s", err.Error())
	}
	for _, gadgetFile := range files {
		srcFile := filepath.Join(classicStateMachine.Args.GadgetTree, gadgetFile.Name())
		if err := osutilCopySpecialFile(srcFile, gadgetDir); err != nil {
			return fmt.Errorf("Error copying gadget tree: %s", err.Error())
		}
	}

	// We assume the gadget tree was built from a gadget source tree using
	// snapcraft prime so the gadget.yaml file is expected in the meta directory
	classicStateMachine.yamlFilePath = filepath.Join(gadgetDir, "meta", "gadget.yaml")

	return nil
}

// runLiveBuild runs `lb config` and `lb build` commands based on the user input
func (stateMachine *StateMachine) runLiveBuild() error {
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

		lbConfig, lbBuild, err := helper.SetupLiveBuildCommands(classicStateMachine.tempDirs.rootfs,
			arch, env, true)
		if err != nil {
			return fmt.Errorf("error setting up live_build: %s", err.Error())
		}

		// now run the "lb config" and "lb build" commands
		saveCWD := helper.SaveCWD()
		defer saveCWD()
		os.Chdir(stateMachine.tempDirs.rootfs)

		if err := lbConfig.Run(); err != nil {
			return err
		}

		if err := lbBuild.Run(); err != nil {
			return err
		}
	}

	return nil
}

// populateClassicRootfsContents takes the results of `lb` commands and copies them over
// to rootfs. It also changes fstab and handles the --cloud-init flag
func (stateMachine *StateMachine) populateClassicRootfsContents() error {
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)

	var src string
	if classicStateMachine.Opts.Filesystem != "" {
		src = classicStateMachine.Opts.Filesystem
	} else {
		src = filepath.Join(classicStateMachine.tempDirs.unpack, "chroot")
	}

	files, err := ioutilReadDir(src)
	if err != nil {
		return fmt.Errorf("Error reading unpack dir: %s", err.Error())
	}

	for _, srcFile := range files {
		srcFile := filepath.Join(src, srcFile.Name())
		if err := osutilCopySpecialFile(srcFile, classicStateMachine.tempDirs.rootfs); err != nil {
			return fmt.Errorf("Error copying rootfs: %s", err.Error())
		}
	}

	fstabPath := filepath.Join(classicStateMachine.tempDirs.rootfs, "etc", "fstab")
	fstabBytes, err := ioutilReadFile(fstabPath)
	if err != nil {
		return fmt.Errorf("Error opening fstab: %s", err.Error())
	}

	if !strings.Contains(string(fstabBytes), "LABEL=writable") {
		newFstab := []byte("LABEL=writable   /    ext4   defaults    0 0")
		err := ioutilWriteFile(fstabPath, newFstab, 0644)
		if err != nil {
			return fmt.Errorf("Error writing to fstab: %s", err.Error())
		}
	}

	if err := stateMachine.processCloudInit(); err != nil {
		return err
	}
	return nil
}
