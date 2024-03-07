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
	asserter := helper.Asserter{T: t}
	type args struct {
		series         string
		components     []string
		mirror         string
		securityMirror string
		pocket         string
	}

	testCases := []struct {
		name                string
		imageDef            ImageDefinition
		args                args
		expectedSourcesList string
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
			expectedSourcesList: `# See http://help.ubuntu.com/community/UpgradeNotes for how to upgrade to
# newer versions of the distribution.
deb http://archive.ubuntu.com/ubuntu/ jammy main universe
`,
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
			expectedSourcesList: `# See http://help.ubuntu.com/community/UpgradeNotes for how to upgrade to
# newer versions of the distribution.
deb http://archive.ubuntu.com/ubuntu/ jammy main
deb http://security.ubuntu.com/ubuntu/ jammy-security main
`,
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
			expectedSourcesList: `# See http://help.ubuntu.com/community/UpgradeNotes for how to upgrade to
# newer versions of the distribution.
deb http://ports.ubuntu.com/ jammy main universe multiverse
deb http://ports.ubuntu.com/ jammy-security main universe multiverse
## Major bug fix updates produced after the final release of the
## distribution.
deb http://ports.ubuntu.com/ jammy-updates main universe multiverse
`,
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
			expectedSourcesList: `# See http://help.ubuntu.com/community/UpgradeNotes for how to upgrade to
# newer versions of the distribution.
deb http://archive.ubuntu.com/ubuntu/ jammy main universe multiverse restricted
deb http://security.ubuntu.com/ubuntu/ jammy-security main universe multiverse restricted
## Major bug fix updates produced after the final release of the
## distribution.
deb http://archive.ubuntu.com/ubuntu/ jammy-updates main universe multiverse restricted
deb http://archive.ubuntu.com/ubuntu/ jammy-proposed main universe multiverse restricted
`,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotSourcesList := generateLegacySourcesList(
				tc.args.series,
				tc.args.components,
				tc.args.mirror,
				tc.args.securityMirror,
				tc.args.pocket,
			)

			asserter.AssertEqual(tc.expectedSourcesList, gotSourcesList)
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

	tests := []struct {
		name     string
		imageDef *ImageDefinition
		want     *ImageDefinition
	}{
		{
			name: "full",
			imageDef: &ImageDefinition{
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
			},
			want: &ImageDefinition{
				Gadget: &Gadget{},
				Rootfs: &Rootfs{
					Seed: &Seed{
						Vcs: helper.BoolPtr(true),
					},
					Tarball:           &Tarball{},
					Components:        []string{"main", "restricted"},
					Archive:           "ubuntu",
					Flavor:            "ubuntu",
					Mirror:            "http://archive.ubuntu.com/ubuntu/",
					Pocket:            "release",
					SourcesListDeb822: helper.BoolPtr(false),
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
			},
		},
		{
			name: "minimal conf",
			imageDef: &ImageDefinition{
				Gadget: &Gadget{},
				Rootfs: &Rootfs{
					Seed:    &Seed{},
					Tarball: &Tarball{},
				},
			},
			want: &ImageDefinition{
				Gadget: &Gadget{},
				Rootfs: &Rootfs{
					Seed: &Seed{
						Vcs: helper.BoolPtr(true),
					},
					Tarball:           &Tarball{},
					Components:        []string{"main", "restricted"},
					Archive:           "ubuntu",
					Flavor:            "ubuntu",
					Mirror:            "http://archive.ubuntu.com/ubuntu/",
					Pocket:            "release",
					SourcesListDeb822: helper.BoolPtr(false),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asserter := helper.Asserter{T: t}
			err := helper.SetDefaults(tt.imageDef)
			asserter.AssertErrNil(err, true)

			asserter.AssertEqual(tt.want, tt.imageDef, cmp.AllowUnexported(ImageDefinition{}))
		})
	}
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

func Test_generateDeb822Section(t *testing.T) {
	asserter := helper.Asserter{T: t}
	type args struct {
		mirror     string
		series     string
		components []string
		pocket     string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "release",
			args: args{
				mirror:     "http://archive.ubuntu.com/ubuntu/",
				series:     "jammy",
				components: []string{"main", "restricted"},
				pocket:     "release",
			},
			want: `Types: deb
URIs: http://archive.ubuntu.com/ubuntu/
Suites: jammy
Components: main restricted
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg

`,
		},
		{
			name: "security",
			args: args{
				mirror:     "http://security.ubuntu.com/ubuntu/",
				series:     "jammy",
				components: []string{"main", "restricted"},
				pocket:     "security",
			},
			want: `Types: deb
URIs: http://security.ubuntu.com/ubuntu/
Suites: jammy-security
Components: main restricted
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg

`,
		},
		{
			name: "proposed",
			args: args{
				mirror:     "http://archive.ubuntu.com/ubuntu/",
				series:     "jammy",
				components: []string{"main", "restricted"},
				pocket:     "proposed",
			},
			want: `Types: deb
URIs: http://archive.ubuntu.com/ubuntu/
Suites: jammy jammy-updates jammy-proposed
Components: main restricted
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg

`,
		},
		{
			name: "updates",
			args: args{
				mirror:     "http://archive.ubuntu.com/ubuntu/",
				series:     "jammy",
				components: []string{"main", "restricted"},
				pocket:     "updates",
			},
			want: `Types: deb
URIs: http://archive.ubuntu.com/ubuntu/
Suites: jammy jammy-updates
Components: main restricted
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg

`,
		},
		{
			name: "no pocket",
			args: args{
				mirror:     "http://archive.ubuntu.com/ubuntu/",
				series:     "jammy",
				components: []string{"main", "restricted"},
				pocket:     "",
			},
			want: `Types: deb
URIs: http://archive.ubuntu.com/ubuntu/
Suites: 
Components: main restricted
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg

`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := generateDeb822Section(tc.args.mirror, tc.args.series, tc.args.components, tc.args.pocket)
			asserter.AssertEqual(tc.want, got)
		})
	}
}
