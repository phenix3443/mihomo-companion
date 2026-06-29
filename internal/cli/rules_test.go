package cli

import (
	"testing"

	"github.com/phenix3443/mihctl/internal/mihomo"
)

func TestRulesUpdateCommandUpdatesRules(t *testing.T) {
	originalLoadEnv := loadEnv
	originalRunRulesUpdate := runRulesUpdate
	t.Cleanup(func() {
		loadEnv = originalLoadEnv
		runRulesUpdate = originalRunRulesUpdate
	})

	env := &mihomo.Env{}
	loadEnv = func() (*mihomo.Env, error) {
		return env, nil
	}

	var gotEnv *mihomo.Env
	runRulesUpdate = func(actualEnv rulesUpdateEnv) error {
		gotEnv, _ = actualEnv.(*mihomo.Env)
		return nil
	}

	cmd := newRulesCmd()
	cmd.SetArgs([]string{"update"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if gotEnv != env {
		t.Fatal("update command did not update rules with loaded env")
	}
}
