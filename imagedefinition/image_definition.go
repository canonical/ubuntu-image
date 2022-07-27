/*
Package imagedefinition provides the structure for the
image definition that will be parsed from a YAML file.

The list of all available YAML fields is:

	name:
	display-name:
	revision:
	architecture:
	series:
	class:
	kernel:
	  name:
	  type:
	gadget:
	  url:
	  type:
	  ref:
	  branch:
	model-assertion:
	rootfs:
	  components:
	  archive:
	  flavor:
	  mirror:
	  pocket:
	  archive-tasks:
	  seed:
	      urls:
	      names:
	      vcs:
	      branch:
	  tarball:
	      url:
	      gpg:
	      sha256sum:
	customization:
	  installer:
	    preseeds:
	    layers:
	  cloud-init:
	    meta-data:
	    user-data:
	    network-config:
	  extra-ppas:
	    -
	      name:
	      fingerprint:
	      auth:
	  extra-packages:
	    -
	      name:
	  extra-snaps:
	    -
	      name:
	      channel:
	      store:
	      revision:
	  manual:
	    copy-file:
	      -
	        source:
	        destination:
	    touch-file:
	      -
	        path:
	    execute:
	      -
	        path:
	    add-user:
	      -
	        name:
	        id:
	    add-group:
	      -
	        name:
	        gid:
	    artifacts:
	      img:
	        path:
	      iso:
	        path:
	        xorriso-command:
	      qcow2:
	        path:
	      manifest:
	        path:
	      filelist:
	        path:
	      changelog:
	        path:

Note that not all of these fields are required. An example used to build
Raspberry Pi images is:

	name: ubuntu-server-raspi-arm64
	display-name: Ubuntu Server Raspberry Pi arm64
	revision: 2
	architecture: arm64
	series: jammy
	class: preinstalled
	kernel:
	  name: linux-raspi
	gadget:
	  url: "https://github.com/snapcore/pi-gadget.git"
	  branch: "classic"
	  type: "git"
	model-assertion: pi-generic.model
	rootfs:
	  archive: ubuntu
	  mirror: "http://ports.ubuntu.com/ubuntu/"
	  seed:
	    urls:
	      - "git://git.launchpad.net/~ubuntu-core-dev/ubuntu-seeds/+git/"
	      - "git://git.launchpad.net/~ubuntu-core-dev/ubuntu-seeds/+git/"
	    branch: jammy
	    names:
	      - server
	      - minimal
	      - standard
	      - cloud-image
	      - ubuntu-server-raspi
	customization:
	  cloud-init:
	    user-data:
	      -
	        name: ubuntu
	        password: ubuntu
	  extra-packages:
	    - name: ubuntu-minimal
	    - name: linux-firmware-raspi
	    - name: pi-bluetooth
	artifacts:
	  img:
	      path: raspi.img
	  manifest:
	      path: raspi.manifest
*/
package imagedefinition

// ImageDefinition is the parent struct for the data
// contained within a classic image definition file
type ImageDefinition struct {
	ImageName      string         `yaml:"name"            json:"ImageName"`
	DisplayName    string         `yaml:"display-name"    json:"DisplayName"`
	Revision       int            `yaml:"revision"        json:"Revision,omitempty"`
	Architecture   string         `yaml:"architecture"    json:"Architecture" jsonschema:"enum=amd64,enum=arm64,enum=armhf,enum=ppc64el,enum=s390x,enum=riscv64"`
	Series         string         `yaml:"series"          json:"Series"`
	Kernel         *Kernel        `yaml:"kernel"          json:"Kernel"`
	Gadget         *Gadget        `yaml:"gadget"          json:"Gadget"`
	ModelAssertion string         `yaml:"model-assertion" json:"ModelAssertion,omitempty"`
	Rootfs         *Rootfs        `yaml:"rootfs"          json:"Rootfs"`
	Customization  *Customization `yaml:"customization"   json:"Customization"`
	Artifacts      *Artifact      `yaml:"artifacts"       json:"Artifacts"`
	Class          string         `yaml:"class"           json:"Class"        jsonschema:"enum=preinstalled,enum=cloud,enum=installer"`
}

