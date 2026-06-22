package mihomo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseStandaloneRuntimeLine(t *testing.T) {
	line := "/opt/homebrew/opt/mihomo/bin/mihomo -d /Users/demo/.config/mihomo"
	binaryPath, configDir, ok := parseStandaloneRuntimeLine(line)
	if !ok {
		t.Fatal("expected standalone runtime line to parse")
	}
	if binaryPath != "/opt/homebrew/opt/mihomo/bin/mihomo" {
		t.Fatalf("binaryPath = %q", binaryPath)
	}
	if configDir != "/Users/demo/.config/mihomo" {
		t.Fatalf("configDir = %q", configDir)
	}
}

func TestParseStandaloneRuntimeLineRejectsNonMihomoExecutable(t *testing.T) {
	line := `/Users/demo/.codex/computer-use/SkyComputerUseClient turn-ended {"last-assistant-message":"run mihomo -d feat/idea-project-agent"} -d polluted/config`
	_, _, ok := parseStandaloneRuntimeLine(line)
	if ok {
		t.Fatal("expected non-mihomo executable to be rejected")
	}
}

func TestParseVergeRuntimeLine(t *testing.T) {
	line := "/Applications/Clash Verge.app/Contents/MacOS/verge-mihomo -d /Users/demo/Library/Application Support/io.github.clash-verge-rev.clash-verge-rev -f /Users/demo/Library/Application Support/io.github.clash-verge-rev.clash-verge-rev/profiles/demo.yaml"
	binaryPath, configFile, ok := parseVergeRuntimeLine(line)
	if !ok {
		t.Fatal("expected verge runtime line to parse")
	}
	if binaryPath != "/Applications/Clash Verge.app/Contents/MacOS/verge-mihomo" {
		t.Fatalf("binaryPath = %q", binaryPath)
	}
	if configFile != "/Users/demo/Library/Application Support/io.github.clash-verge-rev.clash-verge-rev/profiles/demo.yaml" {
		t.Fatalf("configFile = %q", configFile)
	}
}

func TestParseVergeRuntimeLineRejectsNonVergeExecutable(t *testing.T) {
	line := `/Users/demo/.codex/computer-use/SkyComputerUseClient turn-ended {"last-assistant-message":"verge-mihomo"} -d /tmp -f feat/idea-project-agent/config.yaml`
	_, _, ok := parseVergeRuntimeLine(line)
	if ok {
		t.Fatal("expected non-verge executable to be rejected")
	}
}

func TestParseLinuxRuntimeLine(t *testing.T) {
	line := "/usr/local/bin/mihomo -f /etc/clash/config.yaml"
	binaryPath, configFile, ok := parseLinuxRuntimeLine(line)
	if !ok {
		t.Fatal("expected linux runtime line to parse")
	}
	if binaryPath != "/usr/local/bin/mihomo" {
		t.Fatalf("binaryPath = %q", binaryPath)
	}
	if configFile != "/etc/clash/config.yaml" {
		t.Fatalf("configFile = %q", configFile)
	}
}

func TestParseLinuxRuntimeLineRejectsNonMihomoExecutable(t *testing.T) {
	line := `/Users/demo/.codex/computer-use/SkyComputerUseClient turn-ended {"last-assistant-message":"mihomo"} -f feat/idea-project-agent/config.yaml`
	_, _, ok := parseLinuxRuntimeLine(line)
	if ok {
		t.Fatal("expected non-mihomo executable to be rejected")
	}
}

func TestDetectRuntimeDoesNotTreatVergeProfilesAsRunningRuntime(t *testing.T) {
	homeDir := t.TempDir()
	vergeProfilesDir := filepath.Join(homeDir, "Library", "Application Support", "io.github.clash-verge-rev.clash-verge-rev", "profiles")
	if err := os.MkdirAll(vergeProfilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(vergeProfilesDir), "profiles.yaml"), []byte("current: demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vergeProfilesDir, "demo.yaml"), []byte("mixed-port: 7890\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	env := &Env{
		OS:            "darwin",
		HomeDir:       homeDir,
		ClashVergeDir: vergeProfilesDir,
		ConfigDir:     filepath.Join(homeDir, ".config", "mihomo"),
	}

	runtime := env.DetectRuntime()
	if runtime.Client == "verge" {
		t.Fatalf("client = %q, want non-verge runtime when no verge-mihomo process exists", runtime.Client)
	}
}
