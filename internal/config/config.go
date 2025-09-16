package config

import (
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/spf13/viper"
)

// GlobalOptions captures globally available CLI flags.
type GlobalOptions struct {
    Model         string
    Provider      string
    Temperature   float64
    MaxTokens     int
    Session       string
    JSON          bool
    Quiet         bool
    Verbose       int
    NoInteractive bool
    ConfigPath    string
    MCPServers    map[string]string
    Caps          []string
    DryRun        bool
    AutoConfirm   bool
}

// ConfigDir returns the directory that stores sre-ai configuration artifacts.
func ConfigDir() (string, error) {
    home, err := os.UserHomeDir()
    if err != nil {
        return "", fmt.Errorf("resolve home dir: %w", err)
    }
    return filepath.Join(home, ".config", "sre-ai"), nil
}

// DefaultConfigPath resolves the default config file path.
func DefaultConfigPath() (string, error) {
    dir, err := ConfigDir()
    if err != nil {
        return "", err
    }
    return filepath.Join(dir, "config.yaml"), nil
}

// Load merges configuration from file and environment into the provided options.
func Load(opts *GlobalOptions) error {
    if opts.MCPServers == nil {
        opts.MCPServers = make(map[string]string)
    }

    v := viper.New()
    v.SetConfigType("yaml")
    v.AutomaticEnv()
    v.SetEnvPrefix("sre_ai")
    v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

    cfgPath := opts.ConfigPath
    if cfgPath == "" {
        defaultPath, err := DefaultConfigPath()
        if err != nil {
            return err
        }
        cfgPath = defaultPath
    }

    v.SetConfigFile(cfgPath)
    if err := v.ReadInConfig(); err != nil {
        var pathErr *os.PathError
        if errors.As(err, &pathErr) || strings.Contains(err.Error(), "Not Found") {
            return nil
        }
        return err
    }

    var fileCfg struct {
        Model       string            `mapstructure:"model"`
        Provider    string            `mapstructure:"provider"`
        DefaultCaps []string          `mapstructure:"default_caps"`
        MCP         struct {
            Servers map[string]string `mapstructure:"servers"`
        } `mapstructure:"mcp"`
    }

    if err := v.Unmarshal(&fileCfg); err != nil {
        return fmt.Errorf("parse config: %w", err)
    }

    if opts.Model == "" {
        opts.Model = fileCfg.Model
    }
    if opts.Provider == "" {
        opts.Provider = fileCfg.Provider
    }
    if len(opts.Caps) == 0 && len(fileCfg.DefaultCaps) > 0 {
        opts.Caps = append(opts.Caps, fileCfg.DefaultCaps...)
    }
    for k, v := range fileCfg.MCP.Servers {
        if _, ok := opts.MCPServers[k]; !ok {
            opts.MCPServers[k] = v
        }
    }

    return nil
}
