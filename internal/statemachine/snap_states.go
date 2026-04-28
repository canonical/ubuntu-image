package statemachine

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/snapcore/snapd/asserts"
	"github.com/snapcore/snapd/image"
	"github.com/snapcore/snapd/interfaces/builtin"
	"github.com/snapcore/snapd/seed/seedwriter"
	"github.com/snapcore/snapd/snap"
)

var prepareImageState = stateFunc{"prepare_image", (*StateMachine).prepareImage}

// Prepare the image
func (stateMachine *StateMachine) prepareImage() error {
	snapStateMachine := stateMachine.parent.(*SnapStateMachine)

	imageOpts := &image.Options{
		ModelFile:                 snapStateMachine.Args.ModelAssertion,
		Preseed:                   snapStateMachine.Opts.Preseed,
		PreseedSignKey:            snapStateMachine.Opts.PreseedSignKey,
		AppArmorKernelFeaturesDir: snapStateMachine.Opts.AppArmorKernelFeaturesDir,
		SysfsOverlay:              snapStateMachine.Opts.SysfsOverlay,
		SeedManifestPath:          filepath.Join(stateMachine.commonFlags.OutputDir, "seed.manifest"),
		PrepareDir:                snapStateMachine.tempDirs.unpack,
		Channel:                   snapStateMachine.commonFlags.Channel,
		Customizations:            snapStateMachine.imageOptsCustomizations(),
		Components:                snapStateMachine.Opts.Components,
		ExtraAssertionsFiles:      snapStateMachine.Opts.ExtraAssertionFilenames,
		AllowSnapdKernelMismatch:  snapStateMachine.Opts.AllowSnapdKernelMismatch,
	}

	var err error
	imageOpts.Snaps, imageOpts.SnapChannels, err = parseSnapsAndChannels(
		snapStateMachine.Opts.Snaps)
	if err != nil {
		return err
	}

	imageOpts.SeedManifest, err = snapStateMachine.imageOptsSeedManifest()
	if err != nil {
		return fmt.Errorf("Error preparing image: %s", err.Error())
	}

	// plug/slot sanitization needed by provider handling
	snap.SanitizePlugsSlots = builtin.SanitizePlugsSlots

	// image.Prepare automatically has some output that we only want for
	// verbose or greater logging
	if !stateMachine.commonFlags.Debug && !stateMachine.commonFlags.Verbose {
		oldImageStdout := image.Stdout
		image.Stdout = io.Discard
		defer func() {
			image.Stdout = oldImageStdout
		}()
	}

	if err := imagePrepare(imageOpts); err != nil {
		return fmt.Errorf("Error preparing image: %s", err.Error())
	}

	snapStateMachine.YamlFilePath = filepath.Join(stateMachine.tempDirs.unpack, "gadget", gadgetYamlPathInTree)

	return nil
}

// imageOptsSeedManifest sets up the pre-provided manifest when snap revision overrides or validation-set sequence overrides are passed
func (snapStateMachine *SnapStateMachine) imageOptsSeedManifest() (*seedwriter.Manifest, error) {
	if len(snapStateMachine.Opts.Revisions) == 0 && len(snapStateMachine.Opts.Sequences) == 0 {
		return nil, nil
	}

	var modelValidationSets []*asserts.ModelValidationSet
	if len(snapStateMachine.Opts.Sequences) > 0 {
		model, err := snapStateMachine.decodeModelAssertion()
		if err != nil {
			return nil, err
		}

		modelValidationSets = model.ValidationSets()
	}

	seedManifest := seedwriter.NewManifest()
	for snapName, snapRev := range snapStateMachine.Opts.Revisions {
		fmt.Printf("WARNING: revision %d for snap %s may not be the latest available version!\n", snapRev, snapName)
		err := seedManifest.SetAllowedSnapRevision(snapName, snap.R(snapRev))
		if err != nil {
			return nil, fmt.Errorf("error dealing with snap revision %s: %w", snapName, err)
		}
	}

	for validationSetName, sequence := range snapStateMachine.Opts.Sequences {
		validationSet, err := resolveValidationSetSequence(validationSetName, modelValidationSets)
		if err != nil {
			return nil, fmt.Errorf("error dealing with validation-set sequence %s: %w", validationSetName, err)
		}

		fmt.Printf("WARNING: sequence %d for validation-set %s may not be the latest available sequence!\n", sequence, validationSetName)
		err = seedManifest.SetAllowedValidationSet(validationSet.AccountID, validationSet.Name, sequence, false)
		if err != nil {
			return nil, fmt.Errorf("error dealing with validation-set sequence %s: %w", validationSetName, err)
		}
	}

	return seedManifest, nil
}

func resolveValidationSetSequence(validationSetRef string, modelValidationSets []*asserts.ModelValidationSet) (*asserts.ModelValidationSet, error) {
	accountID, validationSetName, hasAccountID := strings.Cut(validationSetRef, "/")
	if !hasAccountID {
		var match *asserts.ModelValidationSet
		for _, validationSet := range modelValidationSets {
			if validationSet.Name != validationSetRef {
				continue
			}
			if match != nil {
				return nil, fmt.Errorf("validation-set name is ambiguous in model assertion")
			}
			match = validationSet
		}
		if match == nil {
			return nil, fmt.Errorf("validation-set not present in model assertion")
		}
		return match, nil
	}

	for _, validationSet := range modelValidationSets {
		if validationSet.AccountID == accountID && validationSet.Name == validationSetName {
			return validationSet, nil
		}
	}

	return nil, fmt.Errorf("validation-set not present in model assertion")
}

// imageOptsCustomizations prepares the Customizations options to give to image.Prepare
func (snapStateMachine *SnapStateMachine) imageOptsCustomizations() image.Customizations {
	customizations := image.Customizations{
		CloudInitUserData: snapStateMachine.Opts.CloudInit,
		Validation:        snapStateMachine.commonFlags.Validation,
	}
	if snapStateMachine.Opts.DisableConsoleConf {
		customizations.ConsoleConf = "disabled"
	}
	if snapStateMachine.Opts.FactoryImage {
		customizations.BootFlags = append(customizations.BootFlags, "factory")
	}

	return customizations
}

var populateSnapRootfsContentsState = stateFunc{"populate_rootfs_contents", (*StateMachine).populateSnapRootfsContents}

// populateSnapRootfsContents populates the rootfs
func (stateMachine *StateMachine) populateSnapRootfsContents() error {
	var src, dst string
	if stateMachine.IsSeeded {
		// For now, since we only create the system-seed partition for
		// uc20+ images, we hard-code to use this path for the rootfs
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

var generateSnapManifestState = stateFunc{"generate_snap_manifest", (*StateMachine).generateSnapManifest}

// Generate the manifest
func (stateMachine *StateMachine) generateSnapManifest() error {
	// We could use snapd's seed.Open() to generate the manifest here, but
	// actually it doesn't make things much easier than doing it manually -
	// like we did in the past. So let's just go with this.

	outputPath := filepath.Join(stateMachine.commonFlags.OutputDir, "snaps.manifest")
	snapsDir := filepath.Join(stateMachine.tempDirs.rootfs, "system-data", "var", "lib", "snapd", "snaps")
	return WriteSnapManifest(snapsDir, outputPath)
}
