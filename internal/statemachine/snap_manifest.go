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
	"strconv"
	"strings"
	"time"

	"github.com/snapcore/snapd/asserts"
	"github.com/snapcore/snapd/asserts/sysdb"
	"github.com/snapcore/snapd/image"
	"github.com/snapcore/snapd/seed/seedwriter"
	"github.com/snapcore/snapd/snap"
	"gopkg.in/yaml.v2"

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
	if err := m.ValidateForBuild(); err != nil {
		return err
	}
	fmt.Printf("   model:        %s\n", m.Model.Name)
	fmt.Printf("   architecture: %s\n", m.Model.Architecture)
	fmt.Printf("   base:         %s\n", m.Model.Base)
	fmt.Printf("   grade:        %s\n", m.Model.Grade)
	fmt.Printf("   model snaps:  %d\n", len(m.Snaps))
	for _, s := range m.Snaps {
		fmt.Printf("     - %-32s %-9s %s\n", s.Name, s.Type, s.Version)
	}
	if len(m.ExtraSnaps) > 0 {
		fmt.Printf("   extra snaps:  %d\n", len(m.ExtraSnaps))
		for _, s := range m.ExtraSnaps {
			fmt.Printf("     - %-32s %s\n", s.Name, s.Version)
		}
	}

	stageDir, err := snapStateMachine.manifestStageDir()
	if err != nil {
		return err
	}
	fmt.Printf("=> manifest staging dir: %s\n", stageDir)

	jsonPath := filepath.Join(stageDir, "model.json")
	if err := writeModelJSON(jsonPath, m); err != nil {
		return fmt.Errorf("generating model.json: %w", err)
	}
	revision, err := pushModelGetRevision(jsonPath, fmt.Sprintf("ubuntu-image build %s", m.Model.Name))
	if err != nil {
		return err
	}
	snapStateMachine.manifestModelRevision = revision

	fmt.Printf("=> fetching model assertion: m2cp store model assertion m2cp %s %d\n",
		m.Model.Name, revision)
	modelPath := filepath.Join(stageDir, "model.assert")
	if err := writeM2cpStdout(modelPath,
		"store", "model", "assertion", "m2cp", m.Model.Name, strconv.Itoa(revision),
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

	allPins := append([]commands.OnlineManifestSnap{}, m.Snaps...)
	allPins = append(allPins, m.ExtraSnaps...)

	sm, err := resolveRevisionsViaM2cp(m.Model.Architecture, allPins)
	if err != nil {
		return err
	}
	snapStateMachine.manifestSeedManifest = sm

	// Inject extras into image.Options.Snaps via SnapOpts. The
	// seedwriter will treat these as additional snaps to include in
	// the seed beyond what the model declares. Revision pinning
	// already lives in the SeedManifest above, so we don't need to
	// pass channel info -- just the names.
	for _, s := range m.ExtraSnaps {
		snapStateMachine.Opts.Snaps = append(snapStateMachine.Opts.Snaps, s.Name)
	}

	storeURL, err := configureStoreURLFromM2cp()
	if err != nil {
		return err
	}
	snapStateMachine.manifestStoreURL = storeURL
	snapStateMachine.manifest = m

	builtBy, err := fetchM2cpBuiltBy()
	if err != nil {
		// Not fatal -- the image still builds; we just lose the
		// "who built it" line in build.yaml.
		fmt.Printf("=> warning: could not fetch builder identity: %v\n", err)
	} else {
		snapStateMachine.manifestBuiltBy = builtBy
		fmt.Printf("=> built by: %s\n", builtBy)
	}

	snapStateMachine.manifestSnapURL = buildSnapDownloadURL(m.Model.Architecture)
	fmt.Printf("=> snapd SnapDownloadURL hook wired (m2cp store app download --url)\n")

	retrieve, err := prefetchAssertionsViaM2cp(m.Model.Architecture, allPins, sm)
	if err != nil {
		return err
	}
	snapStateMachine.manifestAssertionRetrieve = retrieve
	fmt.Printf("=> snapd AssertionRetrieve hook wired (m2cp store snap assertion)\n")

	// Manifest mode = explicit version pinning; the user is taking
	// responsibility for the kernel/snapd combination. Force the
	// snapd-version-vs-kernel check off so a deliberate pin doesn't
	// trip snapd's defensive check.
	if !snapStateMachine.Opts.AllowSnapdKernelMismatch {
		snapStateMachine.Opts.AllowSnapdKernelMismatch = true
		fmt.Printf("=> AllowSnapdKernelMismatch forced on (manifest mode)\n")
	}

	return nil
}

