package statemachine

import (
	"fmt"
	"os"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/invopop/jsonschema"
	"github.com/xeipuuv/gojsonschema"

	"gopkg.in/yaml.v2"
)

// parseImageDefinition parses the provided yaml file and ensures it is valid
func (stateMachine *StateMachine) parseImageDefinition() error {
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)

	// Open and decode the yaml file
	var imageDefinition ImageDefinition
	imageFile, err := os.Open(classicStateMachine.Args.ImageDefinition)
	if err != nil {
		return fmt.Errorf("Error opening image definition file: %s", err.Error())
	}
	defer imageFile.Close()
	if err := yaml.NewDecoder(imageFile).Decode(&imageDefinition); err != nil {
		return err
	}

	// populate the default values for imageDefinition if they were not provided in
	// the image definition YAML file
	if err := helperSetDefaults(&imageDefinition); err != nil {
		return err
	}

	// The official standard for YAML schemas states that they are an extension of
	// JSON schema draft 4. We therefore validate the decoded YAML against a JSON
	// schema. The workflow is as follows:
	// 1. Use the jsonschema library to generate a schema from the struct definition
	// 2. Load the created schema and parsed yaml into types defined by gojsonschema
	// 3. Use the gojsonschema library to validate the parsed YAML against the schema

	var jsonReflector jsonschema.Reflector

	// 1. parse the ImageDefinition struct into a schema using the jsonschema tags
	schema := jsonReflector.Reflect(&ImageDefinition{})

	// 2. load the schema and parsed YAML data into types understood by gojsonschema
	schemaLoader := gojsonschema.NewGoLoader(schema)
	imageDefinitionLoader := gojsonschema.NewGoLoader(imageDefinition)

	// 3. validate the parsed data against the schema
	result, err := gojsonschemaValidate(schemaLoader, imageDefinitionLoader)
	if err != nil {
		return fmt.Errorf("Schema validation returned an error: %s", err.Error())
	}

	// do some custom validation
	if imageDefinition.Gadget.GadgetType == "git" && imageDefinition.Gadget.GadgetURL == "" {
		jsonContext := gojsonschema.NewJsonContext("gadget_validation", nil)
		errDetail := gojsonschema.ErrorDetails{
			"key":   "gadget:type",
			"value": "git",
		}
		result.AddError(
			newMissingURLError(
				gojsonschema.NewJsonContext("missingURL", jsonContext),
				52,
				errDetail,
			),
			errDetail,
		)
	}

	// TODO: I've created a PR upstream in xeipuuv/gojsonschema
	// https://github.com/xeipuuv/gojsonschema/pull/352
	// if it gets merged this can be removed
	err = helperCheckEmptyFields(&imageDefinition, result, schema)
	if err != nil {
		return err
	}

	if !result.Valid() {
		return fmt.Errorf("Schema validation failed: %s", result.Errors())
	}

	// Validation succeeded, so set the value in the parent struct
	classicStateMachine.ImageDef = imageDefinition

	return nil
}

func (stateMachine *StateMachine) calculateStates() error {
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)

	// using the "handled_by" struct tags in image definition, get a list of state function
	// names that need to be added to this state to handle the image definition that we have parsed
	var neededStateNames []string
	neededStateNames, err := helper.GetHandledBy(&classicStateMachine.ImageDef, neededStateNames)
	if err != nil {
		return err
	}

	// iterating through the possibleStateName slice in order ensures that
	// the order of the states is as we expect.
	for _, possibleStateName := range possibleClassicStates {
		for _, neededStateName := range neededStateNames {
			if neededStateName == possibleStateName.name {
				stateMachine.states = append(stateMachine.states, possibleStateName)
			}
		}
	}

	// Add the "always there" states that populate partitions, build the disk, etc.
	// This includes the no-op "finish" state to signify successful setup
	stateMachine.states = append(stateMachine.states, imageCreationStates...)

	// if the --print-states option was passed, print the calculated states
	if classicStateMachine.Opts.PrintStates {
		fmt.Println("The calculated states are as follows:")
		for i, state := range stateMachine.states {
			fmt.Printf("[%d] %s\n", i, state.name)
		}
		os.Exit(0)
	}

	return nil
}

