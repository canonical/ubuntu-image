package imagedefinition

import (
	"strings"
	"testing"

	"github.com/xeipuuv/gojsonschema"

	"github.com/canonical/ubuntu-image/internal/helper"
)

func TestGeneratePocketList(t *testing.T) {
	testCases := []struct {
		name            string
		imageDef        ImageDefinition
		expectedPockets []string
	}{
		{
			"release",
			ImageDefinition{
				Series: "jammy",
				Rootfs: &Rootfs{
					Pocket:     "release",
					Components: []string{"main", "universe"},
					Mirror:     "http://archive.ubuntu.com/ubuntu/",
				},
			},
			[]string{},
		},
		{
			"security",
			ImageDefinition{
				Architecture: "amd64",
				Series:       "jammy",
				Rootfs: &Rootfs{
					Pocket:     "security",
					Components: []string{"main"},
					Mirror:     "http://archive.ubuntu.com/ubuntu/",
				},
			},
			[]string{"deb http://security.ubuntu.com/ubuntu/ jammy-security main\n"},
		},
		{
			"updates",
			ImageDefinition{
				Architecture: "arm64",
				Series:       "jammy",
				Rootfs: &Rootfs{
					Pocket:     "updates",
					Components: []string{"main", "universe", "multiverse"},
					Mirror:     "http://ports.ubuntu.com/",
				},
			},
			[]string{
				"deb http://ports.ubuntu.com/ jammy-security main universe multiverse\n",
				"deb http://ports.ubuntu.com/ jammy-updates main universe multiverse\n",
			},
		},
		{
			"proposed",
			ImageDefinition{
				Architecture: "amd64",
				Series:       "jammy",
				Rootfs: &Rootfs{
					Pocket:     "proposed",
					Components: []string{"main", "universe", "multiverse", "restricted"},
					Mirror:     "http://archive.ubuntu.com/ubuntu/",
				},
			},
			[]string{
				"deb http://security.ubuntu.com/ubuntu/ jammy-security main universe multiverse restricted\n",
				"deb http://archive.ubuntu.com/ubuntu/ jammy-updates main universe multiverse restricted\n",
				"deb http://archive.ubuntu.com/ubuntu/ jammy-proposed main universe multiverse restricted\n",
			},
		},
	}
	for _, tc := range testCases {
		t.Run("test_generate_pocket_list_"+tc.name, func(t *testing.T) {
			pocketList := tc.imageDef.GeneratePocketList()
			for _, expectedPocket := range tc.expectedPockets {
				found := false
				for _, pocket := range pocketList {
					if pocket == expectedPocket {
						found = true
					}
				}
				if !found {
					t.Errorf("Expected %s in pockets list %s, but it was not", expectedPocket, pocketList)
				}
			}
		})
	}
}

// TestCustomErrors tests the custom json schema errors that we define
func TestCustomErrors(t *testing.T) {
	t.Run("test_custom_errors", func(t *testing.T) {
		jsonContext := gojsonschema.NewJsonContext("testContext", nil)
		errDetail := gojsonschema.ErrorDetails{
			"key":   "testKey",
			"value": "testValue",
		}
		missingURLErr := NewMissingURLError(
			gojsonschema.NewJsonContext("testMissingURL", jsonContext),
			52,
			errDetail,
		)
		// spot check the description format
		if !strings.Contains(missingURLErr.DescriptionFormat(),
			"When key {{.key}} is specified as {{.value}}, a URL must be provided") {
			t.Errorf("missingURLError description format \"%s\" is invalid",
				missingURLErr.DescriptionFormat())
		}

		invalidPPAErr := NewInvalidPPAError(
			gojsonschema.NewJsonContext("testInvalidPPA", jsonContext),
			52,
			errDetail,
		)
		// spot check the description format
		if !strings.Contains(invalidPPAErr.DescriptionFormat(),
			"Fingerprint is required for private PPAs") {
			t.Errorf("invalidPPAError description format \"%s\" is invalid",
				invalidPPAErr.DescriptionFormat())
		}

		pathNotAbsoluteErr := NewPathNotAbsoluteError(
			gojsonschema.NewJsonContext("testPathNotAbsolute", jsonContext),
			52,
			errDetail,
		)
		// spot check the description format
		if !strings.Contains(pathNotAbsoluteErr.DescriptionFormat(),
			"Key {{.key}} needs to be an absolute path ({{.value}})") {
			t.Errorf("pathNotAbsoluteError description format \"%s\" is invalid",
				pathNotAbsoluteErr.DescriptionFormat())
		}
		dependentKeyErr := NewDependentKeyError(
			gojsonschema.NewJsonContext("testDependentKey", jsonContext),
			52,
			errDetail,
		)
		// spot check the description format
		if !strings.Contains(dependentKeyErr.DescriptionFormat(),
			"Key {{.key1}} cannot be used without key {{.key2}}") {
			t.Errorf("dependentKeyError description format \"%s\" is invalid",
				dependentKeyErr.DescriptionFormat())
		}
	})
}

// TestImageDefinition_SetDefaults make sure we do not add a boolean field
// with a default value (because we cannot properly apply the default value)
func TestImageDefinition_SetDefaults(t *testing.T) {
	t.Run("test_image_definition_valid_defaults", func(t *testing.T) {
		asserter := helper.Asserter{T: t}
		err := helper.SetDefaults(&ImageDefinition{
			Gadget: &Gadget{},
			Rootfs: &Rootfs{
				Seed:    &Seed{},
				Tarball: &Tarball{},
			},
			Customization: &Customization{
				Installer:     &Installer{},
				CloudInit:     &CloudInit{},
				ExtraPPAs:     []*PPA{{}},
				ExtraPackages: []*Package{{}},
				ExtraSnaps:    []*Snap{{}},
				Fstab:         []*Fstab{{}},
				Manual:        &Manual{},
			},
			Artifacts: &Artifact{
				Img:       &[]Img{{}},
				Iso:       &[]Iso{{}},
				Qcow2:     &[]Qcow2{{}},
				Manifest:  &Manifest{},
				Filelist:  &Filelist{},
				Changelog: &Changelog{},
				RootfsTar: &RootfsTar{},
			},
		})
		asserter.AssertErrNil(err, true)
	})
}
