package cli

import "github.com/spf13/cobra"

func newRulesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rules",
		Short: "Manage rule providers",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "update",
		Short: "Force the running Mihomo instance to refresh all active rule providers",
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := loadEnv()
			if err != nil {
				return err
			}
			return env.UpdateRules()
		},
	})

	return cmd
}
