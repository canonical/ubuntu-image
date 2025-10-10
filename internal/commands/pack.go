package commands

// PackOpts holds all flags that are specific to the pack command
type PackOpts struct {
	ArtifactType string `long:"artifact-type" description:"Type of the resulting disk image file." required:"true" default:"raw" choice:"raw"`
	GadgetDir    string `long:"gadget-dir" description:"Directory containing the gadget tree. The gadget.yaml file is expected to be in a meta subdirectory." required:"true"`
	RootfsDir    string `long:"rootfs-dir" description:"Directory containing the rootfs" required:"true"`
	Architecture string `long:"architecture" description:"CPU architecture of the image. Default to the host architecture executing the tool." required:"false"`
}

type PackCommand struct {
	PackOptsPassed PackOpts `required:"true"`
}
