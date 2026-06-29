package mihomo

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/phenix3443/mihctl/internal/configgen"
	"github.com/phenix3443/mihctl/internal/runtime"
)

func (e *Env) generateConfigArtifacts(options configgen.GenerateOptions) error {
	service := configgen.NewService(e.RepoRoot)
	_, err := service.Generate(options)
	return err
}

func syncProvidersWithConfigSync() bool {
	return false
}

func syncProvidersWithInstall() bool {
	return true
}

func (e *Env) resolveLinuxSyncProfile(profileName string) (RuntimeProfile, error) {
	if strings.TrimSpace(profileName) == "" {
		return e.autoSyncProfile("linux")
	}
	return e.profileByName(profileName)
}

func (e *Env) resolveDarwinSyncProfile(profileName, target string) (RuntimeProfile, error) {
	if strings.TrimSpace(profileName) == "" {
		return e.autoSyncProfile(target)
	}
	return e.profileByName(profileName)
}

func (e *Env) SyncConfig(options configgen.GenerateOptions, profileName string, startedFromService bool) error {
	if e.OS == "darwin" {
		target := e.determineDarwinSyncTarget()
		sourceProfile, err := e.resolveDarwinSyncProfile(profileName, target)
		if err != nil {
			return err
		}
		if sourceProfile.OS != "macos" {
			return fmt.Errorf("profile %s targets %s, but current runtime expects macos", sourceProfile.Name, sourceProfile.OS)
		}
		return e.syncConfigDarwin(options, sourceProfile, target, startedFromService, syncProvidersWithConfigSync())
	}

	sourceProfile, err := e.resolveLinuxSyncProfile(profileName)
	if err != nil {
		return err
	}
	if sourceProfile.OS != "linux" {
		return fmt.Errorf("profile %s targets %s, but current runtime expects linux", sourceProfile.Name, sourceProfile.OS)
	}
	return e.syncConfigLinux(options, sourceProfile, syncProvidersWithConfigSync())
}

func (e *Env) validateDarwinSyncProfile(sourceProfile RuntimeProfile, target string) error {
	if !matchesDarwinSyncTarget(sourceProfile, target) {
		expectedProfile, err := e.autoSyncProfile(target)
		if err != nil {
			return err
		}
		return fmt.Errorf("profile %s does not match macos sync target %s (expected %s)", sourceProfile.Name, target, expectedProfile.Name)
	}
	return nil
}

func (e *Env) determineDarwinSyncTarget() string {
	target := e.resolveSyncTarget()
	if target == "unknown" {
		logWarn("Could not detect Clash Verge vs mihomo; set MIHOMO_MAC_TARGET=verge or standalone")
		if fileExists(e.ClashVergeProfilesYAML()) && dirExists(e.ClashVergeDir) {
			target = "verge"
			logInfo("Falling back to Clash Verge (profiles present)")
		} else {
			target = "standalone"
			logInfo("Falling back to standalone mihomo (%s)", e.ConfigDir)
		}
	} else {
		logInfo("macOS sync target: %s", target)
	}
	return target
}

func (e *Env) syncConfigDarwin(options configgen.GenerateOptions, sourceProfile RuntimeProfile, target string, startedFromService, syncProviders bool) error {
	if err := e.generateConfigArtifacts(options); err != nil {
		return err
	}

	if target == "standalone" {
		if err := e.validateDarwinSyncProfile(sourceProfile, "standalone"); err != nil {
			return err
		}
		return e.syncConfigMacStandalone(sourceProfile, startedFromService, syncProviders)
	}

	if err := e.validateDarwinSyncProfile(sourceProfile, "verge"); err != nil {
		return err
	}

	if !dirExists(e.ClashVergeDir) {
		if e.MacTarget == "verge" {
			return fmt.Errorf("MIHOMO_MAC_TARGET=verge but Clash Verge profiles dir missing: %s", e.ClashVergeDir)
		}
		logWarn("Clash Verge profiles dir missing; deploying standalone bundle to %s", e.ConfigDir)
		return e.syncConfigMacStandalone(sourceProfile, startedFromService, syncProviders)
	}

	profileUID := strings.TrimSpace(runOptional("", "yq", ".current", e.ClashVergeProfilesYAML()))
	if profileUID == "" || profileUID == "null" {
		if e.MacTarget == "verge" {
			return fmt.Errorf("MIHOMO_MAC_TARGET=verge but no active Clash Verge profile in profiles.yaml")
		}
		logWarn("No active Clash Verge profile; deploying standalone bundle to %s", e.ConfigDir)
		return e.syncConfigMacStandalone(sourceProfile, startedFromService, syncProviders)
	}

	targetPath := filepath.Join(e.ClashVergeDir, profileUID+".yaml")
	if !fileExists(targetPath) {
		if e.MacTarget == "verge" {
			return fmt.Errorf("Clash Verge profile file not found: %s", targetPath)
		}
		logWarn("Clash Verge profile file missing; deploying standalone bundle")
		return e.syncConfigMacStandalone(sourceProfile, startedFromService, syncProviders)
	}

	vergeConfigPath := e.generatedConfigPath(sourceProfile.Name)
	info, err := captureDarwinVergeReloadInfo(targetPath, vergeConfigPath)
	if err != nil {
		return err
	}
	if err := copyFilePrivileged(vergeConfigPath, targetPath, 0o644); err != nil {
		return err
	}
	logSuccess("Synced %s -> %s (Clash Verge profile %s)", vergeConfigPath, targetPath, profileUID)
	if err := runtime.Reload(info); err != nil {
		logWarn("API reload failed; restart Clash Verge manually if needed")
	}
	if syncProviders {
		return e.SyncProvidersToLive()
	}
	return nil
}

