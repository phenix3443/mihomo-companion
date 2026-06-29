package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/phenix3443/mihctl/internal/platform"
)

func ServiceStatus() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return darwinStatus(), nil
	case "linux":
		return linuxStatus(), nil
	default:
		return "", fmt.Errorf("unsupported os: %s", runtime.GOOS)
	}
}

func darwinStatus() string {
	configDir := platform.DefaultConfigDir()
	lines := []string{
		"Mihomo (macOS)",
		fmt.Sprintf("  Config: %s/config.yaml", configDir),
	}
	return strings.Join(lines, "\n")
}

func linuxStatus() string {
	lines := []string{"Mihomo (Linux)"}
	if _, err := exec.LookPath("systemctl"); err == nil {
		cmd := exec.Command("systemctl", "show", "-p", "ActiveState", "--value", "mihomo")
		output, err := cmd.Output()
		if err == nil {
			lines = append(lines, "  Active: "+strings.TrimSpace(string(output)))
		}
	}
	configPath := linuxConfigPath()
	if _, err := os.Stat(configPath); err == nil {
		lines = append(lines, "  Config: "+configPath)
	}
	return strings.Join(lines, "\n")
}

func linuxConfigPath() string {
	configDir := strings.TrimSpace(os.Getenv("CONFIG_DIR"))
	if configDir == "" {
		configDir = "/etc/clash"
	}
	return configDir + "/config.yaml"
}
