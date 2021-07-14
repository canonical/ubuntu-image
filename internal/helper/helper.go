package helper

import (
	"io"
	"os"

	"github.com/canonical/ubuntu-image/internal/commands"
)

// CaptureStd returns an io.Reader to read what was printed, and teardown
func CaptureStd(toCap **os.File) (io.Reader, func(), error) {
	stdCap, stdCapW, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	oldStdCap := *toCap
	*toCap = stdCapW
	closed := false
	return stdCap, func() {
		// only teardown once
		if closed {
			return
		}
		*toCap = oldStdCap
		stdCapW.Close()
		closed = true
	}, nil
}

// Setup is a helper test function to clear args between test cases
func Setup() func() {

	origStateMachineOpts := commands.StateMachineOptsPassed
	origCommonOpts := commands.CommonOptsPassed
	origUbuntuImageCommand := commands.UICommand
	return func() {
		commands.StateMachineOptsPassed = origStateMachineOpts
		commands.CommonOptsPassed = origCommonOpts
		commands.UICommand = origUbuntuImageCommand
	}
}
