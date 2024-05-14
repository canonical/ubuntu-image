/*
Package imagedefinition provides the structure for the
image definition that will be parsed from a YAML file.
*/
package imagedefinition

import (
	"fmt"
	"strings"

	"github.com/xeipuuv/gojsonschema"
)

// ImageDefinition is the parent struct for the data
// contained within a classic image definition file
type ImageDefinition struct {
	ImageName      string         `yaml:"name"            json:"ImageName"`
	DisplayName    string         `yaml:"display-name"    json:"DisplayName"`
	Revision       int            `yaml:"revision"        json:"Revision,omitempty"`
	Architecture   string         `yaml:"architecture"    json:"Architecture"`
	Series         string         `yaml:"series"          json:"Series"`
	Kernel         string         `yaml:"kernel"          json:"Kernel,omitempty"`
	Gadget         *Gadget        `yaml:"gadget"          json:"Gadget,omitempty"`
	ModelAssertion string         `yaml:"model-assertion" json:"ModelAssertion,omitempty" jsonschema:"type=string,format=uri"`
	Rootfs         *Rootfs        `yaml:"rootfs"          json:"Rootfs"`
	Customization  *Customization `yaml:"customization"   json:"Customization,omitempty"`
	Artifacts      *Artifact      `yaml:"artifacts"       json:"Artifacts,omitempty"`
	Class          string         `yaml:"class"           json:"Class"                    jsonschema:"enum=preinstalled,enum=cloud,enum=installer"`
}

// Gadget defines the gadget section of the image definition file
type Gadget struct {
	Ref          string `yaml:"ref"    json:"Ref,omitempty"`
	GadgetTarget string `yaml:"target" json:"GadgetTarget,omitempty"`
	GadgetBranch string `yaml:"branch" json:"GadgetBranch,omitempty"`
	GadgetType   string `yaml:"type"   json:"GadgetType"             jsonschema:"enum=git,enum=directory,enum=prebuilt"`
	GadgetURL    string `yaml:"url"    json:"GadgetURL,omitempty"    jsonschema:"type=string,format=uri"`
}

// Rootfs defines the rootfs section of the image definition file
type Rootfs struct {
	Components        []string `yaml:"components"    json:"Components,omitempty"   default:"main,restricted"`
	Archive           string   `yaml:"archive"       json:"Archive"                default:"ubuntu"`
	Flavor            string   `yaml:"flavor"        json:"Flavor"                 default:"ubuntu"`
	Mirror            string   `yaml:"mirror"        json:"Mirror"                 default:"http://archive.ubuntu.com/ubuntu/"`
	Pocket            string   `yaml:"pocket"        json:"Pocket"                 jsonschema:"enum=release,enum=Release,enum=updates,enum=Updates,enum=security,enum=Security,enum=proposed,enum=Proposed" default:"release"`
	Seed              *Seed    `yaml:"seed"          json:"Seed,omitempty"         jsonschema:"oneof_required=Seed"`
	Tarball           *Tarball `yaml:"tarball"       json:"Tarball,omitempty"      jsonschema:"oneof_required=Tarball"`
	ArchiveTasks      []string `yaml:"archive-tasks" json:"ArchiveTasks,omitempty" jsonschema:"oneof_required=ArchiveTasks"`
	SourcesListDeb822 *bool    `yaml:"sources-list-deb822" json:"SourcesListDeb822" default:"false"`
}

// Seed defines the seed section of rootfs, which is used to
// build a rootfs via seed germination
type Seed struct {
	SeedBranch string   `yaml:"branch" json:"SeedBranch,omitempty"`
	SeedURLs   []string `yaml:"urls"   json:"SeedURLs"             jsonschema:"type=array,format=uri"`
	Names      []string `yaml:"names"  json:"Names"`
	Vcs        *bool    `yaml:"vcs"    json:"Vcs"                  default:"true"`
}

// Tarball defines the tarball section of rootfs, which is used
// to create images from a pre-built rootfs
type Tarball struct {
	TarballURL string `yaml:"url"       json:"TarballURL"          jsonschema:"type=string,format=uri"`
	GPG        string `yaml:"gpg"       json:"GPG,omitempty"       jsonschema:"type=string,format=uri"`
	SHA256sum  string `yaml:"sha256sum" json:"SHA256sum,omitempty" jsonschema:"minLength=64,maxLength=64"`
}

