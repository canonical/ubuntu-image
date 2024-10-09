package commands

// SnapArgs holds the model Assertion
type SnapArgs struct {
	ModelAssertion string `positional-arg-name:"model_assertion" description:"Path to the model assertion file. This argument must be given unless the state machine is being resumed, in which case it cannot be given."`
}

// SnapOpts holds all flags that are specific to the snap command
type SnapOpts struct {
	DisableConsoleConf        bool           `long:"disable-console-conf" description:"Disable console-conf on the resulting image."`
	FactoryImage              bool           `long:"factory-image" description:"Hint that the image is meant to boot in a device factory."`
	Preseed                   bool           `long:"preseed" description:"Preseed the image (UC20 only)."`
	AppArmorKernelFeaturesDir string         `long:"apparmor-features-dir" description:"Optional path to apparmor kernel features directory"`
	PreseedSignKey            string         `long:"preseed-sign-key" description:"Name of the key to use to sign preseed assertion, otherwise use the default key"`
	Snaps                     []string       `long:"snap" description:"Install extra snaps. These are passed through to \"snap prepare-image\". The snap argument can include additional information about the channel and/or risk with the following syntax: <snap>=<channel|risk>" value-name:"SNAP"`
	Components                []string       `long:"comp" description:"Install extra components. These are passed through to \"snap prepare-image\"." value-name:"COMPONENT"`
	CloudInit                 string         `long:"cloud-init" description:"cloud-config data to be copied to the image" value-name:"USER-DATA-FILE"`
	Revisions                 map[string]int `long:"revision" description:"The revision of a specific snap to install in the image." value-name:"REVISION"`
	SysfsOverlay              string         `long:"sysfs-overlay" description:"The optional sysfs overlay to used for preseeding. Directories from /sys/class/* and /sys/devices/platform will be bind-mounted to the chroot when preseeding"`
}

type SnapCommand struct {
	SnapArgsPassed SnapArgs `positional-args:"true" required:"false"`
	SnapOptsPassed SnapOpts
}
