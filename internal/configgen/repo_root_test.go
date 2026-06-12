package configgen

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveRepoRootUsesOverride(t *testing.T) {
	override := filepath.Join(t.TempDir(), "instance")
	if err := os.MkdirAll(override, 0o755); err != nil {
		t.Fatalf("mkdir override: %v", err)
	}
	t.Setenv("MIHOMO_REPO_ROOT", override)

	got, err := ResolveRepoRoot()
	if err != nil {
		t.Fatalf("ResolveRepoRoot returned error: %v", err)
	}

	want, err := filepath.Abs(override)
	if err != nil {
		t.Fatalf("filepath.Abs returned error: %v", err)
	}
	if got != want {
		t.Fatalf("ResolveRepoRoot = %q, want %q", got, want)
	}
}

func TestResolveRepoRootFallsBackToDetect(t *testing.T) {
	t.Setenv("MIHOMO_REPO_ROOT", "")

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller returned no file")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))

	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd returned error: %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previousWD)
	})

	got, err := ResolveRepoRoot()
	if err != nil {
		t.Fatalf("ResolveRepoRoot returned error: %v", err)
	}

	want, err := DetectRepoRoot()
	if err != nil {
		t.Fatalf("DetectRepoRoot returned error: %v", err)
	}
	if got != want {
		t.Fatalf("ResolveRepoRoot = %q, want %q", got, want)
	}
}
