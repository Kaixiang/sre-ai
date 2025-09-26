package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/example/sre-ai/internal/mcp"
	"github.com/spf13/cobra"
)

func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Manage MCP server integrations",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return mcp.Warmup(cmd.Context(), &globalOpts)
		},
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
				for idx, info := range infos {
					if idx > 0 {
						builder.WriteString("\n")
					}
					builder.WriteString(fmt.Sprintf("%s [%s]", info.Alias, info.Source))
					if info.Origin != "" {
						builder.WriteString(fmt.Sprintf(" @ %s", info.Origin))
					}
					builder.WriteString("\n")
					if info.ManifestName != "" {
						builder.WriteString("  manifest: ")
						builder.WriteString(info.ManifestName)
						if info.ManifestVersion != "" {
							builder.WriteString(" @ ")
							builder.WriteString(info.ManifestVersion)
						}
						if info.ManifestTransportType != "" {
							builder.WriteString(fmt.Sprintf(" via %s", info.ManifestTransportType))
						}
						builder.WriteString("\n")
						if len(info.ManifestCapabilities) > 0 {
							builder.WriteString("  capabilities: ")
							builder.WriteString(strings.Join(info.ManifestCapabilities, ", "))
							builder.WriteString("\n")
						}
					}
					if info.Command != "" {
						builder.WriteString("  command: ")
						builder.WriteString(info.Command)
						if len(info.Args) > 0 {
							builder.WriteString(" ")
							builder.WriteString(strings.Join(info.Args, " "))
						}
						builder.WriteString("\n")
					}
					if info.Workdir != "" {
						builder.WriteString("  workdir: ")
						builder.WriteString(info.Workdir)
						builder.WriteString("\n")
					}
					if len(info.Env) > 0 {
						keys := make([]string, 0, len(info.Env))
						for k := range info.Env {
							keys = append(keys, k)
						}
						sort.Strings(keys)
						pairs := make([]string, 0, len(keys))
						for _, k := range keys {
							pairs = append(pairs, fmt.Sprintf("%s=%s", k, info.Env[k]))
						}
						builder.WriteString("  env: ")
						builder.WriteString(strings.Join(pairs, ", "))
						builder.WriteString("\n")
					}
					if info.Notes != "" {
						builder.WriteString("  notes: ")
						builder.WriteString(info.Notes)
						builder.WriteString("\n")
					}
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
