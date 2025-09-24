package mcp

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "sort"
    "strings"
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

// ServerDefinition describes how to launch a local MCP server process.
type ServerDefinition struct {
    Command string            `json:"command"`
    Args    []string          `json:"args"`
    Env     map[string]string `json:"env"`
    Workdir string            `json:"workdir"`
    Notes   string            `json:"notes,omitempty"`
}

// Source enumerates how an MCP server was registered.
type Source string

const (
    SourceEmbedded Source = "embedded"
    SourceConfig   Source = "config"
    SourceLocal    Source = "local"
)

// Client represents a connection to an MCP server manifest or local definition.
type Client struct {
    Alias      string
    Manifest   *Manifest
    Definition *ServerDefinition
    Source     Source
    Origin     string
}

// ClientInfo is a serialisable description of a registered server.
type ClientInfo struct {
    Alias   string   `json:"alias"`
    Source  string   `json:"source"`
    Command string   `json:"command,omitempty"`
    Args    []string `json:"args,omitempty"`
    Origin  string   `json:"origin,omitempty"`
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

// Reset removes every entry from the registry.
func (r *Registry) Reset() {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.clients = make(map[string]*Client)
}

// RegisterManifest adds a manifest-based server to the registry.
func (r *Registry) RegisterManifest(alias string, manifest Manifest, source Source, origin string) {
    r.mu.Lock()
    defer r.mu.Unlock()
    copy := manifest
    r.clients[alias] = &Client{Alias: alias, Manifest: &copy, Source: source, Origin: origin}
}

// RegisterLocal stores a local command-based server definition.
func (r *Registry) RegisterLocal(alias string, def ServerDefinition, origin string) {
    r.mu.Lock()
    defer r.mu.Unlock()
    copy := def
    r.clients[alias] = &Client{Alias: alias, Definition: &copy, Source: SourceLocal, Origin: origin}
}

// Remove deletes a server from the registry.
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

// Snapshot returns detailed client information suitable for display.
func (r *Registry) Snapshot() []ClientInfo {
    r.mu.RLock()
    defer r.mu.RUnlock()
    infos := make([]ClientInfo, 0, len(r.clients))
    for _, client := range r.clients {
        info := ClientInfo{Alias: client.Alias, Source: string(client.Source), Origin: client.Origin}
        if client.Definition != nil {
            info.Command = client.Definition.Command
            info.Args = append([]string(nil), client.Definition.Args...)
        }
        infos = append(infos, info)
    }
    sort.Slice(infos, func(i, j int) bool { return infos[i].Alias < infos[j].Alias })
    return infos
}

// DefaultRegistry is the singleton used by the CLI.
var DefaultRegistry = NewRegistry()

// Warmup loads embedded defaults, config manifests, and local server definitions.
func Warmup(ctx context.Context, opts *config.GlobalOptions) error {
    DefaultRegistry.Reset()

    if err := loadEmbeddedDefaults(); err != nil {
        return err
    }

    for alias, location := range opts.MCPServers {
        manifest, err := LoadManifest(location)
        if err != nil {
            return fmt.Errorf("load manifest %s: %w", alias, err)
        }
        DefaultRegistry.RegisterManifest(alias, manifest, SourceConfig, expandPath(location))
    }

    if err := registerLocalServers(); err != nil {
        return err
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
    if strings.HasPrefix(input, "~") {
        home, err := os.UserHomeDir()
        if err == nil {
            return filepath.Join(home, strings.TrimPrefix(input, "~"))
        }
    }
    return input
}

