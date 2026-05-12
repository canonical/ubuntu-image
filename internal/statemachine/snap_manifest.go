package statemachine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/snapcore/snapd/asserts"
	"github.com/snapcore/snapd/asserts/sysdb"
	"github.com/snapcore/snapd/seed/seedwriter"
	"github.com/snapcore/snapd/snap"

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

	if err := injectTrustFromM2cp(); err != nil {
		return err
	}

	sm, err := resolveRevisionsViaM2cp(m.Model.Architecture, m.Snaps)
	if err != nil {
		return err
	}
	snapStateMachine.manifestSeedManifest = sm

	return fmt.Errorf("next step (configure store URL via SNAPPY_FORCE_API_URL) not yet implemented")
}

// resolveRevisionsViaM2cp looks each pinned snap up via
// `m2cp store snap info <name> <arch> --json`, finds the revision
// whose snapVersion matches the manifest's pinned version, and
// returns a seedwriter.Manifest with all of them set as the only
// allowed revision for that snap.
func resolveRevisionsViaM2cp(arch string, pins []commands.OnlineManifestSnap) (*seedwriter.Manifest, error) {
	fmt.Printf("=> resolving %d snap revisions via m2cp (arch=%s)\n", len(pins), arch)
	sm := seedwriter.NewManifest()
	for _, p := range pins {
		rev, err := resolveSnapRevision(p.Name, arch, p.Version)
		if err != nil {
			return nil, err
		}
		if err := sm.SetAllowedSnapRevision(p.Name, snap.R(rev)); err != nil {
			return nil, fmt.Errorf("pinning %s revision %d: %w", p.Name, rev, err)
		}
		fmt.Printf("   %-32s %-10s -> revision %d\n", p.Name, p.Version, rev)
	}
	return sm, nil
}

// snapInfoResponse is the minimal slice of `m2cp store snap info --json`
// we need to map a (name, version) pin to a concrete revision.
type snapInfoResponse struct {
	Output struct {
		SnapDeclaration struct {
			SnapName      string             `json:"snapName"`
			SnapRevisions []snapInfoRevision `json:"snapRevisions"`
		} `json:"snapDeclaration"`
	} `json:"output"`
}

type snapInfoRevision struct {
	Revision    int    `json:"revision"`
	SnapVersion string `json:"snapVersion"`
}

func resolveSnapRevision(name, arch, version string) (int, error) {
	cmd := exec.Command("m2cp", "store", "snap", "info", name, arch, "--json")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("m2cp store snap info %s %s: %w", name, arch, err)
	}
	var resp snapInfoResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return 0, fmt.Errorf("parsing snap info JSON for %s: %w", name, err)
	}
	for _, r := range resp.Output.SnapDeclaration.SnapRevisions {
		if r.SnapVersion == version {
			return r.Revision, nil
		}
	}
	return 0, fmt.Errorf("snap %q has no revision with version %q (arch %s)", name, version, arch)
}

// injectTrustFromM2cp pulls the tenant's trust bundle (account +
// account-keys, all signed by the appstore root) via m2cp and installs
// them into snapd's sysdb so seedwriter chain verification accepts
// assertions signed by this store. The bundle stays in memory --
// nothing is written to disk.
func injectTrustFromM2cp() error {
	fmt.Printf("=> fetching trust bundle: m2cp store system assertion\n")
	cmd := exec.Command("m2cp", "store", "system", "assertion")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("m2cp store system assertion: %w", err)
	}
	dec := asserts.NewDecoder(&stdout)
	var trusted []asserts.Assertion
	for {
		a, err := dec.Decode()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("decoding trust bundle: %w", err)
		}
		describeAssertion(a)
		trusted = append(trusted, a)
	}
	if len(trusted) == 0 {
		return fmt.Errorf("trust bundle is empty")
	}
	sysdb.InjectTrusted(trusted)
	fmt.Printf("   injected %d trusted assertions into snapd sysdb\n", len(trusted))
	return nil
}

func describeAssertion(a asserts.Assertion) {
	switch v := a.(type) {
	case *asserts.Account:
		fmt.Printf("   account      account-id=%s display-name=%q validation=%s\n",
			v.AccountID(), v.DisplayName(), v.Validation())
	case *asserts.AccountKey:
		fmt.Printf("   account-key  account-id=%s name=%s sha3-384=%s\n",
			v.AccountID(), v.Name(), v.PublicKeyID())
	case *asserts.Store:
		url := ""
		if u := v.URL(); u != nil {
			url = u.String()
		}
		fmt.Printf("   store        store=%s operator-id=%s url=%q\n",
			v.Store(), v.OperatorID(), url)
	default:
		fmt.Printf("   %s\n", a.Type().Name)
	}
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
