# L-IoT Image Builder for Ubuntu Core based systems

Based on Canonical's [ubuntu-image](https://github.com/canonical/ubuntu-image),
extended to build images against our self-hosted, snap-store-compatible
appstore from a single YAML recipe file.

The released binary is `liot-image`; the source tree, Go module, and
`cmd/ubuntu-image/` directory keep their upstream names. "L-IoT Image
Builder" refers to this repo and the manifest-mode workflow.

For the fork's architectural changes (snapd hooks, manifest pipeline,
build.yaml generation), see [MODIFICATIONS.md](MODIFICATIONS.md).

## What it does

Given one YAML recipe file and an active Appstore session using the `m2cp` CLI tool, produce a
bootable Ubuntu Core image whose seeded snaps come from our appstore
at the exact revisions pinned in the recipe. The output is
bit-identical to one built against a Canonical-style snap store:
every assertion is verified, every snap blob is sha3-checked, no
`--dangerous` or sideload paths involved.

## Prerequisites

- Linux (Ubuntu 22.04+ recommended)
- `m2cp` CLI on `PATH`, with an active session for the target store
  (`m2cp user login`)
- The matching snapd fork at `/r/snapd` (linked via `go.work`)

## Usage

The recipe file is a YAML descriptor. It can be named anything; we
suggest a self-describing convention that captures the OS line, the
target board, and the build version:

```
liot-uc-imx93-1.2.3.yaml      # ubuntu-core build for imx93 board, v1.2.3
liot-uc-imx8mp-2.0.0.yaml     # ubuntu-core build for imx8mp board, v2.0.0
```

(`uc` = ubuntu-core; other OS lines would use their own prefix.)

To build an image:

```
liot-image liot-uc-imx93-1.2.3.yaml
```

That's the whole interface: one positional argument, the recipe.
The produced image is named after the recipe — `liot-uc-imx93-1.2.3.yaml`
yields `liot-uc-imx93-1.2.3.img` — rather than after the gadget's
internal volume name. Alongside the image, a
`liot-uc-imx93-1.2.3.model.json` sidecar is written so operators
have a self-contained reference for the exact model definition the
appstore was asked to sign. Output lands in `./bin/` by default;
pass `-O DIR` to override.

Three optional flags:

- `--dry-run` runs preflight (m2cp on PATH, session active, every
  recipe snap present in the appstore) plus a model scan — it reports
  whether the recipe's model would reuse an existing appstore revision
  or be pushed as a new one (with a per-revision snap diff) — then
  exits without pushing or building.
- `--xz` xz-compresses the image in place once the build finishes,
  producing `<recipe>.img.xz`. The model.json sidecar stays
  uncompressed. Requires `xz` on PATH.
- `--model` renders the recipe's model.json to stdout and exits.
  Pure transformation: no network, no build. Useful for inspecting
  exactly what will be sent to the appstore.

The builder:

1. Reads the recipe.
2. Fetches the model assertion from the store via m2cp.
3. Pulls the trust bundle from the store and installs it into
   snapd's assertion database for this build.
4. Resolves every pinned snap *version* to a *revision* via m2cp
   (versions are bijective with revisions in the target appstore).
5. Hands snapd a URL resolver and an assertion retriever so its
   normal seed pipeline talks to the appstore instead of the
   Canonical metadata API.
6. Writes a `build.yaml` into the seed describing what was built.

Every step narrates itself to stderr in plain language so the build
log is a readable audit trail. The output of step 6 means an image
is self-identifying: see "Inspecting an image" below.

## Recipe schema

```yaml
# REQUIRED: the user-controlled portion of the model assertion. The
# builder renders model.json from these fields and pushes it to the
# appstore, which fills authority-id, brand-id and per-snap snap-ids
# server-side, signs, and returns the assigned revision. The model
# revision is therefore an *output* of the push step and is not
# specified here.
model:
  name: edge-imx93-uc-vtg
  architecture: arm64
  base: core24
  grade: signed                     # one of: signed, secured, dangerous (default: signed)
  storage-safety: prefer-encrypted  # optional

# OPTIONAL: provenance metadata, copied into the seed's build.yaml.
# Each field can be overridden at build time by the matching BUILD_*
# env var; the override is announced in the build log. If neither
# the manifest nor env supplies a value, "not set" is written.
build:
  version: "1.2.3"
  commit: "abc1234de56789f0fedcba0123456789abcdef01"
  repo: "github.com/foo/bar"

# REQUIRED: every snap the model declares, pinned by version. The
# `type` is required and feeds the generated model.json. The builder
# resolves each version to a concrete revision via
# `m2cp store snap info` before the build starts.
snaps:
  - name: snapd
    type: snapd
    version: "4.3.0.42"
  - name: core24
    type: base
    version: "1.2.0"
  - name: m2cp-os-imx93-gadget
    type: gadget
    version: "5.5.12"
  - name: m2cp-os-imx93-kernel
    type: kernel
    version: "6.1.5"
  - name: m2cp-gateway
    type: app
    version: "3.1.0"
  # ... one entry per snap in the model

# OPTIONAL: snaps not declared by the model but still seeded into
# the image. Only name + version are needed (these aren't part of
# the model, so no `type`). Treated by the seedwriter as additional
# snaps; they end up in the image alongside the model-declared ones.
extra-snaps:
  - name: htop
    version: "3.0.5"
```

The recipe is meant to be self-contained: given the file and an
active m2cp session, the same artifact rebuilds with no additional
flags or env vars.

## What ends up in the image

A bootable disk image (e.g. `.img`) plus, in the seed partition:

```
systems/<label>/
├── model                      # the model assertion
├── assertions/                # account + account-key + snap chain
├── snaps/                     # the pinned .snap blobs
└── build.yaml                 # provenance (see below)
```

And in the output directory alongside the image:

```
seed.manifest                  # name + revision for each seeded snap
```

## build.yaml contents

The builder writes a stable schema with 12 keys; unknown values
become `"not set"` so the shape is the same on every image:

| key | source |
|---|---|
| `builder` | always `"liot-image"` (this binary) |
| `builder-version` | linker-set `Version` (set from `git describe --tags --always --dirty` by build.sh and the release workflow), else `"not set"` |
| `built-by` | developer identity from `m2cp user info --json`, else `"not set"` |
| `appstore-url` | snap-store URL discovered via `m2cp user status --json` |
| `model-name` | from the recipe |
| `model-revision` | revision returned by the appstore when the model was pushed |
| `arch` | from the recipe |
| `date` | UTC ISO 8601 at build start |
| `version` | `$BUILD_VERSION`, else `build.version` from recipe, else `"not set"` |
| `commit` | `$BUILD_COMMIT`, else `build.commit` from recipe, else `"not set"` |
| `repo` | `$BUILD_REPO`, else `build.repo` from recipe, else `"not set"` |
| `grade` | from `model.grade` in the recipe |

## Inspecting an image

`m2cp image analyze <image.img(.xz)>` reads back `build.yaml` and
the model assertion's identity headers. The cryptographically-signed
model name, revision, and brand-id (under `model-name`,
`model-revision`, `model-brand-id`) take precedence over any
descriptive fields in `build.yaml`. Output honors `--json`.

The same tool also reads images that **weren't** built with this
fork — it falls back to whatever `build.yaml` (or none) the other
build pipeline wrote, supplementing with whatever the model
assertion authoritatively provides.

## License

GPL-3.0, inherited from upstream ubuntu-image. See [LICENSE](LICENSE).
