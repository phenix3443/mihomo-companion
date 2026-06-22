package mihomo

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	standaloneCommandPattern = regexp.MustCompile(`^(.*?)\s+-d\s+(.*?)\s*$`)
	linuxCommandPattern      = regexp.MustCompile(`^(.*?)\s+-f\s+(.*?)\s*$`)
	vergeCommandPattern      = regexp.MustCompile(`^(.*?)\s+-d\s+(.*?)\s+-f\s+(.*?)(?:\s+-\S.*)?$`)
)

func (e *Env) Detect() error {
	runtime := e.DetectRuntime()
	fmt.Fprintf(os.Stdout, "%-12s %s\n", "Client:", runtime.Client)
	fmt.Fprintf(os.Stdout, "%-12s %s\n", "Install Dir:", runtime.InstallDir)
	fmt.Fprintf(os.Stdout, "%-12s %s\n", "Config File:", runtime.ConfigFile)
	return nil
}

func (e *Env) detectLiveConfigDir() (string, error) {
	if e.OS == "darwin" {
		return e.detectLiveConfigDirDarwin()
	}
	return e.detectLiveConfigDirLinux()
}

func (e *Env) detectLiveConfigFile() (string, error) {
	runtime := e.DetectRuntime()
	if runtime.ConfigFile != "" && runtime.ConfigFile != "-" {
		return runtime.ConfigFile, nil
	}

	configDir, err := e.detectLiveConfigDir()
	if err != nil {
		return "", fmt.Errorf("could not detect mihomo config file")
	}
	configFile := filepath.Join(configDir, "config.yaml")
	if fileExists(configFile) {
		return configFile, nil
	}
	return "", fmt.Errorf("could not detect mihomo config file")
}

func (e *Env) detectLiveConfigDirDarwin() (string, error) {
	if runtime, ok := e.detectStandaloneRuntime(); ok {
		return filepath.Dir(runtime.ConfigFile), nil
	}

	if runtime, ok := e.detectVergeRuntime(); ok {
		return filepath.Dir(runtime.ConfigFile), nil
	}

	if commandExists("brew") {
		prefix := strings.TrimSpace(runOptional("", "brew", "--prefix"))
		if prefix != "" {
			candidate := filepath.Join(prefix, "etc", "mihomo")
			if fileExists(filepath.Join(candidate, "config.yaml")) {
				return candidate, nil
			}
		}
	}

	xdgCandidate := filepath.Join(e.HomeDir, ".config", "mihomo")
	if fileExists(filepath.Join(xdgCandidate, "config.yaml")) {
		return xdgCandidate, nil
	}
	return "", fmt.Errorf("could not detect mihomo config directory")
}

func (e *Env) detectLiveConfigDirLinux() (string, error) {
	if runtime, ok := e.detectLinuxRuntime(); ok {
		return filepath.Dir(runtime.ConfigFile), nil
	}

	if dirExists("/etc/clash") {
		return "/etc/clash", nil
	}
	return "", fmt.Errorf("could not detect mihomo config directory")
}

func (e *Env) detectActiveClient() string {
	if e.OS != "darwin" {
		return "linux"
	}
	if runCommand("", nil, nil, "pgrep", "-x", "verge-mihomo") == nil || runCommand("", nil, nil, "pgrep", "-f", "[v]erge-mihomo") == nil {
		return "verge"
	}
	if e.macosKernelRunning() {
		return "mihomo"
	}
	return "none"
}

func (e *Env) macosKernelRunning() bool {
	patterns := []string{
		"/opt/homebrew/opt/mihomo/bin/mihomo",
		"/usr/local/opt/mihomo/bin/mihomo",
		"/opt/homebrew/Cellar/mihomo/",
		"/usr/local/Cellar/mihomo/",
		"[/]mihomo[[:space:]]+-d[[:space:]]",
	}
	for _, pattern := range patterns {
		if runCommand("", nil, nil, "pgrep", "-f", pattern) == nil {
			return true
		}
	}
	return false
}

type runtimeInfo struct {
	Client     string
	InstallDir string
	ConfigFile string
}

func (e *Env) DetectRuntime() runtimeInfo {
	if e.OS == "darwin" {
		if runtime, ok := e.detectVergeRuntime(); ok {
			return runtime
		}
		if runtime, ok := e.detectStandaloneRuntime(); ok {
			return runtime
		}
		return runtimeInfo{Client: "none", InstallDir: "-", ConfigFile: "-"}
	}
	if runtime, ok := e.detectLinuxRuntime(); ok {
		return runtime
	}
	return runtimeInfo{Client: "none", InstallDir: "-", ConfigFile: "-"}
}

