package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/canonical/ubuntu-image/internal/commands"
)

// L-IoT bare-recipe entry. Customers invoke `ubuntu-image <recipe.yaml>`
// (no subcommand). This file handles preflight checks (m2cp present,
// session active), prints a session+recipe summary banner, then
// rewrites os.Args so the parser sees the call as
// `snap --manifest <recipe.yaml>` and the existing pipeline runs.
//
// The `snap`, `classic`, and `model` subcommands stay hidden but
// callable explicitly for debugging.

// liotScanArgs detects the bare-yaml form (any arg ending in .yaml
// or .yml) and the L-IoT flags --dry-run, --xz, and --model,
// regardless of position. Any combination is accepted:
//
//	liot-image recipe.yaml
//	liot-image --dry-run recipe.yaml
//	liot-image --xz recipe.yaml --dry-run
//	liot-image --model recipe.yaml
//
// Recipe identification by extension is robust enough for the
// shapes customers actually type. Subcommand invocations
// (snap/classic/model) don't carry .yaml files as their first
// positional so they fall through to go-flags untouched.
func liotScanArgs() (recipe string, dryRun, xz, modelOnly bool) {
	for _, a := range os.Args[1:] {
		switch {
		case a == "--dry-run":
			dryRun = true
		case a == "--xz":
			xz = true
		case a == "--model":
			modelOnly = true
		case isYAMLSuffix(a):
			if recipe == "" {
				recipe = a
			}
		}
	}
	return recipe, dryRun, xz, modelOnly
}

func isYAMLSuffix(p string) bool {
	l := strings.ToLower(p)
	return strings.HasSuffix(l, ".yaml") || strings.HasSuffix(l, ".yml")
}

// LiotPreflightStatus is the three-way return of the bare-yaml
// preflight: failed (exit non-zero), continue (hand off to the
// rewritten state-machine pipeline), or dry-run (preflight passed
// in --dry-run mode; exit zero without pushing or building).
type LiotPreflightStatus int

const (
	LiotPreflightFailed LiotPreflightStatus = iota
	LiotPreflightContinue
	LiotPreflightDryRun
)

// liotMaybeShowQuickHelp prints a short L-IoT-specific usage message
// when the user invoked the tool with no arguments or with -h/--help.
// Returns true if the message was printed and the caller should exit.
// Anything else (a bare recipe, a hidden subcommand, etc.) falls
// through.
func liotMaybeShowQuickHelp() bool {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, liotUsage)
		return true
	}
	switch os.Args[1] {
	case "-h", "--help":
		fmt.Fprint(os.Stderr, liotUsage)
		return true
	}
	return false
}

const liotUsage = `L-IoT Image Builder for Ubuntu Core based systems

Usage:
  liot-image [--dry-run] [--xz] [--model] <recipe.yaml>

Build a bootable image from a single YAML recipe file. The recipe
pins the model and the snaps that go into the image; the builder
syncs the model with the appstore, resolves snap versions, and
produces a .img plus a seed.manifest.

Flags:
  --dry-run   Run preflight (m2cp present, session active, all
              recipe snaps in the appstore) and exit. Nothing
              pushed or built.
  --xz        After building, xz-compress the image in place.
  --model     Render the recipe's model.json to stdout and exit.
              Pure transformation: no network, no build.

Prerequisites:
  m2cp CLI on PATH
  An active appstore session:  m2cp user login

Example:
  m2cp user login
  liot-image liot-uc-imx93-1.2.3.yaml

Output is written to ./bin/ by default; use -O DIR to override.
The image is named after the recipe, so liot-uc-imx93-1.2.3.yaml
produces liot-uc-imx93-1.2.3.img (or .img.xz with --xz).
See README.md for the recipe schema.
`

