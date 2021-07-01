package commands

// positional arguments need their own struct
type snapArgs struct {
	ModelAssertion string `positional-arg-name:"model_assertion" description:"Path to the model assertion file. This argument must be given unless the state machine is being resumed, in which case it cannot be given."`
}

// set up the options that are specific to the snap command
type snapOpts struct {
	Snap               string `long:"snap" description:"Install an extra snap. This is passed through to \"snap prepare-image\". The snap argument can include additional information about the channel and/or risk with the following syntax: <snap>=<channel|risk>" value-name:"SNAP"`
	Channel            string `short:"c" long:"channel" description:"The default snap channel to use" value-name:"CHANNEL"`
	DisableConsoleConf bool   `long:"disable-console-conf" description:"Disable console-conf on the resulting image."`
}

type snapCommand struct {
	SnapArgs snapArgs `positional-args:"true" required:"false"`
	SnapOpts snapOpts
}

var snap snapCommand
