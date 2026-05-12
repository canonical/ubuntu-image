# SNAPD-MODIFICATIONS.md

This file describes the modifications required in our snapd fork to support
the online-store image-building flow used by ubuntu-image's `--manifest` mode.
The fork lives at `/r/snapd`; ubuntu-image consumes it via a `go.work` file
pointing at that directory.

## Patch surface

Two files in the fork, additions only, ~30 lines + one ~70-line helper
function. Symmetric with the pre-existing `AssertionRetrieve` hook.

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

1. Add `snapDownloadURL` field on the `imageSeeder` struct, populated from
   `opts.SnapDownloadURL` in `newImageSeeder`:

   ```go
   snapDownloadURL func(name string, revision snap.Revision, snapID string) (string, error)
   ```

2. Branch at the top of `downloadSnaps` to dispatch to the new helper when
   the hook is set:

   ```go
   if s.snapDownloadURL != nil {
       return s.downloadSnapsViaURLHook(snapsToDownload)
   }
   ```

3. New function `downloadSnapsViaURLHook`. Per snap:
   - Look up the pinned revision in `s.w.Manifest().AllowedSnapRevision(name)`
     (the seedwriter's pre-provided manifest).
   - Call `s.snapDownloadURL(name, rev, snapID)` to get a download URL.
   - HTTP-GET the URL into a temp file.
   - Parse the file with `snapfile.Open` + `snap.ReadInfoFromSnapFile` to
     obtain `*snap.Info`. Stamp Revision and SnapID on the result.
   - `s.w.SetInfo(sn, info, nil)` to register the snap with the seedwriter;
     this assigns the final `sn.Path`.
   - `s.validateSnapArchs(...)` against the model's architecture.
   - Rename (or copy-and-remove on cross-device) the temp file to `sn.Path`.
   - Return a `*tooling.DownloadedSnap{Path, Info}` keyed by snap name.

4. New helper `httpDownloadToFile(url string, dst io.Writer) error`:
   thin `http.Get` + status-code check + `io.Copy`. Transport-level
   integrity is intentionally not checked here — the seedwriter verifies
   the snap-revision sha3-384 against the downloaded bytes downstream.

New import: `net/http`.

## What stays unchanged

Everything else in snapd. The hook strictly replaces the
metadata-fetch-and-URL-resolution step (`tsto.DownloadMany` →
`SnapAction` round-trip). All other behavior runs:

- Trust chain verification (model → brand account-key → root account-key)
- `snap-revision` sha3-384 verification of downloaded blobs
- `snap-revision` / `snap-declaration` / `account` / `account-key`
  resolution (handled via the existing `AssertionRetrieve` hook — see
  below)
- Seed writing (`seedwriter.Writer`)
- Gadget yaml processing, bootloader configuration, disk image generation

The seed produced is bit-identical to what snapd would write against a
snap-store-compatible HTTP API. The resulting image needs no
post-flash manual snap activation.

## Companion: pre-existing `AssertionRetrieve` hook

The online-store flow depends on a hook that already exists in the fork:

```go
// image.Options
AssertionRetrieve func(ref *asserts.Ref) (asserts.Assertion, error)
```

ubuntu-image's manifest pipeline prefetches every required assertion
(snap-revision, snap-declaration, etc.) via m2cp and feeds them through
this hook so snapd resolves them locally instead of going to the store.
Not new in this iteration, but load-bearing for the online-store mode.

## Building / verifying

ubuntu-image uses the fork via `go.work`:

```
go 1.25

use (
    .
    /r/snapd
)
```

Run `bash test-online-store/test.sh` from the ubuntu-image repo to
exercise the full pipeline end-to-end against the live appstore.
A passing run produces a bootable `.img` and a `seed.manifest` whose
pinned revisions match the manifest input exactly.

## Out of scope (still in the tree, candidates for revert)

The fork currently carries additional changes from an earlier
offline-mode experiment that are **not** needed by the online-store
flow and should be reverted in a separate audit pass:

- `asserts/database.go`, `asserts/fetcher.go`: `SkipSignatureCheck`
  field on `DatabaseConfig`. The online flow uses the real chain (trust
  anchors injected via `sysdb.InjectTrusted` from
  `m2cp store system assertion`); no need to skip verification.
- `seed/seedwriter/helpers.go`, `seed20.go`, `writer.go`: offline-mode
  tweaks layered over the seedwriter.
- `snapdenv/trust.go`: previously held a `TrustInsecure` toggle; already
  deleted in the working tree.

Cleaning these up is independent of the two-file patch documented
above.
