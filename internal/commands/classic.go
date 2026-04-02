package commands

// ClassicArgs holds the gadget tree. positional arguments need their own struct
type ClassicArgs struct {
	ImageDefinition string `positional-arg-name:"image_definition" description:"Classic image definition file. This is used to define what should be in the image and the outputs that are created."`
}

type ClassicCommand struct {
	ClassicArgsPassed ClassicArgs `positional-args:"true" required:"false"`
}