// Customization defines the customization section of the image definition file.
type Customization struct {
	Components    []string   `yaml:"components"     json:"Components,omitempty"   default:"main,restricted,universe"`
	Pocket        string     `yaml:"pocket"         json:"Pocket"                 jsonschema:"enum=release,enum=Release,enum=updates,enum=Updates,enum=security,enum=Security,enum=proposed,enum=Proposed" default:"release"`
	Installer     *Installer `yaml:"installer"      json:"Installer,omitempty"`
	CloudInit     *CloudInit `yaml:"cloud-init"     json:"CloudInit,omitempty"`
	ExtraPPAs     []*PPA     `yaml:"extra-ppas"     json:"ExtraPPAs,omitempty"`
	ExtraPackages []*Package `yaml:"extra-packages" json:"ExtraPackages,omitempty"`
	ExtraSnaps    []*Snap    `yaml:"extra-snaps"    json:"ExtraSnaps,omitempty"`
	Fstab         []*Fstab   `yaml:"fstab"          json:"Fstab,omitempty"`
	Manual        *Manual    `yaml:"manual"         json:"Manual,omitempty"`
}

// Installer provides customization options specific to installer images
type Installer struct {
	Preseeds []string `yaml:"preseeds" json:"Preseeds,omitempty"`
	Layers   []string `yaml:"layers"   json:"Layers,omitempty"`
}

// CloudInit provides customizations for running cloud-init
type CloudInit struct {
	MetaData      string `yaml:"meta-data"      json:"MetaData,omitempty"`
	UserData      string `yaml:"user-data"      json:"UserData,omitempty"`
	NetworkConfig string `yaml:"network-config" json:"NetworkConfig,omitempty"`
}

// PPA contains information about a public or private PPA
type PPA struct {
	Name        string `yaml:"name"         json:"PPAName"               jsonschema:"pattern=^[a-zA-Z0-9_.+-]+/[a-zA-Z0-9_.+-]+$"`
	Auth        string `yaml:"auth"         json:"Auth,omitempty"        jsonschema:"pattern=^[a-zA-Z0-9_.+-]+:[a-zA-Z0-9]+$"`
	Fingerprint string `yaml:"fingerprint"  json:"Fingerprint,omitempty"`
	KeepEnabled *bool  `yaml:"keep-enabled" json:"KeepEnabled"           default:"true"`
}

// Package contains information about packages
type Package struct {
	PackageName string `yaml:"name" json:"PackageName"`
}

// Snap contains information about snaps
type Snap struct {
	SnapName     string `yaml:"name"     json:"SnapName"`
	SnapRevision int    `yaml:"revision" json:"SnapRevision,omitempty" jsonschema:"type=integer"`
	Store        string `yaml:"store"    json:"Store"                  default:"canonical"`
	Channel      string `yaml:"channel"  json:"Channel"                default:"stable"`
}

// Manual provides manual customization options
type Manual struct {
	MakeDirs  []*MakeDirs  `yaml:"make-dirs"  json:"MakeDirs,omitempty"`
	CopyFile  []*CopyFile  `yaml:"copy-file"  json:"CopyFile,omitempty"`
	Execute   []*Execute   `yaml:"execute"    json:"Execute,omitempty"`
	TouchFile []*TouchFile `yaml:"touch-file" json:"TouchFile,omitempty"`
	AddGroup  []*AddGroup  `yaml:"add-group"  json:"AddGroup,omitempty"`
	AddUser   []*AddUser   `yaml:"add-user"   json:"AddUser,omitempty"`
}

// Fstab defines the information that gets rendered into an fstab
type Fstab struct {
	Label        string `yaml:"label"           json:"Label"`
	Mountpoint   string `yaml:"mountpoint"      json:"Mountpoint"`
	FSType       string `yaml:"filesystem-type" json:"FSType"`
	MountOptions string `yaml:"mount-options"   json:"MountOptions" default:"defaults"`
	Dump         bool   `yaml:"dump"            json:"Dump,omitempty"`
	FsckOrder    int    `yaml:"fsck-order"      json:"FsckOrder"`
}

// MakeDirs allows users to copy files into the rootfs of an image
type MakeDirs struct {
	Path        string `yaml:"path" json:"Path"`
	Permissions uint32 `yaml:"permissions"      json:"Permissions" default:"0755"`
}

// CopyFile allows users to copy files into the rootfs of an image
type CopyFile struct {
	Dest   string `yaml:"destination" json:"Dest"`
	Source string `yaml:"source"      json:"Source"`
}

// Execute allows users to execute a script in the rootfs of an image
type Execute struct {
	ExecutePath string `yaml:"path" json:"ExecutePath"`
}

