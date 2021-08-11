package statemachine

import (
	"fmt"
	"path/filepath"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/snapcore/snapd/image"
	"github.com/snapcore/snapd/snap"
)

// Prepare the image
func (stateMachine *StateMachine) prepareImage() error {
	var snapStateMachine *SnapStateMachine
	snapStateMachine = stateMachine.parent.(*SnapStateMachine)

	var imageOpts image.Options
	imageOpts.Snaps = snapStateMachine.Opts.Snaps
	imageOpts.PrepareDir = snapStateMachine.tempDirs.unpack
	imageOpts.ModelFile = snapStateMachine.Args.ModelAssertion
	if snapStateMachine.Opts.Channel != "" {
		imageOpts.Channel = snapStateMachine.Opts.Channel
	}

	customizations := *new(image.Customizations)
	if snapStateMachine.Opts.DisableConsoleConf {
		customizations.ConsoleConf = "disabled"
	}
	if snapStateMachine.Opts.FactoryImage {
		customizations.BootFlags = append(customizations.BootFlags, "factory")
	}
	customizations.CloudInitUserData = stateMachine.commonFlags.CloudInit
	imageOpts.Customizations = customizations

	// plug/slot sanitization not used by snap image.Prepare, make it no-op.
	snap.SanitizePlugsSlots = func(snapInfo *snap.Info) {}

	if err := image.Prepare(&imageOpts); err != nil {
		return fmt.Errorf("Error preparing image: %s", err.Error())
	}

	// set the gadget yaml location
	snapStateMachine.yamlFilePath = snapStateMachine.tempDirs.unpack + "/gadget/meta/gadget.yaml"

	return nil
}

// Generate the manifest
func (stateMachine *StateMachine) generateSnapManifest() error {
	// We could use snapd's seed.Open() to generate the manifest here, but
	// actually it doesn't make things much easier than doing it manually -
	// like we did in the past. So let's just go with this.

	// snaps.manifest
	outputPath := filepath.Join(stateMachine.commonFlags.OutputDir, "snaps.manifest")
	snapsDir := filepath.Join(stateMachine.tempDirs.rootfs, "system-data", "var", "lib", "snapd", "snaps")
	err := helper.WriteSnapManifest(snapsDir, outputPath)
	if err != nil {
		return err
	}

	// seed.manifest
	outputPath = filepath.Join(stateMachine.commonFlags.OutputDir, "seed.manifest")
	if stateMachine.isSeeded {
		snapsDir = filepath.Join(stateMachine.tempDirs.rootfs, "snaps")
	} else {
		snapsDir = filepath.Join(stateMachine.tempDirs.rootfs, "system-data", "var", "lib", "snapd", "seed", "snaps")
	}
	err = helper.WriteSnapManifest(snapsDir, outputPath)

	return err
}
