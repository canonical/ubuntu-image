package commands

// SnapArgs holds the model Assertion
type SnapArgs struct {
	ModelAssertion string `positional-arg-name:"model_assertion" description:"Path to the model assertion file. This argument must be given unless the state machine is being resumed, in which case it cannot be given."`
}

// SnapOpts holds all flags that are specific to the snap command
type SnapOpts struct {
	DisableConsoleConf        bool     `long:"disable-console-conf" description:"Disable console-conf on the resulting image."`
	FactoryImage              bool     `long:"factory-image" description:"Hint that the image is meant to boot in a device factory."`
	Preseed                   bool     `long:"preseed" description:"Pressed the image (UC20 only)."`
	AppArmorKernelFeaturesDir string   `long:"apparmor-features-dir" description:"Optional path to apparmor kernel features directory"`
	PreseedSignKey            string   `long:"preseed-sign-key" description:"Name of the key to use to sign preseed assertion, otherwise use the default key"`
	Snaps                     []string `long:"snap" description:"Install extra snaps. These are passed through to \"snap prepare-image\". The snap argument can include additional information about the channel and/or risk with the following syntax: <snap>=<channel|risk>" value-name:"SNAP"`
}

type snapCommand struct {
	SnapArgsPassed SnapArgs `positional-args:"true" required:"false"`
	SnapOptsPassed SnapOpts
}
