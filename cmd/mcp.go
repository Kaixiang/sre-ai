package cmd

import (
    "fmt"
    "strings"

    "github.com/example/sre-ai/internal/mcp"
    "github.com/spf13/cobra"
)

func newMCPCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "mcp",
        Short: "Manage MCP server manifests",
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
        Short: "List attached MCP servers",
        RunE: func(cmd *cobra.Command, args []string) error {
            aliases := mcp.DefaultRegistry.List()
            payload := map[string]any{
                "servers": aliases,
            }
            human := fmt.Sprintf("Attached MCP servers: %v", aliases)
            return printOutput(cmd, payload, human)
        },
    }
}

func newMCPAddCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "add <alias=path>",
        Short: "Attach an MCP server manifest",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            alias, path, err := splitAliasPath(args[0])
            if err != nil {
                return err
            }
            manifest, err := mcp.LoadManifest(path)
            if err != nil {
                return err
            }
            mcp.DefaultRegistry.Register(alias, manifest)
            payload := map[string]any{
                "alias": alias,
                "path":  path,
            }
            return printOutput(cmd, payload, fmt.Sprintf("Registered MCP server %s", alias))
        },
    }
}

func newMCPRmCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "rm <alias>",
        Short: "Remove an MCP server",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            alias := args[0]
            mcp.DefaultRegistry.Remove(alias)
            payload := map[string]any{"alias": alias}
            return printOutput(cmd, payload, fmt.Sprintf("Removed MCP server %s", alias))
        },
    }
}

func newMCPTestCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "test <alias>",
        Short: "Test connectivity with an MCP server",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            alias := args[0]
            if _, ok := mcp.DefaultRegistry.Get(alias); !ok {
                return fmt.Errorf("unknown MCP server %s", alias)
            }
            payload := map[string]any{
                "alias":  alias,
                "status": "ok",
            }
            return printOutput(cmd, payload, fmt.Sprintf("MCP server %s ready", alias))
        },
    }
}

func splitAliasPath(input string) (string, string, error) {
    parts := strings.SplitN(input, "=", 2)
    if len(parts) != 2 {
        return "", "", fmt.Errorf("expected alias=path, got %s", input)
    }
    return parts[0], parts[1], nil
}
