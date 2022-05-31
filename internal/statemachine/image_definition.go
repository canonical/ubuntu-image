package statemachine

import (
	"github.com/xeipuuv/gojsonschema"
)

// ImageDefinition is the parent struct for the data
// contained within a classic image definition file
type ImageDefinition struct {
	ImageName      string             `yaml:"name"            json:"ImageName"`
	DisplayName    string             `yaml:"display-name"    json:"DisplayName"`
	Revision       int                `yaml:"revision"        json:"Revision,omitempty"`
	Architecture   string             `yaml:"architecture"    json:"Architecture"`
	Series         string             `yaml:"series"          json:"Series"`
	Kernel         *KernelType        `yaml:"kernel"          json:"Kernel"`
	Gadget         *GadgetType        `yaml:"gadget"          json:"Gadget"`
	ModelAssertion string             `yaml:"model-assertion" json:"ModelAssertion,omitempty"`
	Rootfs         *RootfsType        `yaml:"rootfs"          json:"Rootfs"`
	Customization  *CustomizationType `yaml:"customization"   json:"Customization"`
	Artifacts      *ArtifactType      `yaml:"artifacts"       json:"Artifacts"`
	Class          string             `yaml:"class"           json:"Class" jsonschema:"enum=preinstalled,enum=cloud,enum=installer"`
}

// KernelType defines the kernel section of the image definition file
type KernelType struct {
	KernelName string `yaml:"name" json:"KernelName" default:"linux"`
	KernelType string `yaml:"type" json:"KernelType,omitempty"`
}

// GadgetType defines the gadget section of the image definition file
type GadgetType struct {
	Ref          string `yaml:"ref"    json:"Ref,omitempty"`
	GadgetBranch string `yaml:"branch" json:"GadgetBranch,omitempty"`
	GadgetType   string `yaml:"type"   json:"GadgetType"             jsonschema:"enum=git,enum=directory,enum=prebuilt"`
	GadgetURL    string `yaml:"url"    json:"GadgetURL,omitempty"    jsonschema:"type=string,format=uri"`
}

// RootfsType defines the rootfs section of the image definition file
type RootfsType struct {
	AptConfig    *AptConfigType `yaml:"apt-config"    json:"AptConfig,omitempty"`
	Seed         *SeedType      `yaml:"seed"          json:"Seed,omitempty"         jsonschema:"oneof_required=Seed"`
	Tarball      *TarballType   `yaml:"tarball"       json:"Tarball,omitempty"      jsonschema:"oneof_required=Tarball"`
	ArchiveTasks []string       `yaml:"archive-tasks" json:"ArchiveTasks,omitempty" jsonschema:"oneof_required=ArchiveTasks"`
}

// AptConfigType defines the apt configuration to use while
// building the rootfs
type AptConfigType struct {
	Components []*string `yaml:"components"    json:"Components,omitempty"`
	Archive    string    `yaml:"archive"       json:"Archive"                default:"ubuntu"`
	Pocket     string    `yaml:"pocket"        json:"Pocket"                 default:"release"`
}

// SeedType defines the seed section of rootfs, which is used to
// build a rootfs via seed germination
type SeedType struct {
	SeedURL    string   `yaml:"url"    json:"SeedURL"    jsonschema:"type=string,format=uri"`
	SeedBranch string   `yaml:"branch" json:"SeedBranch,omitempty"`
	Names      []string `yaml:"names"  json:"Names"`
}

// TarballType defines the tarball section of rootfs, which is used
// to create images from a pre-built rootfs
type TarballType struct {
	TarballURL string `yaml:"url"       json:"TarballURL"    jsonschema:"type=string,format=uri"`
	GPG        string `yaml:"gpg"       json:"GPG,omitempty" jsonschema:"type=string,format=uri"`
	SHA256sum  string `yaml:"sha256sum" json:"SHA256sum,omitempty"`
}

// CustomizationType defines the customization section of the image definition file
type CustomizationType struct {
	Installer     *InstallerType `yaml:"installer"      json:"Installer,omitempty"`
	CloudInit     *CloudInitType `yaml:"cloud-init"     json:"CloudInit,omitempty"`
	ExtraPPAs     []*PPAType     `yaml:"extra-ppas"     json:"ExtraPPAs,omitempty"`
	ExtraPackages []*PackageType `yaml:"extra-packages" json:"ExtraPackages,omitempty"`
	ExtraSnaps    []*SnapType    `yaml:"extra-snaps"    json:"ExtraSnaps,omitempty"`
	Manual        *ManualType    `yaml:"manual"         json:"Manual,omitempty"`
}

// InstallerType provides customization options specific to installer images
type InstallerType struct {
	Preseeds []string `yaml:"preseeds" json:"Preseeds,omitempty"`
	Layers   []string `yaml:"layers"   json:"Layers,omitempty"`
}