func captureDarwinVergeReloadInfo(targetPath, generatedPath string) (runtime.APIReloadInfo, error) {
	info, err := runtime.CaptureReloadInfoFromYAML(targetPath)
	if err == nil {
		return info, nil
	}
	logWarn("Could not read API settings from %s, trying generated config %s", targetPath, generatedPath)
	info, generatedErr := runtime.CaptureReloadInfoFromYAML(generatedPath)
	if generatedErr != nil {
		return runtime.APIReloadInfo{}, fmt.Errorf("cannot determine external-controller for API reload")
	}
	return info, nil
}

func (e *Env) syncConfigMacStandalone(sourceProfile RuntimeProfile, startedFromService, syncProviders bool) error {
	if err := mkdirAllPrivileged(filepath.Join(e.ConfigDir, "providers"), 0o755); err != nil {
		return err
	}
	if err := mkdirAllPrivileged(filepath.Join(e.ConfigDir, "ruleset"), 0o755); err != nil {
		return err
	}
	standaloneConfigPath := e.generatedConfigPath(sourceProfile.Name)
	if !fileExists(standaloneConfigPath) {
		return fmt.Errorf("missing %s. run: mihomo config gen", standaloneConfigPath)
	}
	targetConfigPath := filepath.Join(e.ConfigDir, "config.yaml")
	if err := copyFilePrivileged(standaloneConfigPath, targetConfigPath, 0o644); err != nil {
		return err
	}
	if err := runCommand(e.RepoRoot, os.Stdout, os.Stderr, "yq", "-i", fmt.Sprintf(`."external-ui" = "%s"`, filepath.Join(e.ConfigDir, "ui")), targetConfigPath); err != nil {
		return err
	}
	if syncProviders {
		if err := e.SyncProvidersToLive(); err != nil {
			return err
		}
		logSuccess("Synced %s -> %s and providers/", standaloneConfigPath, targetConfigPath)
	} else {
		logSuccess("Synced %s -> %s", standaloneConfigPath, targetConfigPath)
	}

	info, err := runtime.CaptureReloadInfoFromYAML(targetConfigPath)
	if err == nil && runtime.Reload(info) == nil {
		return nil
	}

	if startedFromService {
		logInfo("API reload failed; service will be (re)started by the start command")
		return nil
	}
	if e.isDarwinServiceLoaded() {
		if err := e.restartDarwinService(); err != nil {
			return err
		}
		logSuccess("mihomo restarted (LaunchDaemon)")
		return nil
	}
	logWarn("Could not reload via API. Run: mihctl service restart")
	return nil
}

func (e *Env) syncConfigLinux(options configgen.GenerateOptions, sourceProfile RuntimeProfile, syncProviders bool) error {
	if err := e.RequireRoot("sync-config"); err != nil {
		return err
	}
	if e.SudoUser() == "" {
		return fmt.Errorf("sync-config on Linux expects sudo from a normal user (SUDO_USER empty)")
	}
	if err := e.generateConfigArtifacts(configgen.GenerateOptions{EnableLinuxTUN: false, EnableMacOSTUN: options.EnableMacOSTUN}); err != nil {
		return err
	}

	liveConfigPath := filepath.Join(e.ConfigDir, "config.yaml")
	if fileExists(liveConfigPath) {
		backup := liveConfigPath + ".bak." + time.Now().Format("20060102150405")
		if err := copyFilePrivileged(liveConfigPath, backup, 0o644); err == nil {
			logInfo("Backed up to %s", backup)
		}
	}

	var reloadInfo runtime.APIReloadInfo
	hadPrev := false
	if fileExists(liveConfigPath) {
		if info, err := runtime.CaptureReloadInfoFromYAML(liveConfigPath); err == nil {
			reloadInfo = info
			hadPrev = true
		}
	}

	if err := e.updateLinuxConfigInternal(sourceProfile); err != nil {
		return err
	}

	needRestart := true
	if hadPrev {
		if err := runtime.Reload(reloadInfo); err == nil {
			needRestart = false
		} else {
			logWarn("API reload failed, falling back to systemctl restart")
		}
	} else {
		logInfo("No previous live config for API settings, using systemctl restart")
	}

	if needRestart {
		if e.isLinuxServiceActive("mihomo") {
			if err := runCommand("", os.Stdout, os.Stderr, "systemctl", "restart", "mihomo"); err != nil {
				return err
			}
			logSuccess("mihomo restarted via systemd")
		} else {
			logWarn("mihomo service not active; start with: sudo mihctl service start")
		}
	}
	if syncProviders {
		return e.SyncProvidersToLive()
	}
	return nil
}

func (e *Env) updateLinuxConfigInternal(sourceProfile RuntimeProfile) error {
	linuxConfigPath := e.generatedConfigPath(sourceProfile.Name)
	if !fileExists(linuxConfigPath) {
		return fmt.Errorf("no source config: %s", linuxConfigPath)
	}
	if err := mkdirAllPrivileged(e.ConfigDir, 0o755); err != nil {
		return err
	}
	tmpPath, err := tempPathInDir(e.ConfigDir, "config.yaml.*")
	if err != nil {
		return err
	}
	defer os.Remove(tmpPath)

	sourceData, err := os.ReadFile(linuxConfigPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmpPath, sourceData, 0o644); err != nil {
		return err
	}
	if err := runCommand(e.RepoRoot, os.Stdout, os.Stderr, "yq", "eval", ".", tmpPath); err != nil {
		return err
	}
	targetConfigPath := filepath.Join(e.ConfigDir, "config.yaml")
	if err := renameFilePrivileged(tmpPath, targetConfigPath); err != nil {
		return err
	}
	logSuccess("Synced %s -> %s", linuxConfigPath, targetConfigPath)
	return nil
}
