package cli

import (
	"github.com/phenix3443/mihomo-companion/internal/configgen"
	"github.com/phenix3443/mihomo-companion/internal/mihomo"
)

var loadEnv = func() (*mihomo.Env, error) {
	repoRoot, err := configgen.ResolveRepoRoot()
	if err != nil {
		return nil, err
	}
	return mihomo.LoadEnv(repoRoot)
}
