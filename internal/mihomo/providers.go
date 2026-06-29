package mihomo

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/phenix3443/mihomo-companion/internal/configgen"
)

func loadGenerationConfigFromRepo(repoRoot string) (*configgen.GenerationConfig, error) {
	path := filepath.Join(repoRoot, "config", "values.yaml")
	cfg, err := configgen.LoadGenerationConfig(path)
	if err == nil {
		return cfg, nil
	}
	if os.IsNotExist(err) || strings.Contains(err.Error(), "read "+path) {
		return nil, fmt.Errorf("missing %s; copy config/values.example.yaml to config/values.yaml and fill in your provider URLs", path)
	}
	return nil, err
}

func providerFallbackURL(rawURL string) (string, bool) {
	const prefix = "https://cdn.jsdelivr.net/gh/"
	if !strings.HasPrefix(rawURL, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(rawURL, prefix)
	parts := strings.SplitN(rest, "@", 2)
	if len(parts) != 2 {
		return "", false
	}
	slash := strings.Index(parts[1], "/")
	if slash == -1 {
		return "", false
	}
	version := parts[1][:slash]
	path := parts[1][slash+1:]
	return "https://raw.githubusercontent.com/" + parts[0] + "/" + version + "/" + path, true
}

func originFromURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func (e *Env) SyncProvidersToLive() error {
	targetDir, err := e.detectLiveConfigDir()
	if err != nil {
		return err
	}
	targetDir = filepath.Join(targetDir, "providers")
	logStep("Syncing providers to %s", targetDir)
	if !dirExists(e.ProvidersDir) {
		return fmt.Errorf("providers source directory missing: %s", e.ProvidersDir)
	}
	if err := mkdirAllPrivileged(targetDir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(e.ProvidersDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		src := filepath.Join(e.ProvidersDir, entry.Name())
		dst := filepath.Join(targetDir, entry.Name())
		if fileExists(dst) {
			srcData, err1 := os.ReadFile(src)
			dstData, err2 := os.ReadFile(dst)
			if err1 == nil && err2 == nil && bytes.Equal(srcData, dstData) {
				continue
			}
		}
		if err := copyFilePrivileged(src, dst, 0o644); err != nil {
			return err
		}
		logInfo("Synced: %s", entry.Name())
	}
	logSuccess("Providers sync complete")
	return nil
}

func (e *Env) UpdateProvidersRemote() error {
	cfg, err := loadGenerationConfigFromRepo(e.RepoRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(e.ProvidersDir, 0o755); err != nil {
		return err
	}

	keys := append([]string(nil), cfg.ProviderOrder...)
	if len(keys) == 0 {
		for key := range cfg.ProxyProviders {
			keys = append(keys, key)
		}
		sort.Strings(keys)
	}

	logStep("Updating repository proxy-providers into %s", e.ProvidersDir)
	type providerFetchJob struct {
		name string
		spec configgen.ProxyProviderSpec
		dest string
	}
	type providerFetchResult struct {
		name    string
		dest    string
		content []byte
		err     error
	}

	jobs := make([]providerFetchJob, 0, len(keys))
	for _, name := range keys {
		spec, ok := cfg.ProxyProviders[name]
		if !ok {
			continue
		}
		if strings.TrimSpace(spec.URL) == "" || strings.TrimSpace(spec.Path) == "" {
			continue
		}
		dest := filepath.Join(e.RepoRoot, strings.TrimPrefix(spec.Path, "./"))
		logInfo("Provider %s -> %s", name, dest)
		jobs = append(jobs, providerFetchJob{name: name, spec: spec, dest: dest})
	}

	workerCount := runtime.NumCPU()
	if workerCount < 4 {
		workerCount = 4
	}
	if workerCount > 8 {
		workerCount = 8
	}
	if len(jobs) < workerCount {
		workerCount = len(jobs)
	}
	if workerCount < 1 {
		workerCount = 1
	}

	jobCh := make(chan providerFetchJob)
	resultCh := make(chan providerFetchResult, len(jobs))
	var wg sync.WaitGroup
	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobCh {
				userAgent := "clash-verge/v1.6.0"
				content, err := e.fetchRemoteBytesWithFallback(job.spec.URL, userAgent, originFromURL(job.spec.URL))
				resultCh <- providerFetchResult{
					name:    job.name,
					dest:    job.dest,
					content: content,
					err:     err,
				}
			}
		}()
	}
	for _, job := range jobs {
		jobCh <- job
	}
	close(jobCh)
	wg.Wait()
	close(resultCh)

	resultsByName := make(map[string]providerFetchResult, len(jobs))
	for result := range resultCh {
		resultsByName[result.name] = result
	}

	for _, job := range jobs {
		result := resultsByName[job.name]
		if result.err != nil {
			if fileExists(result.dest) {
				logWarn("Fetch failed; kept existing %s", filepath.Base(result.dest))
				continue
			}
			return fmt.Errorf("fetch provider %s: %w", result.name, result.err)
		}
		content := result.content
		previous, readErr := os.ReadFile(result.dest)
		if readErr == nil && bytes.Equal(previous, content) {
			logInfo("Unchanged %s", filepath.Base(result.dest))
			continue
		}
		if err := os.WriteFile(result.dest, content, 0o644); err != nil {
			return err
		}
		logSuccess("Updated %s", filepath.Base(result.dest))
	}
	logSuccess("Repository provider update finished")
	return nil
}

func (e *Env) RefreshOfficialSupport() error {
	logStep("Refreshing official support catalog")
	cfg, err := loadGenerationConfigFromRepo(e.RepoRoot)
	if err != nil {
		return err
	}
	state := configgen.BuildOfficialSupportStateFromConfig(cfg, time.Now().UTC())
	if err := configgen.SaveOfficialSupportState(e.OfficialSupportPath, state); err != nil {
		return err
	}
	logSuccess("Wrote %s", e.OfficialSupportPath)
	return nil
}

func (e *Env) fetchRemoteBytesWithFallback(rawURL, userAgent, origin string) ([]byte, error) {
	candidates := []string{rawURL}
	if fallback, ok := providerFallbackURL(rawURL); ok {
		candidates = append(candidates, fallback)
	}

	var lastErr error
	for index, candidate := range candidates {
		content, err := fetchBytes(candidate, userAgent, origin, e.FetchConnectTimeout, e.FetchMaxTime)
		if err == nil {
			return content, nil
		}
		lastErr = err
		if index == 0 && len(candidates) > 1 {
			logWarn("Fetch failed via primary URL, trying fallback")
		} else if candidate != rawURL {
			logWarn("Fetch failed via fallback URL: %s", candidate)
		}
	}
	return nil, lastErr
}

func (e *Env) fetchRemoteWithFallback(dest, rawURL, userAgent, origin string) error {
	content, err := e.fetchRemoteBytesWithFallback(rawURL, userAgent, origin)
	if err != nil {
		return err
	}
	return writeFilePrivileged(dest, content, 0o644)
}
