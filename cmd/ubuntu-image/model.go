package main

import (
	"fmt"
	"os"

	"github.com/canonical/ubuntu-image/internal/commands"
)

// runModelCommand implements `ubuntu-image model <manifest.yaml>`:
// loads the recipe, renders the model.json the appstore push step
// would consume, and writes it to stdout. Pure transformation -- no
// network, no build. Useful for inspecting what we'd send before
// committing to a build.
func runModelCommand(manifestPath string) error {
	if manifestPath == "" {
		return fmt.Errorf("manifest YAML path is required")
	}
	m, err := commands.LoadOnlineManifest(manifestPath)
	if err != nil {
		return err
	}
	body, err := commands.RenderModelJSON(m)
	if err != nil {
		return fmt.Errorf("rendering model.json: %w", err)
	}
	if _, err := os.Stdout.Write(body); err != nil {
		return err
	}
	if len(body) > 0 && body[len(body)-1] != '\n' {
		fmt.Println()
	}
	return nil
}
