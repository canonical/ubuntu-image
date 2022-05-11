package statemachine

// ImageDefinition is the parent struct for the data
// contained within a classic image definition file
type ImageDefinition struct {
	ImageName      string            `yaml:"name"            json:"ImageName"`
	DisplayName    string            `yaml:"display-name"    json:"DisplayName"`
	Revision       int               `yaml:"revision"        json:"Revision,omitempty"`
	Architecture   string            `yaml:"architecture"    json:"Architecture"`
	Series         string            `yaml:"series"          json:"Series"`
	Kernel         *KernelType        `yaml:"kernel"          json:"Kernel"`
	Gadget         *GadgetType        `yaml:"gadget"          json:"Gadget"`
	ModelAssertion string            `yaml:"model-assertion" json:"ModelAssertion,omitempty"`
	Rootfs         *RootfsType        `yaml:"rootfs"          json:"Rootfs"`
	Customization  *CustomizationType `yaml:"customization"   json:"Customization"`
	Artifacts      *ArtifactType      `yaml:"artifacts"       json:"Artifacts"`
	Class          string            `yaml:"class"           json:"Classic" jsonschema:"enum=preinstalled,enum=cloud,enum=installer"`
}

// struct for the kernel section of the image definition file
type KernelType struct {
	KernelName string `yaml:"name" json:"KernelName" default:"linux"`
	KernelType string `yaml:"type" json:"KernelType,omitempty"`
}

// struct for the gadget section of the image definition file
type GadgetType struct {
	Ref          *string `yaml:"ref"    json:"Ref,omitempty"`
	GadgetBranch *string `yaml:"branch" json:"GadgetBranch,omitempty"`
	GadgetType   *string `yaml:"type"   json:"GadgetType"` // TODO enumerate types
	GadgetUrl    *string `yaml:"url"    json:"GadgetUrl,omitempty"  jsonschema:"type=string,format=uri"` // TODO, figure out how to enforce this field exists if type is git
}

// struct for the rootfs-preparation section of the image definition file
type RootfsType struct {
	Components   []*string `yaml:"components"    json:"Components,omitempty"`
	Archive      string    `yaml:"archive"       json:"Archive"                default:"ubuntu"`
	Pocket       string    `yaml:"pocket"        json:"Pocket"                 default:"release"`
	Seed         *SeedType `yaml:"seed"          json:"Seed,omitempty"         jsonschema:"oneof_required=Seed"`
	ArchiveTasks []*string `yaml:"archive-tasks" json:"ArchiveTasks,omitempty" jsonschema:"oneof_required=ArchiveTasks"`
}

// struct for the seed section of rootfs-preparation
type SeedType struct {
	SeedUrl    *string   `yaml:"url"    json:"SeedUrl"    jsonschema:"type=string,format=uri"`
	SeedBranch *string   `yaml:"branch" json:"SeedBranch,omitempty"`
	Names      []*string `yaml:"names"  json:"Names"`
}

// struct for the customization section of the image definition file
type CustomizationType struct {
	Installer     *InstallerType `yaml:"installer"      json:"Installer,omitempty"`
	CloudInit     *CloudInitType `yaml:"cloud-init"     json:"CloudInit,omitempty"`
	ExtraPPAs     []*PPAType     `yaml:"extra-ppas"     json:"ExtraPPAs,omitempty"`
	ExtraPackages []*PackageType `yaml:"extra-packages" json:"ExtraPackages,omitempty"`
	ExtraSnaps    []*SnapType    `yaml:"extra-snaps"    json:"ExtraSnaps,omitempty"`
	Manual        *ManualType    `yaml:"manual"         json:"Manual,omitempty"`
}

// struct for the installer section of customization
type InstallerType struct {
	Preseeds []*string `yaml:"preseeds" json:"Preseeds,omitempty"`
	Layers   []*string `yaml:"layers"   json:"Layers,omitempty"`
}

// struct for the cloud-init section of customization
type CloudInitType struct {
	MetaData      *string         `yaml:"meta-data"      json:"MetaData,omitempty"`
	UserData      *[]UserDataType `yaml:"user-data"      json:"UserData,omitempty"`
	NetworkConfig *string         `yaml:"network-config" json:"NetworkConfig,omitempty"`
}

