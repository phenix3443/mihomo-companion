package configgen

import (
	"path/filepath"
	"strings"
)

type Config map[string]any

type GenerateOptions struct {
	EnableLinuxTUN bool
	EnableMacOSTUN bool
}

type Paths struct {
	RepoRoot       string
	TemplateRoot   string
	TemplateConfig string
	ValuesConfig   string
}

func (p Paths) OutputForProfile(profile string) string {
	return filepath.Join(p.RepoRoot, "config", profile, "mihomo.yaml")
}

func DeepCopy(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, inner := range typed {
			cloned[key] = DeepCopy(inner)
		}
		return cloned
	case []any:
		cloned := make([]any, len(typed))
		for index, inner := range typed {
			cloned[index] = DeepCopy(inner)
		}
		return cloned
	default:
		return typed
	}
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	return DeepCopy(value).(map[string]any)
}

func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}
