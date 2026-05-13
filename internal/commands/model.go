package commands

// ModelArgs holds the positional argument for the `model` subcommand.
type ModelArgs struct {
	Manifest string `positional-arg-name:"manifest.yaml" description:"Path to the L-IoT recipe YAML to render"`
}

// ModelCommand is the `ubuntu-image model <manifest.yaml>` subcommand:
// loads the recipe, generates the model.json the appstore push step
// would consume, and writes it to stdout. Pure transformation -- no
// network, no build.
type ModelCommand struct {
	ModelArgsPassed ModelArgs `positional-args:"true"`
}
