package mihomo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/phenix3443/mihctl/internal/configgen"
)

func TestLoadEnvUsesPublicTemplateServicePathWhenAvailable(t *testing.T) {
	original := configgenExecutablePathForTest(t)

	root := t.TempDir()
	instanceRoot := filepath.Join(root, "instance")
	templateRoot := filepath.Join(root, "mihctl")
	if err := os.MkdirAll(filepath.Join(instanceRoot, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(templateRoot, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(instanceRoot, "config", "values.yaml"), []byte("default-profile: local\nprofiles:\n  local:\n    os: macos\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(templateRoot, "config", "mihomo.yaml.tmpl"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(templateRoot, "config", "mihomo.service.example"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	configgen.ExecutablePathForTest = func() (string, error) {
		return filepath.Join(templateRoot, "bin", "mihctl"), nil
	}

	env, err := LoadEnv(instanceRoot)
	if err != nil {
		t.Fatalf("LoadEnv error = %v", err)
	}
	want := filepath.Join(templateRoot, "config", "mihomo.service.example")
	if env.TemplateServicePath != want {
		t.Fatalf("TemplateServicePath = %q, want %q", env.TemplateServicePath, want)
	}

	_ = original
}

func configgenExecutablePathForTest(t *testing.T) func() (string, error) {
	t.Helper()
	original := configgen.ExecutablePathForTest
	t.Cleanup(func() {
		configgen.ExecutablePathForTest = original
	})
	return original
}
