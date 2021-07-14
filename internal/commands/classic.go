package commands

// ClassicArgs holds the gadget tree. positional arguments need their own struct
type ClassicArgs struct {
	GadgetTree string `positional-arg-name:"gadget_tree" description:"Gadget tree. This is a tree equivalent to an unpacked and primed gadget snap at core image build time."`
}

// ClassicOpts holds all flags that are specific to the classic command
type ClassicOpts struct {
	Project      string   `short:"p" long:"project" description:"Project name to be specified to livecd-rootfs. Mutually exclusive with --filesystem." value-name:"PROJECT"`
	Filesystem   string   `short:"f" long:"filesystem" description:"Unpacked Ubuntu filesystem to be copied to the system partition. Mutually exclusive with --project." value-name:"FILESYSTEM"`
	Suite        string   `short:"s" long:"suite" description:"Distribution name to be specified to livecd-rootfs." value-name:"SUITE"`
	Arch         string   `short:"a" long:"arch" description:"CPU architecture to be specified to livecd-rootfs. default value is builder arch." value-name:"CPU-ARCHITECTURE"`
	Subproject   string   `long:"subproject" description:"Sub project name to be specified to livecd-rootfs." value-name:"SUBPROJECT"`
	Subarch      string   `long:"subarch" description:"Sub architecture to be specified to livecd-rootfs." value-name:"SUBARCH"`
	WithProposed bool     `long:"with-proposed" description:"Proposed repo to install, This is passed through to livecd-rootfs."`
	ExtraPPAs    []string `long:"extra-ppas" description:"Extra ppas to install. This is passed through to livecd-rootfs."`
}

type classicCommand struct {
	ClassicArgsPassed ClassicArgs `positional-args:"true" required:"false"`
	ClassicOptsPassed ClassicOpts
}

var classic classicCommand
