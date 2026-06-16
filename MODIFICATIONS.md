# MODIFICATIONS.md

This file describes the modifications layered on top of upstream
`canonical/ubuntu-image` and `canonical/snapd` to support building
images against our self-hosted, snap-store-compatible appstore.

The flow is: user provides a manifest (model + snap versions), the
ubuntu-image side drives m2cp to fetch trust + revisions + download
URLs, and snapd does the actual seed pipeline through two small
fork-side hooks. The resulting image is bit-identical to one built
against a Canonical-style snap store.

---

## Part 1 — snapd fork (`/r/snapd`)

The fork is consumed by ubuntu-image via `go.work` pointing at the
local directory (branch `liot-image`). The patch surface is
additions only, symmetric with the pre-existing `AssertionRetrieve`
hook: the online-store flow (`SnapDownloadURL`, ~30 lines + one
~70-line helper) plus the `AllowExtraSnaps`/`SnapIDForName`
seed-composition relaxations across `image/` and `seed/seedwriter/`.

### `image/options.go`

Add one field to the existing `image.Options` struct:

```go
// SnapDownloadURL, when set, replaces the snap store's
// metadata/URL-resolution step with a caller-provided lookup.
// The hook returns a fully-formed HTTPS URL pointing at the
// .snap blob for the given snap at the given revision; snapd
// HTTP-GETs that URL, populates snap.Info from the downloaded
// file, and runs the rest of the seed pipeline (snap-revision
// sha3 match, assertion chain, seed writing) unchanged. The
// resulting image is identical to one built via the normal
// store path.
//
// Intended for closed-ecosystem appstores whose metadata API
// is not snap-store-compatible (e.g. GraphQL-fronted stores)
// but whose download endpoint is a plain signed HTTPS URL.
SnapDownloadURL func(name string, revision snap.Revision, snapID string) (url string, err error)
```

New import: `github.com/snapcore/snapd/snap` (for `snap.Revision`).

### `image/image_linux.go`

1. Add `snapDownloadURL` field on the `imageSeeder` struct, populated
   from `opts.SnapDownloadURL` in `newImageSeeder`:

   ```go
   snapDownloadURL func(name string, revision snap.Revision, snapID string) (string, error)
   ```

2. Branch at the top of `downloadSnaps` to dispatch to the new helper
   when the hook is set:

   ```go
   if s.snapDownloadURL != nil {
       return s.downloadSnapsViaURLHook(snapsToDownload)
   }
   ```

3. New function `downloadSnapsViaURLHook`. Per snap:
   - Look up the pinned revision in
     `s.w.Manifest().AllowedSnapRevision(name)` (the seedwriter's
     pre-provided manifest).
   - Call `s.snapDownloadURL(name, rev, snapID)` to get a download URL.
   - HTTP-GET the URL into a temp file.
   - Parse the file with `snapfile.Open` +
     `snap.ReadInfoFromSnapFile` to obtain `*snap.Info`. Stamp Revision
     and SnapID on the result.
   - `s.w.SetInfo(sn, info, nil)` to register the snap with the
     seedwriter; this assigns the final `sn.Path`.
   - `s.validateSnapArchs(...)` against the model's architecture.
   - Rename (or copy-and-remove on cross-device) the temp file to
     `sn.Path`.
   - Return a `*tooling.DownloadedSnap{Path, Info}` keyed by snap name.

4. New helper `httpDownloadToFile(url string, dst io.Writer) error`:
   thin `http.Get` + status-code check + `io.Copy`. Transport-level
   integrity is intentionally not checked here — the seedwriter
   verifies the snap-revision sha3-384 against the downloaded bytes
   downstream.

New import: `net/http`.

### `image/options.go` + `seed/seedwriter` (`AllowExtraSnaps`, `SnapIDForName`)

A second, independent addition supports seeding snaps that are **not
declared in the model** at `signed`/`secured` grade. Manifest mode
injects `extra-snaps` (e.g. `core24`, pulled in as the base of a
modeled app) via `image.Options.Snaps` (the `--snap` path); snapd
classifies them as extra snaps. A seed of any non-dangerous grade
otherwise forbids extra snaps in four distinct places, so the fix is
one opt-in flag honored at all four, plus a snap-id resolver:

**`AllowExtraSnaps`** — `bool` on both `image.Options` and
`seedwriter.Options`; `image_linux.go` threads one into the other.
When set, the seedwriter treats a model of any grade as dangerous for
these seed-composition checks only (`seed/seedwriter/seed20.go`,
`writer.go`):

1. `policy20.checkAllowedDangerous` — the up-front gate behind *"cannot
   override channels, add devmode snaps, local snaps, or extra
   snaps/components with a model of grade higher than dangerous"*.
   Returns nil when the flag is set.
2. `Writer.checkBase` — a model app's base may be supplied by an extra
   snap downloaded in a *later* batch, so it isn't in `availableByMode`
   when the app is checked. The flag defers (skips) the base check, as
   dangerous mode already does for option snaps.
