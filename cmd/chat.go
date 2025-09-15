package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newChatCmd() *cobra.Command {
	var session string

	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Open an interactive chat session",
		RunE: func(cmd *cobra.Command, args []string) error {
			payload := map[string]any{
				"session": session,
				"status":  "ready",
			}
			human := fmt.Sprintf("Chat session %s ready", session)
			return printOutput(cmd, payload, human)
		},
	}

	cmd.Flags().StringVar(&session, "session", "default", "Session id to reuse")
	return cmd
}