// TouchFile allows users to touch a file in the rootfs of an image
type TouchFile struct {
	TouchPath string `yaml:"path" json:"TouchPath"`
}

// AddGroup allows users to add a group in the image that is being built
type AddGroup struct {
	GroupName string `yaml:"name" json:"GroupName"`
	GroupID   string `yaml:"id"   json:"GroupID,omitempty"`
}

// AddUser allows users to add a user in the image that is being built
type AddUser struct {
	UserName     string `yaml:"name"          json:"UserName"`
	UserID       string `yaml:"id"            json:"UserID,omitempty"`
	Password     string `yaml:"password"      json:"Password,omitempty"`
	PasswordType string `yaml:"password-type" json:"PasswordType"        default:"hash" jsonschema:"enum=text,enum=hash"`
}

// Artifact contains information about the files that are created
// during and as a result of the image build process
type Artifact struct {
	Img       *[]Img     `yaml:"img"            json:"Img,omitempty"       is_disk:"true"`
	Iso       *[]Iso     `yaml:"iso"            json:"Iso,omitempty"       is_disk:"true"`
	Qcow2     *[]Qcow2   `yaml:"qcow2"          json:"Qcow2,omitempty"     is_disk:"true"`
	Manifest  *Manifest  `yaml:"manifest"       json:"Manifest,omitempty"  is_disk:"false"`
	Filelist  *Filelist  `yaml:"filelist"       json:"Filelist,omitempty"  is_disk:"false"`
	Changelog *Changelog `yaml:"changelog"      json:"Changelog,omitempty" is_disk:"false"`
	RootfsTar *RootfsTar `yaml:"rootfs-tarball" json:"RootfsTar,omitempty" is_disk:"false"`
}

// Img specifies the name of the resulting .img file.
// If left emtpy no .img file will be created
type Img struct {
	ImgName   string `yaml:"name"   json:"ImgName"`
	ImgVolume string `yaml:"volume" json:"ImgVolume"`
}

// Iso specifies the name of the resulting .iso file
// and optionally the xorrisofs command used to create it.
// If left emtpy no .iso file will be created
type Iso struct {
	IsoName   string `yaml:"name"            json:"IsoName"`
	IsoVolume string `yaml:"volume"          json:"IsoVolume"`
	Command   string `yaml:"xorriso-command" json:"Command,omitempty"`
}

// Qcow2 specifies the name of the resulting .qcow2 file
// If left emtpy no .qcow2 file will be created
type Qcow2 struct {
	Qcow2Name   string `yaml:"name"   json:"Qcow2Name"`
	Qcow2Volume string `yaml:"volume" json:"Qcow2Volume"`
}

// Manifest specifies the name of the manifest file.
// If left emtpy no manifest file will be created
type Manifest struct {
	ManifestName string `yaml:"name" json:"ManifestName"`
}

// Filelist specifies the name of the filelist file.
// If left emtpy no filelist file will be created
type Filelist struct {
	FilelistName string `yaml:"name" json:"FilelistName"`
}

// Changelog specifies the name of the changelog file.
// If left emtpy no changelog file will be created
type Changelog struct {
	ChangelogName string `yaml:"name" json:"ChangelogName"`
}

// RootfsTar specifies the name of a tarball to create from the
// rootfs build steps and the compression to use on it
type RootfsTar struct {
	RootfsTarName string `yaml:"name"        json:"RootfsTarName"`
	Compression   string `yaml:"compression" json:"Compression"   jsonschema:"enum=uncompressed,enum=bzip2,enum=gzip,enum=xz,enum=zstd" default:"uncompressed"`
}

// NewMissingURLError fails the image definition parsing when a dict
// requires a URL conditionally based on the value of other keys
// in the dict but does not have one included
func NewMissingURLError(context *gojsonschema.JsonContext, value interface{}, details gojsonschema.ErrorDetails) *MissingURLError {
	err := MissingURLError{}
	err.SetContext(context)
	err.SetType("missing_url_error")
	err.SetDescriptionFormat("When key {{.key}} is specified as {{.value}}, a URL must be provided")
	err.SetValue(value)
	err.SetDetails(details)

	return &err
}

// MissingURLError implements gojsonschema.ErrorType. It is used for custom errors for
// fields that require a url based on the value of other fields
// based on the values in other fields
type MissingURLError struct {
	gojsonschema.ResultErrorFields
}

