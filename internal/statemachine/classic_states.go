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
	"reflect"
	"regexp"
	"strings"

	"github.com/canonical/ubuntu-image/internal/helper"
	"github.com/canonical/ubuntu-image/internal/imagedefinition"
	"github.com/invopop/jsonschema"
	"github.com/snapcore/snapd/gadget"
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
	var imageDefinition imagedefinition.ImageDefinition
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
	schema := jsonReflector.Reflect(&imagedefinition.ImageDefinition{})

	// 2. load the schema and parsed YAML data into types understood by gojsonschema
	schemaLoader := gojsonschema.NewGoLoader(schema)
	imageDefinitionLoader := gojsonschema.NewGoLoader(imageDefinition)

	// 3. validate the parsed data against the schema
	result, err := gojsonschemaValidate(schemaLoader, imageDefinitionLoader)
	if err != nil {
		return fmt.Errorf("Schema validation returned an error: %s", err.Error())
	}

	// do custom validation for gadgetURL being required if gadget is not pre-built
	if imageDefinition.Gadget != nil {
		if imageDefinition.Gadget.GadgetType != "prebuilt" && imageDefinition.Gadget.GadgetURL == "" {
			jsonContext := gojsonschema.NewJsonContext("gadget_validation", nil)
			errDetail := gojsonschema.ErrorDetails{
				"key":   "gadget:type",
				"value": imageDefinition.Gadget.GadgetType,
			}
			result.AddError(
				imagedefinition.NewMissingURLError(
					gojsonschema.NewJsonContext("missingURL", jsonContext),
					52,
					errDetail,
				),
				errDetail,
			)
		}
	}

	// don't allow any images to be created without a gadget
	if imageDefinition.Gadget == nil {
		diskUsed, err := helperCheckTags(imageDefinition.Artifacts, "is_disk")
		if err != nil {
			return fmt.Errorf("Error checking struct tags for Artifacts: \"%s\"", err.Error())
		}
		if diskUsed != "" {
			jsonContext := gojsonschema.NewJsonContext("image_without_gadget", nil)
			errDetail := gojsonschema.ErrorDetails{
				"key1": diskUsed,
				"key2": "gadget:",
			}
			result.AddError(
				imagedefinition.NewDependentKeyError(
					gojsonschema.NewJsonContext("dependentKey", jsonContext),
					52,
					errDetail,
				),
				errDetail,
			)
		}
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
					imagedefinition.NewInvalidPPAError(
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
							imagedefinition.NewPathNotAbsoluteError(
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
							imagedefinition.NewPathNotAbsoluteError(
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

// State responsible for dynamically calculating all the remaining states
// needed to build the image, as defined by the image-definition file
// that was loaded in the previous 'state'.
// If a new possible state is added to the classic build state machine, it
// should be added here (usually basing on contents of the image definition)
func (stateMachine *StateMachine) calculateStates() error {
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)

	var rootfsCreationStates []stateFunc

	if classicStateMachine.ImageDef.Gadget != nil {
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
	}

	// if artifacts are specified, verify the correctness and store them in the struct
	diskUsed, err := helperCheckTags(classicStateMachine.ImageDef.Artifacts, "is_disk")
	if err != nil {
		return fmt.Errorf("Error checking struct tags for Artifacts: \"%s\"", err.Error())
	}
	if diskUsed != "" {
		rootfsCreationStates = append(rootfsCreationStates,
			stateFunc{"verify_artifact_names", (*StateMachine).verifyArtifactNames})
	}

	// determine the states needed for preparing the rootfs.
	// The rootfs is either created from a seed, from
	// archive-tasks or as a prebuilt tarball. These
	// options are mutually exclusive and have been validated
	// by the schema already
	if classicStateMachine.ImageDef.Rootfs.Tarball != nil {
		rootfsCreationStates = append(rootfsCreationStates,
			stateFunc{"extract_rootfs_tar", (*StateMachine).extractRootfsTar})
		// if there are extra snaps or packages to install, these will have
		// to be done as separate steps. To add one of these extra steps, add the
		// struct tag "extra_step_prebuilt_rootfs" to a field in the image definition
		// that should trigger an extra step
		if classicStateMachine.ImageDef.Customization != nil {
			extraStates := checkCustomizationSteps(classicStateMachine.ImageDef.Customization,
				"extra_step_prebuilt_rootfs",
			)
			rootfsCreationStates = append(rootfsCreationStates, extraStates...)
		}
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

	// if the --disk-info flag was used on the command line place it in the correct
	// location in the rootfs
	if stateMachine.commonFlags.DiskInfo != "" {
		rootfsCreationStates = append(rootfsCreationStates,
			stateFunc{"generate_disk_info", (*StateMachine).generateDiskInfo})
	}

	if classicStateMachine.ImageDef.Gadget != nil {
		// Add the "always there" states that populate partitions, build the disk, etc.
		// This includes the no-op "finish" state to signify successful setup
		rootfsCreationStates = append(rootfsCreationStates, imageCreationStates...)

		// only run makeDisk if there is an artifact to make
		if classicStateMachine.ImageDef.Artifacts.Img != nil {
			rootfsCreationStates = append(rootfsCreationStates,
				stateFunc{"make_disk", (*StateMachine).makeDisk},
				stateFunc{"update_bootloader", (*StateMachine).updateGrub},
			)
		}
	}

	// only run makeDisk if there is an artifact to make
	if classicStateMachine.ImageDef.Artifacts.Qcow2 != nil {
		// only run make_disk once
		found := false
		for _, stateFunc := range rootfsCreationStates {
			if stateFunc.name == "make_disk" {
				found = true
			}
		}
		if !found {
			rootfsCreationStates = append(rootfsCreationStates,
				stateFunc{"make_disk", (*StateMachine).makeDisk},
				stateFunc{"update_grub", (*StateMachine).updateGrub},
			)
		}
		rootfsCreationStates = append(rootfsCreationStates,
			stateFunc{"make_qcow2_image", (*StateMachine).makeQcow2Img})
	}

	// only run generatePackageManifest if there is a manifest in the image definition
	if classicStateMachine.ImageDef.Artifacts.Manifest != nil {
		rootfsCreationStates = append(rootfsCreationStates,
			stateFunc{"generate_manifest", (*StateMachine).generatePackageManifest})
	}

	// only run generateFilelist if there is a filelist in the image definition
	if classicStateMachine.ImageDef.Artifacts.Filelist != nil {
		rootfsCreationStates = append(rootfsCreationStates,
			stateFunc{"generate_filelist", (*StateMachine).generateFilelist})
	}

	// only run generateRootfsTarball if there is a rootfs-tarball in the image definition
	if classicStateMachine.ImageDef.Artifacts.RootfsTar != nil {
		rootfsCreationStates = append(rootfsCreationStates,
			stateFunc{"generate_rootfs_tarball", (*StateMachine).generateRootfsTarball})
	}

	// add the no-op "finish" state
	rootfsCreationStates = append(rootfsCreationStates,
		stateFunc{"finish", (*StateMachine).finish})

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
		files, err := ioutilReadDir(sourceURL.Path)
		if err != nil {
			return fmt.Errorf("Error reading gadget tree: %s", err.Error())
		}
		for _, gadgetFile := range files {
			srcFile := filepath.Join(sourceURL.Path, gadgetFile.Name())
			if err := osutilCopySpecialFile(srcFile, gadgetDir); err != nil {
				return fmt.Errorf("Error copying gadget source: %s", err.Error())
			}
		}

		sourceDir = filepath.Join(gadgetDir)
		break
	}

	// now run "make" to build the gadget tree
	makeCmd := execCommand("make")
	// add ARCH and SERIES environment variables for making the gadget tree
	makeCmd.Env = append(makeCmd.Env, []string{
		fmt.Sprintf("ARCH=%s", classicStateMachine.ImageDef.Architecture),
		fmt.Sprintf("SERIES=%s", classicStateMachine.ImageDef.Series),
	}...)
	// add the current ENV to the command
	makeCmd.Env = append(makeCmd.Env, os.Environ()...)
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
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)
	gadgetDir := filepath.Join(classicStateMachine.tempDirs.unpack, "gadget")
	err := osMkdirAll(gadgetDir, 0755)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("Error creating unpack directory: %s", err.Error())
	}
	// recursively copy the gadget tree to unpack/gadget
	var gadgetTree string
	if classicStateMachine.ImageDef.Gadget.GadgetType == "prebuilt" {
		gadgetTree = strings.TrimPrefix(classicStateMachine.ImageDef.Gadget.GadgetURL, "file://")
		if !filepath.IsAbs(gadgetTree) {
			gadgetTree, _ = filepath.Abs(gadgetTree)
		}
	} else {
		gadgetTree = filepath.Join(classicStateMachine.tempDirs.scratch, "gadget")
	}
	files, err := ioutilReadDir(gadgetTree)
	if err != nil {
		return fmt.Errorf("Error reading gadget tree: %s", err.Error())
	}
	for _, gadgetFile := range files {
		srcFile := filepath.Join(gadgetTree, gadgetFile.Name())
		if err := osutilCopySpecialFile(srcFile, gadgetDir); err != nil {
			return fmt.Errorf("Error copying gadget tree: %s", err.Error())
		}
	}

	classicStateMachine.YamlFilePath = filepath.Join(gadgetDir, "gadget.yaml")

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
	aptSources := classicStateMachine.ImageDef.GeneratePocketList()

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
// 1. Mount /proc /sys /dev and /run in the chroot
// 2. Run `apt update` in the chroot
// 3. Run `apt install <package list>` in the chroot
// 4. Unmount /proc /sys /dev and /run
func (stateMachine *StateMachine) installPackages() error {
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)

	// copy /etc/resolv.conf from the host system into the chroot
	err := helperBackupAndCopyResolvConf(classicStateMachine.tempDirs.chroot)
	if err != nil {
		return fmt.Errorf("Error setting up /etc/resolv.conf in the chroot: \"%s\"", err.Error())
	}

	// if any extra packages are specified, install them alongside the seeded packages
	if classicStateMachine.ImageDef.Customization != nil {
		for _, packageInfo := range classicStateMachine.ImageDef.Customization.ExtraPackages {
			classicStateMachine.Packages = append(classicStateMachine.Packages,
				packageInfo.PackageName)
		}
	}

	// Make sure to install the extra kernel if it is specified
	if classicStateMachine.ImageDef.Kernel != "" {
		classicStateMachine.Packages = append(classicStateMachine.Packages,
			classicStateMachine.ImageDef.Kernel)
	}

	// Slice used to store all the commands that need to be run
	// to install the packages
	var installPackagesCmds []*exec.Cmd

	// mount some necessary partitions from the host in the chroot
	type mountPoint struct {
		dest     string
		fromHost bool
	}
	mountPoints := []mountPoint{
		{
			dest:     "/dev",
			fromHost: true,
		},
		{
			dest:     "/proc",
			fromHost: true,
		},
		{
			dest:     "/sys",
			fromHost: true,
		},
		{
			dest:     "/run",
			fromHost: false,
		},
	}

	var umounts []*exec.Cmd
	for _, mount := range mountPoints {
		var mountCmd, umountCmd *exec.Cmd
		if mount.fromHost {
			mountCmd, umountCmd = mountFromHost(stateMachine.tempDirs.chroot, mount.dest)
		} else {
			var err error
			mountCmd, umountCmd, err = mountTempFS(stateMachine.tempDirs.chroot,
				stateMachine.tempDirs.scratch,
				mount.dest,
			)
			if err != nil {
				return fmt.Errorf("Error mounting temporary directory for mountpoint \"%s\": \"%s\"",
					mount.dest,
					err.Error(),
				)

			}
		}
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

// Verify artifact names have volumes listed for multi-volume gadgets and set
// the volume names in the struct
func (stateMachine *StateMachine) verifyArtifactNames() error {
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)

	stateMachine.VolumeNames = make(map[string]string)

	if len(stateMachine.GadgetInfo.Volumes) > 1 {
		// first handle .img files if they are specified
		if classicStateMachine.ImageDef.Artifacts.Img != nil {
			for _, img := range *classicStateMachine.ImageDef.Artifacts.Img {
				if img.ImgVolume == "" {
					return fmt.Errorf("Volume names must be specified for each image when using a gadget with more than one volume")
				}
				stateMachine.VolumeNames[img.ImgVolume] = img.ImgName
			}
		}
		// qcow2 img logic is more complicated. If .img artifacts are already specified
		// in the image definition for corresponding volumes, we will re-use those and
		// convert them to a qcow2 image. Otherwise, we will create a raw .img file to
		// use as an input file for the conversion.
		// The names of these images are placed in the VolumeNames map, which is used
		// as an input file for an eventual `qemu-convert` operation.
		if classicStateMachine.ImageDef.Artifacts.Qcow2 != nil {
			for _, qcow2 := range *classicStateMachine.ImageDef.Artifacts.Qcow2 {
				if qcow2.Qcow2Volume == "" {
					return fmt.Errorf("Volume names must be specified for each image when using a gadget with more than one volume")
				}
				// We can save a whole lot of disk I/O here if the volume is
				// already specified as a .img file
				if classicStateMachine.ImageDef.Artifacts.Img != nil {
					found := false
					for _, img := range *classicStateMachine.ImageDef.Artifacts.Img {
						if img.ImgVolume == qcow2.Qcow2Volume {
							found = true
						}
					}
					if !found {
						// if a .img artifact for this volume isn't explicitly stated in
						// the image definition, then create one
						stateMachine.VolumeNames[qcow2.Qcow2Volume] = fmt.Sprintf("%s.img", qcow2.Qcow2Name)
					}
				} else {
					// no .img artifacts exist in the image definition,
					// but we still need to create one to convert to qcow2
					stateMachine.VolumeNames[qcow2.Qcow2Volume] = fmt.Sprintf("%s.img", qcow2.Qcow2Name)
				}
			}
		}
	} else {
		if classicStateMachine.ImageDef.Artifacts.Img != nil {
			img := (*classicStateMachine.ImageDef.Artifacts.Img)[0]
			if img.ImgVolume == "" {
				// there is only one volume, so get it from the map
				volName := reflect.ValueOf(stateMachine.GadgetInfo.Volumes).MapKeys()[0].String()
				stateMachine.VolumeNames[volName] = img.ImgName
			} else {
				stateMachine.VolumeNames[img.ImgVolume] = img.ImgName
			}
		}
		// qcow2 img logic is more complicated. If .img artifacts are already specified
		// in the image definition for corresponding volumes, we will re-use those and
		// convert them to a qcow2 image. Otherwise, we will create a raw .img file to
		// use as an input file for the conversion.
		// The names of these images are placed in the VolumeNames map, which is used
		// as an input file for an eventual `qemu-convert` operation.
		if classicStateMachine.ImageDef.Artifacts.Qcow2 != nil {
			qcow2 := (*classicStateMachine.ImageDef.Artifacts.Qcow2)[0]
			if qcow2.Qcow2Volume == "" {
				volName := reflect.ValueOf(stateMachine.GadgetInfo.Volumes).MapKeys()[0].String()
				if classicStateMachine.ImageDef.Artifacts.Img != nil {
					qcow2.Qcow2Volume = volName
					(*classicStateMachine.ImageDef.Artifacts.Qcow2)[0] = qcow2
					return nil // We will re-use the .img file in this case
				}
				// there is only one volume, so get it from the map
				stateMachine.VolumeNames[volName] = fmt.Sprintf("%s.img", qcow2.Qcow2Name)
				qcow2.Qcow2Volume = volName
				(*classicStateMachine.ImageDef.Artifacts.Qcow2)[0] = qcow2
			} else {
				if classicStateMachine.ImageDef.Artifacts.Img != nil {
					return nil // We will re-use the .img file in this case
				}
				stateMachine.VolumeNames[qcow2.Qcow2Volume] = fmt.Sprintf("%s.img", qcow2.Qcow2Name)
			}
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
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)

	// make the chroot directory to which we will extract the tar
	if err := osMkdir(stateMachine.tempDirs.chroot, 0755); err != nil {
		return fmt.Errorf("Failed to create chroot directory: %s", err.Error())
	}

	// convert the URL to a file path
	// no need to check error here as the validity of the URL
	// has been confirmed by the schema validation
	tarPath := strings.TrimPrefix(classicStateMachine.ImageDef.Rootfs.Tarball.TarballURL, "file://")
	if !filepath.IsAbs(tarPath) {
		tarPath, _ = filepath.Abs(tarPath)
	}

	// if the sha256 sum of the tarball is provided, make sure it matches
	if classicStateMachine.ImageDef.Rootfs.Tarball.SHA256sum != "" {
		tarSHA256, err := helper.CalculateSHA256(tarPath)
		if err != nil {
			return err
		}
		if tarSHA256 != classicStateMachine.ImageDef.Rootfs.Tarball.SHA256sum {
			return fmt.Errorf("Calculated SHA256 sum of rootfs tarball \"%s\" does not match "+
				"the expected value specified in the image definition: \"%s\"",
				tarSHA256, classicStateMachine.ImageDef.Rootfs.Tarball.SHA256sum)
		}
	}

	// now extract the archive
	return helper.ExtractTarArchive(tarPath, stateMachine.tempDirs.chroot,
		stateMachine.commonFlags.Verbose, stateMachine.commonFlags.Debug)
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
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

	cloudInitCustomization := classicStateMachine.ImageDef.Customization.CloudInit

	seedPath := path.Join(classicStateMachine.tempDirs.chroot, "var/lib/cloud/seed/nocloud")
	err := osMkdirAll(seedPath, 0755)
	if err != nil {
		return err
	}

	if cloudInitCustomization.MetaData != "" {
		metaDataFile, err := osCreate(path.Join(seedPath, "meta-data"))
		if err != nil {
			return err
		}
		defer metaDataFile.Close()

		_, err = metaDataFile.WriteString(cloudInitCustomization.MetaData)
		if err != nil {
			return err
		}
	}

	if cloudInitCustomization.UserData != "" {
		userDataFile, err := osCreate(path.Join(seedPath, "user-data"))
		if err != nil {
			return err
		}
		defer userDataFile.Close()

		_, err = userDataFile.WriteString(cloudInitCustomization.UserData)
		if err != nil {
			return err
		}
	}

	if cloudInitCustomization.NetworkConfig != "" {
		networkConfigFile, err := osCreate(path.Join(seedPath, "network-config"))
		if err != nil {
			return err
		}
		defer networkConfigFile.Close()

		_, err = networkConfigFile.WriteString(cloudInitCustomization.NetworkConfig)
		if err != nil {
			return err
		}
	}

	datasourceConfig := "# to update this file, run dpkg-reconfigure cloud-init\ndatasource_list: [ NoCloud ]\n"

	dpkgConfigPath := path.Join(classicStateMachine.tempDirs.chroot, "etc/cloud/cloud.cfg.d/90_dpkg.cfg")
	dpkgConfigFile, err := osCreate(dpkgConfigPath)
	if err != nil {
		return err
	}
	defer dpkgConfigFile.Close()

	_, err = dpkgConfigFile.WriteString(datasourceConfig)

	return err
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

	// copy /etc/resolv.conf from the host system into the chroot if it hasn't already been done
	err := helperBackupAndCopyResolvConf(classicStateMachine.tempDirs.chroot)
	if err != nil {
		return fmt.Errorf("Error setting up /etc/resolv.conf in the chroot: \"%s\"", err.Error())
	}

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
	if stateMachine.commonFlags.Channel != "" {
		imageOpts.Channel = stateMachine.commonFlags.Channel
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
	imageOpts.ModelFile = strings.TrimPrefix(classicStateMachine.ImageDef.ModelAssertion, "file://")
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
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)

	// if we backed up resolv.conf then restore it here
	err := helperRestoreResolvConf(classicStateMachine.tempDirs.chroot)
	if err != nil {
		return fmt.Errorf("Error restoring /etc/resolv.conf in the chroot: \"%s\"", err.Error())
	}

	files, err := ioutilReadDir(stateMachine.tempDirs.chroot)
	if err != nil {
		return fmt.Errorf("Error reading unpack/chroot dir: %s", err.Error())
	}

	for _, srcFile := range files {
		srcFile := filepath.Join(stateMachine.tempDirs.chroot, srcFile.Name())
		if err := osutilCopySpecialFile(srcFile, classicStateMachine.tempDirs.rootfs); err != nil {
			return fmt.Errorf("Error copying rootfs: %s", err.Error())
		}
	}

	if classicStateMachine.ImageDef.Customization != nil {
		if len(classicStateMachine.ImageDef.Customization.Fstab) == 0 {
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
		}
	}
	return nil
}

// Generate the manifest
func (stateMachine *StateMachine) generatePackageManifest() error {
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)

	// This is basically just a wrapper around dpkg-query
	outputPath := filepath.Join(stateMachine.commonFlags.OutputDir,
		classicStateMachine.ImageDef.Artifacts.Manifest.ManifestName)
	cmd := execCommand("chroot", stateMachine.tempDirs.rootfs, "dpkg-query", "-W", "--showformat=${Package} ${Version}\n")
	cmdOutput := helper.SetCommandOutput(cmd, classicStateMachine.commonFlags.Debug)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Error generating package manifest with command \"%s\". "+
			"Error is \"%s\". Full output below:\n%s",
			cmd.String(), err.Error(), cmdOutput.String())
	}

	// write the output to a file on successful executions
	manifest, err := osCreate(outputPath)
	if err != nil {
		return fmt.Errorf("Error creating manifest file: %s", err.Error())
	}
	defer manifest.Close()
	manifest.Write(cmdOutput.Bytes())
	return nil
}