// prefetchAssertionsViaM2cp pulls the snap-revision + snap-declaration
// (and any prerequisite) assertions for every pinned snap via
// `m2cp store snap assertion <name> <arch> -r <rev>`, decodes them
// into an in-memory index, and returns a function suitable for
// image.Options.AssertionRetrieve. Trust anchors (account,
// account-key) already live in sysdb via injectTrustFromM2cp, so the
// chain verifies as usual.
func prefetchAssertionsViaM2cp(arch string, pins []commands.OnlineManifestSnap, sm *seedwriter.Manifest) (func(*asserts.Ref) (asserts.Assertion, error), error) {
	fmt.Printf("=> prefetching %d snap-assertion bundles via m2cp (arch=%s)\n", len(pins), arch)
	idx := map[string]asserts.Assertion{}
	for _, p := range pins {
		rev := sm.AllowedSnapRevision(p.Name)
		if rev.Unset() {
			return nil, fmt.Errorf("internal: no allowed revision pinned for %q", p.Name)
		}
		cmd := exec.Command("m2cp", "store", "snap", "assertion", p.Name, arch, "-r", rev.String())
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("m2cp store snap assertion %s %s -r %s: %w (stderr: %s)",
				p.Name, arch, rev.String(), err, strings.TrimSpace(stderr.String()))
		}
		dec := asserts.NewDecoder(&stdout)
		added := 0
		for {
			a, err := dec.Decode()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("decoding assertion bundle for %s: %w", p.Name, err)
			}
			idx[a.Ref().Unique()] = a
			added++
		}
		fmt.Printf("   %-32s %-10s -> %d assertions indexed\n", p.Name, p.Version, added)
	}
	fmt.Printf("   total assertions in index: %d\n", len(idx))
	return func(ref *asserts.Ref) (asserts.Assertion, error) {
		if a, ok := idx[ref.Unique()]; ok {
			return a, nil
		}
		return nil, &asserts.NotFoundError{Type: ref.Type, Headers: refPrimaryKeyHeaders(ref)}
	}, nil
}

func refPrimaryKeyHeaders(ref *asserts.Ref) map[string]string {
	h := make(map[string]string, len(ref.PrimaryKey))
	for i, name := range ref.Type.PrimaryKey {
		if i < len(ref.PrimaryKey) {
			h[name] = ref.PrimaryKey[i]
		}
	}
	return h
}

// buildSnapDownloadURL returns the callback handed to
// image.Options.SnapDownloadURL: per requested snap, it shells out to
// `m2cp store app download <name> <ARCH> <revision> --url` and returns
// the temporary signed URL. snapd then HTTP-GETs that URL and runs the
// rest of the seed pipeline unchanged. m2cp is expected to be on PATH.
func buildSnapDownloadURL(arch string) func(name string, rev snap.Revision, snapID string) (string, error) {
	upperArch := strings.ToUpper(arch)
	return func(name string, rev snap.Revision, snapID string) (string, error) {
		revStr := rev.String()
		fmt.Printf("=> URL: m2cp store app download %s %s %s --url\n", name, upperArch, revStr)
		cmd := exec.Command("m2cp", "store", "app", "download", name, upperArch, revStr, "--url")
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("m2cp app download --url %s %s %s: %w (stderr: %s)",
				name, upperArch, revStr, err, strings.TrimSpace(stderr.String()))
		}
		url := strings.TrimSpace(stdout.String())
		if url == "" {
			return "", fmt.Errorf("m2cp returned empty URL for %s %s (stderr: %s)",
				name, revStr, strings.TrimSpace(stderr.String()))
		}
		fmt.Printf("   -> %s\n", redactSignedURL(url))
		return url, nil
	}
}

