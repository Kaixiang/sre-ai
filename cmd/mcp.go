package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
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
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()

			logger := newMCPLogger(cmd)
			if logger != nil {
				logger.Printf("probe start alias=%s", alias)
			}
			result, err := mcp.ProbeLocalServerWithLogger(ctx, alias, logger)
			if err != nil {
				return err
			}
			if logger != nil {
				logger.Printf("probe success alias=%s", alias)
			}

			payload := map[string]any{
				"alias":            alias,
				"server_name":      result.ServerName,
				"server_version":   result.ServerVersion,
				"protocol_version": result.ProtocolVersion,
				"capabilities":     result.Capabilities,
				"tools":            result.Tools,
				"notifications":    result.Notifications,
				"duration_ms":      result.Duration.Milliseconds(),
			}
			if result.Instructions != "" {
				payload["instructions"] = result.Instructions
			}
			if result.Stderr != "" {
				payload["stderr"] = result.Stderr
			}

			human := formatProbeHuman(alias, result)
			return printOutput(cmd, payload, human)
		},
	}
}

func formatProbeHuman(alias string, result *mcp.ProbeResult) string {
	var builder strings.Builder

	header := alias
	if result.ServerName != "" {
		header = fmt.Sprintf("%s - %s", header, result.ServerName)
	}
	if result.ServerVersion != "" {
		header = fmt.Sprintf("%s %s", header, result.ServerVersion)
	}
	builder.WriteString(strings.TrimSpace(header))
	builder.WriteString("\n")

	protocol := result.ProtocolVersion
	if protocol == "" {
		protocol = "unknown"
	}
	toolsCount := len(result.Tools)
	builder.WriteString(fmt.Sprintf("Protocol %s - %d tool", protocol, toolsCount))
	if toolsCount != 1 {
		builder.WriteString("s")
	}
	if result.Duration > 0 {
		builder.WriteString(fmt.Sprintf(" - %s", result.Duration.Round(10*time.Millisecond)))
	}
	builder.WriteString("\n")

	caps := describeCapabilities(result.Capabilities)
	if len(caps) == 0 {
		builder.WriteString("Capabilities: none reported\n")
	} else {
		builder.WriteString("Capabilities:\n")
		for _, cap := range caps {
			builder.WriteString("  - ")
			builder.WriteString(cap)
			builder.WriteString("\n")
		}
	}

	if toolsCount == 0 {
		builder.WriteString("Tools: none reported\n")
	} else {
		builder.WriteString("Tools:\n")
		for _, tool := range result.Tools {
			display := tool.Title
			if display == "" {
				display = tool.Name
			}
			if display == "" {
				display = "(unnamed tool)"
			}
			desc := tool.Description
			if desc == "" {
				desc = "(no description)"
			}
			builder.WriteString(fmt.Sprintf("  - %s: %s\n", display, desc))
			if len(tool.InputSchema) > 0 {
				builder.WriteString("    schema: ")
				builder.WriteString(compactPreview(tool.InputSchema))
				builder.WriteString("\n")
			}
		}
	}

	if instr := strings.TrimSpace(result.Instructions); instr != "" {
		builder.WriteString("Instructions:\n")
		for _, line := range strings.Split(instr, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			builder.WriteString("  ")
			builder.WriteString(trimmed)
			builder.WriteString("\n")
		}
	}

	if len(result.Notifications) > 0 {
		builder.WriteString("Notifications:\n")
		for _, note := range result.Notifications {
			builder.WriteString("  - ")
			builder.WriteString(note.Method)
			if note.Detail != "" {
				builder.WriteString(": ")
				builder.WriteString(note.Detail)
			}
			builder.WriteString("\n")
		}
	}

	if result.Stderr != "" {
		builder.WriteString("stderr:\n")
		for _, line := range strings.Split(result.Stderr, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			builder.WriteString("  ")
			builder.WriteString(trimmed)
			builder.WriteString("\n")
		}
	}

	return strings.TrimRight(builder.String(), "\n")
}

func describeCapabilities(caps map[string]interface{}) []string {
	if len(caps) == 0 {
		return nil
	}
	keys := make([]string, 0, len(caps))
	for key := range caps {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		value := caps[key]
		switch typed := value.(type) {
		case map[string]interface{}:
			innerKeys := make([]string, 0, len(typed))
			for innerKey, innerValue := range typed {
				switch val := innerValue.(type) {
				case bool:
					innerKeys = append(innerKeys, fmt.Sprintf("%s=%t", innerKey, val))
				case string:
					if val == "" {
						innerKeys = append(innerKeys, innerKey)
					} else {
						innerKeys = append(innerKeys, fmt.Sprintf("%s=%s", innerKey, val))
					}
				default:
					innerKeys = append(innerKeys, fmt.Sprintf("%s=%s", innerKey, compactPreview(innerValue)))
				}
			}
			sort.Strings(innerKeys)
			if len(innerKeys) > 0 {
				lines = append(lines, fmt.Sprintf("%s (%s)", key, strings.Join(innerKeys, ", ")))
			} else {
				lines = append(lines, key)
			}
		case bool:
			if typed {
				lines = append(lines, key)
			} else {
				lines = append(lines, fmt.Sprintf("%s=false", key))
			}
		default:
			lines = append(lines, fmt.Sprintf("%s=%s", key, compactPreview(value)))
		}
	}
	return lines
}

func compactPreview(value interface{}) string {
	if value == nil {
		return "null"
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "<unserializable>"
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, data); err == nil {
		data = buf.Bytes()
	}
	if len(data) > 160 {
		data = append(data[:157], '.', '.', '.')
	}
	return string(data)
}

func newMCPLogger(cmd *cobra.Command) mcp.Logger {
	if globalOpts.Verbose == 0 {
		return nil
	}

	writer := cmd.ErrOrStderr()
	if writer == nil {
		writer = os.Stderr
	}

	return log.New(writer, "[mcp] ", 0)
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
