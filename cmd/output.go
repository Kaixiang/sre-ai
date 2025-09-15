package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func printOutput(cmd *cobra.Command, payload any, human string) error {
	if globalOpts.JSON {
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}

	if !globalOpts.Quiet && human != "" {
		fmt.Fprintln(cmd.OutOrStdout(), human)
	}
	return nil
}
