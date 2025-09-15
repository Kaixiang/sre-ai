package cmd

import (
    "fmt"
    "time"

    "github.com/spf13/cobra"
)

type agentPlan struct {
    Session string          `json:"session"`
    Goal    string          `json:"goal"`
    Tools   []string        `json:"tools"`
    Caps    []string        `json:"caps"`
    Steps   []agentPlanStep `json:"steps"`
}

type agentPlanStep struct {
    ID      string `json:"id"`
    Intent  string `json:"intent"`
    Command string `json:"command"`
}

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
    var goal string
    var tools []string
    var caps []string
    var session string
    var planOnly bool

    cmd := &cobra.Command{
        Use:   "run",
        Short: "Plan and execute an agent goal",
        RunE: func(cmd *cobra.Command, args []string) error {
            sessionID := session
            if sessionID == "" {
                sessionID = fmt.Sprintf("agent-%d", time.Now().Unix())
            }

            plan := agentPlan{
                Session: sessionID,
                Goal:    goal,
                Tools:   tools,
                Caps:    caps,
                Steps: []agentPlanStep{
                    {ID: "step-1", Intent: "Inspect rollout", Command: "kubectl -n checkout get deploy checkout-web"},
                    {ID: "step-2", Intent: "Check pods", Command: "kubectl -n checkout get pods"},
                },
            }

            human := fmt.Sprintf("Agent plan for %s with %d steps", goal, len(plan.Steps))
            if err := printOutput(cmd, plan, human); err != nil {
                return err
            }

            if planOnly || globalOpts.DryRun {
                return nil
            }

            if !globalOpts.AutoConfirm {
                if globalOpts.NoInteractive {
                    return fmt.Errorf("cannot execute agent without --confirm in no-interactive mode")
                }
                confirmed, err := promptForConfirmation(cmd, "Execute agent steps in dry-run mode?")
                if err != nil {
                    return err
                }
                if !confirmed {
                    return nil
                }
            }

            actions := make([]map[string]any, 0, len(plan.Steps))
            for _, step := range plan.Steps {
                actions = append(actions, map[string]any{
                    "intent":  step.Intent,
                    "command": step.Command,
                })
            }

            return runKubectlDryRun(cmd, actions)
        },
    }

    cmd.Flags().StringVar(&goal, "goal", "", "Goal statement for the agent")
    cmd.Flags().StringSliceVar(&tools, "tools", nil, "Explicit tool allowlist")
    cmd.Flags().StringSliceVar(&caps, "cap", nil, "Capabilities for the run")
    cmd.Flags().StringVar(&session, "session", "", "Session id to reuse")
    cmd.Flags().BoolVar(&planOnly, "plan", false, "Only output plan stage")

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
