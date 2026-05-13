# L-IoT Image Builder for Ubuntu Core based systems

Based on Canonical's [ubuntu-image](https://github.com/canonical/ubuntu-image),
extended to build images against our self-hosted, snap-store-compatible
appstore from a single YAML recipe file.

The binary is still invoked as `ubuntu-image` (the file is renamed to
keep the upstream interface stable); this repo and the manifest-mode
workflow are what the "L-IoT Image Builder" name refers to.

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
ubuntu-image snap --manifest liot-uc-imx93-1.2.3.yaml -O ./out/
```

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
# REQUIRED: which Ubuntu Core model to build.
# Fetched from the store via `m2cp store model assertion`.
model:
  name: edge-imx93-uc-vtg
  revision: 1
  architecture: arm64

# OPTIONAL: provenance metadata, copied into the seed's build.yaml.
# Each field can be overridden at build time by the matching BUILD_*
# env var; the override is announced in the build log. If neither
# the manifest nor env supplies a value, "not set" is written.
build:
  version: "1.2.3"
  commit: "abc1234de56789f0fedcba0123456789abcdef01"
  repo: "github.com/foo/bar"
  grade: "stable"     # one of: stable, experimental, edge

# REQUIRED: every snap the model declares, pinned by version.
# The builder resolves each to a concrete revision via
# `m2cp store snap info` before the build starts.
snaps:
  - name: snapd
    version: "4.3.0.42"
  - name: core24
    version: "1.2.0"
  - name: mlpa-os-imx-kernel
    version: "6.1.5"
  # ... one entry per snap in the model

# OPTIONAL: snaps not declared by the model but still seeded into
# the image. Same name+version format. Treated by the seedwriter as
# additional snaps; they end up in the image alongside the
# model-declared ones.
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

The builder writes a stable schema with 11 keys; unknown values
become `"not set"` so the shape is the same on every image:

| key | source |
|---|---|
| `builder` | always `"ubuntu-image"` (this binary) |
| `builder-version` | linker-set `Version`, else `"not set"` |
| `appstore-url` | snap-store URL discovered via `m2cp user status --json` |
| `model-name` | from the recipe |
| `model-revision` | from the recipe |
| `arch` | from the recipe |
| `date` | UTC ISO 8601 at build start |
| `version` | `$BUILD_VERSION`, else `build.version` from recipe, else `"not set"` |
| `commit` | `$BUILD_COMMIT`, else `build.commit` from recipe, else `"not set"` |
| `repo` | `$BUILD_REPO`, else `build.repo` from recipe, else `"not set"` |
| `grade` | `$BUILD_GRADE`, else `build.grade` from recipe, else `"not set"` |

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
