package cmd

import (
    "bufio"
    "errors"
    "fmt"
    "io"
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
    "strings"

    "github.com/example/sre-ai/internal/config"
    "github.com/example/sre-ai/internal/credentials"
    "github.com/spf13/cobra"
)

const geminiAPIKeyURL = "https://aistudio.google.com/app/apikey"

func newConfigCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "config",
        Short: "Inspect or bootstrap CLI configuration",
    }

    cmd.AddCommand(newConfigInitCmd())
    cmd.AddCommand(newConfigShowCmd())
    cmd.AddCommand(newConfigLoginCmd())
    return cmd
}

func newConfigInitCmd() *cobra.Command {
    return &cobra.Command{
        Use:   "init",
        Short: "Create a starter configuration file",
        RunE: func(cmd *cobra.Command, args []string) error {
            if globalOpts.DryRun {
                path, err := resolveConfigPath()
                if err != nil {
                    return err
                }
                payload := map[string]any{
                    "path":   path,
                    "status": "dry-run",
                }
                return printOutput(cmd, payload, fmt.Sprintf("Dry-run: would create config at %s", path))
            }

            cfgPath, err := resolveConfigPath()
            if err != nil {
                return err
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
            return printOutput(cmd, payload, fmt.Sprintf("Wrote config to %s\nRun 'sre-ai config login --provider gemini' to add credentials", cfgPath))
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

func newConfigLoginCmd() *cobra.Command {
    var provider string
    var noBrowser bool

    cmd := &cobra.Command{
        Use:   "login",
        Short: "Authenticate with an AI provider",
        RunE: func(cmd *cobra.Command, args []string) error {
            if globalOpts.NoInteractive {
                return errors.New("login requires interactive mode; rerun without --no-interactive")
            }

            switch strings.ToLower(provider) {
            case "gemini":
                return runGeminiLogin(cmd, !noBrowser)
            default:
                return fmt.Errorf("unsupported provider %s", provider)
            }
        },
    }

    cmd.Flags().StringVar(&provider, "provider", "gemini", "AI provider to authenticate (gemini)")
    cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "Do not attempt to launch a browser automatically")

    return cmd
}

func runGeminiLogin(cmd *cobra.Command, launchBrowser bool) error {
    targetPath, err := credentials.GeminiKeyPath()
    if err != nil {
        return err
    }

    fmt.Fprintf(cmd.OutOrStdout(), "Open Gemini API key page to create or view a key:\n  %s\n", geminiAPIKeyURL)
    if launchBrowser && !globalOpts.DryRun {
        if err := openBrowser(geminiAPIKeyURL); err != nil {
            if globalOpts.Verbose > 0 && !globalOpts.Quiet {
                fmt.Fprintf(cmd.ErrOrStderr(), "warning: unable to launch browser: %v\n", err)
            }
        }
    }

    if globalOpts.DryRun {
        payload := map[string]any{
            "provider":        "gemini",
            "credential_file": targetPath,
            "status":          "dry-run",
        }
        return printOutput(cmd, payload, fmt.Sprintf("Dry-run: would store Gemini API key at %s", targetPath))
    }

    key, err := promptForAPIKey(cmd, "Paste your Gemini API key: ")
    if err != nil {
        return err
    }
    if key == "" {
        return errors.New("no API key provided")
    }

    savedPath, err := credentials.SaveGeminiKey(key)
    if err != nil {
        return err
    }

    payload := map[string]any{
        "provider":        "gemini",
        "credential_file": savedPath,
    }
    return printOutput(cmd, payload, fmt.Sprintf("Gemini API key stored at %s", savedPath))
}

func promptForAPIKey(cmd *cobra.Command, prompt string) (string, error) {
    fmt.Fprint(cmd.OutOrStdout(), prompt)
    reader := bufio.NewReader(cmd.InOrStdin())
    value, err := reader.ReadString('\n')
    if err != nil && !errors.Is(err, io.EOF) {
        return "", err
    }
    return strings.TrimSpace(value), nil
}

func openBrowser(url string) error {
    var command *exec.Cmd
    switch runtime.GOOS {
    case "windows":
        command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
    case "darwin":
        command = exec.Command("open", url)
    default:
        command = exec.Command("xdg-open", url)
    }
    return command.Start()
}

func resolveConfigPath() (string, error) {
    if globalOpts.ConfigPath != "" {
        return globalOpts.ConfigPath, nil
    }
    return config.DefaultConfigPath()
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
auth:
  gemini:
    credential_file: ~/.config/sre-ai/credentials/gemini.json
logging:
  level: info
  redact: true
`
}