// struct for cloud-init user data
type UserDataType struct {
	UserName *string     `yaml:"name"     json:"UserName"`
	UserPassword *string `yaml:"password" json:"UserPassword"`
}

// struct for the extra-ppas section of customization
type PPAType struct {
	PPAName     *string `yaml:"name"         json:"PPAName"`
	Auth        *string `yaml:"auth"         json:"Auth,omitempty"`
	Fingerprint *string `yaml:"fingerprint"  json:"Fingerprint,omitempty"`
	KeepEnabled *bool   `yaml:"keep-enabled" json:"KeepEnabled"           default:"true"`
}

// struct for the extra-packages section of customization
type PackageType struct {
	PackageName *string `yaml:"name" json:"PackageName"`
}

// struct for the extra-snaps section of customization
type SnapType struct {
	SnapName     *string `yaml:"name"     json:"SnapName"`
	SnapRevision *string `yaml:"revision" json:"SnapRevision,omitempty"`
	Store        *string `yaml:"store"    json:"Store"                  default:"canonical"`
	Channel      *string `yaml:"channel"  json:"Channel"                default:"stable"`
}

// struct for the manual section of customization
type ManualType struct {
	CopyFile  []*CopyFileType  `yaml:"copy-file"  json:"CopyFile,omitempty"`
	Execute   []*ExecuteType   `yaml:"execute"    json:"Execute,omitempty"`
	TouchFile []*TouchFileType `yaml:"touch-file" json:"TouchFile,omitempty"`
	AddGroup  []*AddGroupType  `yaml:"add-group"  json:"AddGroup,omitempty"`
	AddUser   []*AddUserType   `yaml:"add-user"   json:"AddUser,omitempty"`
}

// struct for the copy-file manual customization
type CopyFileType struct {
	Dest   *string `yaml:"destination" json:"Dest"`
	Source *string `yaml:"source"      json:"Source"`
}

// struct for the execute manual customization
type ExecuteType struct {
	ExecutePath *string `yaml:"path" json:"ExecutePath"`
}

// struct for the touch-file manual customization
type TouchFileType struct {
	TouchPath *string `yaml:"path" json:"TouchPath"`
}

// struct for the add-group manual customization
type AddGroupType struct {
	GroupName *string `yaml:"name" json:"GroupName"`
	GroupId   *string `yaml:"id"   json:"GroupId,omitempty"`
}

// struct for the add-user manual customization
type AddUserType struct {
	UserName *string `yaml:"name" json:"UserName"`
	UserId   *string `yaml:"id"   json:"UserId,omitempty"`
}

// struct for the artifacts section of the image definition file
type ArtifactType struct {
	Img       *ImgType       `yaml:"img"        json:"Img,omitempty"`
	Iso       *IsoType       `yaml:"iso"        json:"Iso,omitempty"`
	Qcow2     *Qcow2Type     `yaml:"qcow2"     json:"Qcow2,omitempty"`
	Manifest  *ManifestType  `yaml:"manifest"  json:"Manifest,omitempty"`
	Filelist  *FilelistType  `yaml:"filelist"  json:"Filelist,omitempty"`
	Changelog *ChangelogType `yaml:"changelog" json:"Changelog,omitempty"`
}

// struct for the img section of artifacts
type ImgType struct {
	ImgPath *string `yaml:"path" json:"ImgPath"`
}

// struct for the ISO section of artifacts
type IsoType struct {
	IsoPath *string `yaml:"path"    json:"IsoPath"`
	Command *string `yaml:"command" json:"Command,omitempty"`
}

// struct for the qcow2 section of artifacts
type Qcow2Type struct {
	Qcow2Path *string `yaml:"path" json:"Qcow2Path"`
}

// struct for the manifest section of artifacts
type ManifestType struct {
	ManifestPath *string `yaml:"path" json:"ManifestPath"`
}

// struct for the filelist section of artifacts
type FilelistType struct {
	FilelistPath *string `yaml:"path" json:"FilelistPath"`
}

// struct for the changelog section of artifacts
type ChangelogType struct {
	ChangelogPath *string `yaml:"path" json:"ChangelogPath"`
}
