package commands

type RootfsCommand struct {
	RootfsArgsPassed ClassicArgs `positional-args:"true" required:"false"`
}
