package cli

import (
	"github.com/phenix3443/mihomo-companion/internal/mihomo"
	"github.com/spf13/cobra"
)

var runProvidersUpdate = func(env *mihomo.Env) error {
	return env.UpdateProvidersRemote()
}

func newProvidersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "providers",
		Short: "Manage proxy providers",
	}

	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Copy repository providers into the detected live config directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := loadEnv()
			if err != nil {
				return err
			}
			return env.SyncProvidersToLive()
		},
	}
	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "Fetch provider URLs into repository providers/",
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := loadEnv()
			if err != nil {
				return err
			}
			return runProvidersUpdate(env)
		},
	}

	cmd.AddCommand(
		syncCmd,
		updateCmd,
	)

	return cmd
}
