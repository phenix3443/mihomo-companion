package configgen

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTemplateRootUsesEnvOverride(t *testing.T) {
	t.Setenv("MIHCTL_TEMPLATE_ROOT", "/tmp/custom-root")
	root, err := ResolveTemplateRoot(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveTemplateRoot error = %v", err)
	}
	if root != "/tmp/custom-root" {
		t.Fatalf("root = %q, want /tmp/custom-root", root)
	}
}

func TestResolveTemplateRootDetectsSourceRepoFromExecutable(t *testing.T) {
	original := ExecutablePathForTest
	t.Cleanup(func() { ExecutablePathForTest = original })

	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "mihomo.yaml.tmpl"), []byte("template"), 0o644); err != nil {
		t.Fatal(err)
	}
	ExecutablePathForTest = func() (string, error) {
		return filepath.Join(binDir, "mihctl"), nil
	}

	resolved, err := ResolveTemplateRoot(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveTemplateRoot error = %v", err)
	}
	if resolved != root {
		t.Fatalf("resolved = %q, want %q", resolved, root)
	}
}

func TestResolveTemplateRootFallsBackToInstanceRoot(t *testing.T) {
	original := ExecutablePathForTest
	t.Cleanup(func() { ExecutablePathForTest = original })

	ExecutablePathForTest = func() (string, error) {
		return filepath.Join(t.TempDir(), "bin", "mihctl"), nil
	}

	instanceRoot := t.TempDir()
	resolved, err := ResolveTemplateRoot(instanceRoot)
	if err != nil {
		t.Fatalf("ResolveTemplateRoot error = %v", err)
	}
	if resolved != instanceRoot {
		t.Fatalf("resolved = %q, want %q", resolved, instanceRoot)
	}
}
