package statemachine

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/invopop/jsonschema"
	"github.com/snapcore/snapd/image"
	"github.com/snapcore/snapd/snap"
	"github.com/snapcore/snapd/store"
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

	// do custom validation for gadgetURL being required if gadget is not pre-built
	if imageDefinition.Gadget.GadgetType != "prebuilt" && imageDefinition.Gadget.GadgetURL == "" {
		jsonContext := gojsonschema.NewJsonContext("gadget_validation", nil)
		errDetail := gojsonschema.ErrorDetails{
			"key":   "gadget:type",
			"value": imageDefinition.Gadget.GadgetType,
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

	if imageDefinition.Customization != nil {
		// do custom validation for private PPAs requiring fingerprint
		for _, ppa := range imageDefinition.Customization.ExtraPPAs {
			if ppa.Auth != "" && ppa.Fingerprint == "" {
				jsonContext := gojsonschema.NewJsonContext("ppa_validation", nil)
				errDetail := gojsonschema.ErrorDetails{
					"ppaName": ppa.PPAName,
				}
				result.AddError(
					newInvalidPPAError(
						gojsonschema.NewJsonContext("missingPrivatePPAFingerprint",
							jsonContext),
						52,
						errDetail,
					),
					errDetail,
				)
			}
		}
		// do custom validation for manual customization paths
		if imageDefinition.Customization.Manual != nil {
			jsonContext := gojsonschema.NewJsonContext("manual_path_validation", nil)
			if imageDefinition.Customization.Manual.CopyFile != nil {
				for _, copy := range imageDefinition.Customization.Manual.CopyFile {
					// XXX: filepath.IsAbs() does returns true for paths like /../../something
					// and those are NOT absolute paths.
					if !filepath.IsAbs(copy.Dest) || strings.Contains(copy.Dest, "/../") {
						errDetail := gojsonschema.ErrorDetails{
							"key":   "customization:manual:copy-file:destination",
							"value": copy.Dest,
						}
						result.AddError(
							newPathNotAbsoluteError(
								gojsonschema.NewJsonContext("nonAbsoluteManualPath",
									jsonContext),
								52,
								errDetail,
							),
							errDetail,
						)
					}
				}
			}
			if imageDefinition.Customization.Manual.TouchFile != nil {
				for _, touch := range imageDefinition.Customization.Manual.TouchFile {
					// XXX: filepath.IsAbs() does returns true for paths like /../../something
					// and those are NOT absolute paths.
					if !filepath.IsAbs(touch.TouchPath) || strings.Contains(touch.TouchPath, "/../") {
						errDetail := gojsonschema.ErrorDetails{
							"key":   "customization:manual:touch-file:path",
							"value": touch.TouchPath,
						}
						result.AddError(
							newPathNotAbsoluteError(
								gojsonschema.NewJsonContext("nonAbsoluteManualPath",
									jsonContext),
								52,
								errDetail,
							),
							errDetail,
						)
					}
				}
			}
		}
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

	var rootfsCreationStates []stateFunc

	// determine the states needed for preparing the gadget
	switch classicStateMachine.ImageDef.Gadget.GadgetType {
	case "git":
		fallthrough
	case "directory":
		rootfsCreationStates = append(rootfsCreationStates,
			stateFunc{"build_gadget_tree", (*StateMachine).buildGadgetTree})
		fallthrough
	case "prebuilt":
		rootfsCreationStates = append(rootfsCreationStates,
			stateFunc{"prepare_gadget_tree", (*StateMachine).prepareGadgetTree})
		break
	}

	// Load the gadget yaml after the gadget is built
	rootfsCreationStates = append(rootfsCreationStates,
		stateFunc{"load_gadget_yaml", (*StateMachine).loadGadgetYaml})

	// determine the states needed for preparing the rootfs.
	// The rootfs is either created from a seed, from
	// archive-tasks or as a prebuilt tarball. These
	// options are mutually exclusive and have been validated
	// by the schema already
	if classicStateMachine.ImageDef.Rootfs.Tarball != nil {
		rootfsCreationStates = append(rootfsCreationStates,
			stateFunc{"extract_rootfs_tar", (*StateMachine).extractRootfsTar})
	} else if classicStateMachine.ImageDef.Rootfs.Seed != nil {
		rootfsCreationStates = append(rootfsCreationStates, rootfsSeedStates...)
		if classicStateMachine.ImageDef.Customization != nil {
			if len(classicStateMachine.ImageDef.Customization.ExtraPPAs) > 0 {
				rootfsCreationStates = append(rootfsCreationStates,
					stateFunc{"add_extra_ppas", (*StateMachine).addExtraPPAs})
			}
		}
		rootfsCreationStates = append(rootfsCreationStates,
			[]stateFunc{
				{"install_packages", (*StateMachine).installPackages},
				{"preseed_image", (*StateMachine).preseedClassicImage},
			}...,
		)
	} else {
		rootfsCreationStates = append(rootfsCreationStates,
			stateFunc{"build_rootfs_from_tasks", (*StateMachine).buildRootfsFromTasks})
	}

	// Determine any customization that needs to run before the image is created
	//TODO: installer image customization... eventually.
	if classicStateMachine.ImageDef.Customization != nil {
		if classicStateMachine.ImageDef.Customization.CloudInit != nil {
			rootfsCreationStates = append(rootfsCreationStates,
				stateFunc{"customize_cloud_init", (*StateMachine).customizeCloudInit})
		}
		if len(classicStateMachine.ImageDef.Customization.Fstab) > 0 {
			rootfsCreationStates = append(rootfsCreationStates,
				stateFunc{"customize_fstab", (*StateMachine).customizeFstab})
		}
		if classicStateMachine.ImageDef.Customization.Manual != nil {
			rootfsCreationStates = append(rootfsCreationStates,
				stateFunc{"perform_manual_customization", (*StateMachine).manualCustomization})
		}
	}

	// The rootfs is laid out in a staging area, now populate it in the correct location
	rootfsCreationStates = append(rootfsCreationStates,
		stateFunc{"populate_rootfs_contents", (*StateMachine).populateClassicRootfsContents})

	// Add the "always there" states that populate partitions, build the disk, etc.
	// This includes the no-op "finish" state to signify successful setup
	rootfsCreationStates = append(rootfsCreationStates, imageCreationStates...)

	// Append the newly calculated states to the slice of funcs in the parent struct
	stateMachine.states = append(stateMachine.states, rootfsCreationStates...)

	// if the --debug option was passed, print the calculated states
	if stateMachine.commonFlags.Debug {
		fmt.Println("\nThe calculated states are as follows:")
		for i, state := range stateMachine.states {
			fmt.Printf("[%d] %s\n", i, state.name)
		}
		fmt.Println("\n\nContinuing")
	}

	if err := stateMachine.validateUntilThru(); err != nil {
		return err
	}

	return nil
}

// Build the gadget tree
func (stateMachine *StateMachine) buildGadgetTree() error {
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)

	// make the gadget directory under scratch
	gadgetDir := filepath.Join(stateMachine.tempDirs.scratch, "gadget")

	err := osMkdir(gadgetDir, 0755)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("Error creating scratch/gadget directory: %s", err.Error())
	}

	var sourceDir string
	switch classicStateMachine.ImageDef.Gadget.GadgetType {
	case "git":
		err := cloneGitRepo(classicStateMachine.ImageDef, gadgetDir)
		if err != nil {
			return fmt.Errorf("Error cloning gadget repository: \"%s\"", err.Error())
		}
		sourceDir = gadgetDir
		break
	case "directory":
		// no need to check error here as the validity of the URL
		// has been confirmed by the schema validation
		sourceURL, _ := url.Parse(classicStateMachine.ImageDef.Gadget.GadgetURL)

		// copy the source tree to the workdir
		err := osutilCopySpecialFile(sourceURL.Path, gadgetDir)
		if err != nil {
			return fmt.Errorf("Error copying gadget source: %s", err.Error())
		}

		sourceDir = filepath.Join(gadgetDir, path.Base(sourceURL.Path))
		break
	}

	// now run "make" to build the gadget tree
	makeCmd := execCommand("make")
	makeCmd.Dir = sourceDir

	makeOutput := helper.SetCommandOutput(makeCmd, classicStateMachine.commonFlags.Debug)

	if err := makeCmd.Run(); err != nil {
		return fmt.Errorf("Error running \"make\" in gadget source. "+
			"Error is \"%s\". Full output below:\n%s",
			err.Error(), makeOutput.String())
	}

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

// Bootstrap a chroot environment to install packages in. It will eventually
// become the rootfs of the image
func (stateMachine *StateMachine) createChroot() error {
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)

	if err := osMkdir(stateMachine.tempDirs.chroot, 0755); err != nil {
		return fmt.Errorf("Failed to create chroot directory: %s", err.Error())
	}

	debootstrapCmd := generateDebootstrapCmd(classicStateMachine.ImageDef,
		stateMachine.tempDirs.chroot,
		classicStateMachine.Packages,
	)

	debootstrapOutput := helper.SetCommandOutput(debootstrapCmd, classicStateMachine.commonFlags.Debug)

	if err := debootstrapCmd.Run(); err != nil {
		return fmt.Errorf("Error running debootstrap command \"%s\". Error is \"%s\". Output is: \n%s",
			debootstrapCmd.String(), err.Error(), debootstrapOutput.String())
	}

	// add any extra apt sources to /etc/apt/sources.list
	aptSources := classicStateMachine.ImageDef.generatePocketList()

	sourcesList := filepath.Join(stateMachine.tempDirs.chroot, "etc", "apt", "sources.list")
	sourcesListFile, _ := os.OpenFile(sourcesList, os.O_APPEND|os.O_WRONLY, 0644)
	for _, aptSource := range aptSources {
		sourcesListFile.WriteString(aptSource)
	}

	return nil
}