// liotPreflightAndBanner runs the preflight checks (m2cp on PATH,
// m2cp session active, recipe parses, every recipe snap exists in
// the appstore), prints the session + recipe summary, and rewrites
// os.Args so the existing snap+manifest pipeline picks up from
// here.
//
// Returns:
//   - LiotPreflightFailed: a preflight check failed; caller exits 1.
//   - LiotPreflightDryRun: --dry-run requested and preflight passed;
//     caller exits 0 without pushing or building.
//   - LiotPreflightContinue: preflight passed; caller hands off to
//     the state-machine pipeline through the rewritten args.
func liotPreflightAndBanner(recipePath string, dryRun, xz bool) LiotPreflightStatus {

	fmt.Fprintln(os.Stderr, "[L-IoT Image Builder]")
	if dryRun {
		fmt.Fprintln(os.Stderr, "(dry-run mode: preflight only; nothing will be pushed or built)")
	}
	fmt.Fprintln(os.Stderr)

	if _, err := exec.LookPath(commands.M2cpCLI); err != nil {
		fmt.Fprintf(os.Stderr, "%s CLI not found on PATH.\n", commands.M2cpCLI)
		fmt.Fprintln(os.Stderr, "Install it first, then re-run this command.")
		return LiotPreflightFailed
	}

	if xz {
		if _, err := exec.LookPath("xz"); err != nil {
			fmt.Fprintln(os.Stderr, "xz not found on PATH; install it or drop --xz.")
			return LiotPreflightFailed
		}
	}

	session, err := m2cpSessionStatus()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s session check failed: %v\n", commands.M2cpCLI, err)
		return LiotPreflightFailed
	}
	if !session.LoggedIn {
		fmt.Fprintln(os.Stderr, "Not logged in to an appstore.")
		fmt.Fprintf(os.Stderr, "Run '%s user login' first, then re-run this command.\n", commands.M2cpCLI)
		return LiotPreflightFailed
	}
	fmt.Fprintf(os.Stderr, "Tenant: %s\n", session.Tenant)
	fmt.Fprintf(os.Stderr, "Store:  %s\n", strings.TrimSuffix(session.StoreURL, "/graphql"))
	fmt.Fprintln(os.Stderr)

	m, err := commands.LoadOnlineManifest(recipePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Recipe %s: %v\n", recipePath, err)
		return LiotPreflightFailed
	}
	fmt.Fprintf(os.Stderr, "Recipe: %s\n", recipePath)
	fmt.Fprintf(os.Stderr, "  Model:        %s\n", m.Model.Name)
	fmt.Fprintf(os.Stderr, "  Architecture: %s\n", m.Model.Architecture)
	fmt.Fprintf(os.Stderr, "  Base:         %s\n", m.Model.Base)
	fmt.Fprintf(os.Stderr, "  Grade:        %s\n", m.Model.Grade)
	fmt.Fprintf(os.Stderr, "  Model snaps:  %d\n", len(m.Snaps))
	if len(m.ExtraSnaps) > 0 {
		fmt.Fprintf(os.Stderr, "  Extra snaps:  %d\n", len(m.ExtraSnaps))
	}
	fmt.Fprintln(os.Stderr)

	if !liotPreflightSnapsExist(m) {
		return LiotPreflightFailed
	}

	if dryRun {
		fmt.Fprintln(os.Stderr, "Dry-run: preflight passed. Re-run without --dry-run to push and build.")
		return LiotPreflightDryRun
	}

	// Rewrite os.Args: insert `snap --manifest <recipe>` after the
	// program name, drop the original recipe arg and any L-IoT-only
	// flags (which go-flags wouldn't recognise), and inject a
	// default `-O ./bin/` if the user didn't already pass one. The
	// recipe path may be anywhere in args, not just position 1.
	rest := make([]string, 0, len(os.Args)-1)
	for _, a := range os.Args[1:] {
		if a == recipePath || a == "--xz" || a == "--dry-run" || a == "--model" {
			continue
		}
		rest = append(rest, a)
	}
	newArgs := make([]string, 0, len(os.Args)+4)
	newArgs = append(newArgs, os.Args[0], "snap", "--manifest", recipePath)
	if !hasOutputDirFlag(rest) {
		newArgs = append(newArgs, "-O", "./bin/")
	}
	newArgs = append(newArgs, rest...)
	os.Args = newArgs
	return LiotPreflightContinue
}

