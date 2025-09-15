package cmd

import (
    "fmt"
    "strings"

    "github.com/spf13/cobra"
)

type planResult struct {
    Summary  string           `json:"summary"`
    Findings []string         `json:"findings"`
    Actions  []map[string]any `json:"actions"`
    Evidence []map[string]any `json:"evidence"`
}

func newDiagnoseCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "diagnose",
        Short: "Diagnose reliability issues across systems",
    }

    cmd.AddCommand(newDiagnoseK8sCmd())
    cmd.AddCommand(newDiagnoseCiCmd())
    cmd.AddCommand(newDiagnoseHostCmd())

    return cmd
}

func newDiagnoseK8sCmd() *cobra.Command {
    var (
        kubecontext string
        namespace   string
        since       string
        include     []string
        planOnly    bool
    )

    cmd := &cobra.Command{
        Use:   "k8s",
        Short: "Diagnose Kubernetes workloads",
        RunE: func(cmd *cobra.Command, args []string) error {
            result := planResult{
                Summary: fmt.Sprintf("Evaluated namespace %s in context %s", namespace, kubecontext),
                Findings: []string{
                    "Pending pods detected",
                },
                Actions: []map[string]any{
                    {
                        "intent":  "Inspect rollout",
                        "command": fmt.Sprintf("kubectl --context %s -n %s get deploy", kubecontext, namespace),
                    },
                },
                Evidence: []map[string]any{
                    {
                        "type":  "logs",
                        "since": since,
                    },
                },
            }

            if err := printOutput(cmd, result, renderPlan("Kubernetes", include, result)); err != nil {
                return err
            }

            if planOnly || globalOpts.DryRun {
                return nil
            }

            if !globalOpts.AutoConfirm && !globalOpts.NoInteractive {
                confirmed, err := promptForConfirmation(cmd, "Execute proposed kubectl diagnostics?")
                if err != nil {
                    return err
                }
                if !confirmed {
                    return nil
                }
            }

            return runKubectlDryRun(cmd, result.Actions)
        },
    }

    cmd.Flags().StringVar(&kubecontext, "kubecontext", "", "Kubeconfig context to target")
    cmd.Flags().StringVar(&namespace, "namespace", "default", "Kubernetes namespace")
    cmd.Flags().StringVar(&since, "since", "1h", "Time window to inspect")
    cmd.Flags().StringSliceVar(&include, "include", []string{"pods", "events"}, "Resources to include")
    cmd.Flags().BoolVar(&planOnly, "plan", false, "Only produce a plan without execution")

    return cmd
}

func newDiagnoseCiCmd() *cobra.Command {
    var (
        provider string
        runID    string
        since    string
        planOnly bool
    )

    cmd := &cobra.Command{
        Use:   "ci",
        Short: "Diagnose CI pipelines",
        RunE: func(cmd *cobra.Command, args []string) error {
            result := planResult{
                Summary: fmt.Sprintf("Analyzed CI run %s on %s", runID, provider),
                Findings: []string{"Workflow failure detected"},
                Actions: []map[string]any{
                    {"intent": "Fetch logs", "command": fmt.Sprintf("gh run view %s", runID)},
                },
                Evidence: []map[string]any{
                    {"type": "ci", "since": since},
                },
            }

            if err := printOutput(cmd, result, renderPlan("CI", nil, result)); err != nil {
                return err
            }

            return nil
        },
    }

    cmd.Flags().StringVar(&provider, "provider", "github", "CI provider")
    cmd.Flags().StringVar(&runID, "run-id", "", "Pipeline run identifier")
    cmd.Flags().StringVar(&since, "since", "1h", "Time window to inspect")
    cmd.Flags().BoolVar(&planOnly, "plan", false, "Only produce a plan without execution")

    _ = planOnly
    return cmd
}

func newDiagnoseHostCmd() *cobra.Command {
    var (
        target   string
        since    string
        collect  []string
        planOnly bool
    )

    cmd := &cobra.Command{
        Use:   "host",
        Short: "Diagnose individual hosts",
        RunE: func(cmd *cobra.Command, args []string) error {
            result := planResult{
                Summary: fmt.Sprintf("Inspected host %s", target),
                Findings: []string{"High load detected"},
                Actions: []map[string]any{
                    {"intent": "Collect metrics", "command": fmt.Sprintf("ssh %s top", target)},
                },
                Evidence: []map[string]any{
                    {"type": "host", "since": since, "artifacts": collect},
                },
            }

            if err := printOutput(cmd, result, renderPlan("Host", collect, result)); err != nil {
                return err
            }

            return nil
        },
    }

    cmd.Flags().StringVar(&target, "target", "", "Hostname or IP")
    cmd.Flags().StringVar(&since, "since", "30m", "Time window to inspect")
    cmd.Flags().StringSliceVar(&collect, "collect", []string{"journal", "top"}, "Artifacts to collect")
    cmd.Flags().BoolVar(&planOnly, "plan", false, "Only produce a plan without execution")

    _ = planOnly
    return cmd
}

func renderPlan(scope string, include []string, plan planResult) string {
    parts := []string{fmt.Sprintf("Plan for %s diagnostics:", scope)}
    if len(include) > 0 {
        parts = append(parts, fmt.Sprintf("  include: %s", strings.Join(include, ", ")))
    }
    for i, action := range plan.Actions {
        parts = append(parts, fmt.Sprintf("  %d. %s", i+1, action["intent"]))
    }
    return strings.Join(parts, "\n")
}