// add PPAs to the apt sources list
func (stateMachine *StateMachine) addExtraPPAs() error {
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)

	// create /etc/apt/sources.list.d in the chroot if it doesn't already exist
	sourcesListD := filepath.Join(stateMachine.tempDirs.chroot, "etc", "apt", "sources.list.d")
	err := osMkdir(sourcesListD, 0755)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("Failed to create apt sources.list.d: %s", err.Error())
	}

	// now create the ppa sources.list files
	tmpGPGDir, err := osMkdirTemp("/tmp", "ubuntu-image-gpg")
	if err != nil {
		return fmt.Errorf("Error creating temp dir for gpg imports: %s", err.Error())
	}
	defer osRemoveAll(tmpGPGDir)
	for _, ppa := range classicStateMachine.ImageDef.Customization.ExtraPPAs {
		ppaFileName, ppaFileContents := createPPAInfo(ppa,
			classicStateMachine.ImageDef.Series)

		ppaFile := filepath.Join(sourcesListD, ppaFileName)
		ppaIO, err := osOpenFile(ppaFile, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("Error creating %s: %s", ppaFile, err.Error())
		}
		ppaIO.Write([]byte(ppaFileContents))
		ppaIO.Close()

		// Import keys either from the specified fingerprint or via the Launchpad API
		/* TODO: this is the logic for deb822 sources. When other projects
		(software-properties, ubuntu-release-upgrader) are ready, update
		to this logic instead.
		keyFileName := strings.Replace(ppaFileName, ".sources", ".gpg", 1)
		*/
		keyFileName := strings.Replace(ppaFileName, ".list", ".gpg", 1)
		keyFilePath := filepath.Join(classicStateMachine.tempDirs.chroot,
			"etc", "apt", "trusted.gpg.d", keyFileName)
		err = importPPAKeys(ppa, tmpGPGDir, keyFilePath, stateMachine.commonFlags.Debug)
		if err != nil {
			return fmt.Errorf("Error retrieving signing key for ppa \"%s\": %s",
				ppa.PPAName, err.Error())
		}
	}
	if err := osRemoveAll(tmpGPGDir); err != nil {
		return fmt.Errorf("Error removing temporary gpg directory \"%s\": %s", tmpGPGDir, err.Error())
	}

	return nil
}