// Generate the manifest
func (stateMachine *StateMachine) generateFilelist() error {
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)

	// This is basically just a wrapper around find (similar to what we do in livecd-rootfs)
	outputPath := filepath.Join(stateMachine.commonFlags.OutputDir,
		classicStateMachine.ImageDef.Artifacts.Filelist.FilelistName)
	cmd := execCommand("chroot", stateMachine.tempDirs.rootfs, "find", "-xdev")
	cmdOutput := helper.SetCommandOutput(cmd, classicStateMachine.commonFlags.Debug)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Error generating file list with command \"%s\". "+
			"Error is \"%s\". Full output below:\n%s",
			cmd.String(), err.Error(), cmdOutput.String())
	}

	// write the output to a file on successful executions
	filelist, err := osCreate(outputPath)
	if err != nil {
		return fmt.Errorf("Error creating filelist file: %s", err.Error())
	}
	defer filelist.Close()
	filelist.Write(cmdOutput.Bytes())
	return nil
}

// Generate the rootfs tarball
func (stateMachine *StateMachine) generateRootfsTarball() error {
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)

	// first create a vanilla uncompressed tar archive
	rootfsSrc := filepath.Join(stateMachine.stateMachineFlags.WorkDir, "root")
	rootfsDst := filepath.Join(stateMachine.commonFlags.OutputDir,
		classicStateMachine.ImageDef.Artifacts.RootfsTar.RootfsTarName)
	return helper.CreateTarArchive(rootfsSrc, rootfsDst,
		classicStateMachine.ImageDef.Artifacts.RootfsTar.Compression,
		stateMachine.commonFlags.Verbose, stateMachine.commonFlags.Debug)
}

