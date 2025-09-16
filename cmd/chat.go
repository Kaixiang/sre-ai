package cmd

import (
    "bufio"
    "errors"
    "fmt"
    "io"
    "strings"

    "github.com/example/sre-ai/internal/credentials"
    "github.com/example/sre-ai/internal/providers"
    "github.com/spf13/cobra"
)

func newChatCmd() *cobra.Command {
    var session string
    var prompt string

    cmd := &cobra.Command{
        Use:   "chat",
        Short: "Send a single prompt to the configured chat model",
        RunE: func(cmd *cobra.Command, args []string) error {
            text := prompt
            if text == "" && len(args) > 0 {
                text = strings.Join(args, " ")
            }

            if text == "" {
                if globalOpts.NoInteractive {
                    return errors.New("prompt required; pass text as arguments or via --prompt")
                }
                fmt.Fprint(cmd.OutOrStdout(), "Prompt: ")
                reader := bufio.NewReader(cmd.InOrStdin())
                input, err := reader.ReadString('\n')
                if err != nil && !errors.Is(err, io.EOF) {
                    return err
                }
                text = strings.TrimSpace(input)
            }

            if text == "" {
                return errors.New("no prompt provided")
            }

            model := globalOpts.Model
            if model == "" {
                model = providers.DefaultGeminiModel()
            }

            if globalOpts.DryRun {
                payload := map[string]any{
                    "session": session,
                    "model":   model,
                    "prompt":  text,
                    "status":  "dry-run",
                }
                return printOutput(cmd, payload, "Dry-run: would query Gemini chat")
            }

            apiKey, err := credentials.LoadGeminiKey()
            if err != nil {
                return err
            }

            client := providers.NewGeminiClient(apiKey, model)
            reply, err := client.Generate(cmd.Context(), text)
            if err != nil {
                return err
            }

            payload := map[string]any{
                "session": session,
                "model":   model,
                "prompt":  text,
                "reply":   reply,
            }
            human := fmt.Sprintf("[%s] %s", session, reply)
            return printOutput(cmd, payload, human)
        },
    }

    cmd.Flags().StringVar(&session, "session", "default", "Session id to reuse")
    cmd.Flags().StringVarP(&prompt, "prompt", "p", "", "Prompt text to send")

    return cmd
}
