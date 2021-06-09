package main

import (
	"fmt"
	"os"

	"github.com/canonical/ubuntu-image/commands"
	"github.com/jessevdk/go-flags"
)

var osExit = os.Exit

var stateMachineLongDesc = `Options for controlling the internal state machine.
Other than -w, these options are mutually exclusive. When -u or -t is given,
the state machine can be resumed later with -r, but -w must be given in that
case since the state is saved in a .ubuntu-image.pck file in the working directory.`


func main() {

	parser := flags.NewParser(&commands.UbuntuImageCommand, flags.Default)
	parser.AddGroup("[State Machine Options]", stateMachineLongDesc, &commands.StateMachineOpts)
	parser.AddGroup("[Common Options]", "Options common to both commands", &commands.CommonOpts)

	if _, err := parser.Parse(); err != nil {
		fmt.Printf("Error %s\n", err.Error())
		osExit(1)
	}

	if os.Args[1] == "snap" {
		fmt.Println("snap functionality to be added")
	} else if os.Args[1] == "classic" {
		fmt.Println("classic functionality to be added")
	}
}
