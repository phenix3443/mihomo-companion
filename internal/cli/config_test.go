package cli

import (
	"testing"

	"github.com/phenix3443/mihctl/internal/configgen"
	"github.com/phenix3443/mihctl/internal/mihomo"
)

func TestConfigSyncCommandPassesExplicitProfile(t *testing.T) {
	originalLoadEnv := loadEnv
	originalRunConfigSync := runConfigSync
	t.Cleanup(func() {
		loadEnv = originalLoadEnv
		runConfigSync = originalRunConfigSync
	})

	env := &mihomo.Env{}
	loadEnv = func() (*mihomo.Env, error) {
		return env, nil
	}

	var gotEnv *mihomo.Env
	var gotOptions configgen.GenerateOptions
	var gotProfile string
	runConfigSync = func(actualEnv *mihomo.Env, options configgen.GenerateOptions, profile string) error {
		gotEnv = actualEnv
		gotOptions = options
		gotProfile = profile
		return nil
	}

	cmd := newConfigCmd()
	cmd.SetArgs([]string{"sync", "--profile", "local"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if gotEnv != env {
		t.Fatal("sync command did not use loaded env")
	}
	if gotProfile != "local" {
		t.Fatalf("profile = %q, want local", gotProfile)
	}
	wantOptions := configgen.GenerateOptions{
		EnableLinuxTUN: false,
		EnableMacOSTUN: true,
	}
	if gotOptions != wantOptions {
		t.Fatalf("options = %#v, want %#v", gotOptions, wantOptions)
	}
}

func TestConfigSyncCommandLeavesProfileEmptyForAutoSelection(t *testing.T) {
	originalLoadEnv := loadEnv
	originalRunConfigSync := runConfigSync
	t.Cleanup(func() {
		loadEnv = originalLoadEnv
		runConfigSync = originalRunConfigSync
	})

	loadEnv = func() (*mihomo.Env, error) {
		return &mihomo.Env{DefaultProfile: "local"}, nil
	}

	var gotProfile string
	runConfigSync = func(actualEnv *mihomo.Env, options configgen.GenerateOptions, profile string) error {
		gotProfile = profile
		return nil
	}

	cmd := newConfigCmd()
	cmd.SetArgs([]string{"sync"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if gotProfile != "" {
		t.Fatalf("profile = %q, want empty string for auto-selection", gotProfile)
	}
}
