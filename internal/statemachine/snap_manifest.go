package statemachine

import (
	"fmt"

	"github.com/canonical/ubuntu-image/internal/commands"
)

// prepareFromManifest loads the online manifest and (eventually) drives
// m2cp to fetch the model assertion, bootstrap trust, and resolve snap
// versions to revisions. For now it just parses and reports what was
// found so test.sh can move past flag parsing.
func (snapStateMachine *SnapStateMachine) prepareFromManifest() error {
	m, err := commands.LoadOnlineManifest(snapStateMachine.Opts.Manifest)
	if err != nil {
		return err
	}

	fmt.Printf("manifest: %s rev %d (%s) with %d pinned snaps\n",
		m.Model.Name, m.Model.Revision, m.Model.Architecture, len(m.Snaps))
	for _, s := range m.Snaps {
		fmt.Printf("  - %s @ %s\n", s.Name, s.Version)
	}

	return fmt.Errorf("manifest mode: next step (fetch model via m2cp) not yet implemented")
}