// makeQcow2Img converts raw .img artifacts into qcow2 artifacts
func (stateMachine *StateMachine) makeQcow2Img() error {
	var classicStateMachine *ClassicStateMachine
	classicStateMachine = stateMachine.parent.(*ClassicStateMachine)

	for _, qcow2 := range *classicStateMachine.ImageDef.Artifacts.Qcow2 {
		backingFile := filepath.Join(stateMachine.commonFlags.OutputDir, stateMachine.VolumeNames[qcow2.Qcow2Volume])
		resultingFile := filepath.Join(stateMachine.commonFlags.OutputDir, qcow2.Qcow2Name)
		qemuImgCommand := execCommand("qemu-img",
			"convert",
			"-c",
			"-O",
			"qcow2",
			"-o",
			"compat=0.10",
			backingFile,
			resultingFile,
		)
		qemuOutput := helper.SetCommandOutput(qemuImgCommand, classicStateMachine.commonFlags.Debug)
		if err := qemuImgCommand.Run(); err != nil {
			return fmt.Errorf("Error creating qcow2 artifact with command \"%s\". "+
				"Error is \"%s\". Full output below:\n%s",
				qemuImgCommand.String(), err.Error(), qemuOutput.String())
		}
	}
	return nil
}

// updateGrub sets up a loopback device, creates appropriate mountpoint, and runs update-grub
func (stateMachine *StateMachine) updateGrub() error {
	// make sure /dev/loop99 is not already in use
	loops, err := filepath.Glob("/dev/mapper/loop99*")
	if err != nil {
		return fmt.Errorf("Error globbing for /dev/mapper/loop99", err.Error())
	}
	if len(loops) > 0 {
		return fmt.Errorf("Error, /dev/loop99 already in use")
	}

	// create a directory in which to mount the rootfs
	mountDir := filepath.Join(stateMachine.tempDirs.scratch, "loopback")
	err = osMkdir(mountDir, 0755)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("Error creating scratch/loopback directory: %s", err.Error())
	}

	// determine which partition number is the rootfs and which volume it is in
	// TODO should this be stored in the struct earlier on?
	rootfsPartNum := -1
	var rootfsVolName string
	for _, volumeName := range stateMachine.VolumeOrder {
		volume := stateMachine.GadgetInfo.Volumes[volumeName]
		for structureNumber, structure := range volume.Structure {
			if structure.Role == gadget.SystemData {
				rootfsPartNum = structureNumber
				rootfsVolName = volumeName
				if volume.Bootloader != "grub" {
					fmt.Println("WARNING: bootloader is not grub, so this step is a no-op")
					return nil
				}
			}
		}
	}
	if rootfsPartNum == -1 {
		return fmt.Errorf("error determining partition number of the root filesystem")
	}

	// Slice used to store all the commands that need to be run
	// to properly update grub.cfg in the chroot
	var updateGrubCmds []*exec.Cmd

	imgPath := filepath.Join(stateMachine.commonFlags.OutputDir, stateMachine.VolumeNames[rootfsVolName])

	// set up the loopback
	var umounts []*exec.Cmd
	updateGrubCmds = append(updateGrubCmds,
		[]*exec.Cmd{
			// set up the loopback
			exec.Command("losetup",
				filepath.Join("/dev", "loop99"),
				imgPath,
			),
			exec.Command("kpartx",
				"-a",
				filepath.Join("/dev", "loop99"),
			),
			// mount the rootfs partition in which to run update-grub
			exec.Command("mount",
				filepath.Join("/dev", "mapper", fmt.Sprintf("loop99p%d", rootfsPartNum)),
				mountDir,
			),
		}...,
	)


	// set up the mountpoints
	mountPoints := []string{"/dev", "/proc", "/sys"}
	for _, mountPoint := range mountPoints {
		mountCmd, umountCmd := mountFromHost(mountDir, mountPoint)
		updateGrubCmds = append(updateGrubCmds, mountCmd)
		umounts = append(umounts, umountCmd)
		defer umountCmd.Run()
	}
	// make sure to unmount the disk too
	umounts = append(umounts, exec.Command("umount", mountDir))

	// actually run update-grub
	updateGrubCmds = append(updateGrubCmds,
		exec.Command("chroot",
			mountDir,
			"update-grub",
		),
	)

	// unmount /dev /proc and /sys
	updateGrubCmds = append(updateGrubCmds, umounts...)

	// tear down the loopback
	teardownCmds := []*exec.Cmd{
		exec.Command("kpartx",
			"-d",
			filepath.Join("/dev", "loop99"),
		),
		exec.Command("losetup",
			"--detach",
			filepath.Join("/dev", "loop99"),
		),
	}

	for _, teardownCmd := range teardownCmds {
		defer teardownCmd.Run()
	}
	updateGrubCmds = append(updateGrubCmds, teardownCmds...)

	// now run all the commands
	for _, cmd := range updateGrubCmds {
		cmdOutput := helper.SetCommandOutput(cmd, stateMachine.commonFlags.Debug)
		err := cmd.Run()
		if err != nil {
			return fmt.Errorf("Error running command \"%s\". Error is \"%s\". Output is: \n%s",
				cmd.String(), err.Error(), cmdOutput.String())
		}
	}

	return nil
}
