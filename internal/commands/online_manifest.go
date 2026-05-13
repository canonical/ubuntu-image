package commands

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// OnlineManifest is the on-disk format for the --manifest flag. It
// pins a model and the snap versions that go with it. Versions (not
// revisions) because the user-facing input is human-readable;
// version<->revision is bijective in the target appstore and the CLI
// resolves the revision via m2cp before snapd fetches.
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

type OnlineManifestModel struct {
	Name         string `yaml:"name"`
	Revision     int    `yaml:"revision"`
	Architecture string `yaml:"architecture"`
}

// OnlineManifestBuild carries the four free-form provenance fields
// that end up in build.yaml. All fields are optional. At write time
// the matching BUILD_* env var (if set) takes precedence; otherwise
// the manifest value is used; otherwise "not set".
type OnlineManifestBuild struct {
	Version string `yaml:"version"`
	Commit  string `yaml:"commit"`
	Repo    string `yaml:"repo"`
	Grade   string `yaml:"grade"`
}

type OnlineManifestSnap struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
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

func (m *OnlineManifest) validate() error {
	if m.Model.Name == "" {
		return fmt.Errorf("model.name is required")
	}
	if m.Model.Revision <= 0 {
		return fmt.Errorf("model.revision must be > 0")
	}
	if m.Model.Architecture == "" {
		return fmt.Errorf("model.architecture is required")
	}
	if len(m.Snaps) == 0 {
		return fmt.Errorf("at least one snap pin is required")
	}
	if err := validatePins("snaps", m.Snaps); err != nil {
		return err
	}
	if err := validatePins("extra-snaps", m.ExtraSnaps); err != nil {
		return err
	}
	return nil
}

func validatePins(section string, pins []OnlineManifestSnap) error {
	for i, s := range pins {
		if s.Name == "" {
			return fmt.Errorf("%s[%d].name is required", section, i)
		}
		if s.Version == "" {
			return fmt.Errorf("%s[%d].version is required (snap %q)", section, i, s.Name)
		}
	}
	return nil
}
