package imagedefinition

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/xeipuuv/gojsonschema"

	"github.com/canonical/ubuntu-image/internal/helper"
)

func TestGeneratePocketList(t *testing.T) {
	t.Parallel()
	type args struct {
		series         string
		components     []string
		mirror         string
		securityMirror string
		pocket         string
	}

	testCases := []struct {
		name            string
		imageDef        ImageDefinition
		args            args
		expectedPockets []string
	}{
		{
			name: "release",
			args: args{
				series:         "jammy",
				components:     []string{"main", "universe"},
				mirror:         "http://archive.ubuntu.com/ubuntu/",
				securityMirror: "http://security.ubuntu.com/ubuntu/",
				pocket:         "release",
			},
			expectedPockets: []string{"deb http://archive.ubuntu.com/ubuntu/ jammy main universe\n"},
		},
		{
			name: "security",
			args: args{
				series:         "jammy",
				components:     []string{"main"},
				mirror:         "http://archive.ubuntu.com/ubuntu/",
				pocket:         "security",
				securityMirror: "http://security.ubuntu.com/ubuntu/",
			},
			expectedPockets: []string{
				"deb http://archive.ubuntu.com/ubuntu/ jammy main\n",
				"deb http://security.ubuntu.com/ubuntu/ jammy-security main\n",
			},
		},
		{
			name: "updates",
			args: args{
				series:         "jammy",
				components:     []string{"main", "universe", "multiverse"},
				mirror:         "http://ports.ubuntu.com/",
				securityMirror: "http://ports.ubuntu.com/",
				pocket:         "updates",
			},
			expectedPockets: []string{
				"deb http://ports.ubuntu.com/ jammy main universe multiverse\n",
				"deb http://ports.ubuntu.com/ jammy-security main universe multiverse\n",
				"deb http://ports.ubuntu.com/ jammy-updates main universe multiverse\n",
			},
		},
		{
			name: "proposed",
			args: args{
				series:         "jammy",
				components:     []string{"main", "universe", "multiverse", "restricted"},
				mirror:         "http://archive.ubuntu.com/ubuntu/",
				securityMirror: "http://security.ubuntu.com/ubuntu/",
				pocket:         "proposed",
			},
			expectedPockets: []string{
				"deb http://archive.ubuntu.com/ubuntu/ jammy main universe multiverse restricted\n",
				"deb http://security.ubuntu.com/ubuntu/ jammy-security main universe multiverse restricted\n",
				"deb http://archive.ubuntu.com/ubuntu/ jammy-updates main universe multiverse restricted\n",
				"deb http://archive.ubuntu.com/ubuntu/ jammy-proposed main universe multiverse restricted\n",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pocketList := generatePocketList(
				tc.args.series,
				tc.args.components,
				tc.args.mirror,
				tc.args.securityMirror,
				tc.args.pocket,
			)
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
	t.Parallel()
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
}

// TestImageDefinition_SetDefaults make sure we do not add a boolean field
// with a default value (because we cannot properly apply the default value)
func TestImageDefinition_SetDefaults(t *testing.T) {
	t.Parallel()
	asserter := helper.Asserter{T: t}
	imageDef := &ImageDefinition{
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
			Manual: &Manual{
				AddUser: []*AddUser{
					{},
				},
			},
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
	}

	want := &ImageDefinition{
		Gadget: &Gadget{},
		Rootfs: &Rootfs{
			Seed: &Seed{
				Vcs: helper.BoolPtr(true),
			},
			Tarball:    &Tarball{},
			Components: []string{"main", "restricted"},
			Archive:    "ubuntu",
			Flavor:     "ubuntu",
			Mirror:     "http://archive.ubuntu.com/ubuntu/",
			Pocket:     "release",
		},
		Customization: &Customization{
			Components: []string{"main", "restricted", "universe"},
			Pocket:     "release",
			Installer:  &Installer{},
			CloudInit:  &CloudInit{},
			ExtraPPAs: []*PPA{{
				KeepEnabled: helper.BoolPtr(true),
			}},
			ExtraPackages: []*Package{{}},
			ExtraSnaps: []*Snap{{
				Store:   "canonical",
				Channel: "stable",
			}},
			Fstab: []*Fstab{{
				MountOptions: "defaults",
			}},
			Manual: &Manual{
				AddUser: []*AddUser{
					{
						PasswordType: "hash",
						Expire:       helper.BoolPtr(true),
					},
				},
			},
		},
		Artifacts: &Artifact{
			Img:       &[]Img{{}},
			Iso:       &[]Iso{{}},
			Qcow2:     &[]Qcow2{{}},
			Manifest:  &Manifest{},
			Filelist:  &Filelist{},
			Changelog: &Changelog{},
			RootfsTar: &RootfsTar{
				Compression: "uncompressed",
			},
		},
	}

	err := helper.SetDefaults(imageDef)
	asserter.AssertErrNil(err, true)

	asserter.AssertEqual(want, imageDef, cmp.AllowUnexported(ImageDefinition{}))
}

func TestImageDefinition_securityMirror(t *testing.T) {
	type fields struct {
		Architecture string
		Rootfs       *Rootfs
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "amd64",
			fields: fields{
				Architecture: "amd64",
				Rootfs: &Rootfs{
					Mirror: "http://archive.ubuntu.com/ubuntu/",
				},
			},
			want: "http://security.ubuntu.com/ubuntu/",
		},
		{
			name: "i386",
			fields: fields{
				Architecture: "i386",
				Rootfs: &Rootfs{
					Mirror: "http://archive.ubuntu.com/ubuntu/",
				},
			},
			want: "http://security.ubuntu.com/ubuntu/",
		},
		{
			name: "arm64",
			fields: fields{
				Architecture: "arm64",
				Rootfs: &Rootfs{
					Mirror: "http://archive.ubuntu.com/ubuntu/",
				},
			},
			want: "http://archive.ubuntu.com/ubuntu/",
		},
		{
			name: "no arch",
			fields: fields{
				Rootfs: &Rootfs{
					Mirror: "http://archive.ubuntu.com/ubuntu/",
				},
			},
			want: "http://archive.ubuntu.com/ubuntu/",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imageDef := ImageDefinition{
				Architecture: tt.fields.Architecture,
				Rootfs:       tt.fields.Rootfs,
			}
			if got := imageDef.securityMirror(); got != tt.want {
				t.Errorf("ImageDefinition.securityMirror() = %v, want %v", got, tt.want)
			}
		})
	}
}
