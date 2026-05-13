package commands

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// OnlineManifest is the on-disk format for the --manifest flag. It
// fully describes a buildable image: the model definition (pushed to
// the appstore via m2cp), the pinned snap versions, and optional
// provenance metadata for the seed's build.yaml.
//
// Versions (not revisions) are user-facing because the user-facing
// input is human-readable; version<->revision is bijective in the
// target appstore and the CLI resolves the revision via m2cp before
// snapd fetches.
//
// The manifest is intended to be self-contained: given the file and
// an active m2cp session, the same artifact can be reproduced without
// additional flags or env vars.
type OnlineManifest struct {
	Model      OnlineManifestModel  `yaml:"model"`
	Build      OnlineManifestBuild  `yaml:"build"`
	Snaps      []OnlineManifestSnap `yaml:"snaps"`
	ExtraSnaps []OnlineManifestSnap `yaml:"extra-snaps"`
}

// OnlineManifestModel is the user-controlled portion of the model
// assertion. The builder uses it to generate the model.json payload
// pushed to the appstore (which fills authority-id, brand-id, and
// per-snap snap-id server-side, then signs and returns the assigned
// revision).
//
// `revision` is NOT in this struct: it's an output of the push step,
// not an input.
type OnlineManifestModel struct {
	Name          string `yaml:"name"`
	Architecture  string `yaml:"architecture"`
	Base          string `yaml:"base"`
	Grade         string `yaml:"grade"`          // signed | secured | dangerous
	StorageSafety string `yaml:"storage-safety"` // optional, e.g. prefer-encrypted
}

// OnlineManifestBuild carries free-form provenance fields that end up
// in the seed's build.yaml. All fields are optional. At write time
// the matching BUILD_* env var (if set) takes precedence; otherwise
// the manifest value is used; otherwise "not set".
//
// `grade` was removed: it conflated release-channel grade
// (stable/experimental/edge) with snapd's model grade. The build.yaml
// `grade` column now sources from model.grade.
type OnlineManifestBuild struct {
	Version string `yaml:"version"`
	Commit  string `yaml:"commit"`
	Repo    string `yaml:"repo"`
}

// OnlineManifestSnap describes one snap. `type` feeds the generated
// model.json; `version` drives the build-time revision pinning. The
// model.json's `default-channel` is hardcoded to "latest/stable" by
// the generator -- the appstore doesn't use channels in our flow.
// Snaps in `extra-snaps` only need name + version since they aren't
// part of the model.
type OnlineManifestSnap struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
	Type    string `yaml:"type"` // snapd | gadget | kernel | base | app (model snaps only)
}

func LoadOnlineManifest(path string) (*OnlineManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read manifest %q: %w", path, err)
	}
	var m OnlineManifest
	if err := yaml.UnmarshalStrict(data, &m); err != nil {
		return nil, fmt.Errorf("cannot parse manifest %q: %w", path, err)
	}
	if err := m.validate(); err != nil {
		return nil, fmt.Errorf("invalid manifest %q: %w", path, err)
	}
	return &m, nil
}

var validSnapTypes = map[string]bool{
	"snapd":  true,
	"gadget": true,
	"kernel": true,
	"base":   true,
	"app":    true,
}

var validModelGrades = map[string]bool{
	"signed":    true,
	"secured":   true,
	"dangerous": true,
}

// validate applies the load-time checks: enough of the manifest is
// structurally valid that we can render the model.json. Version
// pins are NOT required here -- the build flow calls
// ValidateForBuild for that, and the `ubuntu-image model` debug
// subcommand stops at this point.
func (m *OnlineManifest) validate() error {
	if m.Model.Name == "" {
		return fmt.Errorf("model.name is required")
	}
	if m.Model.Architecture == "" {
		return fmt.Errorf("model.architecture is required")
	}
	if m.Model.Base == "" {
		return fmt.Errorf("model.base is required")
	}
	if m.Model.Grade == "" {
		m.Model.Grade = "signed"
	}
	if !validModelGrades[m.Model.Grade] {
		return fmt.Errorf("model.grade %q is not one of: signed, secured, dangerous", m.Model.Grade)
	}
	if len(m.Snaps) == 0 {
		return fmt.Errorf("at least one snap is required")
	}
	for i, s := range m.Snaps {
		if s.Name == "" {
			return fmt.Errorf("snaps[%d].name is required", i)
		}
		if s.Type == "" {
			return fmt.Errorf("snaps[%d].type is required (snap %q)", i, s.Name)
		}
		if !validSnapTypes[s.Type] {
			return fmt.Errorf("snaps[%d].type %q (snap %q) is not one of: snapd, gadget, kernel, base, app",
				i, s.Type, s.Name)
		}
	}
	for i, s := range m.ExtraSnaps {
		if s.Name == "" {
			return fmt.Errorf("extra-snaps[%d].name is required", i)
		}
	}
	return nil
}

// ValidateForBuild adds the version-pin checks needed before
// driving a real build. The model-render subcommand doesn't need
// these and skips this step.
func (m *OnlineManifest) ValidateForBuild() error {
	for i, s := range m.Snaps {
		if s.Version == "" {
			return fmt.Errorf("snaps[%d].version is required for builds (snap %q)", i, s.Name)
		}
	}
	for i, s := range m.ExtraSnaps {
		if s.Version == "" {
			return fmt.Errorf("extra-snaps[%d].version is required for builds (snap %q)", i, s.Name)
		}
	}
	return nil
}
