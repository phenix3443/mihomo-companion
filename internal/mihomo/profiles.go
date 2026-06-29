package mihomo

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/phenix3443/mihctl/internal/configgen"
)

type RuntimeProfile struct {
	Name string
	OS   string
}

func loadRuntimeProfiles(repoRoot string) ([]RuntimeProfile, error) {
	cfg, err := configgen.LoadGenerationConfig(filepath.Join(repoRoot, "config", "values.yaml"))
	if err != nil {
		return nil, err
	}

	profiles := make([]RuntimeProfile, 0, len(cfg.ProfileOrder))
	for _, profileName := range cfg.ProfileOrder {
		spec, ok := cfg.Profiles[profileName]
		if !ok {
			continue
		}
		profiles = append(profiles, RuntimeProfile{
			Name: profileName,
			OS:   strings.TrimSpace(spec.OS),
		})
	}
	return profiles, nil
}

func loadDefaultProfile(repoRoot string) (string, error) {
	cfg, err := configgen.LoadGenerationConfig(filepath.Join(repoRoot, "config", "values.yaml"))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(cfg.DefaultProfile), nil
}

func (e *Env) generatedConfigPath(profile string) string {
	return configgen.Paths{RepoRoot: e.RepoRoot}.OutputForProfile(profile)
}

func (e *Env) generatedConfigPaths() []string {
	paths := make([]string, 0, len(e.RuntimeProfiles))
	for _, profile := range e.RuntimeProfiles {
		paths = append(paths, e.generatedConfigPath(profile.Name))
	}
	return paths
}

func (e *Env) profileByName(name string) (RuntimeProfile, error) {
	name = strings.TrimSpace(name)
	for _, profile := range e.RuntimeProfiles {
		if profile.Name == name {
			return profile, nil
		}
	}
	return RuntimeProfile{}, fmt.Errorf("no profile configured with name %s", name)
}

func (e *Env) autoSyncProfile(target string) (RuntimeProfile, error) {
	switch target {
	case "linux":
		if profile, ok := onlyProfileForOS(e.RuntimeProfiles, "linux"); ok {
			return profile, nil
		}
		return RuntimeProfile{}, fmt.Errorf("no unique linux profile configured")
	case "standalone", "verge":
		if profile, ok := onlyProfileForOS(e.RuntimeProfiles, "macos"); ok {
			return profile, nil
		}
		if profile, ok := preferredDarwinProfile(e.RuntimeProfiles, target); ok {
			return profile, nil
		}
		if profile, ok := onlyDarwinProfileForTarget(e.RuntimeProfiles, target); ok {
			return profile, nil
		}
		return RuntimeProfile{}, fmt.Errorf("no profile configured for macos sync target %s", target)
	}

	return RuntimeProfile{}, fmt.Errorf("no profile configured for sync target %s", target)
}

func namedProfileForOS(profiles []RuntimeProfile, profileName, targetOS string) (RuntimeProfile, bool) {
	for _, profile := range profiles {
		if profile.Name == profileName && profile.OS == targetOS {
			return profile, true
		}
	}
	return RuntimeProfile{}, false
}

func preferredDarwinProfile(profiles []RuntimeProfile, target string) (RuntimeProfile, bool) {
	preferred := "local"
	if target == "verge" {
		preferred = "clash-verge"
	}
	return namedProfileForOS(profiles, preferred, "macos")
}

func onlyDarwinProfileForTarget(profiles []RuntimeProfile, target string) (RuntimeProfile, bool) {
	var matched RuntimeProfile
	count := 0
	for _, profile := range profiles {
		if !matchesDarwinSyncTarget(profile, target) {
			continue
		}
		matched = profile
		count++
	}
	return matched, count == 1
}

func matchesDarwinSyncTarget(profile RuntimeProfile, target string) bool {
	if profile.OS != "macos" {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(profile.Name))
	isVerge := strings.Contains(name, "verge")
	switch target {
	case "verge":
		return isVerge
	case "standalone":
		return !isVerge
	default:
		return false
	}
}

func onlyProfileForOS(profiles []RuntimeProfile, targetOS string) (RuntimeProfile, bool) {
	var matched RuntimeProfile
	count := 0
	for _, profile := range profiles {
		if profile.OS != targetOS {
			continue
		}
		matched = profile
		count++
	}
	return matched, count == 1
}