3. `tree20.writeMeta` — extra snaps are written to `options.yaml`,
   which was rejected (*"unexpected non-model snap overrides with grade
   …"*) above dangerous grade. The flag permits it.

**`SnapIDForName`** — `func(name string) (string, error)` on
`image.Options`, used by the `SnapDownloadURL` download path
(`downloadSnapsViaURLHook`). An extra snap's store-assigned snap-id is
neither in the model nor in the `.snap` blob, so without it the
seedwriter fails (*"snap-id should have been set"*). The hook resolves
it (ubuntu-image supplies it from the prefetched snap-declarations,
see Part 2) and stamps `snap.Info.SnapID` before `SetInfo`.

Additions only. **Build-time only** — these affect which snaps the
image builder composes into the seed; the model assertion is never
modified (an extra snap stays out of the signed model). The device is
unaffected: the on-device snapd reads the seed and installs every
seeded snap at run-mode boot independently of these flags (the
run-mode load path has no grade gate). The trust model is "the
manifest author pins an exact snap set and verifies the image out of
band." Verified end-to-end: an arm64 `signed`-grade build with
`core24` in `extra-snaps` produces an image whose `seed.manifest`
lists `core24` while the seeded model assertion does not.

### What stays unchanged in snapd

The hook strictly replaces the metadata-fetch-and-URL-resolution step
(`tsto.DownloadMany` → `SnapAction` round-trip). All other behavior
runs:

- Trust chain verification (model → brand account-key → root account-key)
- `snap-revision` sha3-384 verification of downloaded blobs
- `snap-revision` / `snap-declaration` / `account` / `account-key`
  resolution (handled via the existing `AssertionRetrieve` hook)
- Seed writing (`seedwriter.Writer`)
- Gadget yaml processing, bootloader configuration, disk image generation

### Companion: pre-existing `AssertionRetrieve` hook

The online-store flow depends on a hook that already exists in the
fork:

```go
// image.Options
AssertionRetrieve func(ref *asserts.Ref) (asserts.Assertion, error)
```

ubuntu-image's manifest pipeline prefetches every required assertion
via m2cp and feeds them through this hook so snapd resolves them
locally instead of going to the store. Not new in this iteration, but
load-bearing for the online-store mode.

### Out of scope (snapd, still in the tree, candidates for revert)

The fork currently carries additional changes from an earlier
offline-mode experiment that are **not** needed by the online-store
flow and should be reverted in a separate audit pass:

- `asserts/database.go`, `asserts/fetcher.go`: `SkipSignatureCheck`
  field on `DatabaseConfig`. The online flow uses the real chain
  (trust anchors injected via `sysdb.InjectTrusted` from
  `m2cp store system assertion`); no need to skip verification.
- `seed/seedwriter/helpers.go`, `seed20.go`, `writer.go`: offline-mode
  tweaks layered over the seedwriter.
- `snapdenv/trust.go`: previously held a `TrustInsecure` toggle.

---

## Part 2 — ubuntu-image fork

All the m2cp shell-outs, manifest parsing, and orchestration. The
`snap` subcommand grows a `--manifest` flag that, when set, runs the
manifest pipeline before the existing state machine takes over.

### Manifest format

A small YAML file pinning a model and a set of snap versions:

```yaml
model:
  name: edge-imx93-uc-vtg
  revision: 1
  architecture: arm64

snaps:
  - name: snapd
    version: "4.3.0.42"
  - name: core24
    version: "1.2.0"
  - name: mlpa-os-imx-kernel
    version: "6.1.5"
  # ...
```

Versions, not revisions, because the user's appstore guarantees
version↔revision bijection; the resolution happens via
`m2cp store snap info --json` during the build.

There is no `model.type` field — only one model type exists in this
fork ("m2cp", meaning the Ubuntu Core family).

### New files

- **`internal/commands/online_manifest.go`** — the YAML schema
  (`OnlineManifest`, `OnlineManifestModel`, `OnlineManifestSnap`),
  `LoadOnlineManifest()`, and the validator.

- **`internal/statemachine/snap_manifest.go`** — the entire pipeline:
  - `prepareFromManifest` (the orchestrator, called from
    `SnapStateMachine.Setup` when `--manifest` is set)
  - `injectTrustFromM2cp` — runs `m2cp store system assertion`,
    decodes account + account-keys + store assertion, filters to trust
    anchors (account, account-key), calls `sysdb.InjectTrusted`, then
    `image.MockTrusted(sysdb.Trusted())` to refresh the package-level
    cache in the `image` package
  - `resolveRevisionsViaM2cp` / `resolveSnapRevision` — per snap,
    `m2cp store snap info <name> <arch> --json`, finds the matching
    version, populates a `seedwriter.Manifest` via
    `SetAllowedSnapRevision`
  - `buildSnapDownloadURL` — returns the callback handed to the snapd
    `SnapDownloadURL` hook; shells out to
    `m2cp store app download <name> <ARCH> <revision> --url` and
    returns the temporary signed URL
  - `prefetchAssertionsViaM2cp` — per snap,
    `m2cp store snap assertion <name> <arch> -r <revision>`, decodes
    the bundle, indexes by `Ref.Unique()`, returns a function suitable
    for `image.Options.AssertionRetrieve`. Also harvests a
    snap-name→snap-id map from the decoded `snap-declaration`
    assertions and returns a resolver for `image.Options.SnapIDForName`
    (needed to stamp the snap-id of extra snaps)
  - `manifestStageDir` — workdir layout for the model.assert file
  - Helpers: `writeM2cpStdout`, `describeAssertion`,
    `redactSignedURL`, `refPrimaryKeyHeaders`,
    `configureStoreURLFromM2cp` (legacy, leftover; the URL hook makes
    `SNAPPY_FORCE_API_URL` unused but harmless)

- **`test-online-store/manifest.yaml`** — concrete pin for
  `edge-imx93-uc-vtg` rev 1, 12 snaps at the revisions of the vanilla
  shipping device.

- **`test-online-store/test.sh`** — end-to-end driver: builds
  ubuntu-image, runs it against the manifest, dumps the resulting
  `seed.manifest` for inspection.

### Modified files

- **`internal/commands/snap.go`** — added the `--manifest` flag to
  `SnapOpts`; dropped the `required:"false"` tag on the positional
  args struct (go-flags treats *any* `required:"..."` value as
  truthy, which was making `model_assertion` falsely required at
  parse time when `--manifest` should be the input instead).

- **`internal/statemachine/snap.go`** — added fields to
  `SnapStateMachine`:
  - `manifestSeedManifest *seedwriter.Manifest`
  - `manifestSnapURL func(name, revision, snapID) (string, error)`
  - `manifestAssertionRetrieve func(*asserts.Ref) (asserts.Assertion, error)`
  - `manifestSnapIDForName func(name string) (string, error)`

  And added the `prepareFromManifest()` call at the top of `Setup`
  when `Opts.Manifest` is set.

- **`internal/statemachine/snap_states.go`** — wires the hooks
  into `image.Options`:
  - `SnapDownloadURL: snapStateMachine.manifestSnapURL`
  - `AssertionRetrieve: snapStateMachine.manifestAssertionRetrieve`
  - `AllowExtraSnaps: snapStateMachine.Opts.Manifest != ""` — on in
    manifest mode only, so the recipe's `extra-snaps` (snaps not in
    the signed model) are accepted into the seed; normal builds keep
    upstream's dangerous-only restriction
  - `SnapIDForName: snapStateMachine.manifestSnapIDForName` — resolves
    an extra snap's snap-id (not known from the model) for the URL-hook
    download path
  - Precedence check in `imageOptsSeedManifest` so
    `manifestSeedManifest` wins over the existing `Opts.Revisions`
    path

