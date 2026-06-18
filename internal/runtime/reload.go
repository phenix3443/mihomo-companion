package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type APIReloadInfo struct {
	BaseURL string
	Secret  string
}

type ConfigAccessInfo struct {
	Controller string
	MixedPort  int
}

func CaptureReloadInfoFromYAML(path string) (APIReloadInfo, error) {
	raw, err := loadRawConfigYAML(path)
	if err != nil {
		return APIReloadInfo{}, err
	}

	controller, _ := raw["external-controller"].(string)
	controller = strings.TrimSpace(controller)
	if controller == "" {
		return APIReloadInfo{}, fmt.Errorf("missing external-controller in %s", path)
	}

	switch {
	case strings.HasPrefix(controller, "0.0.0.0:"):
		controller = "127.0.0.1:" + strings.TrimPrefix(controller, "0.0.0.0:")
	case strings.HasPrefix(controller, "[::]:"):
		controller = "127.0.0.1:" + strings.TrimPrefix(controller, "[::]:")
	}

	info := APIReloadInfo{
		BaseURL: "http://" + controller,
	}
	if secret, ok := raw["secret"].(string); ok {
		info.Secret = secret
	}

	return info, nil
}

func CaptureAccessInfoFromYAML(path string) (ConfigAccessInfo, error) {
	raw, err := loadRawConfigYAML(path)
	if err != nil {
		return ConfigAccessInfo{}, err
	}

	controller, _ := raw["external-controller"].(string)
	controller = strings.TrimSpace(controller)
	if controller == "" {
		return ConfigAccessInfo{}, fmt.Errorf("missing external-controller in %s", path)
	}

	mixedPort, ok := intFromYAMLValue(raw["mixed-port"])
	if !ok {
		return ConfigAccessInfo{}, fmt.Errorf("missing mixed-port in %s", path)
	}

	return ConfigAccessInfo{
		Controller: controller,
		MixedPort:  mixedPort,
	}, nil
}

func loadRawConfigYAML(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func intFromYAMLValue(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func Reload(info APIReloadInfo) error {
	req, err := http.NewRequest(http.MethodPut, info.BaseURL+"/configs?force=true", bytes.NewBufferString("{}"))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if info.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+info.Secret)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("reload failed with status %d", resp.StatusCode)
	}
	return nil
}

func RuleProviders(info APIReloadInfo) ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, info.BaseURL+"/providers/rules", nil)
	if err != nil {
		return nil, err
	}
	if info.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+info.Secret)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("list rule providers failed with status %d", resp.StatusCode)
	}

	var payload struct {
		Providers map[string]any `json:"providers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(payload.Providers))
	for name := range payload.Providers {
		names = append(names, name)
	}
	return names, nil
}

func UpdateRuleProvider(info APIReloadInfo, name string) error {
	req, err := http.NewRequest(http.MethodPut, info.BaseURL+"/providers/rules/"+name, bytes.NewBufferString("{}"))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if info.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+info.Secret)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update rule provider %s failed with status %d", name, resp.StatusCode)
	}
	return nil
}
