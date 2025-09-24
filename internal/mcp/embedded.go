package mcp

import (
    "encoding/json"
    "fmt"

    _ "embed"
)

//go:embed testdata/github.json
var githubManifestBytes []byte

//go:embed testdata/files.json
var filesManifestBytes []byte

func loadEmbeddedDefaults() error {
    defaultManifests := map[string][]byte{
        "github": githubManifestBytes,
        "files":  filesManifestBytes,
    }

    for alias, data := range defaultManifests {
        manifest, err := parseManifest(data)
        if err != nil {
            return fmt.Errorf("parse embedded manifest %s: %w", alias, err)
        }
        DefaultRegistry.RegisterManifest(alias, manifest, SourceEmbedded, "embedded")
    }

    return nil
}

// MarshalManifest provides a copy of the manifest JSON for display or persistence.
func MarshalManifest(m Manifest) ([]byte, error) {
    if len(m.Raw) > 0 {
        return append([]byte(nil), m.Raw...), nil
    }
    return json.MarshalIndent(m, "", "  ")
}
