package statemachine

import (
	"testing"

	"github.com/canonical/ubuntu-image/internal/commands"
	"github.com/canonical/ubuntu-image/internal/helper"
)

/* This function tests command line input for classic images */
func TestFailedStdoutStderrCapture(t *testing.T) {
        testCases := []struct {
                name       string
		project    string
		filesystem string
        }{
                {"neither_project_nor_filesystem", "", ""},
                {"both_project_and_filesystem", "ubuntu-cpc", "/tmp"},
        }
        for _, tc := range testCases {
                t.Run("test "+tc.name, func(t *testing.T) {
                        restoreArgs := helper.Setup()
                        defer restoreArgs()
			var stateMachine classicStateMachine
			commands.UICommand.Classic.ClassicOptsPassed.Project = tc.project
			commands.UICommand.Classic.ClassicOptsPassed.Filesystem = tc.filesystem
			if err := stateMachine.Setup(); err == nil {
				t.Error("Expected an error but there was none")
			}
		})
	}
}
