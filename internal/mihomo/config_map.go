package mihomo

import "github.com/phenix3443/mihctl/internal/configgen"

func asConfigMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case configgen.Config:
		return map[string]any(typed), true
	default:
		return nil, false
	}
}
