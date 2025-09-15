package cmd

import (
    "fmt"
    "os"
    "path/filepath"

    "github.com/example/sre-ai/internal/config"
    "github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "config",
        Short: "Inspect or bootstrap CLI configuration",
    }

    cmd.AddCommand(newConfigInitCmd())
    cmd.AddCommand(newConfigShowCmd())
    return cmd
}

func newConfigInitCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "init",
        Short: "Create a starter configuration file",
        RunE: func(cmd *cobra.Command, args []string) error {
            cfgPath := globalOpts.ConfigPath
            if cfgPath == "" {
                path, err := config.DefaultConfigPath()
                if err != nil {
                    return err
                }
                cfgPath = path
            }

            if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
                return err
            }

            if _, err := os.Stat(cfgPath); err == nil {
                return fmt.Errorf("config exists at %s", cfgPath)
            }

            sample := defaultConfigYAML()
            if err := os.WriteFile(cfgPath, []byte(sample), 0o644); err != nil {
                return err
            }

            payload := map[string]any{"path": cfgPath}
            return printOutput(cmd, payload, fmt.Sprintf("Wrote config to %s", cfgPath))
        },
    }
}

func newConfigShowCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "show",
        Short: "Print effective configuration",
        RunE: func(cmd *cobra.Command, args []string) error {
            payload := map[string]any{
                "model":       globalOpts.Model,
                "provider":    globalOpts.Provider,
                "session":     globalOpts.Session,
                "caps":        globalOpts.Caps,
                "mcp_servers": globalOpts.MCPServers,
                "dry_run":     globalOpts.DryRun,
            }
            human := fmt.Sprintf("Model=%s Provider=%s", globalOpts.Model, globalOpts.Provider)
            return printOutput(cmd, payload, human)
        },
    }
}

func defaultConfigYAML() string {
    return `model: gpt-4o-mini
provider: openai
default_caps: [read_files]
mcp:
  servers:
    github: ~/.config/sre-ai/mcp/github.json
    files: ~/.config/sre-ai/mcp/files.json
contexts:
  k8s:
    kubecontext: prod-us
    namespace: default
iac:
  engine: terraform
  stacks:
    prod:
      path: ./infra/prod
logging:
  level: info
  redact: true
`
}