// Install packages in the chroot environment. This is accomplished by
// running commands to do the following:
// 1. Mount /proc /sys and /dev in the chroot
// 2. Run `apt update` in the chroot
// 3. Run `apt install <package list>` in the chroot
// 4. Unmount /proc /sys and /dev
func (stateMachine *StateMachine) installPackages() error {
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)

	// if any extra packages are specified, install them alongside the seeded packages
	if classicStateMachine.ImageDef.Customization != nil {
		for _, packageInfo := range classicStateMachine.ImageDef.Customization.ExtraPackages {
			classicStateMachine.Packages = append(classicStateMachine.Packages,
				packageInfo.PackageName)
		}
	}

	// Slice used to store all the commands that need to be run
	// to install the packages
	var installPackagesCmds []*exec.Cmd

	// mount some necessary partitions from the host in the chroot
	mounts := []string{"/dev", "/proc", "/sys"}
	var umounts []*exec.Cmd
	for _, mount := range mounts {
		mountCmd, umountCmd := mountFromHost(stateMachine.tempDirs.chroot, mount)
		defer umountCmd.Run()
		installPackagesCmds = append(installPackagesCmds, mountCmd)
		umounts = append(umounts, umountCmd)
	}

	// generate the apt update/install commands and append them to the slice of commands
	aptCmds := generateAptCmds(stateMachine.tempDirs.chroot, classicStateMachine.Packages)
	installPackagesCmds = append(installPackagesCmds, aptCmds...)
	installPackagesCmds = append(installPackagesCmds, umounts...) // don't forget to unmount!

	for _, cmd := range installPackagesCmds {
		cmdOutput := helper.SetCommandOutput(cmd, classicStateMachine.commonFlags.Debug)
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("Error running command \"%s\". Error is \"%s\". Output is: \n%s",
				cmd.String(), err.Error(), cmdOutput.String())
		}
	}

	return nil
}

