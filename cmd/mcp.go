package cmd

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "time"

    "github.com/example/sre-ai/internal/mcp"
    "github.com/spf13/cobra"
)

func newMCPCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "mcp",
        Short: "Manage MCP server integrations",
    }

    cmd.AddCommand(newMCPLsCmd())
    cmd.AddCommand(newMCPAddCmd())
    cmd.AddCommand(newMCPRmCmd())
    cmd.AddCommand(newMCPTestCmd())
    return cmd
}

func newMCPLsCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "ls",
        Short: "List configured MCP servers",
        RunE: func(cmd *cobra.Command, args []string) error {
            infos := mcp.DefaultRegistry.Snapshot()
            payload := map[string]any{"servers": infos}

            var builder strings.Builder
            if len(infos) == 0 {
                builder.WriteString("No MCP servers configured")
            } else {
                for _, info := range infos {
                    builder.WriteString(fmt.Sprintf("- %s [%s]", info.Alias, info.Source))
                    if info.Command != "" {
                        builder.WriteString(fmt.Sprintf(" -> %s %s", info.Command, strings.Join(info.Args, " ")))
                    }
                    if info.Origin != "" {
                        builder.WriteString(fmt.Sprintf(" (from %s)", info.Origin))
                    }
                    builder.WriteString("\n")
                }
            }

            return printOutput(cmd, payload, strings.TrimSpace(builder.String()))
        },
    }
}

func newMCPAddCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "add <alias=path>",
        Short: "Add or update a local MCP server definition",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            alias, path, err := splitAliasPath(args[0])
            if err != nil {
                return err
            }
            def, err := mcp.LoadLocalDefinitionFromFile(alias, path)
            if err != nil {
                return err
            }
            if err := mcp.AddLocalServer(alias, def, expandPathForDisplay(path)); err != nil {
                return err
            }
            payload := map[string]any{
                "alias":   alias,
                "command": def.Command,
                "args":    def.Args,
            }
            human := fmt.Sprintf("Saved MCP server %s", alias)
            return printOutput(cmd, payload, human)
        },
    }
}

func newMCPRmCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "rm <alias>",
        Short: "Remove a configured MCP server",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            alias := args[0]
            if err := mcp.RemoveLocalServer(alias); err != nil {
                return err
            }
            payload := map[string]any{"alias": alias}
            return printOutput(cmd, payload, fmt.Sprintf("Removed MCP server %s", alias))
        },
    }
}

func newMCPTestCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "test <alias>",
        Short: "Launch a local MCP server to verify configuration",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            alias := args[0]
            ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
            defer cancel()
            if err := mcp.TestLocalServer(ctx, alias); err != nil {
                return err
            }
            payload := map[string]any{
                "alias":  alias,
                "status": "ok",
            }
            return printOutput(cmd, payload, fmt.Sprintf("MCP server %s is launchable", alias))
        },
    }
}

func splitAliasPath(input string) (string, string, error) {
    parts := strings.SplitN(input, "=", 2)
    if len(parts) != 2 {
        return "", "", fmt.Errorf("expected alias=path, got %s", input)
    }
    alias := strings.TrimSpace(parts[0])
    path := strings.TrimSpace(parts[1])
    if alias == "" || path == "" {
        return "", "", fmt.Errorf("invalid alias=path expression: %s", input)
    }
    return alias, path, nil
}

func expandPathForDisplay(path string) string {
    expanded := path
    if strings.HasPrefix(path, "~") {
        home, err := os.UserHomeDir()
        if err == nil {
            expanded = filepath.Join(home, strings.TrimPrefix(path, "~"))
        }
    }
    if abs, err := filepath.Abs(expanded); err == nil {
        return abs
    }
    return expanded
}



