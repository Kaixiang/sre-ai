package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func newPlanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Plan infrastructure or operational changes",
	}

	cmd.AddCommand(newPlanIacCmd())
	return cmd
}

func newPlanIacCmd() *cobra.Command {
	var stack string

	cmd := &cobra.Command{
		Use:   "iac",
		Short: "Plan IaC changes",
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{
				"stack":    stack,
				"generated": time.Now().UTC().Format(time.RFC3339),
				"diff": []map[string]string{
					{"action": "update", "target": "aws_s3_bucket.payments"},
				},
			}
			human := fmt.Sprintf("IaC plan ready for stack %s", stack)
			return printOutput(cmd, payload, human)
		},
	}

	cmd.Flags().StringVar(&stack, "stack", "", "Named IaC stack to plan")
	_ = cmd.MarkFlagRequired("stack")

	return cmd
}
