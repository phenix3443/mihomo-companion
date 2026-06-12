package configgen

import (
	"os"
	"path/filepath"
	"strings"
)

func ResolveRepoRoot() (string, error) {
	if value := strings.TrimSpace(os.Getenv("MIHCTL_INSTANCE_ROOT")); value != "" {
		return filepath.Abs(value)
	}
	return DetectRepoRoot()
}
