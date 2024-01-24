package statemachine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/invopop/jsonschema"
	"github.com/xeipuuv/gojsonschema"
	"gopkg.in/yaml.v2"

	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/canonical/ubuntu-image/internal/imagedefinition"
)

// classicStates are the names and function variables to be executed by the state machine for classic images
var startingClassicStates = []stateFunc{
	{"make_temporary_directories", (*StateMachine).makeTemporaryDirectories},
	{"determine_output_directory", (*StateMachine).determineOutputDirectory},
}

var rootfsSeedStates = []stateFunc{
	{"germinate", (*StateMachine).germinate},
	{"create_chroot", (*StateMachine).createChroot},
}

var imageCreationStates = []stateFunc{
	{"calculate_rootfs_size", (*StateMachine).calculateRootfsSize},
	{"populate_bootfs_contents", (*StateMachine).populateBootfsContents},
	{"populate_prepare_partitions", (*StateMachine).populatePreparePartitions},
}

// ClassicStateMachine embeds StateMachine and adds the command line flags specific to classic images
type ClassicStateMachine struct {
	StateMachine
	ImageDef imagedefinition.ImageDefinition
	Opts     commands.ClassicOpts
	Args     commands.ClassicArgs
}

// Setup assigns variables and calls other functions that must be executed before Run()
func (classicStateMachine *ClassicStateMachine) Setup() error {
	// set the parent pointer of the embedded struct
	classicStateMachine.parent = classicStateMachine

	// set the beginning states that will be used by all classic image builds
	classicStateMachine.states = startingClassicStates

	if err := classicStateMachine.setConfDefDir(classicStateMachine.parent.(*ClassicStateMachine).Args.ImageDefinition); err != nil {
		return err
	}

	// do the validation common to all image types
	if err := classicStateMachine.validateInput(); err != nil {
		return err
	}

	if err := classicStateMachine.parseImageDefinition(); err != nil {
		return err
	}

	if err := classicStateMachine.calculateStates(); err != nil {
		return err
	}

	// validate values of until and thru
	if err := classicStateMachine.validateUntilThru(); err != nil {
		return err
	}

	// if --resume was passed, figure out where to start
	if err := classicStateMachine.readMetadata(metadataStateFile); err != nil {
		return err
	}

	return classicStateMachine.SetSeries()
}

func (classicStateMachine *ClassicStateMachine) SetSeries() error {
	classicStateMachine.series = classicStateMachine.ImageDef.Series
	return nil
}

