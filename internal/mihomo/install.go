package mihomo

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/phenix3443/mihctl/internal/configgen"
)

func (e *Env) runAsSudoUser(name string, args ...string) error {
	if e.SudoUser() == "" {
		return runCommand("", os.Stdout, os.Stderr, name, args...)
	}
	commandArgs := append([]string{"-u", e.SudoUser(), "-H", "--", name}, args...)
	return runCommand("", os.Stdout, os.Stderr, "sudo", commandArgs...)
}

func (e *Env) ensureGeneratedConfigOwnedBySudoUser() {
	user := e.SudoUser()
	if user == "" {
		return
	}
	uidText := strings.TrimSpace(runOptional("", "id", "-u", user))
	gidText := strings.TrimSpace(runOptional("", "id", "-g", user))
	uid, err1 := strconv.Atoi(uidText)
	gid, err2 := strconv.Atoi(gidText)
	if err1 != nil || err2 != nil {
		return
	}
	for _, path := range e.generatedConfigPaths() {
		if fileExists(path) {
			_ = os.Chown(path, uid, gid)
		}
	}
}

func (e *Env) Install() error {
	if e.OS == "darwin" {
		return e.installDarwin()
	}
	return e.installLinux()
}

func (e *Env) installLinux() error {
	if err := e.RequireRoot("install"); err != nil {
		return err
	}
	logStep("Installing mihomo v%s", e.MihomoVersion)
	archive, err := e.downloadMihomo()
	if err != nil {
		return err
	}
	if err := e.installBinary(archive); err != nil {
		return err
	}
	if err := e.generateConfigArtifacts(configgen.GenerateOptions{EnableLinuxTUN: false, EnableMacOSTUN: true}); err != nil {
		return err
	}
	linuxProfile, err := e.autoSyncProfile("linux")
	if err != nil {
		return err
	}
	if err := e.updateLinuxConfigInternal(linuxProfile); err != nil {
		return err
	}
	e.SetupGeodata()
	if err := e.writeSystemdUnit(); err != nil {
		return err
	}
	logSuccess("Install done. Run: sudo mihctl service start")
	return nil
}

func (e *Env) installDarwin() error {
	method := e.MacInstallMethod
	if method == "brew" && !commandExists("brew") {
		logWarn("Homebrew not found; using direct download (needs sudo for %s)", e.InstallDir)
		method = "download"
	}
	if method == "brew" && runCommand("", nil, nil, "brew", "list", "mihomo") != nil {
		logStep("brew install mihomo (as current user)")
		if err := e.runAsSudoUser("brew", "install", "mihomo"); err != nil {
			return err
		}
		e.ConfigDir = defaultDarwinConfigDir()
		logInfo("Config directory: %s", e.ConfigDir)
	} else if method == "download" {
		if err := e.RequireRoot("install"); err != nil {
			return err
		}
		logStep("Installing mihomo v%s (direct download, Darwin)", e.MihomoVersion)
		archive, err := e.downloadMihomo()
		if err != nil {
			return err
		}
		if err := e.installBinary(archive); err != nil {
			return err
		}
	}
	if err := e.generateConfigArtifacts(configgen.GenerateOptions{EnableLinuxTUN: false, EnableMacOSTUN: true}); err != nil {
		return err
	}
	e.ensureGeneratedConfigOwnedBySudoUser()
	standaloneProfile, err := e.autoSyncProfile("standalone")
	if err != nil {
		return err
	}
	if err := e.syncConfigMacStandalone(standaloneProfile, false, syncProvidersWithInstall()); err != nil {
		return err
	}
	e.SetupGeodata()
	if err := e.RequireRoot("install"); err != nil {
		return err
	}
	if err := e.writeDarwinPlist(); err != nil {
		return err
	}
	if err := e.writeDarwinSudoers(); err != nil {
		return err
	}
	logSuccess("macOS install done.")
	logInfo("Start: mihctl service start")
	return nil
}

func (e *Env) Uninstall() error {
	if e.OS == "darwin" {
		if err := e.RequireRoot("uninstall"); err != nil {
			return err
		}
		if e.isDarwinServiceLoaded() {
			_ = e.stopDarwinService()
		}
		_ = e.stopBrewServices()
		_ = removeFilePrivileged(e.PlistPath)
		_ = removeFilePrivileged(e.SudoersPath)
		if commandExists("brew") && runCommand("", nil, nil, "brew", "list", "mihomo") == nil {
			_ = e.runAsSudoUser("brew", "uninstall", "mihomo")
		} else {
			_ = removeFilePrivileged(filepath.Join(e.InstallDir, "mihomo"))
		}
		logSuccess("Uninstalled. Config kept at %s", e.ConfigDir)
		return nil
	}

	if err := e.RequireRoot("uninstall"); err != nil {
		return err
	}
	_ = runCommand("", os.Stdout, os.Stderr, "systemctl", "stop", "mihomo")
	_ = runCommand("", os.Stdout, os.Stderr, "systemctl", "disable", "mihomo")
	_ = e.removeSystemdUnit()
	_ = removeFilePrivileged(filepath.Join(e.InstallDir, "mihomo"))
	logSuccess("Uninstalled. Config and UI kept at %s", e.ConfigDir)
	return nil
}

func (e *Env) Update() error {
	if e.OS == "darwin" {
		if err := e.RequireRoot("update"); err != nil {
			return err
		}
		if commandExists("brew") && runCommand("", nil, nil, "brew", "list", "mihomo") == nil {
			logStep("brew upgrade mihomo")
			if err := e.runAsSudoUser("brew", "upgrade", "mihomo"); err != nil {
				return err
			}
		} else {
			logStep("Updating mihomo binary to v%s (download)", e.MihomoVersion)
			archive, err := e.downloadMihomo()
			if err != nil {
				return err
			}
			if err := e.installBinary(archive); err != nil {
				return err
			}
		}
		if err := e.writeDarwinPlist(); err != nil {
			return err
		}
		if e.isDarwinServiceLoaded() {
			if err := e.restartDarwinService(); err != nil {
				return err
			}
			logSuccess("mihomo updated and restarted")
		} else {
			logSuccess("mihomo updated. Start: mihctl service start")
		}
		return nil
	}

	if err := e.RequireRoot("update"); err != nil {
		return err
	}
	logStep("Updating mihomo binary to v%s", e.MihomoVersion)
	archive, err := e.downloadMihomo()
	if err != nil {
		return err
	}
	if err := e.installBinary(archive); err != nil {
		return err
	}
	if e.isLinuxServiceActive("mihomo") {
		if err := runCommand("", os.Stdout, os.Stderr, "systemctl", "restart", "mihomo"); err != nil {
			return err
		}
		logSuccess("mihomo updated and restarted")
	} else {
		logSuccess("mihomo updated. Start with: sudo mihctl service start")
	}
	return nil
}
