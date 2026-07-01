package configgen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ExecutablePathForTest = os.Executable

func ResolveTemplateRoot(instanceRoot string) (string, error) {
	if value := strings.TrimSpace(os.Getenv("MIHCTL_TEMPLATE_ROOT")); value != "" {
		return filepath.Abs(value)
	}

	if root, err := detectTemplateRootFromExecutable(); err == nil {
		return root, nil
	}

	return filepath.Abs(instanceRoot)
}

func detectTemplateRootFromExecutable() (string, error) {
	executable, err := ExecutablePathForTest()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err == nil && strings.TrimSpace(resolved) != "" {
		executable = resolved
	}
	dir := filepath.Dir(executable)
	candidates := []string{
		filepath.Clean(filepath.Join(dir, "..")),
		filepath.Clean(filepath.Join(dir, "..", "share", "mihctl")),
	}
	for _, candidate := range candidates {
		if pathExists(filepath.Join(candidate, "config", "mihomo.yaml.tmpl")) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("template root not found from executable %s", executable)
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