// liotPostBuild does the bare-recipe finishing touches after the
// state machine has produced the raw image:
//
//  1. Rename `<volume>.img` to `<recipe-basename>.img`. The volume
//     name is whatever the gadget snap's gadget.yaml declared (e.g.
//     "imx93.img"), which rarely matches anything the operator
//     recognises; the recipe basename is the only filename they have
//     direct control over.
//  2. Write a `<recipe-basename>.model.json` sidecar next to the
//     image -- the same model.json the appstore push step consumed
//     -- so operators have a self-contained reference for what
//     model definition produced this image.
//  3. If xz is set, compress the renamed image in place to
//     `<recipe-basename>.img.xz`. The model.json sidecar stays
//     uncompressed (tiny + meant to be read directly).
//
// Skipped (with a note) when the build produced more than one .img,
// since each volume needs its own name and we can't collapse them
// onto a single recipe basename.
func liotPostBuild(recipePath, outputDir string, xz bool) {
	base := strings.TrimSuffix(filepath.Base(recipePath), filepath.Ext(recipePath))
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not read output dir %s: %v\n", outputDir, err)
		return
	}
	var imgs []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".img") {
			imgs = append(imgs, e.Name())
		}
	}
	if len(imgs) == 0 {
		return
	}
	if len(imgs) > 1 {
		fmt.Fprintf(os.Stderr, "Multi-volume build (%d images); skipping rename and --xz.\n", len(imgs))
		return
	}

	src := filepath.Join(outputDir, imgs[0])
	dst := filepath.Join(outputDir, base+".img")
	if src != dst {
		fmt.Fprintf(os.Stderr, "Renaming output: %s -> %s\n", filepath.Base(src), filepath.Base(dst))
		if err := os.Rename(src, dst); err != nil {
			fmt.Fprintf(os.Stderr, "warning: rename failed: %v\n", err)
			return
		}
	}

	modelPath := filepath.Join(outputDir, base+".model.json")
	if err := writeModelSidecar(recipePath, modelPath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not write %s: %v\n", filepath.Base(modelPath), err)
	} else {
		fmt.Fprintf(os.Stderr, "Model:  %s\n", modelPath)
	}

	if !xz {
		fmt.Fprintf(os.Stderr, "Image:  %s\n", dst)
		return
	}

	fmt.Fprintf(os.Stderr, "Compressing with xz: %s\n", filepath.Base(dst))
	cmd := exec.Command("xz", "-T0", "-v", dst)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: xz compression failed: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "Image:  %s.xz\n", dst)
}

// writeModelSidecar renders the recipe's model definition through the
// same path the build flow uses to push to the appstore
// (commands.RenderModelJSON) and writes it next to the image.
func writeModelSidecar(recipePath, dst string) error {
	m, err := commands.LoadOnlineManifest(recipePath)
	if err != nil {
		return err
	}
	body, err := commands.RenderModelJSON(m)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, body, 0644)
}

// hasOutputDirFlag reports whether -O / --output-dir was passed
// among the user's args. Matches the bare flag forms and the
// `--output-dir=DIR` / `-O=DIR` equals forms.
func hasOutputDirFlag(args []string) bool {
	for _, a := range args {
		switch {
		case a == "-O", a == "--output-dir":
			return true
		case strings.HasPrefix(a, "-O="), strings.HasPrefix(a, "--output-dir="):
			return true
		}
	}
	return false
}

// m2cpUserStatus is the slim view of `m2cp user status --json` we
// need for preflight: enough to tell the user where they're logged
// in and to refuse to run when they aren't.
type m2cpUserStatus struct {
	LoggedIn bool
	Tenant   string
	StoreURL string
}

