package mihomo

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/phenix3443/mihomo-companion/internal/configgen"
)

func TestUpdateProvidersRemoteOnlyRefreshesProviderFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("proxies: []\n"))
	}))
	defer server.Close()

	repoRoot := t.TempDir()
	configDir := filepath.Join(repoRoot, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "values.yaml"), []byte(`
proxy-providers:
  demo:
    type: http
    url: `+server.URL+`/demo.yaml
    interval: 60
    path: ./providers/demo.yaml
`), 0o644); err != nil {
		t.Fatal(err)
	}

	env := &Env{
		RepoRoot:            repoRoot,
		ProvidersDir:        filepath.Join(repoRoot, "providers"),
		FetchConnectTimeout: time.Second,
		FetchMaxTime:        time.Second,
	}
	if err := env.UpdateProvidersRemote(); err != nil {
		t.Fatal(err)
	}

	providerPath := filepath.Join(repoRoot, "providers", "demo.yaml")
	content, err := os.ReadFile(providerPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "proxies: []\n" {
		t.Fatalf("provider content = %q", string(content))
	}
}

func TestUpdateProvidersRemoteWithoutExtraLocalState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("proxies: []\n"))
	}))
	defer server.Close()

	repoRoot := t.TempDir()
	configDir := filepath.Join(repoRoot, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "values.yaml"), []byte(`
proxy-providers:
  demo:
    type: http
    url: `+server.URL+`/demo.yaml
    interval: 60
    path: ./providers/demo.yaml
`), 0o644); err != nil {
		t.Fatal(err)
	}

	env := &Env{
		RepoRoot:            repoRoot,
		ProvidersDir:        filepath.Join(repoRoot, "providers"),
		FetchConnectTimeout: time.Second,
		FetchMaxTime:        time.Second,
	}
	if err := env.UpdateProvidersRemote(); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(filepath.Join(repoRoot, "providers"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "demo.yaml" {
		t.Fatalf("providers dir entries = %#v", entries)
	}
}

func TestUpdateProvidersRemoteFetchesProvidersConcurrently(t *testing.T) {
	var inFlight int32
	var maxInFlight int32
	release := make(chan struct{})
	var seen sync.Map
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt32(&inFlight, 1)
		for {
			previous := atomic.LoadInt32(&maxInFlight)
			if current <= previous || atomic.CompareAndSwapInt32(&maxInFlight, previous, current) {
				break
			}
		}
		seen.Store(r.URL.Path, true)
		<-release
		atomic.AddInt32(&inFlight, -1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("proxies: []\n"))
	}))
	defer server.Close()

	repoRoot := t.TempDir()
	configDir := filepath.Join(repoRoot, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "values.yaml"), []byte(`
proxy-providers:
  alpha:
    type: http
    url: `+server.URL+`/alpha.yaml
    interval: 60
    path: ./providers/alpha.yaml
  beta:
    type: http
    url: `+server.URL+`/beta.yaml
    interval: 60
    path: ./providers/beta.yaml
`), 0o644); err != nil {
		t.Fatal(err)
	}

	env := &Env{
		RepoRoot:            repoRoot,
		ProvidersDir:        filepath.Join(repoRoot, "providers"),
		FetchConnectTimeout: 2 * time.Second,
		FetchMaxTime:        2 * time.Second,
	}

	done := make(chan error, 1)
	go func() {
		done <- env.UpdateProvidersRemote()
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		alphaSeen, _ := seen.Load("/alpha.yaml")
		betaSeen, _ := seen.Load("/beta.yaml")
		if alphaSeen == true && betaSeen == true && atomic.LoadInt32(&maxInFlight) >= 2 {
			close(release)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"alpha", "beta"} {
				content, err := os.ReadFile(filepath.Join(repoRoot, "providers", name+".yaml"))
				if err != nil {
					t.Fatal(err)
				}
				if strings.TrimSpace(string(content)) != "proxies: []" {
					t.Fatalf("%s content = %q", name, string(content))
				}
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	close(release)
	err := <-done
	t.Fatalf("expected concurrent fetches, maxInFlight=%d, err=%v", atomic.LoadInt32(&maxInFlight), err)
}

func TestRefreshOfficialSupportLoadsConfigDrivenCatalog(t *testing.T) {
	repoRoot := t.TempDir()
	configDir := filepath.Join(repoRoot, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "values.yaml"), []byte(`
official-support:
  binance:
    source-url: https://example.com/binance
    supported: [HK, us]
  bitget:
    source-url: https://example.com/bitget
    prohibited: [sg, US]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	statePath := filepath.Join(t.TempDir(), "official-support.yaml")
	env := &Env{
		RepoRoot:            repoRoot,
		OfficialSupportPath: statePath,
	}
	if err := env.RefreshOfficialSupport(); err != nil {
		t.Fatal(err)
	}

	state, err := configgen.LoadOfficialSupportState(statePath)
	if err != nil {
		t.Fatal(err)
	}
	binance, ok := state.Services["binance"]
	if !ok {
		t.Fatal("missing binance state")
	}
	if got := strings.Join(binance.Supported, ","); got != "HK,US" {
		t.Fatalf("binance supported = %q", got)
	}
	bitget, ok := state.Services["bitget"]
	if !ok {
		t.Fatal("missing bitget state")
	}
	if got := strings.Join(bitget.Prohibited, ","); got != "SG,US" {
		t.Fatalf("bitget prohibited = %q", got)
	}
}
