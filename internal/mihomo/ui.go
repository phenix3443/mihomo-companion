package mihomo

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/phenix3443/mihctl/internal/runtime"
)

func (e *Env) SetupUI() error {
	uiDir := filepath.Join(e.ConfigDir, "ui")
	gitDir := filepath.Join(uiDir, ".git")
	if dirExists(gitDir) {
		remote := strings.TrimSpace(runOptional("", "git", "-C", uiDir, "remote", "get-url", "origin"))
		indexHTML := filepath.Join(uiDir, "index.html")
		if !strings.Contains(remote, "MetaCubeX/metacubexd") || !fileExists(indexHTML) {
			logWarn("UI dir is invalid, recreating: %s", uiDir)
			if err := os.RemoveAll(uiDir); err != nil {
				return err
			}
		} else {
			logInfo("Updating metacubexd ui: %s", uiDir)
			if err := runCommand("", os.Stdout, os.Stderr, "git", "-C", uiDir, "fetch", "--depth", "1", "origin", "gh-pages"); err != nil {
				return err
			}
			if err := runCommand("", os.Stdout, os.Stderr, "git", "-C", uiDir, "reset", "--hard", "FETCH_HEAD"); err != nil {
				return err
			}
			logSuccess("UI updated: %s", uiDir)
			return nil
		}
	}

	if !dirExists(uiDir) {
		logInfo("Cloning metacubexd (gh-pages) to %s", uiDir)
		if err := runCommand("", os.Stdout, os.Stderr, "git", "clone", "--depth", "1", "--single-branch", "https://github.com/MetaCubeX/metacubexd.git", "-b", "gh-pages", uiDir); err != nil {
			return err
		}
		logSuccess("UI: %s", uiDir)
	} else {
		logInfo("UI dir already exists: %s", uiDir)
	}
	return nil
}

func (e *Env) PrintAccessURLs() {
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "Access URLs:")
	access, err := e.uiAccessInfo()
	if err != nil {
		fmt.Fprintln(os.Stdout, "  Local: unavailable (could not read live config)")
		fmt.Fprintln(os.Stdout)
		fmt.Fprintln(os.Stdout, "Proxy port: unavailable")
		return
	}
	if e.OS == "darwin" {
		fmt.Fprintf(os.Stdout, "  Local: http://%s:%d/ui\n", access.Host, access.ControllerPort)
		fmt.Fprintln(os.Stdout)
		fmt.Fprintf(os.Stdout, "Proxy port: %d (mixed-port)\n", access.MixedPort)
		return
	}
	if !access.Wildcard {
		fmt.Fprintf(os.Stdout, "  Local: http://%s:%d/ui\n", access.Host, access.ControllerPort)
		fmt.Fprintln(os.Stdout)
		fmt.Fprintf(os.Stdout, "Proxy port: %d (mixed-port)\n", access.MixedPort)
		return
	}
	output := runOptional("", "ip", "-4", "addr", "show")
	hasIP := false
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "inet ") || strings.Contains(line, "127.0.0.1") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		ip := strings.Split(fields[1], "/")[0]
		if ip == "" {
			continue
		}
		fmt.Fprintf(os.Stdout, "  Local: http://%s:%d/ui\n", ip, access.ControllerPort)
		hasIP = true
	}
	if !hasIP {
		fmt.Fprintf(os.Stdout, "  Local: http://%s:%d/ui\n", access.Host, access.ControllerPort)
	}
	fmt.Fprintln(os.Stdout)
	fmt.Fprintf(os.Stdout, "Proxy port: %d (mixed-port)\n", access.MixedPort)
}

type uiAccessInfo struct {
	Host           string
	ControllerPort int
	MixedPort      int
	Wildcard       bool
}

func (e *Env) uiAccessInfo() (uiAccessInfo, error) {
	configPath, err := e.detectLiveConfigFile()
	if err != nil {
		return uiAccessInfo{}, err
	}
	access, err := runtime.CaptureAccessInfoFromYAML(configPath)
	if err != nil {
		return uiAccessInfo{}, err
	}
	host, port, wildcard, err := parseControllerAddress(access.Controller)
	if err != nil {
		return uiAccessInfo{}, err
	}
	return uiAccessInfo{
		Host:           host,
		ControllerPort: port,
		MixedPort:      access.MixedPort,
		Wildcard:       wildcard,
	}, nil
}

func parseControllerAddress(controller string) (string, int, bool, error) {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(controller))
	if err != nil {
		return "", 0, false, err
	}
	port, err := strconv.Atoi(strings.TrimSpace(portText))
	if err != nil {
		return "", 0, false, err
	}
	wildcard := host == "" || host == "0.0.0.0" || host == "::" || host == "*"
	if wildcard || host == "localhost" || host == "::1" {
		host = "127.0.0.1"
	}
	return host, port, wildcard, nil
}