// Kernel defines the kernel section of the image definition file
type Kernel struct {
	KernelName string `yaml:"name" json:"KernelName" default:"linux"`
	KernelType string `yaml:"type" json:"KernelType,omitempty"`
}

// Gadget defines the gadget section of the image definition file
type Gadget struct {
	Ref          string `yaml:"ref"    json:"Ref,omitempty"`
	GadgetBranch string `yaml:"branch" json:"GadgetBranch,omitempty"`
	GadgetType   string `yaml:"type"   json:"GadgetType"             jsonschema:"enum=git,enum=directory,enum=prebuilt"`
	GadgetURL    string `yaml:"url"    json:"GadgetURL,omitempty"    jsonschema:"type=string,format=uri"`
}

// Rootfs defines the rootfs section of the image definition file
type Rootfs struct {
	Components   []string `yaml:"components"    json:"Components,omitempty"`
	Archive      string   `yaml:"archive"       json:"Archive"                default:"ubuntu"`
	Flavor       string   `yaml:"flavor"        json:"Flavor"                 default:"ubuntu"`
	Mirror       string   `yaml:"mirror"        json:"Mirror"                 default:"http://archive.ubuntu.com/ubuntu/"`
	Pocket       string   `yaml:"pocket"        json:"Pocket"                 jsonschema:"enum=release,enum=Release,enum=updates,enum=Updates,enum=security,enum=Security,enum=proposed,enum=Proposed" default:"release"`
	Seed         *Seed    `yaml:"seed"          json:"Seed,omitempty"         jsonschema:"oneof_required=Seed"`
	Tarball      *Tarball `yaml:"tarball"       json:"Tarball,omitempty"      jsonschema:"oneof_required=Tarball"`
	ArchiveTasks []string `yaml:"archive-tasks" json:"ArchiveTasks,omitempty" jsonschema:"oneof_required=ArchiveTasks"`
}

// Seed defines the seed section of rootfs, which is used to
// build a rootfs via seed germination
type Seed struct {
	SeedBranch string   `yaml:"branch" json:"SeedBranch,omitempty"`
	SeedURLs   []string `yaml:"urls"   json:"SeedURLs"             jsonschema:"type=array,format=uri"`
	Names      []string `yaml:"names"  json:"Names"`
	Vcs        bool     `yaml:"vcs"    json:"Vcs"                  default:"true"`
}

// Tarball defines the tarball section of rootfs, which is used
// to create images from a pre-built rootfs
type Tarball struct {
	TarballURL string `yaml:"url"       json:"TarballURL"    jsonschema:"type=string,format=uri"`
	GPG        string `yaml:"gpg"       json:"GPG,omitempty" jsonschema:"type=string,format=uri"`
	SHA256sum  string `yaml:"sha256sum" json:"SHA256sum,omitempty"`
}

// Customization defines the customization section of the image definition file
type Customization struct {
	Installer     *Installer `yaml:"installer"      json:"Installer,omitempty"`
	CloudInit     *CloudInit `yaml:"cloud-init"     json:"CloudInit,omitempty"`
	ExtraPPAs     []*PPA     `yaml:"extra-ppas"     json:"ExtraPPAs,omitempty"`
	ExtraPackages []*Package `yaml:"extra-packages" json:"ExtraPackages,omitempty"`
	ExtraSnaps    []*Snap    `yaml:"extra-snaps"    json:"ExtraSnaps,omitempty"`
	Manual        *Manual    `yaml:"manual"         json:"Manual,omitempty"`
}

// Installer provides customization options specific to installer images
type Installer struct {
	Preseeds []string `yaml:"preseeds" json:"Preseeds,omitempty"`
	Layers   []string `yaml:"layers"   json:"Layers,omitempty"`
}