// Build a rootfs from a list of archive tasks
func (stateMachine *StateMachine) buildRootfsFromTasks() error {
	// currently a no-op pending implementation of the classic image redesign
	return nil
}

// Extract the rootfs from a tar archive
func (stateMachine *StateMachine) extractRootfsTar() error {
	// currently a no-op pending implementation of the classic image redesign
	return nil
}

// germinate runs the germinate binary and parses the output to create
// a list of packages from the seed section of the image definition
func (stateMachine *StateMachine) germinate() error {
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)

	// create a scratch directory to run germinate in
	germinateDir := filepath.Join(classicStateMachine.stateMachineFlags.WorkDir, "germinate")
	err := osMkdir(germinateDir, 0755)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("Error creating germinate directory: \"%s\"", err.Error())
	}

	germinateCmd := generateGerminateCmd(classicStateMachine.ImageDef)
	germinateCmd.Dir = germinateDir

	germinateOutput := helper.SetCommandOutput(germinateCmd, classicStateMachine.commonFlags.Debug)

	if err := germinateCmd.Run(); err != nil {
		return fmt.Errorf("Error running germinate command \"%s\". Error is \"%s\". Output is: \n%s",
			germinateCmd.String(), err.Error(), germinateOutput.String())
	}

	packageMap := make(map[string]*[]string)
	packageMap[".seed"] = &classicStateMachine.Packages
	packageMap[".snaps"] = &classicStateMachine.Snaps
	for fileExtension, packageList := range packageMap {
		for _, fileName := range classicStateMachine.ImageDef.Rootfs.Seed.Names {
			seedFilePath := filepath.Join(germinateDir, fileName+fileExtension)
			seedFile, err := osOpen(seedFilePath)
			if err != nil {
				return fmt.Errorf("Error opening seed file %s: \"%s\"", seedFilePath, err.Error())
			}
			defer seedFile.Close()

			seedScanner := bufio.NewScanner(seedFile)
			for seedScanner.Scan() {
				seedLine := seedScanner.Bytes()
				matched, _ := regexp.Match(`^[a-z0-9].*`, seedLine)
				if matched {
					packageName := strings.Split(string(seedLine), " ")[0]
					*packageList = append(*packageList, packageName)
				}
			}
		}
	}

	return nil
}

// Customize Cloud init with the values in the image definition YAML
func (stateMachine *StateMachine) customizeCloudInit() error {
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

// Customize /etc/fstab based on values in the image definition
func (stateMachine *StateMachine) customizeFstab() error {
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)

	// open /etc/fstab for writing
	fstabIO, err := osOpenFile(filepath.Join(stateMachine.tempDirs.chroot, "etc", "fstab"),
		os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("Error opening fstab: %s", err.Error())
	}
	defer fstabIO.Close()

	var fstabEntries []string
	for _, fstab := range classicStateMachine.ImageDef.Customization.Fstab {
		var dumpString string
		if fstab.Dump {
			dumpString = "1"
		} else {
			dumpString = "0"
		}
		fstabEntry := fmt.Sprintf("LABEL=%s\t%s\t%s\t%s\t%s\t%d",
			fstab.Label,
			fstab.Mountpoint,
			fstab.FSType,
			fstab.MountOptions,
			dumpString,
			fstab.FsckOrder,
		)
		fstabEntries = append(fstabEntries, fstabEntry)
	}
	fstabIO.Write([]byte(strings.Join(fstabEntries, "\n")))
	return nil
}