// redactSignedURL trims the Azure Blob SAS query so logs don't carry
// a long-lived-looking signature. The path + start/end times are kept;
// the sig= parameter is masked.
func redactSignedURL(u string) string {
	q := strings.Index(u, "?")
	if q < 0 {
		return u
	}
	host := u[:q]
	params := strings.Split(u[q+1:], "&")
	for i, p := range params {
		if strings.HasPrefix(p, "sig=") {
			params[i] = "sig=<redacted>"
		}
	}
	return host + "?" + strings.Join(params, "&")
}

// configureStoreURLFromM2cp reads the active store URL from
// `m2cp user status --json` (it's the GraphQL endpoint, e.g.
// https://host/graphql), strips the /graphql suffix to land on the
// snap-store HTTP base, exports SNAPPY_FORCE_API_URL so snapd's
// store client targets this appstore for the rest of the build, and
// returns the snap-store base URL so it can be recorded in build.yaml.
func configureStoreURLFromM2cp() (string, error) {
	fmt.Printf("=> discovering store URL: m2cp user status --json\n")
	cmd := exec.Command("m2cp", "user", "status", "--json")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("m2cp user status --json: %w", err)
	}
	var resp userStatusResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return "", fmt.Errorf("parsing m2cp user status JSON: %w", err)
	}
	graphqlURL := resp.Output.Session.Store
	if graphqlURL == "" {
		return "", fmt.Errorf("m2cp user status returned empty store URL (not logged in?)")
	}
	if !strings.HasSuffix(graphqlURL, "/graphql") {
		return "", fmt.Errorf("expected store URL to end in /graphql, got %q", graphqlURL)
	}
	base := strings.TrimSuffix(graphqlURL, "/graphql")
	fmt.Printf("   graphql:    %s\n", graphqlURL)
	fmt.Printf("   snap-store: %s\n", base)
	if err := os.Setenv("SNAPPY_FORCE_API_URL", base); err != nil {
		return "", fmt.Errorf("setting SNAPPY_FORCE_API_URL: %w", err)
	}
	return base, nil
}

type userStatusResponse struct {
	Output struct {
		Session struct {
			Store string `json:"store"`
		} `json:"session"`
	} `json:"output"`
}