// parseImageDefinition parses the provided yaml file and ensures it is valid
func (stateMachine *StateMachine) parseImageDefinition() error {
	classicStateMachine := stateMachine.parent.(*ClassicStateMachine)

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
			if imageDefinition.Customization.Manual.MakeDirs != nil {
				for _, mkdir := range imageDefinition.Customization.Manual.MakeDirs {
					// XXX: filepath.IsAbs() does returns true for paths like /../../something
					// and those are NOT absolute paths.
					if !filepath.IsAbs(mkdir.Path) || strings.Contains(mkdir.Path, "/../") {
						errDetail := gojsonschema.ErrorDetails{
							"key":   "customization:manual:mkdir:destination",
							"value": mkdir.Path,
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
func (s *StateMachine) calculateStates() error {
	c := s.parent.(*ClassicStateMachine)

	var rootfsCreationStates []stateFunc

	if c.ImageDef.Gadget != nil {
		s.addGadgetStates(&rootfsCreationStates)
	}

	// if artifacts are specified, verify the correctness and store them in the struct
	diskUsed, err := helperCheckTags(c.ImageDef.Artifacts, "is_disk")
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
	if c.ImageDef.Rootfs.Tarball != nil {
		s.addRootfsFromTarballStates(&rootfsCreationStates)
	} else if c.ImageDef.Rootfs.Seed != nil {
		s.addRootfsFromSeedStates(&rootfsCreationStates)
	} else {
		rootfsCreationStates = append(rootfsCreationStates,
			stateFunc{"build_rootfs_from_tasks", (*StateMachine).buildRootfsFromTasks})
	}

	// Before customization, make sure we clean unwanted secrets/values that
	// are supposed to be unique per machine
	rootfsCreationStates = append(rootfsCreationStates,
		stateFunc{"clean_rootfs", (*StateMachine).cleanRootfs})

	rootfsCreationStates = append(rootfsCreationStates,
		stateFunc{"customize_sources_list", (*StateMachine).customizeSourcesList})

	if c.ImageDef.Customization != nil {
		s.addCustomizationStates(&rootfsCreationStates)
	}

	// Make sure that the rootfs has the correct locale set
	rootfsCreationStates = append(rootfsCreationStates,
		stateFunc{"set_default_locale", (*StateMachine).setDefaultLocale})

	// The rootfs is laid out in a staging area, now populate it in the correct location
	rootfsCreationStates = append(rootfsCreationStates,
		stateFunc{"populate_rootfs_contents", (*StateMachine).populateClassicRootfsContents})

	if s.commonFlags.DiskInfo != "" {
		rootfsCreationStates = append(rootfsCreationStates,
			stateFunc{"generate_disk_info", (*StateMachine).generateDiskInfo})
	}

	if c.ImageDef.Gadget != nil {
		s.addImgStates(&rootfsCreationStates)
	}

	if c.ImageDef.Artifacts.Qcow2 != nil {
		s.addQcow2States(&rootfsCreationStates)
	}

	if c.ImageDef.Artifacts.Manifest != nil {
		rootfsCreationStates = append(rootfsCreationStates,
			stateFunc{"generate_manifest", (*StateMachine).generatePackageManifest})
	}

	if c.ImageDef.Artifacts.Filelist != nil {
		rootfsCreationStates = append(rootfsCreationStates,
			stateFunc{"generate_filelist", (*StateMachine).generateFilelist})
	}

	if c.ImageDef.Artifacts.RootfsTar != nil {
		rootfsCreationStates = append(rootfsCreationStates,
			stateFunc{"generate_rootfs_tarball", (*StateMachine).generateRootfsTarball})
	}

	// add the no-op "finish" state
	rootfsCreationStates = append(rootfsCreationStates,
		stateFunc{"finish", (*StateMachine).finish})

	// Append the newly calculated states to the slice of funcs in the parent struct
	s.states = append(s.states, rootfsCreationStates...)

	s.displayCalculatedStates()

	return nil
}

func (s *StateMachine) addGadgetStates(states *[]stateFunc) {
	c := s.parent.(*ClassicStateMachine)

	// determine the states needed for preparing the gadget
	switch c.ImageDef.Gadget.GadgetType {
	case "git", "directory":
		*states = append(*states,
			stateFunc{"build_gadget_tree", (*StateMachine).buildGadgetTree})
		fallthrough
	case "prebuilt":
		*states = append(*states,
			stateFunc{"prepare_gadget_tree", (*StateMachine).prepareGadgetTree})
	}

	// Load the gadget yaml after the gadget is built
	*states = append(*states,
		stateFunc{"load_gadget_yaml", (*StateMachine).loadGadgetYaml})
}

func (s *StateMachine) addRootfsFromTarballStates(states *[]stateFunc) {
	c := s.parent.(*ClassicStateMachine)

	*states = append(*states,
		stateFunc{"extract_rootfs_tar", (*StateMachine).extractRootfsTar})
	if c.ImageDef.Customization == nil {
		return
	}

	if len(c.ImageDef.Customization.ExtraPPAs) > 0 {
		*states = append(*states,
			[]stateFunc{
				{"add_extra_ppas", (*StateMachine).addExtraPPAs},
				{"install_packages", (*StateMachine).installPackages},
				{"clean_extra_ppas", (*StateMachine).cleanExtraPPAs},
			}...)
	} else if len(c.ImageDef.Customization.ExtraPackages) > 0 {
		*states = append(*states,
			stateFunc{"install_packages", (*StateMachine).installPackages},
		)
	}

	if len(c.ImageDef.Customization.ExtraSnaps) > 0 {
		*states = append(*states,
			[]stateFunc{
				{"install_extra_snaps", (*StateMachine).prepareClassicImage},
				{"preseed_extra_snaps", (*StateMachine).preseedClassicImage},
			}...)
	}
}

func (s *StateMachine) addRootfsFromSeedStates(states *[]stateFunc) {
	c := s.parent.(*ClassicStateMachine)

	*states = append(*states, rootfsSeedStates...)

	if c.ImageDef.Customization == nil {
		*states = append(*states,
			stateFunc{"install_packages", (*StateMachine).installPackages},
		)
	} else if len(c.ImageDef.Customization.ExtraPPAs) > 0 {
		*states = append(*states,
			[]stateFunc{
				{"add_extra_ppas", (*StateMachine).addExtraPPAs},
				{"install_packages", (*StateMachine).installPackages},
				{"clean_extra_ppas", (*StateMachine).cleanExtraPPAs},
			}...)
	} else {
		*states = append(*states,
			stateFunc{"install_packages", (*StateMachine).installPackages},
		)
	}

	*states = append(*states,
		[]stateFunc{
			{"prepare_image", (*StateMachine).prepareClassicImage},
			{"preseed_image", (*StateMachine).preseedClassicImage},
		}...,
	)
}

// addCustomizationStates determines any customization that needs to run before the image
// is created
// TODO: installer image customization... eventually.
func (s *StateMachine) addCustomizationStates(states *[]stateFunc) {
	c := s.parent.(*ClassicStateMachine)

	if c.ImageDef.Customization.CloudInit != nil {
		*states = append(*states,
			stateFunc{"customize_cloud_init", (*StateMachine).customizeCloudInit})
	}
	if len(c.ImageDef.Customization.Fstab) > 0 {
		*states = append(*states,
			stateFunc{"customize_fstab", (*StateMachine).customizeFstab})
	}
	if c.ImageDef.Customization.Manual != nil {
		*states = append(*states,
			stateFunc{"perform_manual_customization", (*StateMachine).manualCustomization})
	}
}

func (s *StateMachine) addImgStates(states *[]stateFunc) {
	c := s.parent.(*ClassicStateMachine)
	*states = append(*states, imageCreationStates...)

	if c.ImageDef.Artifacts.Img == nil {
		return
	}

	*states = append(*states,
		stateFunc{"make_disk", (*StateMachine).makeDisk},
		stateFunc{"update_bootloader", (*StateMachine).updateBootloader},
	)
}

func (s *StateMachine) addQcow2States(states *[]stateFunc) {
	// Only run make_disk once
	found := false
	for _, stateFunc := range *states {
		if stateFunc.name == "make_disk" {
			found = true
		}
	}
	if !found {
		*states = append(*states,
			stateFunc{"make_disk", (*StateMachine).makeDisk},
			stateFunc{"update_bootloader", (*StateMachine).updateBootloader},
		)
	}
	*states = append(*states,
		stateFunc{"make_qcow2_image", (*StateMachine).makeQcow2Img})
}

// displayStates print the calculated states
func (s *StateMachine) displayCalculatedStates() {
	if !s.commonFlags.Debug {
		return
	}
	fmt.Println("\nThe calculated states are as follows:")
	for i, state := range s.states {
		fmt.Printf("[%d] %s\n", i, state.name)
	}
	fmt.Println("\n\nContinuing")
}