// Handle any manual customizations specified in the image definition
func (stateMachine *StateMachine) manualCustomization() error {
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)

	type customizationHandler struct {
		inputData   interface{}
		handlerFunc func(interface{}, string, bool) error
	}
	customizationHandlers := []customizationHandler{
		{
			inputData:   classicStateMachine.ImageDef.Customization.Manual.CopyFile,
			handlerFunc: manualCopyFile,
		},
		{
			inputData:   classicStateMachine.ImageDef.Customization.Manual.Execute,
			handlerFunc: manualExecute,
		},
		{
			inputData:   classicStateMachine.ImageDef.Customization.Manual.TouchFile,
			handlerFunc: manualTouchFile,
		},
		{
			inputData:   classicStateMachine.ImageDef.Customization.Manual.AddGroup,
			handlerFunc: manualAddGroup,
		},
		{
			inputData:   classicStateMachine.ImageDef.Customization.Manual.AddUser,
			handlerFunc: manualAddUser,
		},
	}

	for _, customization := range customizationHandlers {
		err := customization.handlerFunc(customization.inputData, stateMachine.tempDirs.chroot, stateMachine.commonFlags.Debug)
		if err != nil {
			return err
		}
	}

	return nil
}

// preseedClassicImage calls image.Prepare to seed snaps in classic images
func (stateMachine *StateMachine) preseedClassicImage() error {
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)

	var imageOpts image.Options

	var err error
	imageOpts.Snaps, imageOpts.SnapChannels, err = parseSnapsAndChannels(classicStateMachine.Snaps)
	if err != nil {
		return err
	}

	// plug/slot sanitization not used by snap image.Prepare, make it no-op.
	snap.SanitizePlugsSlots = func(snapInfo *snap.Info) {}

	// iterate through the list of snaps and ensure that all of their bases
	// are also set to be installed. Note we only do this for snaps that are
	// seeded. Users are expected to specify all base and content provider
	// snaps in the image definition.
	for _, seededSnap := range imageOpts.Snaps {
		snapStore := store.New(nil, nil)
		snapSpec := store.SnapSpec{Name: seededSnap}
		snapContext := context.TODO() //context can be empty, just not nil
		snapInfo, err := snapStore.SnapInfo(snapContext, snapSpec, nil)
		if err != nil {
			return fmt.Errorf("Error getting info for snap %s: \"%s\"",
				seededSnap, err.Error())
		}
		if !helper.SliceHasElement(imageOpts.Snaps, snapInfo.Base) {
			imageOpts.Snaps = append(imageOpts.Snaps, snapInfo.Base)
		}
	}

	// add any extra snaps from the image definition to the list
	if classicStateMachine.ImageDef.Customization != nil {
		for _, extraSnap := range classicStateMachine.ImageDef.Customization.ExtraSnaps {
			if !helper.SliceHasElement(classicStateMachine.Snaps, extraSnap.SnapName) {
				imageOpts.Snaps = append(imageOpts.Snaps, extraSnap.SnapName)
			}
			if extraSnap.Channel != "" {
				imageOpts.SnapChannels[extraSnap.SnapName] = extraSnap.Channel
			}
		}
	}

	imageOpts.Classic = true
	imageOpts.Architecture = classicStateMachine.ImageDef.Architecture
	imageOpts.PrepareDir = classicStateMachine.tempDirs.chroot
	imageOpts.Customizations = *new(image.Customizations)
	imageOpts.Customizations.Validation = stateMachine.commonFlags.Validation

	// image.Prepare automatically has some output that we only want for
	// verbose or greater logging
	if !stateMachine.commonFlags.Debug && !stateMachine.commonFlags.Verbose {
		oldImageStdout := image.Stdout
		image.Stdout = ioutil.Discard
		defer func() {
			image.Stdout = oldImageStdout
		}()
	}

	if err := imagePrepare(&imageOpts); err != nil {
		return fmt.Errorf("Error preparing image: %s", err.Error())
	}

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
