package cli

import (
	"github.com/phenix3443/mihctl/internal/configgen"
	"github.com/phenix3443/mihctl/internal/mihomo"
	"github.com/spf13/cobra"
)

var runConfigSync = func(env *mihomo.Env, options configgen.GenerateOptions, profile string) error {
	return env.SyncConfig(options, profile, false)
}

func newConfigCmd() *cobra.Command {
	var linuxTUN bool
	var macosTUN bool
	var noLinuxTUN bool
	var noMacOSTUN bool
	var tun bool
	var noTUN bool
	syncProfile := ""

	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage generated Mihomo config",
	}

	genCmd := &cobra.Command{
		Use:     "gen",
		Aliases: []string{"generate"},
		Short:   "Generate repository config artifacts",
		RunE: func(cmd *cobra.Command, args []string) error {
			options := resolveTUNOptions(linuxTUN, macosTUN, noLinuxTUN, noMacOSTUN, tun, noTUN)
			return runNativeConfigGen(options)
		},
	}
	genCmd.Flags().BoolVar(&linuxTUN, "linux-tun", false, "enable TUN in generated Linux config")
	genCmd.Flags().BoolVar(&macosTUN, "macos-tun", true, "enable TUN in generated macOS config")
	genCmd.Flags().BoolVar(&noLinuxTUN, "no-linux-tun", false, "disable TUN in generated Linux config")
	genCmd.Flags().BoolVar(&noMacOSTUN, "no-macos-tun", false, "disable TUN in generated macOS config")
	genCmd.Flags().BoolVar(&tun, "tun", false, "enable TUN in generated Linux and macOS config")
	genCmd.Flags().BoolVar(&noTUN, "no-tun", false, "disable TUN in generated Linux and macOS config")

	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Generate config and deploy it to the live Mihomo target",
		RunE: func(cmd *cobra.Command, args []string) error {
			options := resolveTUNOptions(linuxTUN, macosTUN, noLinuxTUN, noMacOSTUN, tun, noTUN)
			env, err := loadEnv()
			if err != nil {
				return err
			}
			return runConfigSync(env, configgen.GenerateOptions{
				EnableLinuxTUN: options.EnableLinuxTUN,
				EnableMacOSTUN: options.EnableMacOSTUN,
			}, syncProfile)
		},
	}
	syncCmd.Flags().BoolVar(&linuxTUN, "linux-tun", false, "enable TUN in generated Linux config")
	syncCmd.Flags().BoolVar(&macosTUN, "macos-tun", true, "enable TUN in generated macOS config")
	syncCmd.Flags().BoolVar(&noLinuxTUN, "no-linux-tun", false, "disable TUN in generated Linux config")
	syncCmd.Flags().BoolVar(&noMacOSTUN, "no-macos-tun", false, "disable TUN in generated macOS config")
	syncCmd.Flags().BoolVar(&tun, "tun", false, "enable TUN in generated Linux and macOS config")
	syncCmd.Flags().BoolVar(&noTUN, "no-tun", false, "disable TUN in generated Linux and macOS config")
	syncCmd.Flags().StringVar(&syncProfile, "profile", syncProfile, "generated profile to sync; defaults to runtime target auto-selection")

	cmd.AddCommand(genCmd, syncCmd)
	return cmd
}

func resolveTUNOptions(linuxTUN, macosTUN, noLinuxTUN, noMacOSTUN, tun, noTUN bool) nativeConfigGenOptions {
	options := nativeConfigGenOptions{
		EnableLinuxTUN: linuxTUN,
		EnableMacOSTUN: macosTUN,
	}
	if tun {
		options.EnableLinuxTUN = true
		options.EnableMacOSTUN = true
	}
	if noLinuxTUN {
		options.EnableLinuxTUN = false
	}
	if noMacOSTUN {
		options.EnableMacOSTUN = false
	}
	if noTUN {
		options.EnableLinuxTUN = false
		options.EnableMacOSTUN = false
	}
	return options
}
