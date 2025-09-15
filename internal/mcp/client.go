package mcp

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "sort"
    "sync"

    "github.com/example/sre-ai/internal/config"
)

// Manifest models the subset of an MCP manifest understood by the CLI.
type Manifest struct {
    Name         string           `json:"name"`
    Version      string           `json:"version"`
    Transport    map[string]any   `json:"transport"`
    Auth         map[string]any   `json:"auth"`
    Tools        []map[string]any `json:"tools"`
    Resources    []map[string]any `json:"resources"`
    Capabilities []string         `json:"capabilities"`
    Raw          json.RawMessage  `json:"-"`
}

// Client represents a connection to an MCP server manifest.
type Client struct {
    Alias    string
    Manifest Manifest
}

// Registry maintains MCP clients keyed by alias.
type Registry struct {
    mu      sync.RWMutex
    clients map[string]*Client
}

// NewRegistry constructs an empty registry.
func NewRegistry() *Registry {
    return &Registry{clients: make(map[string]*Client)}
}

// Register adds a manifest to the registry.
func (r *Registry) Register(alias string, manifest Manifest) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.clients[alias] = &Client{Alias: alias, Manifest: manifest}
}

// Remove deletes a manifest from the registry.
func (r *Registry) Remove(alias string) {
    r.mu.Lock()
    defer r.mu.Unlock()
    delete(r.clients, alias)
}

// Get returns a registered client if present.
func (r *Registry) Get(alias string) (*Client, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    client, ok := r.clients[alias]
    return client, ok
}

// List returns all client aliases in lexical order.
func (r *Registry) List() []string {
    r.mu.RLock()
    defer r.mu.RUnlock()
    aliases := make([]string, 0, len(r.clients))
    for alias := range r.clients {
        aliases = append(aliases, alias)
    }
    sort.Strings(aliases)
    return aliases
}

// DefaultRegistry is the singleton used by the CLI.
var DefaultRegistry = NewRegistry()

// Warmup loads manifests configured via flags or config file.
func Warmup(ctx context.Context, opts *config.GlobalOptions) error {
    if len(opts.MCPServers) == 0 {
        return loadEmbeddedDefaults()
    }

    for alias, location := range opts.MCPServers {
        manifest, err := LoadManifest(location)
        if err != nil {
            return fmt.Errorf("load manifest %s: %w", alias, err)
        }
        DefaultRegistry.Register(alias, manifest)
    }

    return nil
}

// LoadManifest reads a manifest file from disk.
func LoadManifest(path string) (Manifest, error) {
    expanded := expandPath(path)
    data, err := os.ReadFile(expanded)
    if err != nil {
        return Manifest{}, err
    }
    return parseManifest(data)
}

func parseManifest(data []byte) (Manifest, error) {
    var m Manifest
    if err := json.Unmarshal(data, &m); err != nil {
        return Manifest{}, err
    }
    m.Raw = append([]byte(nil), data...)
    return m, nil
}

func expandPath(input string) string {
    if input == "" {
        return input
    }
    if input[0] == '~' {
        home, err := os.UserHomeDir()
        if err == nil {
            return filepath.Join(home, input[1:])
        }
    }
    return input
}