// CloudInitType provides customizations for running cloud-init
type CloudInitType struct {
	MetaData      string          `yaml:"meta-data"      json:"MetaData,omitempty"`
	UserData      *[]UserDataType `yaml:"user-data"      json:"UserData,omitempty"`
	NetworkConfig string          `yaml:"network-config" json:"NetworkConfig,omitempty"`
}

// UserDataType defines the user information to be used by cloud-init
type UserDataType struct {
	UserName     string `yaml:"name"     json:"UserName"`
	UserPassword string `yaml:"password" json:"UserPassword"`
}

// PPAType contains information about a public or private PPA
type PPAType struct {
	PPAName     string `yaml:"name"         json:"PPAName"`
	Auth        string `yaml:"auth"         json:"Auth,omitempty"`
	Fingerprint string `yaml:"fingerprint"  json:"Fingerprint,omitempty"`
	KeepEnabled bool   `yaml:"keep-enabled" json:"KeepEnabled"           default:"true"`
}

// PackageType contains information about packages
type PackageType struct {
	PackageName string `yaml:"name" json:"PackageName"`
}

// SnapType contains information about snaps
type SnapType struct {
	SnapName     string `yaml:"name"     json:"SnapName"`
	SnapRevision string `yaml:"revision" json:"SnapRevision,omitempty"`
	Store        string `yaml:"store"    json:"Store"                  default:"canonical"`
	Channel      string `yaml:"channel"  json:"Channel"                default:"stable"`
}

// ManualType provides manual customization options
type ManualType struct {
	CopyFile  []*CopyFileType  `yaml:"copy-file"  json:"CopyFile,omitempty"`
	Execute   []*ExecuteType   `yaml:"execute"    json:"Execute,omitempty"`
	TouchFile []*TouchFileType `yaml:"touch-file" json:"TouchFile,omitempty"`
	AddGroup  []*AddGroupType  `yaml:"add-group"  json:"AddGroup,omitempty"`
	AddUser   []*AddUserType   `yaml:"add-user"   json:"AddUser,omitempty"`
}

// CopyFileType allows users to copy files into the rootfs of an image
type CopyFileType struct {
	Dest   string `yaml:"destination" json:"Dest"`
	Source string `yaml:"source"      json:"Source"`
}

// ExecuteType allows users to execute a script in the rootfs of an image
type ExecuteType struct {
	ExecutePath string `yaml:"path" json:"ExecutePath"`
}

// TouchFileType allows users to touch a file in the rootfs of an image
type TouchFileType struct {
	TouchPath string `yaml:"path" json:"TouchPath"`
}

// AddGroupType allows users to add a group in the image that is being built
type AddGroupType struct {
	GroupName string `yaml:"name" json:"GroupName"`
	GroupID   string `yaml:"id"   json:"GroupID,omitempty"`
}

// AddUserType allows users to add a user in the image that is being built
type AddUserType struct {
	UserName string `yaml:"name" json:"UserName"`
	UserID   string `yaml:"id"   json:"UserID,omitempty"`
}

// ArtifactType contains information about the files that are created
// during and as a result of the image build process
type ArtifactType struct {
	Img       *ImgType       `yaml:"img"       json:"Img,omitempty"`
	Iso       *IsoType       `yaml:"iso"       json:"Iso,omitempty"`
	Qcow2     *Qcow2Type     `yaml:"qcow2"     json:"Qcow2,omitempty"`
	Manifest  *ManifestType  `yaml:"manifest"  json:"Manifest,omitempty"`
	Filelist  *FilelistType  `yaml:"filelist"  json:"Filelist,omitempty"`
	Changelog *ChangelogType `yaml:"changelog" json:"Changelog,omitempty"`
}

// ImgType specifies the name of the resulting .img file.
// If left emtpy no .img file will be created
type ImgType struct {
	ImgPath string `yaml:"path" json:"ImgPath"`
}

// IsoType specifies the name of the resulting .iso file
// and optionally the xorrisofs command used to create it.
// If left emtpy no .iso file will be created
type IsoType struct {
	IsoPath string `yaml:"path"            json:"IsoPath"`
	Command string `yaml:"xorriso-command" json:"Command,omitempty"`
}

// Qcow2Type specifies the name of the resulting .qcow2 file
// If left emtpy no .qcow2 file will be created
type Qcow2Type struct {
	Qcow2Path string `yaml:"path" json:"Qcow2Path"`
}

// ManifestType specifies the name of the manifest file.
// If left emtpy no manifest file will be created
type ManifestType struct {
	ManifestPath string `yaml:"path" json:"ManifestPath"`
}

// FilelistType specifies the name of the filelist file.
// If left emtpy no filelist file will be created
type FilelistType struct {
	FilelistPath string `yaml:"path" json:"FilelistPath"`
}

// ChangelogType specifies the name of the changelog file.
// If left emtpy no changelog file will be created
type ChangelogType struct {
	ChangelogPath string `yaml:"path" json:"ChangelogPath"`
}

func newMissingURLError(context *gojsonschema.JsonContext, value interface{}, details gojsonschema.ErrorDetails) *MissingURLError {
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
