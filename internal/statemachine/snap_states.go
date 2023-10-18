package statemachine

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/snapcore/snapd/image"
	"github.com/snapcore/snapd/seed/seedwriter"
	"github.com/snapcore/snapd/snap"
)

// Prepare the image
func (stateMachine *StateMachine) prepareImage() error {
	snapStateMachine := stateMachine.parent.(*SnapStateMachine)

	var imageOpts image.Options

	var err error
	imageOpts.Snaps, imageOpts.SnapChannels, err = parseSnapsAndChannels(
		snapStateMachine.Opts.Snaps)
	if err != nil {
		return err
	}

	imageOpts.PrepareDir = snapStateMachine.tempDirs.unpack
	imageOpts.ModelFile = snapStateMachine.Args.ModelAssertion
	if snapStateMachine.commonFlags.Channel != "" {
		imageOpts.Channel = snapStateMachine.commonFlags.Channel
	}

	// setup the pre-provided manifest if revisions are passed
	if len(snapStateMachine.Opts.Revisions) > 0 {
		imageOpts.SeedManifest = seedwriter.NewManifest()
		for snapName, snapRev := range snapStateMachine.Opts.Revisions {
			fmt.Printf("WARNING: revision %d for snap %s may not be the latest available version!\n", snapRev, snapName)
			err = imageOpts.SeedManifest.SetAllowedSnapRevision(snapName, snap.R(snapRev))
			if err != nil {
				return fmt.Errorf("Error preparing image: error dealing with snap revision %s: %w", snapName, err)
			}
		}
	}

	// preseeding-related
	imageOpts.Preseed = snapStateMachine.Opts.Preseed
	imageOpts.PreseedSignKey = snapStateMachine.Opts.PreseedSignKey
	imageOpts.AppArmorKernelFeaturesDir = snapStateMachine.Opts.AppArmorKernelFeaturesDir
	imageOpts.SeedManifestPath = filepath.Join(stateMachine.commonFlags.OutputDir, "seed.manifest")

	customizations := *new(image.Customizations)
	if snapStateMachine.Opts.DisableConsoleConf {
		customizations.ConsoleConf = "disabled"
	}
	if snapStateMachine.Opts.FactoryImage {
		customizations.BootFlags = append(customizations.BootFlags, "factory")
	}
	customizations.CloudInitUserData = snapStateMachine.Opts.CloudInit
	customizations.Validation = stateMachine.commonFlags.Validation
	imageOpts.Customizations = customizations

	// plug/slot sanitization not used by snap image.Prepare, make it no-op.
	snap.SanitizePlugsSlots = func(snapInfo *snap.Info) {}

	// image.Prepare automatically has some output that we only want for
	// verbose or greater logging
	if !stateMachine.commonFlags.Debug && !stateMachine.commonFlags.Verbose {
		oldImageStdout := image.Stdout
		image.Stdout = io.Discard
		defer func() {
			image.Stdout = oldImageStdout
		}()
	}

	if err := imagePrepare(&imageOpts); err != nil {
		return fmt.Errorf("Error preparing image: %s", err.Error())
	}

	// set the gadget yaml location
	snapStateMachine.YamlFilePath = filepath.Join(stateMachine.tempDirs.unpack, "gadget", "meta", "gadget.yaml")

	return nil
}

// populateSnapRootfsContents uses a NewMountedFileSystemWriter to populate the rootfs
func (stateMachine *StateMachine) populateSnapRootfsContents() error {
	var src, dst string
	if stateMachine.IsSeeded {
		// For now, since we only create the system-seed partition for
		// uc20 images, we hard-code to use this path for the rootfs
		// seed population.  In the future we might want to consider
		// populating other partitions from `snap prepare-image` output
		// as well, so looking into directories like system-data/ etc.
		src = filepath.Join(stateMachine.tempDirs.unpack, "system-seed")
		dst = stateMachine.tempDirs.rootfs
	} else {
		src = filepath.Join(stateMachine.tempDirs.unpack, "image")
		dst = filepath.Join(stateMachine.tempDirs.rootfs, "system-data")
		err := osMkdirAll(filepath.Join(dst, "boot"), 0755)
		if err != nil && !os.IsExist(err) {
			return fmt.Errorf("Error creating boot dir: %s", err.Error())
		}
	}

	// recursively copy the src to dst, skipping /boot for non-seeded images
	files, err := osReadDir(src)
	if err != nil {
		return fmt.Errorf("Error reading unpack dir: %s", err.Error())
	}
	for _, srcFile := range files {
		if !stateMachine.IsSeeded && srcFile.Name() == "boot" {
			continue
		}
		srcFileName := filepath.Join(src, srcFile.Name())
		dstFileName := filepath.Join(dst, srcFile.Name())
		if err := osRename(srcFileName, dstFileName); err != nil {
			return fmt.Errorf("Error moving rootfs: %s", err.Error())
		}
	}

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
	return WriteSnapManifest(snapsDir, outputPath)
}
