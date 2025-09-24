package mcp

import (
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "sort"

    "github.com/example/sre-ai/internal/config"
)

type serverStore struct {
    Servers map[string]ServerDefinition `json:"mcpServers"`
}

func registerLocalServers() error {
    store, path, err := loadServerStore()
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return nil
        }
        return err
    }
    for alias, def := range store.Servers {
        DefaultRegistry.RegisterLocal(alias, def, path)
    }
    return nil
}

// AddLocalServer persists a server definition and updates the registry.
func AddLocalServer(alias string, def ServerDefinition, origin string) error {
    if alias == "" {
        return errors.New("alias cannot be empty")
    }
    if def.Command == "" {
        return errors.New("server command cannot be empty")
    }

    store, path, err := loadServerStore()
    if err != nil && !errors.Is(err, os.ErrNotExist) {
        return err
    }
    if store.Servers == nil {
        store.Servers = make(map[string]ServerDefinition)
    }
    store.Servers[alias] = def
    if err := saveServerStore(path, store); err != nil {
        return err
    }

    DefaultRegistry.RegisterLocal(alias, def, path)
    return nil
}

// RemoveLocalServer deletes a stored definition and updates the registry.
func RemoveLocalServer(alias string) error {
    store, path, err := loadServerStore()
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return fmt.Errorf("no MCP servers registered")
        }
        return err
    }

    if _, ok := store.Servers[alias]; !ok {
        return fmt.Errorf("unknown MCP server %s", alias)
    }
    delete(store.Servers, alias)
    if err := saveServerStore(path, store); err != nil {
        return err
    }
    DefaultRegistry.Remove(alias)
    return nil
}

// ListLocalServers returns all persisted local server definitions.
func ListLocalServers() (map[string]ServerDefinition, error) {
    store, _, err := loadServerStore()
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return map[string]ServerDefinition{}, nil
        }
        return nil, err
    }

    out := make(map[string]ServerDefinition, len(store.Servers))
    for alias, def := range store.Servers {
        out[alias] = def
    }
    return out, nil
}

// GetLocalServer fetches a single stored definition.
func GetLocalServer(alias string) (ServerDefinition, error) {
    store, _, err := loadServerStore()
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return ServerDefinition{}, fmt.Errorf("no MCP servers registered")
        }
        return ServerDefinition{}, err
    }
    def, ok := store.Servers[alias]
    if !ok {
        return ServerDefinition{}, fmt.Errorf("unknown MCP server %s", alias)
    }
    return def, nil
}

func loadServerStore() (serverStore, string, error) {
    path, err := serverStorePath()
    if err != nil {
        return serverStore{}, "", err
    }
    data, err := os.ReadFile(path)
    if err != nil {
        return serverStore{Servers: make(map[string]ServerDefinition)}, path, err
    }
    var store serverStore
    if err := json.Unmarshal(data, &store); err != nil {
        return serverStore{}, path, err
    }
    if store.Servers == nil {
        store.Servers = make(map[string]ServerDefinition)
    }
    return store, path, nil
}

func saveServerStore(path string, store serverStore) error {
    if path == "" {
        return errors.New("invalid server store path")
    }
    dir := filepath.Dir(path)
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return err
    }
    data, err := json.MarshalIndent(store, "", "  ")
    if err != nil {
        return err
    }
    return os.WriteFile(path, data, 0o600)
}

func serverStorePath() (string, error) {
    base, err := config.ConfigDir()
    if err != nil {
        return "", err
    }
    return filepath.Join(base, "mcp", "servers.json"), nil
}

// LoadLocalDefinitionFromFile parses a server definition from disk.
func LoadLocalDefinitionFromFile(alias, path string) (ServerDefinition, error) {
    expanded := expandPath(path)
    data, err := os.ReadFile(expanded)
    if err != nil {
        return ServerDefinition{}, err
    }

    // Attempt to parse full store form first.
    var store serverStore
    if err := json.Unmarshal(data, &store); err == nil && len(store.Servers) > 0 {
        if alias == "" {
            // choose deterministic alias when only one entry exists
            if len(store.Servers) == 1 {
                keys := make([]string, 0, len(store.Servers))
                for k := range store.Servers {
                    keys = append(keys, k)
                }
                sort.Strings(keys)
                return store.Servers[keys[0]], nil
            }
            return ServerDefinition{}, errors.New("alias required when file contains multiple servers")
        }
        if def, ok := store.Servers[alias]; ok {
            return def, nil
        }
        return ServerDefinition{}, fmt.Errorf("alias %s not found in %s", alias, path)
    }

    // Fallback to a single definition structure.
    var def ServerDefinition
    if err := json.Unmarshal(data, &def); err != nil {
        return ServerDefinition{}, fmt.Errorf("parse server definition: %w", err)
    }
    return def, nil
}
