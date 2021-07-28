package statemachine

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/snapcore/snapd/osutil"
)

// Prepare the gadget tree
func (stateMachine *StateMachine) prepareGadgetTree() error {
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)
	gadgetDir := classicStateMachine.tempDirs.unpack + "/gadget"
	err := os.MkdirAll(gadgetDir, 0755)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("Error creating unpack directory: %s", err.Error())
	}
	// recursively copy the gadget tree to unpack/gadget
	files, err := ioutil.ReadDir(classicStateMachine.Args.GadgetTree)
	if err != nil {
		return fmt.Errorf("Error reading gadget tree: %s", err.Error())
	}
	for _, gadgetFile := range files {
		srcFile := classicStateMachine.Args.GadgetTree + "/" + gadgetFile.Name()
		if err := osutil.CopySpecialFile(srcFile, gadgetDir); err != nil {
			return fmt.Errorf("Error copying gadget tree: %s", err.Error())
		}
	}

	// We assume the gadget tree was built from a gadget source tree using
	// snapcraft prime so the gadget.yaml file is expected in the meta directory
	classicStateMachine.yamlFilePath = gadgetDir + "/meta/gadget.yaml"

	return nil
}

// runLiveBuild runs `lb config` and `lb build` commands based on the user input
func (stateMachine *StateMachine) runLiveBuild() error {
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)
	if classicStateMachine.Opts.Filesystem == "" {
		// --filesystem was not provided, so we use live-build to create one
		var env []string
		env = append(env, "PROJECT="+classicStateMachine.Opts.Project)
		if classicStateMachine.Opts.Suite != "" {
			env = append(env, "SUITE="+classicStateMachine.Opts.Suite)
		} else {
			env = append(env, "SUITE="+helper.GetHostSuite())
		}
		if classicStateMachine.Opts.Arch == "" {
			env = append(env, "ARCH="+helper.GetHostArch())
		} else {
			env = append(env, "ARCH="+classicStateMachine.Opts.Arch)
		}
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
		if err := helper.RunLiveBuild(classicStateMachine.tempDirs.rootfs, env, true); err != nil {
			return fmt.Errorf("error running live_build: %s", err.Error())
		}
	}

	return nil
}
