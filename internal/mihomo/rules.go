package mihomo

import (
	"fmt"
	"sort"

	"github.com/phenix3443/mihctl/internal/runtime"
)

func (e *Env) UpdateRules() error {
	sourceYAML, err := e.detectLiveConfigFile()
	if err != nil {
		return err
	}
	info, err := runtime.CaptureReloadInfoFromYAML(sourceYAML)
	if err != nil {
		return err
	}
	return e.UpdateRulesRemote(info)
}

func (e *Env) UpdateRulesRemote(info runtime.APIReloadInfo) error {
	names, err := runtime.RuleProviders(info)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		logInfo("No runtime rule-providers exposed by external-controller")
		return nil
	}

	sort.Strings(names)
	logStep("Updating runtime rule-providers via external-controller")
	for _, name := range names {
		logInfo("Rule provider %s", name)
		if err := runtime.UpdateRuleProvider(info, name); err != nil {
			return fmt.Errorf("update runtime rule provider %s: %w", name, err)
		}
		logSuccess("Updated %s", name)
	}

	logSuccess("Runtime rule update complete")
	return nil
}