// NewInvalidPPAError fails the image definition parsing when a private PPA
// is configured with no fingerprint
func NewInvalidPPAError(context *gojsonschema.JsonContext, value interface{}, details gojsonschema.ErrorDetails) *InvalidPPAError {
	err := InvalidPPAError{}
	err.SetContext(context)
	err.SetType("private_ppa_without_fingerprint")
	err.SetDescriptionFormat("Fingerprint is required for private PPAs")
	err.SetValue(value)
	err.SetDetails(details)

	return &err
}

// InvalidPPAError implements gojsonschema.ErrorType. It is used for custom errors
// when a private PPA does not have a fingerprint specified
type InvalidPPAError struct {
	gojsonschema.ResultErrorFields
}

// NewPathNotAbsoluteError fails the image definition parsing when a relative path is given
func NewPathNotAbsoluteError(context *gojsonschema.JsonContext, value interface{}, details gojsonschema.ErrorDetails) *PathNotAbsoluteError {
	err := PathNotAbsoluteError{}
	err.SetContext(context)
	err.SetType("path_not_absolute_error")
	err.SetDescriptionFormat("Key {{.key}} needs to be an absolute path ({{.value}})")
	err.SetValue(value)
	err.SetDetails(details)

	return &err
}

// PathNotAbsoluteError implements gojsonschema.ErrorType. It is used for custom errors for
// fields that should be absolute but are not
type PathNotAbsoluteError struct {
	gojsonschema.ResultErrorFields
}

// NewDependentKeyError fails the image definition parsing when one
// field depends on another being specified
func NewDependentKeyError(context *gojsonschema.JsonContext, value interface{}, details gojsonschema.ErrorDetails) *DependentKeyError {
	err := DependentKeyError{}
	err.SetContext(context)
	err.SetType("dependent_key_error")
	err.SetDescriptionFormat("Key {{.key1}} cannot be used without key {{.key2}}")
	err.SetValue(value)
	err.SetDetails(details)

	return &err
}

// DependentKeyError implements gojsonschema.ErrorType.
// It is used for custom errors for keys that depend on
// other keys being specified
type DependentKeyError struct {
	gojsonschema.ResultErrorFields
}

func (i ImageDefinition) securityMirror() string {
	if i.Architecture == "amd64" || i.Architecture == "i386" {
		return "http://security.ubuntu.com/ubuntu/"
	}
	return i.Rootfs.Mirror
}

// List of valid pockets
const (
	RELEASE_POCKET  = "release"
	SECURITY_POCKET = "security"
	UPDATES_POCKET  = "updates"
	PROPOSED_POCKET = "proposed"
)

// generateLegacySourcesList returns the content to write to the sources.list file
// under the legacy format.
func generateLegacySourcesList(series string, components []string, mirror string, securityMirror string, pocket string) string {
	baseList := fmt.Sprintf("deb %%s %s%%s %s", series, strings.Join(components, " "))

	releaseSourceComment := `# See http://help.ubuntu.com/community/UpgradeNotes for how to upgrade to
# newer versions of the distribution.
`
	updatesSourceComment := `## Major bug fix updates produced after the final release of the
## distribution.
`

	releaseSource := releaseSourceComment + fmt.Sprintf(baseList, mirror, "")
	securitySource := fmt.Sprintf(baseList, securityMirror, "-security")
	updatesSource := updatesSourceComment + fmt.Sprintf(baseList, mirror, "-updates")
	proposedSource := fmt.Sprintf(baseList, mirror, "-proposed")

	sourcesList := make([]string, 0)

	switch pocket {
	case RELEASE_POCKET:
		sourcesList = append(sourcesList, releaseSource)
	case SECURITY_POCKET:
		sourcesList = append(sourcesList, releaseSource, securitySource)
	case UPDATES_POCKET:
		sourcesList = append(sourcesList, releaseSource, securitySource, updatesSource)
	case PROPOSED_POCKET:
		sourcesList = append(sourcesList, releaseSource, securitySource, updatesSource, proposedSource)
	}

	return strings.Join(sourcesList, "\n") + "\n"
}

// LegacyBuildSourcesList returns the content of the /etc/apt/sources.list to be used
// during the build process
func (i *ImageDefinition) LegacyBuildSourcesList() string {
	return i.legacySourcesList(false)
}

// LegacyTargetSourcesList returns the content of the /etc/apt/sources.list for the target
// image
func (i *ImageDefinition) LegacyTargetSourcesList() string {
	return i.legacySourcesList(true)
}

