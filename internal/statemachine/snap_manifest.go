package statemachine

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/canonical/ubuntu-image/internal/commands"
)

// prepareFromManifest loads the online manifest and (eventually) drives
// m2cp to fetch the model assertion, bootstrap trust, and resolve snap
// versions to revisions. Every step narrates itself.
func (snapStateMachine *SnapStateMachine) prepareFromManifest() error {
	path := snapStateMachine.Opts.Manifest
	fmt.Printf("=> reading manifest: %s\n", path)
	m, err := commands.LoadOnlineManifest(path)
	if err != nil {
		return err
	}
	fmt.Printf("   model:        %s rev %d\n", m.Model.Name, m.Model.Revision)
	fmt.Printf("   architecture: %s\n", m.Model.Architecture)
	fmt.Printf("   pinned snaps: %d\n", len(m.Snaps))
	for _, s := range m.Snaps {
		fmt.Printf("     - %-32s %s\n", s.Name, s.Version)
	}

	stageDir, err := snapStateMachine.manifestStageDir()
	if err != nil {
		return err
	}
	fmt.Printf("=> manifest staging dir: %s\n", stageDir)

	fmt.Printf("=> fetching model assertion: m2cp store model assertion m2cp %s %d\n",
		m.Model.Name, m.Model.Revision)
	modelPath := filepath.Join(stageDir, "model.assert")
	if err := writeM2cpStdout(modelPath,
		"store", "model", "assertion", "m2cp", m.Model.Name, fmt.Sprintf("%d", m.Model.Revision),
	); err != nil {
		return fmt.Errorf("fetching model assertion: %w", err)
	}
	info, err := os.Stat(modelPath)
	if err != nil {
		return err
	}
	fmt.Printf("   wrote %s (%d bytes)\n", modelPath, info.Size())
	snapStateMachine.Args.ModelAssertion = modelPath

	return fmt.Errorf("next step (fetch trust bundle + InjectTrusted) not yet implemented")
}

// manifestStageDir returns a directory under the active workdir (or a
// fresh temp dir if none was given) where the manifest pipeline can
// stash the model assertion and the trust bundle.
func (snapStateMachine *SnapStateMachine) manifestStageDir() (string, error) {
	base := snapStateMachine.stateMachineFlags.WorkDir
	if base == "" {
		var err error
		base, err = os.MkdirTemp("", "ubuntu-image-manifest-")
		if err != nil {
			return "", fmt.Errorf("creating manifest stage dir: %w", err)
		}
	}
	dir := filepath.Join(base, "manifest")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating manifest stage dir %q: %w", dir, err)
	}
	return dir, nil
}

// writeM2cpStdout runs `m2cp <args...>` and writes its stdout to dst.
// m2cp's `Task-Id:` chatter goes to stderr, so stdout is the clean
// payload (an assertion bundle, in the cases we use this for).
func writeM2cpStdout(dst string, args ...string) error {
	cmd := exec.Command("m2cp", args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("m2cp %s: %w", strings.Join(args, " "), err)
	}
	return os.WriteFile(dst, stdout.Bytes(), 0o644)
}
