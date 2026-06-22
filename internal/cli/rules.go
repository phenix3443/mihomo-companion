package cli

import "github.com/spf13/cobra"

var runRulesUpdate = func(env rulesUpdateEnv) error {
	return env.UpdateRules()
}

type rulesUpdateEnv interface {
	UpdateRules() error
}

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
			return runRulesUpdate(env)
		},
	})

	return cmd
}
