package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate artifacts like runbooks or IaC",
	}
	cmd.AddCommand(newGenerateRunbookCmd())
	cmd.AddCommand(newGenerateIacCmd())
	return cmd
}

func newGenerateRunbookCmd() *cobra.Command {
	var service string
	var from string
	var output string

	cmd := &cobra.Command{
		Use:   "runbook",
		Short: "Generate a runbook draft",
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{
				"service": service,
				"source":  from,
				"output":  output,
			}
			human := fmt.Sprintf("Generated runbook draft for %s", service)
			return printOutput(cmd, payload, human)
		},
	}

	cmd.Flags().StringVar(&service, "service", "", "Service name")
	cmd.Flags().StringVar(&from, "from", "", "Source incident or runbook")
	cmd.Flags().StringVar(&output, "output", "runbooks/out.md", "Path to write the draft")

	return cmd
}

func newGenerateIacCmd() *cobra.Command {
	var provider string
	var resource string
	var tags []string
	var out string

	cmd := &cobra.Command{
		Use:   "iac",
		Short: "Generate IaC snippets",
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{
				"provider": provider,
				"resource": resource,
				"tags":     tags,
				"output":   out,
			}
			human := fmt.Sprintf("Generated IaC snippet for %s", resource)
			return printOutput(cmd, payload, human)
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "aws", "IaC provider")
	cmd.Flags().StringVar(&resource, "resource", "", "Resource type")
	cmd.Flags().StringSliceVar(&tags, "tags", nil, "Tags to annotate")
	cmd.Flags().StringVar(&out, "out", "iac/out.tf", "Output file path")

	return cmd
}