// liotPreflightSnapsExist verifies, before any push, that every
// snap the recipe references already exists in the appstore for
// the target architecture. A single `m2cp store snap list -a <arch>
// --json` call gives us the full catalog; we then compare names.
//
// If anything is missing we print a clear, actionable list:
//
//	Missing from appstore:
//	  - edge-os-kernel-vtg
//	  - edge-os-gadget
//
//	Upload them first with:
//	  m2cp store app push <snap-file>
//
// and refuse to proceed -- otherwise the build would fail later
// at the model-push step with a single error per call, hiding the
// full set of work the operator has to do.
func liotPreflightSnapsExist(m *commands.OnlineManifest) bool {
	fmt.Fprintf(os.Stderr, "Checking that recipe snaps exist in the appstore (arch=%s)...\n", m.Model.Architecture)
	available, err := listAppstoreSnapNames(m.Model.Architecture)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  could not list appstore snaps: %v\n", err)
		return false
	}
	var missing []string
	seen := map[string]bool{}
	for _, s := range m.Snaps {
		if !available[s.Name] && !seen[s.Name] {
			missing = append(missing, s.Name)
			seen[s.Name] = true
		}
	}
	for _, s := range m.ExtraSnaps {
		if !available[s.Name] && !seen[s.Name] {
			missing = append(missing, s.Name)
			seen[s.Name] = true
		}
	}
	if len(missing) == 0 {
		fmt.Fprintf(os.Stderr, "  all %d snaps present\n\n", len(m.Snaps)+len(m.ExtraSnaps))
		return true
	}
	sort.Strings(missing)
	fmt.Fprintln(os.Stderr, "  Missing from the appstore:")
	for _, n := range missing {
		fmt.Fprintf(os.Stderr, "    - %s\n", n)
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "Upload each missing snap with:\n  %s store app push <path-to-snap-file>\n\n",
		commands.M2cpCLI)
	fmt.Fprintln(os.Stderr, "Then re-run this command.")
	return false
}

// listAppstoreSnapNames returns the set of snap names available in
// the appstore for the given architecture. One m2cp call, cheap to
// scan even for stores with thousands of snaps.
func listAppstoreSnapNames(arch string) (map[string]bool, error) {
	cmd := exec.Command(commands.M2cpCLI, "store", "snap", "list", "-a", arch, "--json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s store snap list: %w (stderr=%q)",
			commands.M2cpCLI, err, strings.TrimSpace(stderr.String()))
	}
	var resp struct {
		Output struct {
			SnapDeclarations []struct {
				SnapName string `json:"snapName"`
			} `json:"snapDeclarations"`
		} `json:"output"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("parsing %s snap list: %w", commands.M2cpCLI, err)
	}
	out := map[string]bool{}
	for _, d := range resp.Output.SnapDeclarations {
		out[d.SnapName] = true
	}
	return out, nil
}

type m2cpStatusJSON struct {
	Output struct {
		Status  string `json:"status"`
		Session struct {
			Store  string `json:"store"`
			Tenant struct {
				TenantName string `json:"tenantName"`
				Alias      string `json:"alias"`
			} `json:"tenant"`
		} `json:"session"`
	} `json:"output"`
}

func m2cpSessionStatus() (m2cpUserStatus, error) {
	cmd := exec.Command(commands.M2cpCLI, "user", "status", "--json")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return m2cpUserStatus{}, fmt.Errorf("m2cp user status --json: %w", err)
	}
	var raw m2cpStatusJSON
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		return m2cpUserStatus{}, fmt.Errorf("parsing m2cp status: %w", err)
	}
	tenant := raw.Output.Session.Tenant.Alias
	if tenant == "" {
		tenant = raw.Output.Session.Tenant.TenantName
	}
	out := m2cpUserStatus{
		LoggedIn: raw.Output.Status == "logged in",
		Tenant:   tenant,
		StoreURL: raw.Output.Session.Store,
	}
	if out.LoggedIn && out.StoreURL == "" {
		return out, errors.New("m2cp reports logged in but no store URL")
	}
	return out, nil
}