// fetchM2cpBuiltBy pulls the developer identity (from
// `m2cp user info --json`) so we can record who built the image in
// build.yaml. Format is "Display Name <email>" when both fields
// are present, else whichever one we have.
func fetchM2cpBuiltBy() (string, error) {
	cmd := exec.Command("m2cp", "user", "info", "--json")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("m2cp user info --json: %w", err)
	}
	var resp struct {
		Output struct {
			Email string `json:"developerAccountSshKeyEmail"`
			Name  string `json:"developerAccountSshKeyname"`
		} `json:"output"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return "", fmt.Errorf("parsing m2cp user info: %w", err)
	}
	name := strings.TrimSpace(resp.Output.Name)
	email := strings.TrimSpace(resp.Output.Email)
	switch {
	case name != "" && email != "":
		return fmt.Sprintf("%s <%s>", name, email), nil
	case email != "":
		return email, nil
	case name != "":
		return name, nil
	default:
		return "", fmt.Errorf("m2cp user info returned no name or email")
	}
}

// writeModelJSON renders the recipe's model definition (via
// commands.RenderModelJSON, the shared definition) and writes it to
// dst with narration.
func writeModelJSON(dst string, m *commands.OnlineManifest) error {
	body, err := commands.RenderModelJSON(m)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, body, 0o644); err != nil {
		return err
	}
	fmt.Printf("=> wrote model.json: %s (%d bytes, %d snaps, grade=%s)\n",
		dst, len(body), len(m.Snaps), m.Model.Grade)
	return nil
}

// modelPushResponse maps the --json output of
// `m2cp store system model push -i`. The -i (idempotent) flag
// guarantees a revision in the response whether the model was
// created, updated, or already at the requested content.
type modelPushResponse struct {
	Output struct {
		Revision int    `json:"revision"`
		Error    string `json:"error"`
	} `json:"output"`
}

// pushModelGetRevision drives the appstore's model push with the
// -i (idempotent) flag: a single call that always succeeds when
// the requested state is achievable, returning the revision
// regardless of whether the model was created, updated, or
// already at the requested content.
//
// Requires m2cp >= v5.7.x with the idempotent flag.
func pushModelGetRevision(jsonPath, msg string) (int, error) {
	fmt.Printf("=> m2cp store system model push -i --json %s %q\n", jsonPath, msg)
	cmd := exec.Command("m2cp", "store", "system", "model", "push", "-i", "--json", jsonPath, msg)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("m2cp store system model push -i: %w (stderr=%q)",
			err, strings.TrimSpace(stderr.String()))
	}
	var resp modelPushResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return 0, fmt.Errorf("parsing m2cp push response: %w (stdout=%q)",
			err, strings.TrimSpace(stdout.String()))
	}
	if resp.Output.Error != "" {
		return 0, fmt.Errorf("m2cp: %s", resp.Output.Error)
	}
	if resp.Output.Revision <= 0 {
		return 0, fmt.Errorf("m2cp returned no revision (raw=%q)",
			strings.TrimSpace(stdout.String()))
	}
	fmt.Printf("   revision: %d\n", resp.Output.Revision)
	return resp.Output.Revision, nil
}

// buildYAML is the schema written to <seed>/systems/<label>/build.yaml
// at the end of a manifest-mode build. Field order is the order
// keys appear in the YAML output. Every field is always present;
// unknown values fall back to "not set" so the schema is stable
// regardless of env or linker flags.
type buildYAML struct {
	Builder        string `yaml:"builder"`
	BuilderVersion string `yaml:"builder-version"`
	BuiltBy        string `yaml:"built-by"`
	AppstoreURL    string `yaml:"appstore-url"`
	ModelName      string `yaml:"model-name"`
	ModelRevision  string `yaml:"model-revision"`
	Arch           string `yaml:"arch"`
	Date           string `yaml:"date"`
	Version        string `yaml:"version"`
	Commit         string `yaml:"commit"`
	Repo           string `yaml:"repo"`
	Grade          string `yaml:"grade"`
}

const notSet = "not set"

// writeBuildYAML emits build.yaml into each /systems/<label>/ entry
// of the seedwriter's output. Called at the end of prepareImage in
// manifest mode so the resulting image carries machine-readable
// provenance (which builder, which store, which model, when).
func writeBuildYAML(snapStateMachine *SnapStateMachine) error {
	seedRoot := filepath.Join(snapStateMachine.tempDirs.unpack, "system-seed", "systems")
	entries, err := os.ReadDir(seedRoot)
	if err != nil {
		return fmt.Errorf("scanning %s for system labels: %w", seedRoot, err)
	}

	info := assembleBuildYAML(snapStateMachine)
	body, err := yaml.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshalling build.yaml: %w", err)
	}

	wrote := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(seedRoot, e.Name(), "build.yaml")
		fmt.Printf("=> writing %s (%d bytes)\n", path, len(body))
		if err := os.WriteFile(path, body, 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		wrote++
	}
	if wrote == 0 {
		return fmt.Errorf("no /systems/<label>/ directories under %s; build.yaml not written", seedRoot)
	}
	return nil
}

func assembleBuildYAML(snapStateMachine *SnapStateMachine) buildYAML {
	builderVersion := commands.BuilderVersion
	if builderVersion == "" {
		builderVersion = notSet
	}
	storeURL := snapStateMachine.manifestStoreURL
	if storeURL == "" {
		storeURL = notSet
	}
	builtBy := snapStateMachine.manifestBuiltBy
	if builtBy == "" {
		builtBy = notSet
	}
	mb := snapStateMachine.manifest.Build
	return buildYAML{
		Builder:        "ubuntu-image",
		BuilderVersion: builderVersion,
		BuiltBy:        builtBy,
		AppstoreURL:    storeURL,
		ModelName:      snapStateMachine.manifest.Model.Name,
		ModelRevision:  strconv.Itoa(snapStateMachine.manifestModelRevision),
		Arch:           snapStateMachine.manifest.Model.Architecture,
		Date:           time.Now().UTC().Format(time.RFC3339),
		Version:        resolveBuildField("BUILD_VERSION", "version", mb.Version),
		Commit:         resolveBuildField("BUILD_COMMIT", "commit", mb.Commit),
		Repo:           resolveBuildField("BUILD_REPO", "repo", mb.Repo),
		Grade:          snapStateMachine.manifest.Model.Grade,
	}
}

// resolveBuildField applies env-over-manifest-over-notset precedence
// to one of the BUILD_* fields. Every non-default outcome (env in
// use, manifest in use, env overriding manifest) is logged so the
// build trace explains the value that landed in build.yaml.
func resolveBuildField(envKey, manifestKey, manifestVal string) string {
	envVal := os.Getenv(envKey)
	switch {
	case envVal != "" && manifestVal != "" && envVal != manifestVal:
		fmt.Printf("=> build.%s: env %s=%q OVERRIDES manifest build.%s=%q\n",
			manifestKey, envKey, envVal, manifestKey, manifestVal)
		return envVal
	case envVal != "" && manifestVal != "" && envVal == manifestVal:
		fmt.Printf("=> build.%s: env %s and manifest agree (%q)\n",
			manifestKey, envKey, envVal)
		return envVal
	case envVal != "" && manifestVal == "":
		fmt.Printf("=> build.%s: %s=%q (manifest unset)\n",
			manifestKey, envKey, envVal)
		return envVal
	case envVal == "" && manifestVal != "":
		fmt.Printf("=> build.%s: manifest=%q (env %s unset)\n",
			manifestKey, manifestVal, envKey)
		return manifestVal
	default:
		return notSet
	}
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
	var skipped int
	for {
		a, err := dec.Decode()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("decoding trust bundle: %w", err)
		}
		describeAssertion(a)
		// Only account and account-key can be trust anchors. Anything
		// else (e.g. the store assertion) is informational here.
		switch a.(type) {
		case *asserts.Account, *asserts.AccountKey:
			trusted = append(trusted, a)
		default:
			skipped++
		}
	}
	if len(trusted) == 0 {
		return fmt.Errorf("trust bundle is empty")
	}
	sysdb.InjectTrusted(trusted)
	// image.Prepare caches sysdb.Trusted() in a package-level var at
	// init time, so InjectTrusted alone won't reach it. MockTrusted
	// (named for tests, but the right tool here) refreshes that cache
	// with the now-expanded trust set.
	image.MockTrusted(sysdb.Trusted())
	fmt.Printf("   injected %d trust anchors into snapd sysdb (+ %d non-anchor assertions skipped, image package cache refreshed)\n",
		len(trusted), skipped)
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
//
// Trailing whitespace is stripped and a single trailing newline is
// added: snapd's assertion decoder uses bytes.LastIndex(buf, "\n\n")
// to split content from signature, so a trailing blank line at EOF
// would make it mistake the signature for the body.
func writeM2cpStdout(dst string, args ...string) error {
	cmd := exec.Command("m2cp", args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("m2cp %s: %w", strings.Join(args, " "), err)
	}
	payload := append(bytes.TrimRight(stdout.Bytes(), "\r\n \t"), '\n')
	return os.WriteFile(dst, payload, 0o644)
}
