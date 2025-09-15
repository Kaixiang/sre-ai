package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func promptForConfirmation(cmd *cobra.Command, question string) (bool, error) {
	fmt.Fprintf(cmd.OutOrStdout(), "%s [y/N]: ", question)
	reader := bufio.NewReader(cmd.InOrStdin())
	resp, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	resp = strings.TrimSpace(strings.ToLower(resp))
	return resp == "y" || resp == "yes", nil
}

func runKubectlDryRun(cmd *cobra.Command, actions []map[string]any) error {
	for _, action := range actions {
		commandStr, _ := action["command"].(string)
		if commandStr == "" {
			continue
		}
		if !strings.Contains(commandStr, "kubectl") {
			continue
		}
		dry := ensureDryRun(commandStr)
		if globalOpts.JSON {
			fmt.Fprintf(cmd.OutOrStdout(), "{\"action\":\"dry-run\",\"command\":\"%s\"}\n", escapeJSON(dry))
		} else if !globalOpts.Quiet {
			fmt.Fprintf(cmd.OutOrStdout(), "dry-run kubectl: %s\n", dry)
		}
	}
	return nil
}

func ensureDryRun(command string) string {
	if strings.Contains(command, "--dry-run") {
		return command
	}
	if strings.Contains(command, "kubectl apply") {
		return command + " --dry-run=client"
	}
	return command + " --dry-run=client"
}

func escapeJSON(input string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "\"", "\\\"")
	return replacer.Replace(input)
}

func ensurePlanFile(path string) (*os.File, error) {
	return os.Create(path)
}