func (e *Env) detectStandaloneRuntime() (runtimeInfo, bool) {
	for _, line := range processCommandLines() {
		if strings.Contains(line, "verge-mihomo") || !strings.Contains(line, "mihomo") {
			continue
		}
		binaryPath, configDir, ok := parseStandaloneRuntimeLine(line)
		if !ok {
			continue
		}
		return runtimeInfo{
			Client:     "mihomo",
			InstallDir: filepath.Dir(binaryPath),
			ConfigFile: filepath.Join(configDir, "config.yaml"),
		}, true
	}

	binaryPath, err := e.resolveDarwinMihomoBinary()
	if err != nil || !fileExists(filepath.Join(e.ConfigDir, "config.yaml")) {
		return runtimeInfo{}, false
	}
	return runtimeInfo{
		Client:     "mihomo",
		InstallDir: filepath.Dir(binaryPath),
		ConfigFile: filepath.Join(e.ConfigDir, "config.yaml"),
	}, true
}

func (e *Env) detectVergeRuntime() (runtimeInfo, bool) {
	for _, line := range processCommandLines() {
		if !strings.Contains(line, "verge-mihomo") {
			continue
		}
		binaryPath, configFile, ok := parseVergeRuntimeLine(line)
		if !ok {
			continue
		}
		return runtimeInfo{
			Client:     "verge",
			InstallDir: filepath.Dir(binaryPath),
			ConfigFile: configFile,
		}, true
	}
	return runtimeInfo{}, false
}

func (e *Env) detectLinuxRuntime() (runtimeInfo, bool) {
	for _, line := range processCommandLines() {
		if !strings.Contains(line, "mihomo") {
			continue
		}
		binaryPath, configFile, ok := parseLinuxRuntimeLine(line)
		if !ok {
			continue
		}
		return runtimeInfo{
			Client:     "mihomo",
			InstallDir: filepath.Dir(binaryPath),
			ConfigFile: configFile,
		}, true
	}

	liveConfig := filepath.Join("/etc/clash", "config.yaml")
	binaryPath := filepath.Join(e.InstallDir, "mihomo")
	if fileExists(liveConfig) && fileExists(binaryPath) {
		return runtimeInfo{
			Client:     "mihomo",
			InstallDir: e.InstallDir,
			ConfigFile: liveConfig,
		}, true
	}
	return runtimeInfo{}, false
}

func processCommandLines() []string {
	output := runOptional("", "ps", "-axo", "command")
	lines := strings.Split(output, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		filtered = append(filtered, line)
	}
	return filtered
}

func parseStandaloneRuntimeLine(line string) (string, string, bool) {
	match := standaloneCommandPattern.FindStringSubmatch(strings.TrimSpace(line))
	if len(match) != 3 {
		return "", "", false
	}
	binaryPath := strings.TrimSpace(match[1])
	configDir := strings.TrimSpace(match[2])
	if binaryPath == "" || configDir == "" {
		return "", "", false
	}
	if filepath.Base(binaryPath) != "mihomo" {
		return "", "", false
	}
	return binaryPath, configDir, true
}

func parseVergeRuntimeLine(line string) (string, string, bool) {
	match := vergeCommandPattern.FindStringSubmatch(strings.TrimSpace(line))
	if len(match) != 4 {
		return "", "", false
	}
	binaryPath := strings.TrimSpace(match[1])
	configFile := strings.TrimSpace(match[3])
	if binaryPath == "" || configFile == "" {
		return "", "", false
	}
	if filepath.Base(binaryPath) != "verge-mihomo" {
		return "", "", false
	}
	return binaryPath, configFile, true
}

func parseLinuxRuntimeLine(line string) (string, string, bool) {
	match := linuxCommandPattern.FindStringSubmatch(strings.TrimSpace(line))
	if len(match) != 3 {
		return "", "", false
	}
	binaryPath := strings.TrimSpace(match[1])
	configFile := strings.TrimSpace(match[2])
	if binaryPath == "" || configFile == "" {
		return "", "", false
	}
	if filepath.Base(binaryPath) != "mihomo" {
		return "", "", false
	}
	return binaryPath, configFile, true
}

func (e *Env) resolveSyncTarget() string {
	switch e.MacTarget {
	case "verge":
		return "verge"
	case "standalone", "mihomo":
		return "standalone"
	}

	active := e.detectActiveClient()
	if active == "verge" {
		return "verge"
	}
	if active == "mihomo" {
		return "standalone"
	}
	if commandExists("brew") && runCommand("", nil, nil, "brew", "list", "mihomo") == nil && fileExists(filepath.Join(e.ConfigDir, "config.yaml")) {
		return "standalone"
	}
	if fileExists(e.ClashVergeProfilesYAML()) && dirExists(e.ClashVergeDir) {
		return "verge"
	}
	return "unknown"
}
