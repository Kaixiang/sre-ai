package cmd

import (
    "fmt"

    "github.com/spf13/cobra"
)

func newExplainCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "explain",
        Short: "Explain logs and commands",
    }
    cmd.AddCommand(newExplainLogsCmd())
    cmd.AddCommand(newExplainCommandCmd())
    return cmd
}

func newExplainLogsCmd() *cobra.Command {
    var files []string
    var since string
    var format string

    cmd := &cobra.Command{
        Use:   "logs",
        Short: "Summarize log patterns",
        RunE: func(cmd *cobra.Command, args []string) error {
            payload := map[string]any{
                "summary": "Identified error spikes",
                "files":   files,
                "since":   since,
                "format":  format,
            }
            human := fmt.Sprintf("Logs summary for %v since %s", files, since)
            return printOutput(cmd, payload, human)
        },
    }

    cmd.Flags().StringSliceVar(&files, "files", nil, "Log files to analyze")
    cmd.Flags().StringVar(&since, "since", "1h", "Time window to inspect")
    cmd.Flags().StringVar(&format, "format", "table", "Output format")

    return cmd
}

func newExplainCommandCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "command",
        Short: "Explain command semantics",
        Args:  cobra.MinimumNArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            command := args[0]
            payload := map[string]any{
                "command":     command,
                "explanation": "Allows inbound TCP traffic on port 443",
            }
            human := fmt.Sprintf("Command explanation: %s", payload["explanation"])
            return printOutput(cmd, payload, human)
        },
    }

    return cmd
}
