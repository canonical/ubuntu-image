package commands

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// OnlineManifest is the on-disk format for the --manifest flag.
// It pins a model and the snap versions that go with it. Versions
// (not revisions) because the user-facing input is human-readable;
// version<->revision is bijective in the target appstore and the
// CLI resolves the revision via m2cp before snapd fetches.
type OnlineManifest struct {
	Model OnlineManifestModel `yaml:"model"`
	Snaps []OnlineManifestSnap `yaml:"snaps"`
}

type OnlineManifestModel struct {
	Name         string `yaml:"name"`
	Revision     int    `yaml:"revision"`
	Architecture string `yaml:"architecture"`
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
	for i, s := range m.Snaps {
		if s.Name == "" {
			return fmt.Errorf("snaps[%d].name is required", i)
		}
		if s.Version == "" {
			return fmt.Errorf("snaps[%d].version is required (snap %q)", i, s.Name)
		}
	}
	return nil
}