- **`.gitignore`** — ignore `test-online-store/work/`,
  `test-online-store/out/`, and the locally-built `ubuntu-image`
  binary.

### Pipeline (what `prepareFromManifest` runs)

1. Parse the YAML manifest.
2. `m2cp store model assertion m2cp <name> <revision>` →
   write `<workdir>/manifest/model.assert`, set
   `Args.ModelAssertion` to that path so the existing state machine
   picks it up unchanged.
3. `m2cp store system assertion` → decode trust bundle, narrate every
   assertion with its key fingerprint, `sysdb.InjectTrusted`,
   `image.MockTrusted(sysdb.Trusted())`.
4. `m2cp store snap info <name> <arch> --json` per snap → version →
   revision → `seedwriter.Manifest.SetAllowedSnapRevision`.
5. Build the `SnapDownloadURL` callback that wraps
   `m2cp store app download ... --url`.
6. `m2cp store snap assertion <name> <arch> -r <revision>` per snap →
   decode into an in-memory index → build the `AssertionRetrieve`
   callback.
7. Force `AllowSnapdKernelMismatch = true` — manifest mode is the
   user explicitly declaring a tested kernel/snapd combo, so snapd's
   defensive version check is redundant.

Every step narrates itself in plain language so the build log is a
readable audit trail of which trust anchors were installed, which
revision each version resolved to, and which URL served each blob.

### Design decisions worth recording

- **Versions in the manifest, not revisions.** Human-readable input;
  resolution to revision happens via m2cp.
- **Always fetch model fresh from m2cp.** No caching; the manifest
  names a model + revision and the assertion is pulled per build.
- **Trust bundle in memory only.** No on-disk store.assert artifact;
  the audit trail is the narrated log of injected key fingerprints.
- **URL hook, not blob hook.** snapd keeps owning HTTPS GET, caching,
  sha3 verification — only the URL-resolution step is overridden.
  Smaller patch surface, same security properties.
- **m2cp on `PATH`.** No `M2CP_BIN` env-var indirection.
- **Narrate every step.** Transparency is the default for the
  manifest pipeline, not opt-in.
- **Auto `AllowSnapdKernelMismatch` in manifest mode.** Pinning is
  the user explicitly declaring intent; snapd's defensive version
  check is redundant.

### Verifying

```sh
bash test-online-store/test.sh
```

A passing run produces a bootable `.img` and a `seed.manifest` whose
12 pinned revisions match the manifest input exactly (the same set
of revisions as a vanilla install of the reference device).