// CloudInit provides customizations for running cloud-init
type CloudInit struct {
	MetaData      string      `yaml:"meta-data"      json:"MetaData,omitempty"`
	UserData      *[]UserData `yaml:"user-data"      json:"UserData,omitempty"`
	NetworkConfig string      `yaml:"network-config" json:"NetworkConfig,omitempty"`
}

// UserData defines the user information to be used by cloud-init
type UserData struct {
	UserName     string `yaml:"name"     json:"UserName"`
	UserPassword string `yaml:"password" json:"UserPassword"`
}

// PPA contains information about a public or private PPA
type PPA struct {
	PPAName     string `yaml:"name"         json:"PPAName"`
	Auth        string `yaml:"auth"         json:"Auth,omitempty"`
	Fingerprint string `yaml:"fingerprint"  json:"Fingerprint,omitempty"`
	KeepEnabled bool   `yaml:"keep-enabled" json:"KeepEnabled"           default:"true"`
}

// Package contains information about packages
type Package struct {
	PackageName string `yaml:"name" json:"PackageName"`
}

// Snap contains information about snaps
type Snap struct {
	SnapName     string `yaml:"name"     json:"SnapName"`
	SnapRevision string `yaml:"revision" json:"SnapRevision,omitempty"`
	Store        string `yaml:"store"    json:"Store"                  default:"canonical"`
	Channel      string `yaml:"channel"  json:"Channel"                default:"stable"`
}

// Manual provides manual customization options
type Manual struct {
	CopyFile  []*CopyFile  `yaml:"copy-file"  json:"CopyFile,omitempty"`
	Execute   []*Execute   `yaml:"execute"    json:"Execute,omitempty"`
	TouchFile []*TouchFile `yaml:"touch-file" json:"TouchFile,omitempty"`
	AddGroup  []*AddGroup  `yaml:"add-group"  json:"AddGroup,omitempty"`
	AddUser   []*AddUser   `yaml:"add-user"   json:"AddUser,omitempty"`
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
	UserName string `yaml:"name" json:"UserName"`
	UserID   string `yaml:"id"   json:"UserID,omitempty"`
}

// Artifact contains information about the files that are created
// during and as a result of the image build process
type Artifact struct {
	Img       *Img       `yaml:"img"       json:"Img,omitempty"`
	Iso       *Iso       `yaml:"iso"       json:"Iso,omitempty"`
	Qcow2     *Qcow2     `yaml:"qcow2"     json:"Qcow2,omitempty"`
	Manifest  *Manifest  `yaml:"manifest"  json:"Manifest,omitempty"`
	Filelist  *Filelist  `yaml:"filelist"  json:"Filelist,omitempty"`
	Changelog *Changelog `yaml:"changelog" json:"Changelog,omitempty"`
}

// Img specifies the name of the resulting .img file.
// If left emtpy no .img file will be created
type Img struct {
	ImgPath string `yaml:"path" json:"ImgPath"`
}

// Iso specifies the name of the resulting .iso file
// and optionally the xorrisofs command used to create it.
// If left emtpy no .iso file will be created
type Iso struct {
	IsoPath string `yaml:"path"            json:"IsoPath"`
	Command string `yaml:"xorriso-command" json:"Command,omitempty"`
}

// Qcow2 specifies the name of the resulting .qcow2 file
// If left emtpy no .qcow2 file will be created
type Qcow2 struct {
	Qcow2Path string `yaml:"path" json:"Qcow2Path"`
}

// Manifest specifies the name of the manifest file.
// If left emtpy no manifest file will be created
type Manifest struct {
	ManifestPath string `yaml:"path" json:"ManifestPath"`
}

// Filelist specifies the name of the filelist file.
// If left emtpy no filelist file will be created
type Filelist struct {
	FilelistPath string `yaml:"path" json:"FilelistPath"`
}

// Changelog specifies the name of the changelog file.
// If left emtpy no changelog file will be created
type Changelog struct {
	ChangelogPath string `yaml:"path" json:"ChangelogPath"`
}
