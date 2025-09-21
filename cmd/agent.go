package cmd

import (
    "errors"
    "fmt"

    "github.com/example/sre-ai/internal/agent"
    "github.com/spf13/cobra"
)

func newAgentCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "agent",
        Short: "Run autonomous but auditable agent flows",
    }

    cmd.AddCommand(newAgentRunCmd())
    cmd.AddCommand(newAgentOncallCmd())
    return cmd
}

func newAgentRunCmd() *cobra.Command {
    var workflowPath string
    var inputPairs []string
    var planOnly bool

    cmd := &cobra.Command{
        Use:   "run",
        Short: "Execute an agent workflow",
        RunE: func(cmd *cobra.Command, args []string) error {
            if workflowPath == "" {
                return errors.New("--workflow is required")
            }

            provided, err := agent.ParseInputPairs(inputPairs)
            if err != nil {
                return err
            }

            runner, err := agent.NewRunner(workflowPath, &globalOpts, provided)
            if err != nil {
                return err
            }

            result, err := runner.Execute(cmd.Context(), planOnly)
            if err != nil {
                return err
            }

            status := "completed"
            if result.PlanOnly {
                status = "planned"
            }
            human := fmt.Sprintf("Workflow %s %s (%d steps)", result.Workflow, status, len(result.Steps))
            return printOutput(cmd, result, human)
        },
    }

    cmd.Flags().StringVar(&workflowPath, "workflow", "", "Path to workflow YAML definition")
    cmd.Flags().StringSliceVar(&inputPairs, "input", nil, "Workflow input as key=value (repeatable)")
    cmd.Flags().BoolVar(&planOnly, "plan", false, "Only validate the workflow without executing steps")

    return cmd
}

func newAgentOncallCmd() *cobra.Command {
    var start bool
    var stop bool
    var output string

    cmd := &cobra.Command{
        Use:   "oncall",
        Short: "Manage oncall session timelines",
        RunE: func(cmd *cobra.Command, args []string) error {
            status := "standing-by"
            switch {
            case start:
                status = "started"
            case stop:
                status = "stopped"
            }
            payload := map[string]any{
                "status": status,
                "output": output,
            }
            human := fmt.Sprintf("Oncall session %s", status)
            return printOutput(cmd, payload, human)
        },
    }

    cmd.Flags().BoolVar(&start, "start", false, "Start tracking an oncall session")
    cmd.Flags().BoolVar(&stop, "stop", false, "Stop tracking and finalize summary")
    cmd.Flags().StringVar(&output, "output", "", "Optional output file for postmortem draft")

    return cmd
}
