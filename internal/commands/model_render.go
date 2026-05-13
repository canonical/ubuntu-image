package commands

import (
	"encoding/json"
	"time"
)

// modelJSON is the shape m2cp's `store system model push` expects.
// authority-id, brand-id, and each snap id are emitted as the
// literal "PLACEHOLDER" string -- the appstore fills them in when
// it signs the assertion server-side. Timestamp is ignored
// server-side but emitted for completeness.
type modelJSON struct {
	Type          string          `json:"type"`
	Series        string          `json:"series"`
	AuthorityID   string          `json:"authority-id"`
	BrandID       string          `json:"brand-id"`
	Model         string          `json:"model"`
	Architecture  string          `json:"architecture"`
	Timestamp     string          `json:"timestamp"`
	Base          string          `json:"base"`
	Grade         string          `json:"grade"`
	StorageSafety string          `json:"storage-safety,omitempty"`
	Snaps         []modelJSONSnap `json:"snaps"`
}

type modelJSONSnap struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	DefaultChannel string `json:"default-channel"`
	ID             string `json:"id"`
}

// RenderModelJSON renders the recipe's model definition into the JSON
// shape m2cp's `store system model push` consumes. authority-id,
// brand-id, and per-snap id are emitted as "PLACEHOLDER"; the
// appstore replaces them at signing time. default-channel is
// hardcoded to "latest/stable" -- our appstore doesn't use channels.
func RenderModelJSON(m *OnlineManifest) ([]byte, error) {
	snaps := make([]modelJSONSnap, 0, len(m.Snaps))
	for _, s := range m.Snaps {
		snaps = append(snaps, modelJSONSnap{
			Name:           s.Name,
			Type:           s.Type,
			DefaultChannel: "latest/stable",
			ID:             "PLACEHOLDER",
		})
	}
	doc := modelJSON{
		Type:          "model",
		Series:        "16",
		AuthorityID:   "PLACEHOLDER",
		BrandID:       "PLACEHOLDER",
		Model:         m.Model.Name,
		Architecture:  m.Model.Architecture,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Base:          m.Model.Base,
		Grade:         m.Model.Grade,
		StorageSafety: m.Model.StorageSafety,
		Snaps:         snaps,
	}
	return json.MarshalIndent(doc, "", "  ")
}
