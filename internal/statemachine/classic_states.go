package statemachine

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/snapcore/snapd/image"
	"github.com/snapcore/snapd/osutil"
	"github.com/snapcore/snapd/snap"
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
	classicStateMachine.YamlFilePath = filepath.Join(gadgetDir, "meta", "gadget.yaml")

	return nil
}

// runLiveBuild runs `lb config` and `lb build` commands based on the user input
func (stateMachine *StateMachine) runLiveBuild() error {
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)
	// TODO: Move preseeding logic from livecd-rootfs to ubuntu-image
	// for all builds
	if classicStateMachine.Opts.Filesystem == "" {
		// --filesystem was not provided, so we use live-build to create one
		var env []string
		var arch string
		env = append(env, "PROJECT="+classicStateMachine.Opts.Project)
		if classicStateMachine.Opts.Suite != "" {
			env = append(env, "SUITE="+classicStateMachine.Opts.Suite)
		} else {
			env = append(env, "SUITE="+getHostSuite())
		}
		if classicStateMachine.Opts.Arch == "" {
			arch = getHostArch()
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

		lbConfig, lbBuild, err := setupLiveBuildCommands(classicStateMachine.tempDirs.unpack,
			arch, env, true)
		if err != nil {
			return fmt.Errorf("error setting up live_build: %s", err.Error())
		}

		// now run the "lb config" and "lb build" commands
		saveCWD := helper.SaveCWD()
		defer saveCWD()
		os.Chdir(stateMachine.tempDirs.unpack)

		if err := lbConfig.Run(); err != nil {
			return fmt.Errorf("Error running command \"%s\": %s", lbConfig.String(), err.Error())
		}

		// add extra snaps to config/seeded-snaps
		os.MkdirAll(filepath.Join(stateMachine.tempDirs.unpack, "config"), 0755)
		seededSnaps, err := osOpenFile(filepath.Join(stateMachine.tempDirs.unpack,
			"config", "seeded-snaps"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0755)
		if err != nil {
			return fmt.Errorf("Error opening seeded-snaps: %s", err.Error())
		}
		defer seededSnaps.Close()

		for _, snap := range stateMachine.commonFlags.Snaps {
			if !strings.Contains(snap, "=") {
				snap += "=" + classicStateMachine.commonFlags.Channel
			}
			_, err := seededSnaps.WriteString(snap + "\n")
			if err != nil {
				return fmt.Errorf("Error writing snap %s to seeded-snaps: %s", snap, err.Error())
			}
		}

		if err := lbBuild.Run(); err != nil {
			return fmt.Errorf("Error running command \"%s\": %s", lbBuild.String(), err.Error())
		}
	}

	return nil
}

// prepareClassicImage calls image.Prepare to seed extra snaps in classic images
// currently only used when --filesystem is provided
func (stateMachine *StateMachine) prepareClassicImage() error {
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)

	if classicStateMachine.Opts.Filesystem != "" &&
		len(classicStateMachine.commonFlags.Snaps) > 0 {
		var imageOpts image.Options

		var err error
		imageOpts.Snaps, imageOpts.SnapChannels, err = parseSnapsAndChannels(
			classicStateMachine.commonFlags.Snaps)
		if err != nil {
			return err
		}

		// If the rootfs has already been pre-seeded, we need to delete the
		// pre-seeded snaps and redo the preseed with all of the snaps
		stateFile := filepath.Join(classicStateMachine.tempDirs.rootfs,
			"var", "lib", "snapd", "seed", "seed.yaml")
		if _, err := os.Stat(stateFile); err == nil {
			// check for an existing model assertion file, otherwise snapd will use
			// a generic model assertion
			modelFile := filepath.Join(classicStateMachine.tempDirs.rootfs,
				"var", "lib", "snapd", "seed", "assertions", "model")
			if _, err := os.Stat(modelFile); err == nil {
				// create a copy of the model file because it will be deleted soon
				newModelFile := filepath.Join(classicStateMachine.stateMachineFlags.WorkDir,
					"model")
				if err := osutilCopyFile(modelFile, newModelFile, 0); err != nil {
					return fmt.Errorf("Error copying modelFile from preseeded filesystem: %s",
						err.Error())
				}
				imageOpts.ModelFile = newModelFile
			}

			// Now remove all of the seeded snaps
			preseededSnaps, err := removePreseeding(
				classicStateMachine.tempDirs.rootfs)
			if err != nil {
				return fmt.Errorf("Error removing preseeded snaps from existing rootfs: %s",
					err.Error())
			}
			for snap, channel := range preseededSnaps {
				// if a channel is specified on the command line for a snap that was already
				// preseeded, use the channel from the command line instead of the channel
				// that was originally used for the preseeding
				if _, found := imageOpts.SnapChannels[snap]; !found {
					imageOpts.Snaps = append(imageOpts.Snaps, snap)
					imageOpts.SnapChannels[snap] = channel
				}
			}
		}

		imageOpts.Classic = true
		imageOpts.Architecture = classicStateMachine.Opts.Arch
		if imageOpts.Architecture == "" {
			imageOpts.Architecture = getHostArch()
		}

		imageOpts.PrepareDir = classicStateMachine.tempDirs.rootfs

		customizations := *new(image.Customizations)
		imageOpts.Customizations = customizations

		// plug/slot sanitization not used by snap image.Prepare, make it no-op.
		snap.SanitizePlugsSlots = func(snapInfo *snap.Info) {}

		if err := imagePrepare(&imageOpts); err != nil {
			return fmt.Errorf("Error preparing image: %s", err.Error())
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
		return fmt.Errorf("Error reading unpack/chroot dir: %s", err.Error())
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
		re := regexp.MustCompile(`(?m:^LABEL=\S+\s+/\s+(.*)$)`)
		newContents := re.ReplaceAll(fstabBytes, []byte("LABEL=writable\t/\t$1"))
		if !strings.Contains(string(newContents), "LABEL=writable") {
			newContents = []byte("LABEL=writable   /    ext4   defaults    0 0")
		}
		err := ioutilWriteFile(fstabPath, newContents, 0644)
		if err != nil {
			return fmt.Errorf("Error writing to fstab: %s", err.Error())
		}
	}

	if classicStateMachine.commonFlags.CloudInit != "" {
		seedDir := filepath.Join(classicStateMachine.tempDirs.rootfs, "var", "lib", "cloud", "seed")
		cloudDir := filepath.Join(seedDir, "nocloud-net")
		err := osMkdirAll(cloudDir, 0756)
		if err != nil && !os.IsExist(err) {
			return fmt.Errorf("Error creating cloud-init dir: %s", err.Error())
		}
		metadataFile := filepath.Join(cloudDir, "meta-data")
		metadataIO, err := osOpenFile(metadataFile, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("Error opening cloud-init meta-data file: %s", err.Error())
		}
		metadataIO.Write([]byte("instance-id: nocloud-static"))
		metadataIO.Close()

		userdataFile := filepath.Join(cloudDir, "user-data")
		err = osutilCopyFile(classicStateMachine.commonFlags.CloudInit,
			userdataFile, osutil.CopyFlagDefault)
		if err != nil {
			return fmt.Errorf("Error copying cloud-init: %s", err.Error())
		}
	}
	return nil
}

// Generate the manifest
func (stateMachine *StateMachine) generatePackageManifest() error {
	// This is basically just a wrapper around dpkg-query

	outputPath := filepath.Join(stateMachine.commonFlags.OutputDir, "filesystem.manifest")
	cmd := execCommand("sudo", "chroot", stateMachine.tempDirs.rootfs, "dpkg-query", "-W", "--showformat=${Package} ${Version}\n")
	manifest, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("Error creating manifest file: %s", err.Error())
	}
	defer manifest.Close()

	cmd.Stdout = manifest
	err = cmd.Run()
	return err
}
