package credentials

import (
    "encoding/json"
    "os"
    "path/filepath"
    "time"

    "github.com/example/sre-ai/internal/config"
)

const (
    credentialsDirName   = "credentials"
    geminiCredentialFile = "gemini.json"
)

type geminiCredential struct {
    APIKey  string `json:"api_key"`
    Created string `json:"created"`
}

// GeminiKeyPath returns the path where Gemini credentials are stored.
func GeminiKeyPath() (string, error) {
    base, err := config.ConfigDir()
    if err != nil {
        return "", err
    }
    return filepath.Join(base, credentialsDirName, geminiCredentialFile), nil
}

// SaveGeminiKey persists the provided API key to disk.
func SaveGeminiKey(key string) (string, error) {
    path, err := GeminiKeyPath()
    if err != nil {
        return "", err
    }

    if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
        return "", err
    }

    payload := geminiCredential{
        APIKey:  key,
        Created: time.Now().UTC().Format(time.RFC3339),
    }

    data, err := json.MarshalIndent(payload, "", "  ")
    if err != nil {
        return "", err
    }

    if err := os.WriteFile(path, data, 0o600); err != nil {
        return "", err
    }

    return path, nil
}