// Build the gadget tree
func (stateMachine *StateMachine) buildGadgetTree() error {
	// currently a no-op pending implementation of the classic image redesign
	return nil
}

// Prepare the gadget tree
func (stateMachine *StateMachine) prepareGadgetTree() error {
	// currently a no-op pending implementation of the classic image redesign
	/*var classicStateMachine *ClassicStateMachine
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
	  classicStateMachine.YamlFilePath = filepath.Join(gadgetDir, "meta", "gadget.yaml")*/

	return nil
}

// Build a list of packages via seed germination
func (stateMachine *StateMachine) germinate() error {
	// currently a no-op pending implementation of the classic image redesign
	return nil
}

// Build a list of packages from a list of archive tasks
func (stateMachine *StateMachine) expandTasks() error {
	// currently a no-op pending implementation of the classic image redesign
	return nil
}

// Create a chroot in which to install packages
func (stateMachine *StateMachine) createChroot() error {
	// currently a no-op pending implementation of the classic image redesign
	return nil
}

// Configure extra packages from the yaml image definition
func (stateMachine *StateMachine) configureExtraPackages() error {
	// currently a no-op pending implementation of the classic image redesign
	return nil
}

// install the list of packages into the chroot
func (stateMachine *StateMachine) installPackages() error {
	// currently a no-op pending implementation of the classic image redesign
	return nil
}

// Extract the rootfs from a tar archive
func (stateMachine *StateMachine) extractRootfsTar() error {
	// currently a no-op pending implementation of the classic image redesign
	return nil
}

// Configure Extra PPAs
func (stateMachine *StateMachine) setupExtraPPAs() error {
	// currently a no-op pending implementation of the classic image redesign
	return nil
}

// Install extra packages
// TODO: this should probably happen during the rootfs build steps.
// but what about extra packages with a tarball based images...
func (stateMachine *StateMachine) installExtraPackages() error {
	// currently a no-op pending implementation of the classic image redesign
	return nil
}

// Handle any file copies specified in the image definition
func (stateMachine *StateMachine) copyCustomFiles() error {
	// currently a no-op pending implementation of the classic image redesign
	return nil
}

// Handle any executable files specified in the image definition
func (stateMachine *StateMachine) executeCustomFiles() error {
	// currently a no-op pending implementation of the classic image redesign
	return nil
}

// Handle any file touches specified in the image definition
func (stateMachine *StateMachine) touchCustomFiles() error {
	// currently a no-op pending implementation of the classic image redesign
	return nil
}

// Add any groups specified in the image definition
func (stateMachine *StateMachine) addGroups() error {
	// currently a no-op pending implementation of the classic image redesign
	return nil
}

// Add any users specified in the image definition
func (stateMachine *StateMachine) addUsers() error {
	// currently a no-op pending implementation of the classic image redesign
	return nil
}

// prepareClassicImage calls image.Prepare to seed extra snaps in classic images
// currently only used when --filesystem is provided
func (stateMachine *StateMachine) prepareClassicImage() error {
	// currently a no-op pending implementation of the classic image redesign
	/*
		var classicStateMachine *ClassicStateMachine
		classicStateMachine = stateMachine.parent.(*ClassicStateMachine)

		// TODO: Move preseeding logic from livecd-rootfs to ubuntu-image
		// for all builds
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
	*/
	return nil
}

// populateClassicRootfsContents copies over the staged rootfs
// to rootfs. It also changes fstab and handles the --cloud-init flag
func (stateMachine *StateMachine) populateClassicRootfsContents() error {
	/*	var classicStateMachine *ClassicStateMachine
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
		if err == nil {
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
		}*/

	// currently a no-op pending implementation of the classic image redesign
	return nil
}

// Generate the manifest
func (stateMachine *StateMachine) generatePackageManifest() error {
	// currently a no-op pending implementation of the classic image redesign
	/*
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
	*/
	return nil
}