// legacySourcesList returns the content of the /etc/apt/sources.list file in the
// legacy format (not deb822).
func (i *ImageDefinition) legacySourcesList(target bool) string {
	pocket := i.Rootfs.Pocket
	if target {
		pocket = i.Customization.Pocket
	}

	return generateLegacySourcesList(
		i.Series,
		i.Customization.Components,
		i.Rootfs.Mirror,
		i.securityMirror(),
		strings.ToLower(pocket))
}

// generateDeb822Section returns a deb822 section/paragraph to be used in a sources list file
// This function is tailored to what is expected in an official ubuntu image and should not be
// used as is to generate arbitrary deb822 sections.
func generateDeb822Section(mirror string, series string, components []string, pocket string) string {
	sectionTmpl := `Types: deb
URIs: %s
Suites: %s
Components: %s
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg

`

	suites := make([]string, 0)

	switch pocket {
	case SECURITY_POCKET:
		suites = []string{series + "-security"}
	case PROPOSED_POCKET:
		suites = append([]string{series + "-proposed"}, suites...)
		fallthrough
	case UPDATES_POCKET:
		suites = append([]string{series + "-updates"}, suites...)
		fallthrough
	case RELEASE_POCKET:
		suites = append([]string{series}, suites...)
	}

	return fmt.Sprintf(sectionTmpl,
		mirror,
		strings.Join(suites, " "),
		strings.Join(components, " "),
	)
}

var LegacySourcesListComment = `# Ubuntu sources have moved to the /etc/apt/sources.list.d/ubuntu.sources
# file, which uses the deb822 format. Use deb822-formatted .sources files
# to manage package sources in the /etc/apt/sources.list.d/ directory.
# See the sources.list(5) manual page for details.
`

var ubuntuSourceHeader = `## Ubuntu distribution repository
##
## The following settings can be adjusted to configure which packages to use from Ubuntu.
## Mirror your choices (except for URIs and Suites) in the security section below to
## ensure timely security updates.
##
## Types: Append deb-src to enable the fetching of source package.
## URIs: A URL to the repository (you may add multiple URLs)
## Suites: The following additional suites can be configured
##   <name>-updates   - Major bug fix updates produced after the final release of the
##                      distribution.
##   <name>-backports - software from this repository may not have been tested as
##                      extensively as that contained in the main release, although it includes
##                      newer versions of some applications which may provide useful features.
##                      Also, please note that software in backports WILL NOT receive any review
##                      or updates from the Ubuntu security team.
## Components: Aside from main, the following components can be added to the list
##   restricted  - Software that may not be under a free license, or protected by patents.
##   universe    - Community maintained packages. Software in this repository receives maintenance
##                 from volunteers in the Ubuntu community, or a 10 year security maintenance
##                 commitment from Canonical when an Ubuntu Pro subscription is attached.
##   multiverse  - Community maintained of restricted. Software from this repository is
##                 ENTIRELY UNSUPPORTED by the Ubuntu team, and may not be under a free
##                 licence. Please satisfy yourself as to your rights to use the software.
##                 Also, please note that software in multiverse WILL NOT receive any
##                 review or updates from the Ubuntu security team.
##
## See the sources.list(5) manual page for further settings.
`

var ubuntuSourceSecurityHeader = `## Ubuntu security updates. Aside from URIs and Suites,
## this should mirror your choices in the previous section.
`

// deb822SourcesList returns the content of /etc/apt/sources.list.d/ubuntu.sources
// to be used during the build process
func (i *ImageDefinition) Deb822BuildSourcesList() string {
	return i.deb822SourcesList(false)
}

// deb822SourcesList returns the content of /etc/apt/sources.list.d/ubuntu.sources
// for the target image
func (i *ImageDefinition) Deb822TargetSourcesList() string {
	return i.deb822SourcesList(true)
}

// deb822SourcesList returns the content of /etc/apt/sources.list.d/ubuntu.sources
// in the deb822 format.
// The target param defines if the generated sources list will be used in the target image.
func (i *ImageDefinition) deb822SourcesList(target bool) string {
	pocket := i.Rootfs.Pocket
	if target {
		pocket = i.Customization.Pocket
	}
	pocket = strings.ToLower(pocket)

	ubuntuSources := ubuntuSourceHeader + generateDeb822Section(
		i.Rootfs.Mirror,
		i.Series,
		i.Rootfs.Components,
		pocket,
	)

	ubuntuSources += ubuntuSourceSecurityHeader + generateDeb822Section(
		i.securityMirror(),
		i.Series,
		i.Rootfs.Components,
		SECURITY_POCKET,
	)

	return ubuntuSources
}
