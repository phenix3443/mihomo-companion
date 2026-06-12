package cli

import (
	"fmt"

	"github.com/phenix3443/mihomo-companion/internal/configgen"
)

type nativeConfigGenOptions struct {
	EnableLinuxTUN bool
	EnableMacOSTUN bool
}

func runNativeConfigGen(options nativeConfigGenOptions) error {
	repoRoot, err := configgen.ResolveRepoRoot()
	if err != nil {
		return err
	}

	service := configgen.NewService(repoRoot)
	result, err := service.Generate(configgen.GenerateOptions{
		EnableLinuxTUN: options.EnableLinuxTUN,
		EnableMacOSTUN: options.EnableMacOSTUN,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Generated config files:\n")
	for _, artifact := range result.Artifacts {
		fmt.Printf("- %s: %s\n", artifact.Label, artifact.Path)
	}
	return nil
}
