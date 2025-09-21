package cmd

import (
    "fmt"
    "os"

    "github.com/example/sre-ai/internal/config"
    "github.com/example/sre-ai/internal/providers"
    // "github.com/example/sre-ai/internal/mcp"
    "github.com/spf13/cobra"
)

var (
    cfgFile string
    globalOpts = config.GlobalOptions{
        Temperature: 0.2,
        Provider:    "gemini",
        Model:       providers.DefaultGeminiModel(),
    }
)

// rootCmd is the base command for the CLI.
var rootCmd = &cobra.Command{
    Use:   "sre-ai",
    Short: "AI-powered SRE/DevOps assistant with MCP integration",
    PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
        if cfgFile != "" {
            globalOpts.ConfigPath = cfgFile
        }

        if err := config.Load(&globalOpts); err != nil {
            return fmt.Errorf("load config: %w", err)
        }

        // if err := mcp.Warmup(cmd.Context(), &globalOpts); err != nil {
        // 	return fmt.Errorf("warmup MCP: %w", err)
        // }

        return nil
    },
}

// Execute runs the root command.
func Execute() {
    if err := rootCmd.Execute(); err != nil {
        fmt.Fprintf(os.Stderr, "error: %v\n", err)
        os.Exit(1)
    }
}

func init() {
    flags := rootCmd.PersistentFlags()
    flags.StringVar(&globalOpts.Model, "model", globalOpts.Model, "Model identifier (e.g. gemini-1.5-flash-latest)")
    flags.StringVar(&globalOpts.Provider, "provider", globalOpts.Provider, "Model provider (gemini|openai|azure|bedrock|ollama|vllm|http)")
    flags.Float64Var(&globalOpts.Temperature, "temperature", globalOpts.Temperature, "Sampling temperature")
    flags.IntVar(&globalOpts.MaxTokens, "max-tokens", globalOpts.MaxTokens, "Maximum tokens to request")
    flags.StringVar(&globalOpts.Session, "session", globalOpts.Session, "Session name for sticky context")
    flags.BoolVar(&globalOpts.JSON, "json", globalOpts.JSON, "Emit machine-readable JSON output")
    flags.BoolVar(&globalOpts.Text, "text", globalOpts.Text, "Emit raw text output when supported")
    flags.BoolVarP(&globalOpts.Quiet, "quiet", "q", globalOpts.Quiet, "Silence human-readable output")
    flags.CountVarP(&globalOpts.Verbose, "verbose", "v", "Increase verbosity for debugging")
    flags.BoolVar(&globalOpts.NoInteractive, "no-interactive", globalOpts.NoInteractive, "Do not prompt interactively")
    flags.StringVar(&cfgFile, "config", cfgFile, "Override config file path")
    flags.StringToStringVar(&globalOpts.MCPServers, "mcp-server", globalOpts.MCPServers, "Attach MCP server alias=path")
    flags.StringSliceVar(&globalOpts.Caps, "cap", globalOpts.Caps, "Grant capability (repeatable)")
    flags.BoolVar(&globalOpts.DryRun, "dry-run", globalOpts.DryRun, "Never apply mutations")
    flags.BoolVar(&globalOpts.AutoConfirm, "confirm", globalOpts.AutoConfirm, "Auto-confirm prompts")

    rootCmd.AddCommand(newDiagnoseCmd())
    rootCmd.AddCommand(newExplainCmd())
    rootCmd.AddCommand(newGenerateCmd())
    rootCmd.AddCommand(newPlanCmd())
    rootCmd.AddCommand(newApplyCmd())
    rootCmd.AddCommand(newAgentCmd())
    rootCmd.AddCommand(newChatCmd())
    rootCmd.AddCommand(newMCPCmd())
    rootCmd.AddCommand(newConfigCmd())
}
