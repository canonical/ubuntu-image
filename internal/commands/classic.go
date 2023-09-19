package commands

// ClassicArgs holds the gadget tree. positional arguments need their own struct
type ClassicArgs struct {
	ImageDefinition string `positional-arg-name:"image_definition" description:"Classic image definition file. This is used to define how the image is built and the outputs that are created."`
}

// ClassicOpts holds all flags that are specific to the classic command
type ClassicOpts struct {
	AptParams []string `long:"apt-params" description:"Any additional APT specific configuration needed for the image build."` // TODO: is this used?
}
