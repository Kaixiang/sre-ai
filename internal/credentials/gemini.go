package credentials

import (
    "encoding/json"
    "errors"
    "fmt"
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

// LoadGeminiKey retrieves the persisted Gemini API key if present.
func LoadGeminiKey() (string, error) {
    path, err := GeminiKeyPath()
    if err != nil {
        return "", err
    }

    data, err := os.ReadFile(path)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return "", fmt.Errorf("gemini credentials not found; run 'sre-ai config login --provider gemini'")
        }
        return "", err
    }

    var payload geminiCredential
    if err := json.Unmarshal(data, &payload); err != nil {
        return "", err
    }
    if payload.APIKey == "" {
        return "", fmt.Errorf("gemini credential file %s missing api_key", path)
    }
    return payload.APIKey, nil
}
