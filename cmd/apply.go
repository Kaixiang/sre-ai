package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

func newApplyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply infrastructure or operational changes",
	}

	cmd.AddCommand(newApplyIacCmd())
	return cmd
}

func newApplyIacCmd() *cobra.Command {
	var stack string

	cmd := &cobra.Command{
		Use:   "iac",
		Short: "Apply an IaC plan",
		RunE: func(cmd *cobra.Command, args []string) error {
			if globalOpts.DryRun {
				return printOutput(cmd, map[string]string{"status": "dry-run"}, "Dry-run only; not applying changes")
			}

			if !globalOpts.AutoConfirm {
				if globalOpts.NoInteractive {
					return errors.New("refusing to apply without --confirm in no-interactive mode")
				}

				confirmed, err := promptForConfirmation(cmd, fmt.Sprintf("Apply IaC stack %s?", stack))
				if err != nil {
					return err
				}
				if !confirmed {
					return printOutput(cmd, map[string]string{"status": "cancelled"}, "Apply cancelled")
				}
			}

			payload := map[string]any{
				"stack":  stack,
				"status": "applied",
			}
			human := fmt.Sprintf("Applied IaC stack %s", stack)
			return printOutput(cmd, payload, human)
		},
	}

	cmd.Flags().StringVar(&stack, "stack", "", "Named IaC stack to apply")
	_ = cmd.MarkFlagRequired("stack")

	return cmd
}
