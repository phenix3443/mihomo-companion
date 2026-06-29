package mihomo

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/phenix3443/mihctl/internal/configgen"
)

const (
	defaultMihomoVersion = "1.19.19"
	defaultGeoIPURL      = "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geoip.dat"
	defaultGeoSiteURL    = "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geosite.dat"
	defaultMMDBURL       = "https://cdn.jsdelivr.net/gh/Loyalsoldier/geoip@release/Country.mmdb"
	defaultFetchConnect  = 10 * time.Second
	defaultFetchMax      = 30 * time.Second
)

type Env struct {
	RepoRoot string
	OS       string
	HomeDir  string

	MihomoVersion string
	DownloadURL   string
	InstallDir    string
	ConfigDir     string
	ServiceUser   string
	GeoIPURL      string
	GeoSiteURL    string
	MMDBURL       string

	MacInstallMethod string
	MacTarget        string
	ClashVergeDir    string
	LogPath          string

	PlistLabel  string
	PlistPath   string
	SudoersPath string
	Launchctl   string

	TemplateServicePath string
	DefaultProfile      string
	RuntimeProfiles     []RuntimeProfile
	ProvidersDir        string
	OfficialSupportPath string

	FetchConnectTimeout time.Duration
	FetchMaxTime        time.Duration
}

func LoadEnv(repoRoot string) (*Env, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	runtimeProfiles, err := loadRuntimeProfiles(repoRoot)
	if err != nil {
		return nil, err
	}
	defaultProfile, err := loadDefaultProfile(repoRoot)
	if err != nil {
		return nil, err
	}

	env := &Env{
		RepoRoot:            repoRoot,
		OS:                  runtime.GOOS,
		HomeDir:             homeDir,
		MihomoVersion:       getenvDefault("MIHOMO_VERSION", defaultMihomoVersion),
		DownloadURL:         strings.TrimSpace(os.Getenv("MIHOMO_DOWNLOAD_URL")),
		ServiceUser:         getenvDefault("SERVICE_USER", "root"),
		GeoIPURL:            getenvDefault("GEOIP_URL", defaultGeoIPURL),
		GeoSiteURL:          getenvDefault("GEOSITE_URL", defaultGeoSiteURL),
		MMDBURL:             defaultMMDBURL,
		MacInstallMethod:    getenvDefault("MIHOMO_MAC_INSTALL", "brew"),
		MacTarget:           strings.TrimSpace(os.Getenv("MIHOMO_MAC_TARGET")),
		ClashVergeDir:       getenvDefault("CLASH_VERGE_DIR", filepath.Join(homeDir, "Library", "Application Support", "io.github.clash-verge-rev.clash-verge-rev", "profiles")),
		PlistLabel:          "com.metacubex.mihomo",
		PlistPath:           "/Library/LaunchDaemons/com.metacubex.mihomo.plist",
		SudoersPath:         "/etc/sudoers.d/mihomo",
		Launchctl:           "/bin/launchctl",
		TemplateServicePath: filepath.Join(repoRoot, "config", "mihomo.service.example"),
		DefaultProfile:      defaultProfile,
		RuntimeProfiles:     runtimeProfiles,
		ProvidersDir:        filepath.Join(repoRoot, "providers"),
		FetchConnectTimeout: durationFromEnv("MIHOMO_FETCH_CONNECT_TIMEOUT", defaultFetchConnect),
		FetchMaxTime:        durationFromEnv("MIHOMO_FETCH_MAX_TIME", defaultFetchMax),
	}
	officialSupportPath, err := configgen.DefaultOfficialSupportStatePath()
	if err != nil {
		return nil, err
	}
	env.OfficialSupportPath = officialSupportPath

	if env.OS == "darwin" {
		env.ConfigDir = getenvDefault("CONFIG_DIR", defaultDarwinConfigDir())
		env.InstallDir = getenvDefault("INSTALL_DIR", defaultDarwinInstallDir())
		if strings.TrimSpace(os.Getenv("MIHOMO_LOG_PATH")) != "" {
			env.LogPath = strings.TrimSpace(os.Getenv("MIHOMO_LOG_PATH"))
		} else if pathExists("/opt/homebrew/var") {
			env.LogPath = "/opt/homebrew/var/log/mihomo.log"
		} else {
			env.LogPath = "/usr/local/var/log/mihomo.log"
		}
	} else {
		env.ConfigDir = getenvDefault("CONFIG_DIR", "/etc/clash")
		env.InstallDir = getenvDefault("INSTALL_DIR", "/usr/local/bin")
	}

	return env, nil
}

func getenvDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func durationFromEnv(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func defaultDarwinInstallDir() string {
	if pathExists("/opt/homebrew/bin") {
		return "/opt/homebrew/bin"
	}
	return "/usr/local/bin"
}

func defaultDarwinConfigDir() string {
	if brew, err := exec.LookPath("brew"); err == nil {
		command := exec.Command(brew, "--prefix")
		output, err := command.Output()
		if err == nil {
			prefix := strings.TrimSpace(string(output))
			if prefix != "" {
				return filepath.Join(prefix, "etc", "mihomo")
			}
		}
	}
	configHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if configHome == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			configHome = filepath.Join(homeDir, ".config")
		}
	}
	if configHome == "" {
		configHome = ".config"
	}
	return filepath.Join(configHome, "mihomo")
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (e *Env) ValidateSupportedOS() error {
	switch e.OS {
	case "darwin", "linux":
		return nil
	default:
		return fmt.Errorf("unsupported os: %s", e.OS)
	}
}

func (e *Env) IsRoot() bool {
	return os.Geteuid() == 0
}

func (e *Env) RequireRoot(action string) error {
	if e.IsRoot() {
		return nil
	}
	command := action
	switch action {
	case "start", "stop", "restart", "status":
		command = "service " + action
	case "sync-config":
		command = "config sync"
	}
	return fmt.Errorf("need root. run: sudo mihctl %s", command)
}

func (e *Env) SudoUser() string {
	return strings.TrimSpace(os.Getenv("SUDO_USER"))
}

func (e *Env) ClashVergeDataDir() string {
	return filepath.Dir(e.ClashVergeDir)
}

func (e *Env) ClashVergeProfilesYAML() string {
	return filepath.Join(e.ClashVergeDataDir(), "profiles.yaml")
}
