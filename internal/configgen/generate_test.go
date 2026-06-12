package configgen

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestLoadGenerationConfigPreservesOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "values.yaml")
	if err := os.WriteFile(path, []byte(`
probe:
  services:
    latency:
      uri: https://connectivitycheck.gstatic.com/generate_204
proxy-providers:
  zzz:
    type: http
    url: https://example.com/zzz
    interval: 60
    path: ./providers/zzz.yaml
  aaa:
    type: http
    url: https://example.com/aaa
    interval: 60
    path: ./providers/aaa.yaml
service-groups:
  second:
    probe: latency
    type: url-test
    interval: 60
  first:
    probe: latency
    type: url-test
    interval: 60
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadGenerationConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	assertStringSlice(t, cfg.ProviderOrder, []string{"zzz", "aaa"})
	assertStringSlice(t, cfg.GroupOrder, []string{"second", "first"})
}

func TestLoadGenerationConfigLoadsExternalUI(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "values.yaml")
	if err := os.WriteFile(path, []byte(`
template:
  external-ui:
    linux: /custom/ui
probe:
  services:
    latency:
      uri: https://connectivitycheck.gstatic.com/generate_204
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadGenerationConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Template.ExternalUI["linux"] != "/custom/ui" {
		t.Fatalf("linux external-ui = %q, want /custom/ui", cfg.Template.ExternalUI["linux"])
	}
	if cfg.Probe.Services["latency"].URI != "https://connectivitycheck.gstatic.com/generate_204" {
		t.Fatalf("probe service uri = %q", cfg.Probe.Services["latency"].URI)
	}
}

func TestLoadProviderCatalogParsesRawSubscriptions(t *testing.T) {
	dir := t.TempDir()
	bywavePath := filepath.Join(dir, "providers", "bywave.yaml")
	jisuPath := filepath.Join(dir, "providers", "jisu.yaml")
	if err := os.MkdirAll(filepath.Dir(bywavePath), 0o755); err != nil {
		t.Fatal(err)
	}

	ssLine := "ss://YWVzLTEyOC1nY206c2VjcmV0@example.com:443/?plugin=obfs-local%3Bobfs%3Dhttp%3Bobfs-host%3Dcdn.example.com#HK%2001"
	vlessLine := "vless://123e4567-e89b-12d3-a456-426614174000@v.example.com:443?type=ws&security=reality&pbk=public-key&sid=abcd&sni=www.example.com&host=cdn.example.com&path=%2Fws#US%2001"
	hy2Line := "hysteria2://password@hy.example.com:8443?insecure=1&sni=www.example.com&mport=20000-30000#SG%2001"

	if err := os.WriteFile(bywavePath, []byte(base64.StdEncoding.EncodeToString([]byte(ssLine+"\n"))), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jisuPath, []byte(base64.StdEncoding.EncodeToString([]byte(vlessLine+"\n"+hy2Line+"\n"))), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog, err := LoadProviderCatalog(dir, map[string]ProxyProviderSpec{
		"bywave": {Path: "./providers/bywave.yaml"},
		"jisu":   {Path: "./providers/jisu.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	bywave := catalog.Providers["bywave"]
	if len(bywave.Proxies) != 1 {
		t.Fatalf("bywave proxy count = %d, want 1", len(bywave.Proxies))
	}
	if got := stringValue(bywave.Proxies[0].Config["type"]); got != "ss" {
		t.Fatalf("bywave type = %q", got)
	}
	if got := stringValue(bywave.Proxies[0].Config["name"]); got != "HK 01" {
		t.Fatalf("bywave name = %q", got)
	}
	if got := stringValue(bywave.Proxies[0].Config["plugin"]); got != "obfs" {
		t.Fatalf("bywave plugin = %q, want obfs", got)
	}

	jisu := catalog.Providers["jisu"]
	if len(jisu.Proxies) != 2 {
		t.Fatalf("jisu proxy count = %d, want 2", len(jisu.Proxies))
	}
	if got := stringValue(jisu.Proxies[0].Config["type"]); got != "vless" {
		t.Fatalf("jisu first type = %q", got)
	}
	if got := stringValue(jisu.Proxies[1].Config["type"]); got != "hysteria2" {
		t.Fatalf("jisu second type = %q", got)
	}
}

func TestGenerateUsesFreshProbeStateAndExpandsProxies(t *testing.T) {
	repoRoot := t.TempDir()
	configDir := filepath.Join(repoRoot, "config")
	providersDir := filepath.Join(repoRoot, "providers")
	if err := os.MkdirAll(filepath.Join(configDir, "linux"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(configDir, "macos"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(providersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	template := `proxies:
{{ toYAML .Proxies | indent 2 }}
proxy-groups:
{{ toYAML .ProxyGroups | indent 2 }}
proxy-providers:
{{ toYAML .ProxyProviders | indent 2 }}
rule-providers: {}
rules: []
`
	values := `
profiles:
  k3s:
    os: linux
  local:
    os: macos
manual-proxies:
  - name: feilian-proxy
    type: socks5
    server: 192.168.3.104
    port: 1090
probe:
  services:
    openai:
      uri: https://openai.com/
proxy-providers:
  bywave:
    type: http
    url: https://example.com/sub
    interval: 60
    path: ./providers/bywave.yaml
service-groups:
  openai:
    probe: openai
    type: url-test
    interval: 300
    tolerance: 50
    lazy: true
    profiles:
      k3s:
        providers: [bywave]
      local:
        providers: [bywave]
`
	providerYAML := `proxies:
  - name: proxy-a
    type: ss
    server: 1.1.1.1
    port: 443
    cipher: aes-128-gcm
    password: secret-a
  - name: proxy-b
    type: ss
    server: 2.2.2.2
    port: 443
    cipher: aes-128-gcm
    password: secret-b
`
	if err := os.WriteFile(filepath.Join(configDir, "mihomo.yaml.tmpl"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "values.yaml"), []byte(values), 0o644); err != nil {
		t.Fatal(err)
	}
	providerPath := filepath.Join(providersDir, "bywave.yaml")
	if err := os.WriteFile(providerPath, []byte(providerYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog, err := LoadProviderCatalog(repoRoot, map[string]ProxyProviderSpec{
		"bywave": {Path: "./providers/bywave.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := catalog.Providers["bywave"].Digest
	openaiDigest := probeServiceDigest(ProbeServiceSpec{URI: "https://openai.com/"})

	probeStatePath := filepath.Join(t.TempDir(), "probe-results.yaml")
	t.Setenv("MIHOMO_PROBE_STATE_PATH", probeStatePath)
	state := &ProbeState{
		Providers: map[string]ProviderProbeState{
			"bywave": {
				Provider:           "bywave",
				SubscriptionDigest: digest,
				Nodes: map[string]NodeProbeState{
					"proxy-a": {
						NodeName: "proxy-a",
						Services: map[string]ServiceProbeState{
							"openai": {
								OK:          true,
								ProbeDigest: openaiDigest,
								ProbedAt:    time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
							},
						},
					},
					"proxy-b": {
						NodeName: "proxy-b",
						Services: map[string]ServiceProbeState{
							"openai": {
								OK:          true,
								ProbeDigest: "stale-digest",
								ProbedAt:    time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339),
							},
						},
					},
				},
			},
		},
	}
	populateFreshGroupProbeStateForTests(t, repoRoot, filepath.Join(configDir, "values.yaml"), state)
	if err := SaveProbeState(probeStatePath, state); err != nil {
		t.Fatal(err)
	}

	service := newTestService(repoRoot, configDir)
	if _, err := service.Generate(GenerateOptions{}); err != nil {
		t.Fatal(err)
	}

	generated, err := LoadConfig(profileConfigPath(configDir, "k3s"))
	if err != nil {
		t.Fatal(err)
	}
	proxies, ok := generated["proxies"].([]any)
	if !ok {
		t.Fatalf("proxies type = %T", generated["proxies"])
	}
	if len(proxies) != 2 {
		t.Fatalf("proxies len = %d, want 2", len(proxies))
	}

	k3sGenerated, err := LoadConfig(profileConfigPath(configDir, "k3s"))
	if err != nil {
		t.Fatal(err)
	}
	if len(k3sGenerated) == 0 {
		t.Fatal("k3s generated config is empty")
	}

	groupsValue, ok := generated["proxy-groups"].([]any)
	if !ok || len(groupsValue) != 1 {
		t.Fatalf("proxy-groups = %#v", generated["proxy-groups"])
	}
	group, ok := asMap(groupsValue[0])
	if !ok {
		t.Fatal("group is not a map")
	}
	if _, exists := group["use"]; exists {
		t.Fatal("generated group unexpectedly contains use")
	}
	if got, _ := group["url"].(string); got != "https://openai.com/" {
		t.Fatalf("group url = %q, want https://openai.com/", got)
	}
	proxyNames, ok := group["proxies"].([]any)
	if !ok || len(proxyNames) != 1 {
		t.Fatalf("group proxies = %#v", group["proxies"])
	}
	if got, _ := proxyNames[0].(string); got != "proxy-a" {
		t.Fatalf("group proxies[0] = %q, want proxy-a", got)
	}
}

func TestGenerateReportsStaleProbeDigestInError(t *testing.T) {
	repoRoot := t.TempDir()
	configDir := filepath.Join(repoRoot, "config")
	providersDir := filepath.Join(repoRoot, "providers")
	if err := os.MkdirAll(filepath.Join(configDir, "linux"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(providersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	template := `proxies:
{{ toYAML .Proxies | indent 2 }}
proxy-groups:
{{ toYAML .ProxyGroups | indent 2 }}
proxy-providers:
{{ toYAML .ProxyProviders | indent 2 }}
rule-providers: {}
rules: []
`
	values := `
profiles:
  k3s:
    os: linux
probe:
  services:
    latency:
      uri: https://connectivitycheck.gstatic.com/generate_204
    openai:
      uri: https://api.openai.com/v1/models
proxy-providers:
  bywave:
    type: http
    url: https://example.com/sub
    interval: 60
    path: ./providers/bywave.yaml
service-groups:
  auto:
    probe: latency
    type: url-test
    interval: 300
    tolerance: 50
    lazy: true
    profiles:
      k3s:
        providers: [bywave]
`
	providerYAML := `proxies:
  - name: proxy-a
    type: ss
    server: 1.1.1.1
    port: 443
    cipher: aes-128-gcm
    password: secret-a
`
	if err := os.WriteFile(filepath.Join(configDir, "mihomo.yaml.tmpl"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "values.yaml"), []byte(values), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(providersDir, "bywave.yaml"), []byte(providerYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog, err := LoadProviderCatalog(repoRoot, map[string]ProxyProviderSpec{
		"bywave": {Path: "./providers/bywave.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	probeStatePath := filepath.Join(t.TempDir(), "probe-results.yaml")
	t.Setenv("MIHOMO_PROBE_STATE_PATH", probeStatePath)
	now := time.Now().UTC()
	state := &ProbeState{
		Providers: map[string]ProviderProbeState{
			"bywave": {
				Provider:           "bywave",
				SubscriptionDigest: catalog.Providers["bywave"].Digest,
				Nodes: map[string]NodeProbeState{
					"proxy-a": {
						NodeName: "proxy-a",
						Services: map[string]ServiceProbeState{
							"latency": {
								OK:          true,
								ProbeDigest: "old-probe-digest",
								ProbedAt:    now.Add(-time.Hour).Format(time.RFC3339),
							},
						},
					},
				},
			},
		},
	}
	if err := SaveProbeState(probeStatePath, state); err != nil {
		t.Fatal(err)
	}

	service := newTestService(repoRoot, configDir)
	_, err = service.Generate(GenerateOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	message := err.Error()
	if !strings.Contains(message, "stale probe digest=1") {
		t.Fatalf("error = %q, want stale probe digest detail", message)
	}
	if !strings.Contains(message, "bywave: matched=1") {
		t.Fatalf("error = %q, want provider match detail", message)
	}
}

func TestGenerateReportsMissingServiceProbeResultInError(t *testing.T) {
	repoRoot := t.TempDir()
	configDir := filepath.Join(repoRoot, "config")
	providersDir := filepath.Join(repoRoot, "providers")
	if err := os.MkdirAll(filepath.Join(configDir, "linux"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(providersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	template := `proxies:
{{ toYAML .Proxies | indent 2 }}
proxy-groups:
{{ toYAML .ProxyGroups | indent 2 }}
proxy-providers:
{{ toYAML .ProxyProviders | indent 2 }}
rule-providers: {}
rules: []
`
	values := `
profiles:
  k3s:
    os: linux
probe:
  services:
    github:
      uri: ssh://github.com:22
      url-test: https://github.com/
proxy-providers:
  bywave:
    type: http
    url: https://example.com/sub
    interval: 60
    path: ./providers/bywave.yaml
service-groups:
  github:
    probe: github
    type: url-test
    interval: 300
    tolerance: 50
    lazy: true
    profiles:
      k3s:
        providers: [bywave]
`
	providerYAML := `proxies:
  - name: proxy-a
    type: ss
    server: 1.1.1.1
    port: 443
    cipher: aes-128-gcm
    password: secret-a
`
	if err := os.WriteFile(filepath.Join(configDir, "mihomo.yaml.tmpl"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "values.yaml"), []byte(values), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(providersDir, "bywave.yaml"), []byte(providerYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog, err := LoadProviderCatalog(repoRoot, map[string]ProxyProviderSpec{
		"bywave": {Path: "./providers/bywave.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}
	latencyDigest := probeServiceDigest(ProbeServiceSpec{URI: "https://connectivitycheck.gstatic.com/generate_204"})

	probeStatePath := filepath.Join(t.TempDir(), "probe-results.yaml")
	t.Setenv("MIHOMO_PROBE_STATE_PATH", probeStatePath)
	now := time.Now().UTC()
	state := &ProbeState{
		Providers: map[string]ProviderProbeState{
			"bywave": {
				Provider:           "bywave",
				SubscriptionDigest: catalog.Providers["bywave"].Digest,
				Nodes: map[string]NodeProbeState{
					"proxy-a": {
						NodeName: "proxy-a",
						Services: map[string]ServiceProbeState{
							"latency": {
								OK:          true,
								ProbeDigest: latencyDigest,
								ProbedAt:    now.Add(-time.Minute).Format(time.RFC3339),
							},
						},
					},
				},
			},
		},
	}
	if err := SaveProbeState(probeStatePath, state); err != nil {
		t.Fatal(err)
	}

	service := newTestService(repoRoot, configDir)
	_, err = service.Generate(GenerateOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	message := err.Error()
	if !strings.Contains(message, "missing service probe result=1") {
		t.Fatalf("error = %q, want missing service probe result detail", message)
	}
}

func TestGenerateAllowsSameProxyAcrossMultipleGroups(t *testing.T) {
	repoRoot := t.TempDir()
	configDir := filepath.Join(repoRoot, "config")
	providersDir := filepath.Join(repoRoot, "providers")
	if err := os.MkdirAll(filepath.Join(configDir, "linux"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(configDir, "macos"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(providersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	template := `proxies:
{{ toYAML .Proxies | indent 2 }}
proxy-groups:
{{ toYAML .ProxyGroups | indent 2 }}
proxy-providers:
{{ toYAML .ProxyProviders | indent 2 }}
rule-providers: {}
rules: []
`
	values := `
profiles:
  k3s:
    os: linux
  local:
    os: macos
probe:
  services:
    latency:
      uri: https://connectivitycheck.gstatic.com/generate_204
proxy-providers:
  bywave:
    type: http
    url: https://example.com/sub
    interval: 60
    path: ./providers/bywave.yaml
service-groups:
  auto:
    probe: latency
    type: url-test
    interval: 300
    tolerance: 50
    lazy: true
    profiles:
      k3s:
        providers: [bywave]
      local:
        providers: [bywave]
  stable:
    probe: latency
    type: url-test
    interval: 300
    tolerance: 50
    lazy: true
    profiles:
      k3s:
        providers: [bywave]
      local:
        providers: [bywave]
`
	providerYAML := `proxies:
  - name: proxy-a
    type: ss
    server: 1.1.1.1
    port: 443
    cipher: aes-128-gcm
    password: secret-a
`
	if err := os.WriteFile(filepath.Join(configDir, "mihomo.yaml.tmpl"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "values.yaml"), []byte(values), 0o644); err != nil {
		t.Fatal(err)
	}
	providerPath := filepath.Join(providersDir, "bywave.yaml")
	if err := os.WriteFile(providerPath, []byte(providerYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog, err := LoadProviderCatalog(repoRoot, map[string]ProxyProviderSpec{
		"bywave": {Path: "./providers/bywave.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := catalog.Providers["bywave"].Digest
	latencyDigest := probeServiceDigest(ProbeServiceSpec{URI: "https://connectivitycheck.gstatic.com/generate_204"})

	probeStatePath := filepath.Join(t.TempDir(), "probe-results.yaml")
	t.Setenv("MIHOMO_PROBE_STATE_PATH", probeStatePath)
	now := time.Now().UTC()
	state := &ProbeState{
		Providers: map[string]ProviderProbeState{
			"bywave": {
				Provider:           "bywave",
				SubscriptionDigest: digest,
				Nodes: map[string]NodeProbeState{
					"proxy-a": {
						NodeName: "proxy-a",
						Services: map[string]ServiceProbeState{
							"latency": {
								OK:          true,
								ProbeDigest: latencyDigest,
								ProbedAt:    now.Add(-time.Minute).Format(time.RFC3339),
							},
						},
					},
				},
			},
		},
	}
	populateFreshGroupProbeStateForTests(t, repoRoot, filepath.Join(configDir, "values.yaml"), state)
	if err := SaveProbeState(probeStatePath, state); err != nil {
		t.Fatal(err)
	}

	service := newTestService(repoRoot, configDir)
	if _, err := service.Generate(GenerateOptions{}); err != nil {
		t.Fatal(err)
	}

	generated, err := LoadConfig(profileConfigPath(configDir, "k3s"))
	if err != nil {
		t.Fatal(err)
	}
	groupsValue, ok := generated["proxy-groups"].([]any)
	if !ok || len(groupsValue) != 2 {
		t.Fatalf("proxy-groups = %#v", generated["proxy-groups"])
	}
	for _, item := range groupsValue {
		group, ok := asMap(item)
		if !ok {
			t.Fatal("group is not a map")
		}
		proxyNames, ok := group["proxies"].([]any)
		if !ok || len(proxyNames) != 1 {
			t.Fatalf("group proxies = %#v", group["proxies"])
		}
		if got, _ := proxyNames[0].(string); got != "proxy-a" {
			t.Fatalf("group proxies[0] = %q, want proxy-a", got)
		}
	}
}

func TestGenerateGitHubGroupUsesURLTestAndIncludesBywaveAndTag(t *testing.T) {
	repoRoot := t.TempDir()
	configDir := filepath.Join(repoRoot, "config")
	providersDir := filepath.Join(repoRoot, "providers")
	if err := os.MkdirAll(filepath.Join(configDir, "local"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(providersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	template := `proxies:
{{ toYAML .Proxies | indent 2 }}
proxy-groups:
{{ toYAML .ProxyGroups | indent 2 }}
proxy-providers:
{{ toYAML .ProxyProviders | indent 2 }}
rule-providers: {}
rules: []
`
	values := `
profiles:
  local:
    os: macos
probe:
  services:
    github:
      uri: ssh://github.com:22
      url-test: https://github.com/
proxy-providers:
  bywave:
    type: http
    url: https://example.com/bywave
    interval: 60
    path: ./providers/bywave.yaml
  tag:
    type: http
    url: https://example.com/tag
    interval: 60
    path: ./providers/tag.yaml
service-groups:
  github:
    probe: github
    type: url-test
    interval: 300
    tolerance: 50
    lazy: true
    profiles:
      local:
        providers: [bywave, tag]
        provider-match:
          tag:
            - "(香港|香港家宽|澳门|新加坡|日本|台湾|HK|MO|SG|JP|TW)"
        provider-exclude:
          tag:
            - "(美国|英国|德国|法国|印度|韩国|土耳其|US|UK|DE|FR|IN|KR|TU)"
`
	bywaveYAML := `proxies:
  - name: bywave-ssh
    type: ss
    server: 1.1.1.1
    port: 443
    cipher: aes-128-gcm
    password: secret-a
`
	tagYAML := `proxies:
  - name: 🇯🇵 日本 01丨1x JP
    type: ss
    server: 2.2.2.2
    port: 443
    cipher: aes-128-gcm
    password: secret-b
  - name: 🇺🇸 美国-纽约 01丨1x US
    type: ss
    server: 3.3.3.3
    port: 443
    cipher: aes-128-gcm
    password: secret-c
`
	if err := os.WriteFile(filepath.Join(configDir, "mihomo.yaml.tmpl"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "values.yaml"), []byte(values), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(providersDir, "bywave.yaml"), []byte(bywaveYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(providersDir, "tag.yaml"), []byte(tagYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog, err := LoadProviderCatalog(repoRoot, map[string]ProxyProviderSpec{
		"bywave": {Path: "./providers/bywave.yaml"},
		"tag":    {Path: "./providers/tag.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}
	githubDigest := probeServiceDigest(ProbeServiceSpec{
		URI:     "ssh://github.com:22",
		URLTest: "https://github.com/",
	})

	probeStatePath := filepath.Join(t.TempDir(), "probe-results.yaml")
	t.Setenv("MIHOMO_PROBE_STATE_PATH", probeStatePath)
	now := time.Now().UTC()
	state := &ProbeState{
		Providers: map[string]ProviderProbeState{
			"bywave": {
				Provider:           "bywave",
				SubscriptionDigest: catalog.Providers["bywave"].Digest,
				Nodes: map[string]NodeProbeState{
					"bywave-ssh": {
						NodeName: "bywave-ssh",
						Services: map[string]ServiceProbeState{
							"github": {
								OK:            true,
								LatencyMillis: 120,
								ProbeDigest:   githubDigest,
								ProbedAt:      now.Format(time.RFC3339),
							},
						},
					},
				},
			},
			"tag": {
				Provider:           "tag",
				SubscriptionDigest: catalog.Providers["tag"].Digest,
				Nodes: map[string]NodeProbeState{
					"🇯🇵 日本 01丨1x JP": {
						NodeName: "🇯🇵 日本 01丨1x JP",
						Services: map[string]ServiceProbeState{
							"github": {
								OK:            true,
								LatencyMillis: 150,
								ProbeDigest:   githubDigest,
								ProbedAt:      now.Format(time.RFC3339),
							},
						},
					},
					"🇺🇸 美国-纽约 01丨1x US": {
						NodeName: "🇺🇸 美国-纽约 01丨1x US",
						Services: map[string]ServiceProbeState{
							"github": {
								OK:            true,
								LatencyMillis: 180,
								ProbeDigest:   githubDigest,
								ProbedAt:      now.Format(time.RFC3339),
							},
						},
					},
				},
			},
		},
	}
	populateFreshGroupProbeStateForTests(t, repoRoot, filepath.Join(configDir, "values.yaml"), state)
	if err := SaveProbeState(probeStatePath, state); err != nil {
		t.Fatal(err)
	}

	service := newTestService(repoRoot, configDir)
	if _, err := service.Generate(GenerateOptions{}); err != nil {
		t.Fatal(err)
	}

	generated, err := LoadConfig(profileConfigPath(configDir, "local"))
	if err != nil {
		t.Fatal(err)
	}
	groups, ok := generated["proxy-groups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("proxy-groups = %#v", generated["proxy-groups"])
	}
	group, ok := asMap(groups[0])
	if !ok {
		t.Fatalf("group type = %T", groups[0])
	}
	if got := group["type"]; got != "url-test" {
		t.Fatalf("group type = %#v, want url-test", got)
	}
	if got := group["url"]; got != "https://github.com/" {
		t.Fatalf("group url = %#v, want https://github.com/", got)
	}
	assertStringSlice(t, []string{group["proxies"].([]any)[0].(string), group["proxies"].([]any)[1].(string)}, []string{"bywave-ssh", "🇯🇵 日本 01丨1x JP"})
}

func TestGenerateDisablesTunForClashVergeProfile(t *testing.T) {
	originalDarwinExternalUIDirFunc := darwinExternalUIDirFunc
	t.Cleanup(func() {
		darwinExternalUIDirFunc = originalDarwinExternalUIDirFunc
	})
	darwinExternalUIDirFunc = func() string {
		return "/mock/mihomo/ui"
	}

	repoRoot := t.TempDir()
	configDir := filepath.Join(repoRoot, "config")
	providersDir := filepath.Join(repoRoot, "providers")
	if err := os.MkdirAll(filepath.Join(configDir, "linux"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(configDir, "macos"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(configDir, "clash-verge"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(providersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	template := `tun:
{{ toYAML .Tun | indent 2 }}
{{- if .ExternalUI }}
external-ui: {{ .ExternalUI }}
{{- end }}
proxies:
{{ toYAML .Proxies | indent 2 }}
proxy-groups:
{{ toYAML .ProxyGroups | indent 2 }}
proxy-providers:
{{ toYAML .ProxyProviders | indent 2 }}
rule-providers: {}
rules: []
`
	values := `
profiles:
  local:
    os: macos
  clash-verge:
    os: macos
tun:
  macos:
    enable: true
    stack: gvisor
    dns-hijack:
      - any:53
    auto-route: true
    auto-detect-interface: true
    strict-route: false
    mtu: 1500
    exclude-interface: []
probe:
  services:
    latency:
      uri: https://connectivitycheck.gstatic.com/generate_204
proxy-providers:
  bywave:
    type: http
    url: https://example.com/sub
    interval: 60
    path: ./providers/bywave.yaml
service-groups:
  auto:
    probe: latency
    type: url-test
    interval: 300
    tolerance: 50
    lazy: true
    profiles:
      local:
        providers: [bywave]
      clash-verge:
        providers: [bywave]
`
	providerYAML := `proxies:
  - name: proxy-a
    type: ss
    server: 1.1.1.1
    port: 443
    cipher: aes-128-gcm
    password: secret-a
`
	if err := os.WriteFile(filepath.Join(configDir, "mihomo.yaml.tmpl"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "values.yaml"), []byte(values), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(providersDir, "bywave.yaml"), []byte(providerYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog, err := LoadProviderCatalog(repoRoot, map[string]ProxyProviderSpec{
		"bywave": {Path: "./providers/bywave.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := catalog.Providers["bywave"].Digest
	latencyDigest := probeServiceDigest(ProbeServiceSpec{URI: "https://connectivitycheck.gstatic.com/generate_204"})

	probeStatePath := filepath.Join(t.TempDir(), "probe-results.yaml")
	t.Setenv("MIHOMO_PROBE_STATE_PATH", probeStatePath)
	now := time.Now().UTC()
	state := &ProbeState{
		Providers: map[string]ProviderProbeState{
			"bywave": {
				Provider:           "bywave",
				SubscriptionDigest: digest,
				Nodes: map[string]NodeProbeState{
					"proxy-a": {
						NodeName: "proxy-a",
						Services: map[string]ServiceProbeState{
							"latency": {
								OK:          true,
								ProbeDigest: latencyDigest,
								ProbedAt:    now.Add(-time.Minute).Format(time.RFC3339),
							},
						},
					},
				},
			},
		},
	}
	populateFreshGroupProbeStateForTests(t, repoRoot, filepath.Join(configDir, "values.yaml"), state)
	if err := SaveProbeState(probeStatePath, state); err != nil {
		t.Fatal(err)
	}

	service := newTestService(repoRoot, configDir)
	if _, err := service.Generate(GenerateOptions{EnableMacOSTUN: true}); err != nil {
		t.Fatal(err)
	}

	localConfig, err := LoadConfig(profileConfigPath(configDir, "local"))
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := localConfig["tun"]; !exists {
		t.Fatal("local profile should keep tun when macOS tun is enabled")
	}
	if got := localConfig["external-ui"]; got != "/mock/mihomo/ui" {
		t.Fatalf("local external-ui = %#v, want /mock/mihomo/ui", got)
	}

	vergeConfig, err := LoadConfig(profileConfigPath(configDir, "clash-verge"))
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := vergeConfig["external-ui"]; exists {
		t.Fatalf("clash-verge external-ui should be omitted, got %#v", vergeConfig["external-ui"])
	}
	if value, exists := vergeConfig["tun"]; exists && value != nil {
		tunConfig, ok := asMap(value)
		if !ok {
			t.Fatalf("clash-verge tun type = %T", value)
		}
		if enabled, ok := tunConfig["enable"].(bool); ok && enabled {
			t.Fatalf("clash-verge profile should not enable tun, got %#v", tunConfig)
		}
	}
}

func TestGenerateSkipsServiceGroupOutsideProfileScope(t *testing.T) {
	repoRoot := t.TempDir()
	configDir := filepath.Join(repoRoot, "config")
	providersDir := filepath.Join(repoRoot, "providers")
	if err := os.MkdirAll(filepath.Join(configDir, "linux"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(configDir, "macos"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(providersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	template := `proxy-groups:
{{ toYAML .ProxyGroups | indent 2 }}
proxies:
{{ toYAML .Proxies | indent 2 }}
proxy-providers:
{{ toYAML .ProxyProviders | indent 2 }}
rule-providers: {}
rules: []
`
	values := `
profiles:
  k3s:
    os: linux
  local:
    os: macos
probe:
  services:
    latency:
      uri: https://connectivitycheck.gstatic.com/generate_204
proxy-providers:
  jisu:
    type: http
    url: https://example.com/sub
    interval: 60
    path: ./providers/jisu.yaml
service-groups:
  auto:
    probe: latency
    type: url-test
    interval: 300
    tolerance: 50
    lazy: true
    profiles:
      k3s:
        providers: [jisu]
  personal-only:
    probe: latency
    type: url-test
    interval: 300
    tolerance: 50
    lazy: true
    profiles:
      local:
        providers: [jisu]
    match: ["personal-only"]
`
	providerYAML := `proxies:
  - name: proxy-a personal-only
    type: hysteria2
    server: 1.1.1.1
    port: 443
    password: secret-a
`
	if err := os.WriteFile(filepath.Join(configDir, "mihomo.yaml.tmpl"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "values.yaml"), []byte(values), 0o644); err != nil {
		t.Fatal(err)
	}
	providerPath := filepath.Join(providersDir, "jisu.yaml")
	if err := os.WriteFile(providerPath, []byte(providerYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog, err := LoadProviderCatalog(repoRoot, map[string]ProxyProviderSpec{
		"jisu": {Path: "./providers/jisu.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := catalog.Providers["jisu"].Digest
	latencyDigest := probeServiceDigest(ProbeServiceSpec{URI: "https://connectivitycheck.gstatic.com/generate_204"})

	probeStatePath := filepath.Join(t.TempDir(), "probe-results.yaml")
	t.Setenv("MIHOMO_PROBE_STATE_PATH", probeStatePath)
	now := time.Now().UTC()
	state := &ProbeState{
		Providers: map[string]ProviderProbeState{
			"jisu": {
				Provider:           "jisu",
				SubscriptionDigest: digest,
				Nodes: map[string]NodeProbeState{
					"proxy-a personal-only": {
						NodeName: "proxy-a personal-only",
						Services: map[string]ServiceProbeState{
							"latency": {
								OK:          true,
								ProbeDigest: latencyDigest,
								ProbedAt:    now.Add(-time.Minute).Format(time.RFC3339),
							},
						},
					},
				},
			},
		},
	}
	populateFreshGroupProbeStateForTests(t, repoRoot, filepath.Join(configDir, "values.yaml"), state)
	if err := SaveProbeState(probeStatePath, state); err != nil {
		t.Fatal(err)
	}

	service := newTestService(repoRoot, configDir)
	if _, err := service.Generate(GenerateOptions{}); err != nil {
		t.Fatal(err)
	}

	generated, err := LoadConfig(profileConfigPath(configDir, "k3s"))
	if err != nil {
		t.Fatal(err)
	}
	groupsValue, ok := generated["proxy-groups"].([]any)
	if !ok {
		t.Fatalf("proxy-groups = %#v", generated["proxy-groups"])
	}
	if len(groupsValue) != 1 {
		t.Fatalf("proxy-groups len = %d, want 1", len(groupsValue))
	}
	group, ok := asMap(groupsValue[0])
	if !ok {
		t.Fatal("group is not a map")
	}
	if got, _ := group["name"].(string); got != "auto" {
		t.Fatalf("group name = %q, want auto", got)
	}
}

func TestGenerateUsesProfileOSForProviderFiltering(t *testing.T) {
	repoRoot := t.TempDir()
	configDir := filepath.Join(repoRoot, "config")
	providersDir := filepath.Join(repoRoot, "providers")
	if err := os.MkdirAll(filepath.Join(configDir, "linux"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(configDir, "macos"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(providersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	template := `proxies:
{{ toYAML .Proxies | indent 2 }}
proxy-groups:
{{ toYAML .ProxyGroups | indent 2 }}
proxy-providers:
{{ toYAML .ProxyProviders | indent 2 }}
rule-providers: {}
rules: []
`
	values := `
profiles:
  cluster:
    os: linux
  local:
    os: macos
probe:
  services:
    latency:
      uri: https://connectivitycheck.gstatic.com/generate_204
proxy-providers:
  bywave:
    type: http
    url: https://example.com/bywave
    interval: 60
    path: ./providers/bywave.yaml
  tag:
    type: http
    url: https://example.com/tag
    interval: 60
    path: ./providers/tag.yaml
service-groups:
  auto:
    probe: latency
    type: url-test
    url: https://connectivitycheck.gstatic.com/generate_204
    interval: 300
    tolerance: 50
    lazy: true
    profiles:
      cluster:
        providers: [bywave]
      local:
        providers: [tag]
`
	if err := os.WriteFile(filepath.Join(configDir, "mihomo.yaml.tmpl"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "values.yaml"), []byte(values), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(providersDir, "bywave.yaml"), []byte(`proxies:
  - name: cluster-proxy
    type: ss
    server: 1.1.1.1
    port: 443
    cipher: aes-128-gcm
    password: secret
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(providersDir, "tag.yaml"), []byte(`proxies:
  - name: personal-proxy
    type: ss
    server: 2.2.2.2
    port: 443
    cipher: aes-128-gcm
    password: secret
`), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog, err := LoadProviderCatalog(repoRoot, map[string]ProxyProviderSpec{
		"bywave": {Path: "./providers/bywave.yaml"},
		"tag":    {Path: "./providers/tag.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	probeStatePath := filepath.Join(t.TempDir(), "probe-results.yaml")
	t.Setenv("MIHOMO_PROBE_STATE_PATH", probeStatePath)
	now := time.Now().UTC()
	latencyDigest := probeServiceDigest(ProbeServiceSpec{URI: "https://connectivitycheck.gstatic.com/generate_204"})
	state := &ProbeState{
		Providers: map[string]ProviderProbeState{
			"bywave": {
				Provider:           "bywave",
				SubscriptionDigest: catalog.Providers["bywave"].Digest,
				Nodes: map[string]NodeProbeState{
					"cluster-proxy": {
						NodeName: "cluster-proxy",
						Services: map[string]ServiceProbeState{
							"latency": {
								OK:          true,
								ProbeDigest: latencyDigest,
								ProbedAt:    now.Add(-time.Minute).Format(time.RFC3339),
							},
						},
					},
				},
			},
			"tag": {
				Provider:           "tag",
				SubscriptionDigest: catalog.Providers["tag"].Digest,
				Nodes: map[string]NodeProbeState{
					"personal-proxy": {
						NodeName: "personal-proxy",
						Services: map[string]ServiceProbeState{
							"latency": {
								OK:          true,
								ProbeDigest: latencyDigest,
								ProbedAt:    now.Add(-time.Minute).Format(time.RFC3339),
							},
						},
					},
				},
			},
		},
	}
	populateFreshGroupProbeStateForTests(t, repoRoot, filepath.Join(configDir, "values.yaml"), state)
	if err := SaveProbeState(probeStatePath, state); err != nil {
		t.Fatal(err)
	}

	service := newTestService(repoRoot, configDir)
	if _, err := service.Generate(GenerateOptions{}); err != nil {
		t.Fatal(err)
	}

	linuxConfig, err := LoadConfig(profileConfigPath(configDir, "cluster"))
	if err != nil {
		t.Fatal(err)
	}
	linuxGroups, ok := linuxConfig["proxy-groups"].([]any)
	if !ok || len(linuxGroups) != 1 {
		t.Fatalf("linux proxy-groups = %#v", linuxConfig["proxy-groups"])
	}
	linuxGroup, ok := asMap(linuxGroups[0])
	if !ok {
		t.Fatal("linux group is not a map")
	}
	linuxProxyNames, ok := linuxGroup["proxies"].([]any)
	if !ok || len(linuxProxyNames) != 1 {
		t.Fatalf("linux group proxies = %#v", linuxGroup["proxies"])
	}
	if got, _ := linuxProxyNames[0].(string); got != "cluster-proxy" {
		t.Fatalf("linux group proxy = %q, want cluster-proxy", got)
	}

	macosConfig, err := LoadConfig(profileConfigPath(configDir, "local"))
	if err != nil {
		t.Fatal(err)
	}
	macosGroups, ok := macosConfig["proxy-groups"].([]any)
	if !ok || len(macosGroups) != 1 {
		t.Fatalf("macos proxy-groups = %#v", macosConfig["proxy-groups"])
	}
	macosGroup, ok := asMap(macosGroups[0])
	if !ok {
		t.Fatal("macos group is not a map")
	}
	macosProxyNames, ok := macosGroup["proxies"].([]any)
	if !ok || len(macosProxyNames) != 1 {
		t.Fatalf("macos group proxies = %#v", macosGroup["proxies"])
	}
	if got, _ := macosProxyNames[0].(string); got != "personal-proxy" {
		t.Fatalf("macos group proxy = %q, want personal-proxy", got)
	}
}

func TestGenerateUsesProfileProvidersForGroupFiltering(t *testing.T) {
	repoRoot := t.TempDir()
	configDir := filepath.Join(repoRoot, "config")
	providersDir := filepath.Join(repoRoot, "providers")
	if err := os.MkdirAll(filepath.Join(configDir, "linux"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(configDir, "macos"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(providersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	template := `proxies:
{{ toYAML .Proxies | indent 2 }}
proxy-groups:
{{ toYAML .ProxyGroups | indent 2 }}
proxy-providers:
{{ toYAML .ProxyProviders | indent 2 }}
rule-providers: {}
rules: []
`
	values := `
profiles:
  k3s:
    os: linux
  local:
    os: macos
probe:
  services:
    latency:
      uri: https://connectivitycheck.gstatic.com/generate_204
proxy-providers:
  bywave:
    type: http
    url: https://example.com/bywave
    interval: 60
    path: ./providers/bywave.yaml
  jisu:
    type: http
    url: https://example.com/jisu
    interval: 60
    path: ./providers/jisu.yaml
service-groups:
  k3s-image:
    probe: latency
    type: url-test
    url: https://connectivitycheck.gstatic.com/generate_204
    interval: 300
    tolerance: 50
    lazy: true
    profiles:
      k3s:
        providers: [jisu]
      local:
        providers: [jisu]
`
	if err := os.WriteFile(filepath.Join(configDir, "mihomo.yaml.tmpl"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "values.yaml"), []byte(values), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(providersDir, "bywave.yaml"), []byte(`proxies:
  - name: general-proxy
    type: ss
    server: 1.1.1.1
    port: 443
    cipher: aes-128-gcm
    password: secret
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(providersDir, "jisu.yaml"), []byte(`proxies:
  - name: tagged-proxy
    type: ss
    server: 2.2.2.2
    port: 443
    cipher: aes-128-gcm
    password: secret
`), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog, err := LoadProviderCatalog(repoRoot, map[string]ProxyProviderSpec{
		"bywave": {Path: "./providers/bywave.yaml"},
		"jisu":   {Path: "./providers/jisu.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	probeStatePath := filepath.Join(t.TempDir(), "probe-results.yaml")
	t.Setenv("MIHOMO_PROBE_STATE_PATH", probeStatePath)
	now := time.Now().UTC()
	latencyDigest := probeServiceDigest(ProbeServiceSpec{URI: "https://connectivitycheck.gstatic.com/generate_204"})
	state := &ProbeState{
		Providers: map[string]ProviderProbeState{
			"bywave": {
				Provider:           "bywave",
				SubscriptionDigest: catalog.Providers["bywave"].Digest,
				Nodes: map[string]NodeProbeState{
					"general-proxy": {
						NodeName: "general-proxy",
						Services: map[string]ServiceProbeState{
							"latency": {
								OK:          true,
								ProbeDigest: latencyDigest,
								ProbedAt:    now.Add(-time.Minute).Format(time.RFC3339),
							},
						},
					},
				},
			},
			"jisu": {
				Provider:           "jisu",
				SubscriptionDigest: catalog.Providers["jisu"].Digest,
				Nodes: map[string]NodeProbeState{
					"tagged-proxy": {
						NodeName: "tagged-proxy",
						Services: map[string]ServiceProbeState{
							"latency": {
								OK:          true,
								ProbeDigest: latencyDigest,
								ProbedAt:    now.Add(-time.Minute).Format(time.RFC3339),
							},
						},
					},
				},
			},
		},
	}
	populateFreshGroupProbeStateForTests(t, repoRoot, filepath.Join(configDir, "values.yaml"), state)
	if err := SaveProbeState(probeStatePath, state); err != nil {
		t.Fatal(err)
	}

	service := newTestService(repoRoot, configDir)
	if _, err := service.Generate(GenerateOptions{}); err != nil {
		t.Fatal(err)
	}

	generated, err := LoadConfig(profileConfigPath(configDir, "k3s"))
	if err != nil {
		t.Fatal(err)
	}
	groupsValue, ok := generated["proxy-groups"].([]any)
	if !ok || len(groupsValue) != 1 {
		t.Fatalf("proxy-groups = %#v", generated["proxy-groups"])
	}
	group, ok := asMap(groupsValue[0])
	if !ok {
		t.Fatal("group is not a map")
	}
	proxyNames, ok := group["proxies"].([]any)
	if !ok || len(proxyNames) != 1 {
		t.Fatalf("group proxies = %#v", group["proxies"])
	}
	if got, _ := proxyNames[0].(string); got != "tagged-proxy" {
		t.Fatalf("group proxy = %q, want tagged-proxy", got)
	}
}

func TestGenerateFiltersRulesWithoutMatchingProfileGroup(t *testing.T) {
	repoRoot := t.TempDir()
	configDir := filepath.Join(repoRoot, "config")
	providersDir := filepath.Join(repoRoot, "providers")
	if err := os.MkdirAll(filepath.Join(configDir, "linux"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(configDir, "macos"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(providersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	template := `proxies:
{{ toYAML .Proxies | indent 2 }}
proxy-groups:
{{ toYAML .ProxyGroups | indent 2 }}
proxy-providers:
{{ toYAML .ProxyProviders | indent 2 }}
rule-providers:
{{- if index .ProxyGroupNames "streaming" }}
  streaming:
    type: http
    behavior: classical
    format: yaml
    url: https://example.com/streaming.yaml
    path: ./ruleset/streaming.yaml
    interval: 60
{{- end }}
rules:
{{ toYAML .Rules | indent 2 }}
`
	values := `
profiles:
  k3s:
    os: linux
  local:
    os: macos
probe:
  services:
    latency:
      uri: https://connectivitycheck.gstatic.com/generate_204
proxy-providers:
  jisu:
    type: http
    url: https://example.com/jisu
    interval: 60
    path: ./providers/jisu.yaml
service-groups:
  k3s-image:
    probe: latency
    type: url-test
    interval: 300
    tolerance: 50
    lazy: true
    profiles:
      k3s:
        providers: [jisu]
rules:
  - RULE-SET,k3s-image,k3s-image
  - MATCH,DIRECT
`
	if err := os.WriteFile(filepath.Join(configDir, "mihomo.yaml.tmpl"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "values.yaml"), []byte(values), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(providersDir, "jisu.yaml"), []byte(`proxies:
  - name: tagged-proxy
    type: ss
    server: 2.2.2.2
    port: 443
    cipher: aes-128-gcm
    password: secret
`), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog, err := LoadProviderCatalog(repoRoot, map[string]ProxyProviderSpec{
		"jisu": {Path: "./providers/jisu.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	probeStatePath := filepath.Join(t.TempDir(), "probe-results.yaml")
	t.Setenv("MIHOMO_PROBE_STATE_PATH", probeStatePath)
	now := time.Now().UTC()
	latencyDigest := probeServiceDigest(ProbeServiceSpec{URI: "https://connectivitycheck.gstatic.com/generate_204"})
	state := &ProbeState{
		Providers: map[string]ProviderProbeState{
			"jisu": {
				Provider:           "jisu",
				SubscriptionDigest: catalog.Providers["jisu"].Digest,
				Nodes: map[string]NodeProbeState{
					"tagged-proxy": {
						NodeName: "tagged-proxy",
						Services: map[string]ServiceProbeState{
							"latency": {
								OK:          true,
								ProbeDigest: latencyDigest,
								ProbedAt:    now.Add(-time.Minute).Format(time.RFC3339),
							},
						},
					},
				},
			},
		},
	}
	populateFreshGroupProbeStateForTests(t, repoRoot, filepath.Join(configDir, "values.yaml"), state)
	if err := SaveProbeState(probeStatePath, state); err != nil {
		t.Fatal(err)
	}

	service := newTestService(repoRoot, configDir)
	if _, err := service.Generate(GenerateOptions{}); err != nil {
		t.Fatal(err)
	}

	localConfig, err := LoadConfig(profileConfigPath(configDir, "local"))
	if err != nil {
		t.Fatal(err)
	}
	rules, ok := localConfig["rules"].([]any)
	if !ok {
		t.Fatalf("rules type = %T", localConfig["rules"])
	}
	if len(rules) != 1 {
		t.Fatalf("rules len = %d, want 1", len(rules))
	}
	if got, _ := rules[0].(string); got != "MATCH,DIRECT" {
		t.Fatalf("first local rule = %q, want MATCH,DIRECT", got)
	}

	k3sConfig, err := LoadConfig(profileConfigPath(configDir, "k3s"))
	if err != nil {
		t.Fatal(err)
	}
	k3sRules, ok := k3sConfig["rules"].([]any)
	if !ok {
		t.Fatalf("k3s rules type = %T", k3sConfig["rules"])
	}
	if len(k3sRules) != 2 {
		t.Fatalf("k3s rules len = %d, want 2", len(k3sRules))
	}
	if got, _ := k3sRules[0].(string); got != "RULE-SET,k3s-image,k3s-image" {
		t.Fatalf("first k3s rule = %q, want RULE-SET,k3s-image,k3s-image", got)
	}
}

func TestGenerateConfigOmitsDirectFromAllProxyGroups(t *testing.T) {
	repoRoot := t.TempDir()
	configDir := filepath.Join(repoRoot, "config")
	providersDir := filepath.Join(repoRoot, "providers")
	if err := os.MkdirAll(filepath.Join(configDir, "local"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(providersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	template := `proxies:
{{ toYAML .Proxies | indent 2 }}
proxy-groups:
{{ toYAML .ProxyGroups | indent 2 }}
proxy-providers:
{{ toYAML .ProxyProviders | indent 2 }}
rule-providers: {}
rules:
  - MATCH,DIRECT
`
	values := `
profiles:
  local:
    os: macos
probe:
  services:
    latency:
      uri: https://connectivitycheck.gstatic.com/generate_204
proxy-providers:
  bywave:
    type: http
    url: https://example.com/sub
    interval: 60
    path: ./providers/bywave.yaml
service-groups:
  auto:
    probe: latency
    type: url-test
    interval: 300
    tolerance: 50
    lazy: true
    profiles:
      local:
        providers: [bywave]
`
	providerYAML := `proxies:
  - name: DIRECT
    type: ss
    server: 1.1.1.1
    port: 443
    cipher: aes-128-gcm
    password: secret-direct
  - name: proxy-b
    type: ss
    server: 2.2.2.2
    port: 443
    cipher: aes-128-gcm
    password: secret-b
`
	if err := os.WriteFile(filepath.Join(configDir, "mihomo.yaml.tmpl"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "values.yaml"), []byte(values), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(providersDir, "bywave.yaml"), []byte(providerYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog, err := LoadProviderCatalog(repoRoot, map[string]ProxyProviderSpec{
		"bywave": {Path: "./providers/bywave.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	probeStatePath := filepath.Join(t.TempDir(), "probe-results.yaml")
	t.Setenv("MIHOMO_PROBE_STATE_PATH", probeStatePath)
	now := time.Now().UTC()
	latencyDigest := probeServiceDigest(ProbeServiceSpec{URI: "https://connectivitycheck.gstatic.com/generate_204"})
	state := &ProbeState{
		Providers: map[string]ProviderProbeState{
			"bywave": {
				Provider:           "bywave",
				SubscriptionDigest: catalog.Providers["bywave"].Digest,
				Nodes: map[string]NodeProbeState{
					"DIRECT": {
						NodeName: "DIRECT",
						Services: map[string]ServiceProbeState{
							"latency": {
								OK:          true,
								ProbeDigest: latencyDigest,
								ProbedAt:    now.Add(-time.Minute).Format(time.RFC3339),
							},
						},
					},
					"proxy-b": {
						NodeName: "proxy-b",
						Services: map[string]ServiceProbeState{
							"latency": {
								OK:          true,
								ProbeDigest: latencyDigest,
								ProbedAt:    now.Add(-time.Minute).Format(time.RFC3339),
							},
						},
					},
				},
			},
		},
	}
	populateFreshGroupProbeStateForTests(t, repoRoot, filepath.Join(configDir, "values.yaml"), state)
	if err := SaveProbeState(probeStatePath, state); err != nil {
		t.Fatal(err)
	}

	service := newTestService(repoRoot, configDir)
	if _, err := service.Generate(GenerateOptions{}); err != nil {
		t.Fatal(err)
	}

	generated, err := LoadConfig(profileConfigPath(configDir, "local"))
	if err != nil {
		t.Fatal(err)
	}
	groups, ok := generated["proxy-groups"].([]any)
	if !ok {
		t.Fatalf("proxy-groups = %#v", generated["proxy-groups"])
	}
	for _, rawGroup := range groups {
		group, ok := asMap(rawGroup)
		if !ok {
			t.Fatalf("group type = %T", rawGroup)
		}
		rawProxies, ok := group["proxies"].([]any)
		if !ok {
			continue
		}
		for _, rawProxy := range rawProxies {
			name, ok := rawProxy.(string)
			if !ok {
				t.Fatalf("group proxy type = %T", rawProxy)
			}
			if name == "DIRECT" {
				t.Fatalf("group %q still contains DIRECT: %#v", group["name"], rawProxies)
			}
		}
	}

	rules, ok := generated["rules"].([]any)
	if !ok || len(rules) != 1 {
		t.Fatalf("rules = %#v", generated["rules"])
	}
	if got, _ := rules[0].(string); got != "MATCH,DIRECT" {
		t.Fatalf("rule = %q, want MATCH,DIRECT", got)
	}
}

func TestGenerateStableKeepsAllSuccessfulLatencyNodes(t *testing.T) {
	originalStartMihomoProbeRuntimeFunc := startMihomoProbeRuntimeFunc
	originalRuntimeHTTPRequestViaNodeFunc := runtimeHTTPRequestViaNodeFunc
	t.Cleanup(func() {
		startMihomoProbeRuntimeFunc = originalStartMihomoProbeRuntimeFunc
		runtimeHTTPRequestViaNodeFunc = originalRuntimeHTTPRequestViaNodeFunc
	})

	startMihomoProbeRuntimeFunc = func(snapshot ProviderSnapshot) (*mihomoProbeRuntime, error) {
		return &mihomoProbeRuntime{
			availableNodes: map[string]struct{}{
				"node-fast": {},
				"node-mid":  {},
				"node-slow": {},
			},
		}, nil
	}
	runtimeHTTPRequestViaNodeFunc = func(runtime *mihomoProbeRuntime, nodeName, method, requestURL string, headers map[string]string, body io.Reader, timeout time.Duration) (*http.Response, []byte, error) {
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Body:       io.NopCloser(bytes.NewReader(nil)),
		}, nil, nil
	}

	repoRoot := t.TempDir()
	configDir := filepath.Join(repoRoot, "config")
	providersDir := filepath.Join(repoRoot, "providers")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(providersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	template := `proxies:
{{ toYAML .Proxies | indent 2 }}
proxy-groups:
{{ toYAML .ProxyGroups | indent 2 }}
proxy-providers:
{{ toYAML .ProxyProviders | indent 2 }}
rule-providers: {}
rules: []
`
	values := `
profiles:
  local:
    os: macos
manual-proxies:
  - name: feilian-proxy
    type: socks5
    server: 192.168.3.104
    port: 1090
probe:
  services:
    latency:
      uri: https://connectivitycheck.gstatic.com/generate_204
proxy-providers:
  bywave:
    type: http
    url: https://example.com/sub
    interval: 60
    path: ./providers/bywave.yaml
service-groups:
  stable:
    probe: latency
    type: url-test
    interval: 300
    tolerance: 50
    lazy: true
    profiles:
      local:
        providers: [bywave]
`
	providerYAML := `proxies:
  - name: node-fast
    type: ss
    server: 1.1.1.1
    port: 443
    cipher: aes-128-gcm
    password: secret-a
  - name: node-mid
    type: ss
    server: 2.2.2.2
    port: 443
    cipher: aes-128-gcm
    password: secret-b
  - name: node-slow
    type: ss
    server: 3.3.3.3
    port: 443
    cipher: aes-128-gcm
    password: secret-c
`
	if err := os.WriteFile(filepath.Join(configDir, "mihomo.yaml.tmpl"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "values.yaml"), []byte(values), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(providersDir, "bywave.yaml"), []byte(providerYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadGenerationConfig(filepath.Join(configDir, "values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadProviderCatalog(repoRoot, cfg.ProxyProviders)
	if err != nil {
		t.Fatal(err)
	}
	digest := catalog.Providers["bywave"].Digest
	latencyDigest := probeServiceDigest(cfg.Probe.Services["latency"])

	probeStatePath := filepath.Join(t.TempDir(), "probe-results.yaml")
	t.Setenv("MIHOMO_PROBE_STATE_PATH", probeStatePath)
	now := time.Now().UTC().Add(-time.Minute)
	state := &ProbeState{
		Providers: map[string]ProviderProbeState{
			"bywave": {
				Provider:           "bywave",
				SubscriptionDigest: digest,
				Nodes: map[string]NodeProbeState{
					"node-fast": {
						NodeName: "node-fast",
						Services: map[string]ServiceProbeState{
							"latency": {OK: true, LatencyMillis: 50, ProbeDigest: latencyDigest, ProbedAt: now.Format(time.RFC3339)},
						},
					},
					"node-mid": {
						NodeName: "node-mid",
						Services: map[string]ServiceProbeState{
							"latency": {OK: true, LatencyMillis: 150, ProbeDigest: latencyDigest, ProbedAt: now.Format(time.RFC3339)},
						},
					},
					"node-slow": {
						NodeName: "node-slow",
						Services: map[string]ServiceProbeState{
							"latency": {OK: true, LatencyMillis: 260, ProbeDigest: latencyDigest, ProbedAt: now.Format(time.RFC3339)},
						},
					},
				},
			},
		},
	}
	populateFreshGroupProbeStateForTests(t, repoRoot, filepath.Join(configDir, "values.yaml"), state)
	if err := SaveProbeState(probeStatePath, state); err != nil {
		t.Fatal(err)
	}

	if err := RunProbe(repoRoot, probeStatePath, cfg, ProbeScope{
		Providers: []string{"bywave"},
		Services:  []string{"latency"},
		Mode:      ProbeModeService,
	}); err != nil {
		t.Fatal(err)
	}

	service := newTestService(repoRoot, configDir)
	if _, err := service.Generate(GenerateOptions{}); err != nil {
		t.Fatal(err)
	}

	generated, err := LoadConfig(profileConfigPath(configDir, "local"))
	if err != nil {
		t.Fatal(err)
	}
	assertAllGroupProxyNames(t, generated, map[string][]string{
		"stable": {"node-fast", "node-mid", "node-slow"},
	})
	assertTopLevelProxyNamesIgnoreOrder(t, generated, []string{"feilian-proxy", "node-fast", "node-mid", "node-slow"})
	rawProxies, ok := generated["proxies"].([]any)
	if !ok {
		t.Fatalf("proxies type = %T", generated["proxies"])
	}
	foundFeilian := false
	for _, item := range rawProxies {
		proxy, ok := asMap(item)
		if !ok {
			t.Fatalf("proxy type = %T", item)
		}
		if proxy["name"] != "feilian-proxy" {
			continue
		}
		foundFeilian = true
		if proxy["server"] != "192.168.3.104" {
			t.Fatalf("feilian server = %#v", proxy["server"])
		}
		if proxy["port"] != 1090 {
			t.Fatalf("feilian port = %#v", proxy["port"])
		}
	}
	if !foundFeilian {
		t.Fatal("missing feilian-proxy in top-level proxies")
	}
}

func TestGenerateFailsWithoutFreshGroupProbeState(t *testing.T) {
	repoRoot := t.TempDir()
	configDir := filepath.Join(repoRoot, "config")
	providersDir := filepath.Join(repoRoot, "providers")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(providersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	template := `proxies:
{{ toYAML .Proxies | indent 2 }}
proxy-groups:
{{ toYAML .ProxyGroups | indent 2 }}
proxy-providers:
{{ toYAML .ProxyProviders | indent 2 }}
rule-providers: {}
rules: []
`
	values := `
profiles:
  local:
    os: macos
manual-proxies:
  - name: feilian-proxy
    type: socks5
    server: 192.168.3.104
    port: 1090
probe:
  services:
    latency:
      uri: https://connectivitycheck.gstatic.com/generate_204
    openai:
      uri: https://api.openai.com/v1/models
proxy-providers:
  bywave:
    type: http
    url: https://example.com/sub
    interval: 60
    path: ./providers/bywave.yaml
service-groups:
  openai:
    probe: openai
    type: url-test
    interval: 300
    tolerance: 50
    lazy: true
    profiles:
      local:
        providers: [bywave]
`
	providerYAML := `proxies:
  - name: node-fast
    type: ss
    server: 1.1.1.1
    port: 443
    cipher: aes-128-gcm
    password: secret-a
`
	if err := os.WriteFile(filepath.Join(configDir, "mihomo.yaml.tmpl"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "values.yaml"), []byte(values), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(providersDir, "bywave.yaml"), []byte(providerYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadGenerationConfig(filepath.Join(configDir, "values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadProviderCatalog(repoRoot, cfg.ProxyProviders)
	if err != nil {
		t.Fatal(err)
	}
	digest := catalog.Providers["bywave"].Digest
	latencyDigest := probeServiceDigest(cfg.Probe.Services["latency"])

	probeStatePath := filepath.Join(t.TempDir(), "probe-results.yaml")
	t.Setenv("MIHOMO_PROBE_STATE_PATH", probeStatePath)
	now := time.Now().UTC().Add(-time.Minute)
	state := &ProbeState{
		Providers: map[string]ProviderProbeState{
			"bywave": {
				Provider:           "bywave",
				SubscriptionDigest: digest,
				Nodes: map[string]NodeProbeState{
					"node-fast": {
						NodeName: "node-fast",
						Services: map[string]ServiceProbeState{
							"latency": {OK: true, LatencyMillis: 50, ProbeDigest: latencyDigest, ProbedAt: now.Format(time.RFC3339)},
						},
					},
				},
			},
		},
	}
	if err := SaveProbeState(probeStatePath, state); err != nil {
		t.Fatal(err)
	}

	service := newTestService(repoRoot, configDir)
	_, err = service.Generate(GenerateOptions{})
	if err == nil {
		t.Fatal("expected missing group probe state to fail")
	}
	if !strings.Contains(err.Error(), "missing fresh group probe state for openai on local") {
		t.Fatalf("error = %q", err)
	}
}

func TestRenderTemplateIncludesStreamingRuleProviderOnlyWhenProxyGroupPresent(t *testing.T) {
	dir := t.TempDir()
	templatePath := filepath.Join(dir, "mihomo.yaml.tmpl")
	template := `rule-providers:
{{- if index .ProxyGroupNames "streaming" }}
  streaming:
    type: http
    behavior: classical
    format: yaml
    url: https://example.com/streaming.yaml
    path: ./ruleset/streaming.yaml
    interval: 60
{{- end }}
rules:
  - MATCH,auto
{{- if index .ProxyGroupNames "streaming" }}
  - RULE-SET,streaming,streaming
{{- end }}
`
	if err := os.WriteFile(templatePath, []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}

	withStreaming, err := RenderTemplate(templatePath, RenderData{
		ProxyGroupNames: map[string]bool{"streaming": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	withConfig, err := LoadConfigString(withStreaming)
	if err != nil {
		t.Fatal(err)
	}
	withProviders, ok := withConfig["rule-providers"].(Config)
	if !ok {
		t.Fatalf("with-streaming rule-providers type = %T", withConfig["rule-providers"])
	}
	if _, exists := withProviders["streaming"]; !exists {
		t.Fatal("with-streaming rule-providers missing streaming")
	}

	withoutStreaming, err := RenderTemplate(templatePath, RenderData{
		ProxyGroupNames: map[string]bool{},
	})
	if err != nil {
		t.Fatal(err)
	}
	withoutConfig, err := LoadConfigString(withoutStreaming)
	if err != nil {
		t.Fatal(err)
	}
	if value, exists := withoutConfig["rule-providers"]; exists && value != nil {
		withoutProviders, ok := value.(Config)
		if !ok {
			t.Fatalf("without-streaming rule-providers type = %T", value)
		}
		if _, exists := withoutProviders["streaming"]; exists {
			t.Fatal("without-streaming rule-providers unexpectedly contains streaming")
		}
		return
	}
	if _, exists := withoutConfig["streaming"]; exists {
		t.Fatal("without-streaming rule-providers unexpectedly contains streaming")
	}
}

func TestGenerateProbeNoneGroupDoesNotRequireProbeState(t *testing.T) {
	repoRoot := t.TempDir()
	configDir := filepath.Join(repoRoot, "config")
	providersDir := filepath.Join(repoRoot, "providers")
	if err := os.MkdirAll(filepath.Join(configDir, "local"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(providersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	template := `proxies:
{{ toYAML .Proxies | indent 2 }}
proxy-groups:
{{ toYAML .ProxyGroups | indent 2 }}
proxy-providers:
{{ toYAML .ProxyProviders | indent 2 }}
rule-providers: {}
rules: []
`
	values := `
profiles:
  local:
    os: macos
manual-proxies:
  - name: feilian-proxy
    type: socks5
    server: 192.168.3.104
    port: 1090
probe:
  services:
    latency:
      uri: https://connectivitycheck.gstatic.com/generate_204
proxy-providers:
  bywave:
    type: http
    url: https://example.com/sub
    interval: 60
    path: ./providers/bywave.yaml
service-groups:
  stable:
    probe: none
    type: url-test
    interval: 300
    tolerance: 50
    lazy: true
    profiles:
      local:
        providers: [bywave]
`
	providerYAML := `proxies:
  - name: node-fast
    type: ss
    server: 1.1.1.1
    port: 443
    cipher: aes-128-gcm
    password: secret-a
`
	if err := os.WriteFile(filepath.Join(configDir, "mihomo.yaml.tmpl"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "values.yaml"), []byte(values), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(providersDir, "bywave.yaml"), []byte(providerYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	probeStatePath := filepath.Join(t.TempDir(), "probe-results.yaml")
	t.Setenv("MIHOMO_PROBE_STATE_PATH", probeStatePath)
	if err := SaveProbeState(probeStatePath, &ProbeState{Providers: map[string]ProviderProbeState{}}); err != nil {
		t.Fatal(err)
	}

	service := newTestService(repoRoot, configDir)
	if _, err := service.Generate(GenerateOptions{}); err != nil {
		t.Fatal(err)
	}

	generated, err := LoadConfig(profileConfigPath(configDir, "local"))
	if err != nil {
		t.Fatal(err)
	}
	groups, ok := generated["proxy-groups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("proxy-groups = %#v", generated["proxy-groups"])
	}
	group, ok := asMap(groups[0])
	if !ok {
		t.Fatalf("group type = %T", groups[0])
	}
	assertStringSlice(t, []string{group["use"].([]any)[0].(string)}, []string{"bywave"})
	if _, ok := group["proxies"]; ok {
		t.Fatalf("probe:none group should not render static proxies: %#v", group["proxies"])
	}
}

func TestGenerateExcludesUnsupportedHighMultipliersByDefault(t *testing.T) {
	repoRoot := t.TempDir()
	configDir := filepath.Join(repoRoot, "config")
	providersDir := filepath.Join(repoRoot, "providers")
	if err := os.MkdirAll(filepath.Join(configDir, "local"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(providersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	template := `proxies:
{{ toYAML .Proxies | indent 2 }}
proxy-groups:
{{ toYAML .ProxyGroups | indent 2 }}
proxy-providers:
{{ toYAML .ProxyProviders | indent 2 }}
rule-providers: {}
rules: []
`
	values := `
profiles:
  local:
    os: macos
manual-proxies:
  - name: feilian-proxy
    type: socks5
    server: 192.168.3.104
    port: 1090
probe:
  services:
    latency:
      uri: https://connectivitycheck.gstatic.com/generate_204
proxy-providers:
  bywave:
    type: http
    url: https://example.com/sub
    interval: 60
    path: ./providers/bywave.yaml
service-groups:
  auto:
    probe: latency
    multiplier-filters:
      x3: "(?i)(?:^|[^0-9.])(?:x3|3x)(?:[^0-9.]|$)"
      x5: "(?i)(?:^|[^0-9.])(?:x5|5x)(?:[^0-9.]|$)"
    type: url-test
    interval: 300
    tolerance: 50
    lazy: true
    profiles:
      local:
        providers: [bywave]
  openai:
    probe: latency
    multiplier-filters:
      x3: "(?i)(?:^|[^0-9.])(?:x3|3x)(?:[^0-9.]|$)"
      x5: "(?i)(?:^|[^0-9.])(?:x5|5x)(?:[^0-9.]|$)"
    type: url-test
    interval: 300
    tolerance: 50
    lazy: true
    profiles:
      local:
        providers: [bywave]
`
	providerYAML := `proxies:
  - name: low-0.1x
    type: ss
    server: 1.1.1.1
    port: 443
    cipher: aes-128-gcm
    password: secret-a
  - name: keep-1x
    type: ss
    server: 2.2.2.2
    port: 443
    cipher: aes-128-gcm
    password: secret-b
  - name: drop-3x
    type: ss
    server: 3.3.3.3
    port: 443
    cipher: aes-128-gcm
    password: secret-c
  - name: drop-x5
    type: ss
    server: 4.4.4.4
    port: 443
    cipher: aes-128-gcm
    password: secret-d
`
	if err := os.WriteFile(filepath.Join(configDir, "mihomo.yaml.tmpl"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "values.yaml"), []byte(values), 0o644); err != nil {
		t.Fatal(err)
	}
	providerPath := filepath.Join(providersDir, "bywave.yaml")
	if err := os.WriteFile(providerPath, []byte(providerYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog, err := LoadProviderCatalog(repoRoot, map[string]ProxyProviderSpec{
		"bywave": {Path: "./providers/bywave.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := catalog.Providers["bywave"].Digest
	latencyDigest := probeServiceDigest(ProbeServiceSpec{URI: "https://connectivitycheck.gstatic.com/generate_204"})

	probeStatePath := filepath.Join(t.TempDir(), "probe-results.yaml")
	t.Setenv("MIHOMO_PROBE_STATE_PATH", probeStatePath)
	now := time.Now().UTC()
	state := &ProbeState{
		Providers: map[string]ProviderProbeState{
			"bywave": {
				Provider:           "bywave",
				SubscriptionDigest: digest,
				Nodes: map[string]NodeProbeState{
					"low-0.1x": {
						NodeName: "low-0.1x",
						Services: map[string]ServiceProbeState{
							"latency": {OK: true, ProbeDigest: latencyDigest, ProbedAt: now.Format(time.RFC3339)},
						},
					},
					"keep-1x": {
						NodeName: "keep-1x",
						Services: map[string]ServiceProbeState{
							"latency": {OK: true, ProbeDigest: latencyDigest, ProbedAt: now.Format(time.RFC3339)},
						},
					},
					"drop-3x": {
						NodeName: "drop-3x",
						Services: map[string]ServiceProbeState{
							"latency": {OK: true, ProbeDigest: latencyDigest, ProbedAt: now.Format(time.RFC3339)},
						},
					},
					"drop-x5": {
						NodeName: "drop-x5",
						Services: map[string]ServiceProbeState{
							"latency": {OK: true, ProbeDigest: latencyDigest, ProbedAt: now.Format(time.RFC3339)},
						},
					},
				},
			},
		},
	}
	populateFreshGroupProbeStateForTests(t, repoRoot, filepath.Join(configDir, "values.yaml"), state)
	if err := SaveProbeState(probeStatePath, state); err != nil {
		t.Fatal(err)
	}

	service := newTestService(repoRoot, configDir)
	if _, err := service.Generate(GenerateOptions{}); err != nil {
		t.Fatal(err)
	}

	generated, err := LoadConfig(profileConfigPath(configDir, "local"))
	if err != nil {
		t.Fatal(err)
	}
	assertAllGroupProxyNames(t, generated, map[string][]string{
		"auto":   {"low-0.1x", "keep-1x"},
		"openai": {"low-0.1x", "keep-1x"},
	})
	assertTopLevelProxyNamesIgnoreOrder(t, generated, []string{"feilian-proxy", "low-0.1x", "keep-1x"})
	groups, ok := generated["proxy-groups"].([]any)
	if !ok {
		t.Fatalf("proxy-groups = %#v", generated["proxy-groups"])
	}
	for _, item := range groups {
		group, ok := asMap(item)
		if !ok {
			t.Fatalf("group type = %T", item)
		}
		if _, exists := group["exclude-filter"]; exists {
			t.Fatalf("exclude-filter should be omitted for probe-driven groups, got %#v", group["exclude-filter"])
		}
	}
}

func TestGenerateOmitsMultiplierExcludeFilterWithoutConfiguredOptions(t *testing.T) {
	repoRoot := t.TempDir()
	configDir := filepath.Join(repoRoot, "config")
	providersDir := filepath.Join(repoRoot, "providers")
	if err := os.MkdirAll(filepath.Join(configDir, "local"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(providersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	template := `proxies:
{{ toYAML .Proxies | indent 2 }}
proxy-groups:
{{ toYAML .ProxyGroups | indent 2 }}
proxy-providers:
{{ toYAML .ProxyProviders | indent 2 }}
rule-providers: {}
rules: []
`
	values := `
profiles:
  local:
    os: macos
manual-proxies:
  - name: feilian-proxy
    type: socks5
    server: 192.168.3.104
    port: 1090
probe:
  services:
    latency:
      uri: https://connectivitycheck.gstatic.com/generate_204
proxy-providers:
  bywave:
    type: http
    url: https://example.com/sub
    interval: 60
    path: ./providers/bywave.yaml
service-groups:
  auto:
    probe: latency
    type: url-test
    interval: 300
    tolerance: 50
    lazy: true
    profiles:
      local:
        providers: [bywave]
`
	providerYAML := `proxies:
  - name: only-2x
    type: ss
    server: 1.1.1.1
    port: 443
    cipher: aes-128-gcm
    password: secret-a
  - name: only-3x
    type: ss
    server: 2.2.2.2
    port: 443
    cipher: aes-128-gcm
    password: secret-b
`
	if err := os.WriteFile(filepath.Join(configDir, "mihomo.yaml.tmpl"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "values.yaml"), []byte(values), 0o644); err != nil {
		t.Fatal(err)
	}
	providerPath := filepath.Join(providersDir, "bywave.yaml")
	if err := os.WriteFile(providerPath, []byte(providerYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog, err := LoadProviderCatalog(repoRoot, map[string]ProxyProviderSpec{
		"bywave": {Path: "./providers/bywave.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := catalog.Providers["bywave"].Digest
	latencyDigest := probeServiceDigest(ProbeServiceSpec{URI: "https://connectivitycheck.gstatic.com/generate_204"})

	probeStatePath := filepath.Join(t.TempDir(), "probe-results.yaml")
	t.Setenv("MIHOMO_PROBE_STATE_PATH", probeStatePath)
	now := time.Now().UTC()
	state := &ProbeState{
		Providers: map[string]ProviderProbeState{
			"bywave": {
				Provider:           "bywave",
				SubscriptionDigest: digest,
				Nodes: map[string]NodeProbeState{
					"only-2x": {
						NodeName: "only-2x",
						Services: map[string]ServiceProbeState{
							"latency": {OK: true, ProbeDigest: latencyDigest, ProbedAt: now.Format(time.RFC3339)},
						},
					},
					"only-3x": {
						NodeName: "only-3x",
						Services: map[string]ServiceProbeState{
							"latency": {OK: true, ProbeDigest: latencyDigest, ProbedAt: now.Format(time.RFC3339)},
						},
					},
				},
			},
		},
	}
	populateFreshGroupProbeStateForTests(t, repoRoot, filepath.Join(configDir, "values.yaml"), state)
	if err := SaveProbeState(probeStatePath, state); err != nil {
		t.Fatal(err)
	}

	service := newTestService(repoRoot, configDir)
	if _, err := service.Generate(GenerateOptions{}); err != nil {
		t.Fatal(err)
	}

	filtered, err := LoadConfig(profileConfigPath(configDir, "local"))
	if err != nil {
		t.Fatal(err)
	}
	assertAllGroupProxyNames(t, filtered, map[string][]string{
		"auto": {"only-2x", "only-3x"},
	})
	assertTopLevelProxyNamesIgnoreOrder(t, filtered, []string{"feilian-proxy", "only-2x", "only-3x"})
	groups, ok := filtered["proxy-groups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("proxy-groups = %#v", filtered["proxy-groups"])
	}
	group, ok := asMap(groups[0])
	if !ok {
		t.Fatalf("group type = %T", groups[0])
	}
	if _, exists := group["exclude-filter"]; exists {
		t.Fatalf("exclude-filter should be omitted by default, got %#v", group["exclude-filter"])
	}
}

func TestGenerateAllowsSupportedHighMultipliers(t *testing.T) {
	repoRoot := t.TempDir()
	configDir := filepath.Join(repoRoot, "config")
	providersDir := filepath.Join(repoRoot, "providers")
	if err := os.MkdirAll(filepath.Join(configDir, "local"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(providersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	template := `proxies:
{{ toYAML .Proxies | indent 2 }}
proxy-groups:
{{ toYAML .ProxyGroups | indent 2 }}
proxy-providers:
{{ toYAML .ProxyProviders | indent 2 }}
rule-providers: {}
rules: []
`
	values := `
profiles:
  local:
    os: macos
manual-proxies:
  - name: feilian-proxy
    type: socks5
    server: 192.168.3.104
    port: 1090
probe:
  services:
    latency:
      uri: https://connectivitycheck.gstatic.com/generate_204
proxy-providers:
  bywave:
    type: http
    url: https://example.com/sub
    interval: 60
    path: ./providers/bywave.yaml
service-groups:
  auto:
    probe: latency
    multiplier-filters:
      x2: "(?i)(?:^|[^0-9.])(?:x2|2x)(?:[^0-9.]|$)"
      x3: "(?i)(?:^|[^0-9.])(?:x3|3x)(?:[^0-9.]|$)"
    supported-high-multipliers: [x2]
    type: url-test
    interval: 300
    tolerance: 50
    lazy: true
    profiles:
      local:
        providers: [bywave]
`
	providerYAML := `proxies:
  - name: only-2x
    type: ss
    server: 1.1.1.1
    port: 443
    cipher: aes-128-gcm
    password: secret-a
  - name: only-3x
    type: ss
    server: 2.2.2.2
    port: 443
    cipher: aes-128-gcm
    password: secret-b
`
	if err := os.WriteFile(filepath.Join(configDir, "mihomo.yaml.tmpl"), []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "values.yaml"), []byte(values), 0o644); err != nil {
		t.Fatal(err)
	}
	providerPath := filepath.Join(providersDir, "bywave.yaml")
	if err := os.WriteFile(providerPath, []byte(providerYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog, err := LoadProviderCatalog(repoRoot, map[string]ProxyProviderSpec{
		"bywave": {Path: "./providers/bywave.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := catalog.Providers["bywave"].Digest
	latencyDigest := probeServiceDigest(ProbeServiceSpec{URI: "https://connectivitycheck.gstatic.com/generate_204"})

	probeStatePath := filepath.Join(t.TempDir(), "probe-results.yaml")
	t.Setenv("MIHOMO_PROBE_STATE_PATH", probeStatePath)
	now := time.Now().UTC()
	state := &ProbeState{
		Providers: map[string]ProviderProbeState{
			"bywave": {
				Provider:           "bywave",
				SubscriptionDigest: digest,
				Nodes: map[string]NodeProbeState{
					"only-2x": {
						NodeName: "only-2x",
						Services: map[string]ServiceProbeState{
							"latency": {OK: true, ProbeDigest: latencyDigest, ProbedAt: now.Format(time.RFC3339)},
						},
					},
					"only-3x": {
						NodeName: "only-3x",
						Services: map[string]ServiceProbeState{
							"latency": {OK: true, ProbeDigest: latencyDigest, ProbedAt: now.Format(time.RFC3339)},
						},
					},
				},
			},
		},
	}
	populateFreshGroupProbeStateForTests(t, repoRoot, filepath.Join(configDir, "values.yaml"), state)
	if err := SaveProbeState(probeStatePath, state); err != nil {
		t.Fatal(err)
	}

	service := newTestService(repoRoot, configDir)
	if _, err := service.Generate(GenerateOptions{}); err != nil {
		t.Fatal(err)
	}

	filtered, err := LoadConfig(profileConfigPath(configDir, "local"))
	if err != nil {
		t.Fatal(err)
	}
	assertAllGroupProxyNamesIgnoreOrder(t, filtered, map[string][]string{
		"auto": {"only-2x"},
	})
	assertTopLevelProxyNamesIgnoreOrder(t, filtered, []string{"feilian-proxy", "only-2x"})
	groups, ok := filtered["proxy-groups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("proxy-groups = %#v", filtered["proxy-groups"])
	}
	group, ok := asMap(groups[0])
	if !ok {
		t.Fatalf("group type = %T", groups[0])
	}
	if _, exists := group["exclude-filter"]; exists {
		t.Fatalf("exclude-filter should be omitted for probe-driven groups, got %#v", group["exclude-filter"])
	}
}

func TestRenderProxyGroupsPlacesNameFirst(t *testing.T) {
	dir := t.TempDir()
	templatePath := filepath.Join(dir, "mihomo.yaml.tmpl")
	template := `proxy-groups:
{{ toYAML .ProxyGroups | indent 2 }}
`
	if err := os.WriteFile(templatePath, []byte(template), 0o644); err != nil {
		t.Fatal(err)
	}

	rendered, err := RenderTemplate(templatePath, RenderData{
		ProxyGroups: OrderedList{
			Items: []OrderedMap{
				{
					Keys: []string{"name", "type", "interval", "lazy", "url", "use"},
					Values: map[string]any{
						"name":     "auto",
						"type":     "url-test",
						"interval": 300,
						"lazy":     true,
						"url":      "https://example.com/generate_204",
						"use":      []any{"bywave", "tag"},
					},
				},
				{
					Keys: []string{"name", "type", "interval", "lazy", "url", "use"},
					Values: map[string]any{
						"name":     "select",
						"type":     "url-test",
						"interval": 300,
						"lazy":     true,
						"url":      "https://example.com/generate_204",
						"use":      []any{"bywave", "tag"},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	expected := "proxy-groups:\n  - name: auto\n"
	if !strings.Contains(rendered, expected) {
		t.Fatalf("rendered proxy-groups missing name-first auto block:\n%s", rendered)
	}
	expected = "  - name: select\n"
	if !strings.Contains(rendered, expected) {
		t.Fatalf("rendered proxy-groups missing name-first select block:\n%s", rendered)
	}
	if strings.Contains(rendered, "  - interval: 300\n    name: auto") {
		t.Fatalf("rendered proxy-groups still place interval before name:\n%s", rendered)
	}
}

func TestResolveServiceGroupURLPrefersProbeServiceURLTest(t *testing.T) {
	cfg := &GenerationConfig{
		Probe: ProbeConfig{
			Services: map[string]ProbeServiceSpec{
				"github": {
					URI:     "ssh://github.com:22",
					URLTest: "https://github.com/",
				},
			},
		},
	}

	got, err := resolveServiceGroupURL("github", ServiceGroupSpec{Probe: "github"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://github.com/" {
		t.Fatalf("group url = %q, want https://github.com/", got)
	}
}

func TestResolveServiceGroupURLRejectsNonHTTPProbeWithoutURLTest(t *testing.T) {
	cfg := &GenerationConfig{
		Probe: ProbeConfig{
			Services: map[string]ProbeServiceSpec{
				"github": {
					URI: "ssh://github.com:22",
				},
			},
		},
	}

	_, err := resolveServiceGroupURL("github", ServiceGroupSpec{Probe: "github"}, cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != `probe service "github" uses non-http uri "ssh://github.com:22"; set probe.services.github.url-test or service-groups.github.url` {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestResolveServiceGroupURLUsesGroupNamedServiceWhenProbeNone(t *testing.T) {
	cfg := &GenerationConfig{
		Probe: ProbeConfig{
			Services: map[string]ProbeServiceSpec{
				"latency": {
					URI: "https://connectivitycheck.gstatic.com/generate_204",
				},
				"github": {
					URI:     "ssh://github.com:22",
					URLTest: "https://github.com/",
				},
			},
		},
	}

	got, err := resolveServiceGroupURL("github", ServiceGroupSpec{Probe: "none"}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://github.com/" {
		t.Fatalf("group url = %q, want https://github.com/", got)
	}
}

func TestSelectTailscaleInterfaceFromIfconfigPrefersRouteInterface(t *testing.T) {
	output := `utun4: flags=8051<UP,POINTOPOINT,RUNNING,MULTICAST> mtu 1380
	inet 100.64.0.8 --> 100.64.0.8 netmask 0xffffffff
utun5: flags=8051<UP,POINTOPOINT,RUNNING,MULTICAST> mtu 1380
	inet 100.71.2.3 --> 100.71.2.3 netmask 0xffffffff
`

	got, err := selectTailscaleInterfaceFromIfconfig("100.100.100.100", "utun5", output)
	if err != nil {
		t.Fatal(err)
	}
	if got != "utun5" {
		t.Fatalf("interface = %q, want utun5", got)
	}
}

func TestSelectTailscaleInterfaceFromIfconfigFallsBackToFirstUTUN(t *testing.T) {
	output := `utun3: flags=8051<UP,POINTOPOINT,RUNNING,MULTICAST> mtu 1380
	inet 100.80.1.2 --> 100.80.1.2 netmask 0xffffffff
utun7: flags=8051<UP,POINTOPOINT,RUNNING,MULTICAST> mtu 1380
	inet 100.90.1.2 --> 100.90.1.2 netmask 0xffffffff
`

	got, err := selectTailscaleInterfaceFromIfconfig("", "", output)
	if err != nil {
		t.Fatal(err)
	}
	if got != "utun3" {
		t.Fatalf("interface = %q, want utun3", got)
	}
}

func newTestService(repoRoot, configDir string) *Service {
	return &Service{
		Paths: Paths{
			RepoRoot:       repoRoot,
			TemplateConfig: filepath.Join(configDir, "mihomo.yaml.tmpl"),
			ValuesConfig:   filepath.Join(configDir, "values.yaml"),
		},
	}
}

func populateFreshGroupProbeStateForTests(t *testing.T, repoRoot, valuesPath string, state *ProbeState) {
	t.Helper()
	if state == nil {
		t.Fatal("probe state is nil")
	}
	cfg, err := LoadGenerationConfig(valuesPath)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadProviderCatalog(repoRoot, cfg.ProxyProviders)
	if err != nil {
		t.Fatal(err)
	}
	serviceDigests, err := BuildProbeServiceDigests(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if state.Groups == nil {
		state.Groups = map[string]GroupProbeState{}
	}
	for _, profileName := range probeProfileNames(cfg) {
		targetGroups, err := resolveTargetGroups(cfg, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, groupName := range targetGroups {
			groupSpec, ok := cfg.ServiceGroups[groupName]
			if !ok {
				continue
			}
			candidates, groupProfile, err := groupCandidatesForProfile(profileName, groupName, groupSpec, cfg, catalog, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(candidates) == 0 {
				continue
			}
			probeDigest, ok := serviceDigests[groupSpec.Probe]
			if !ok {
				t.Fatalf("missing probe digest for %s", groupSpec.Probe)
			}
			groupState := GroupProbeState{
				Group:           groupName,
				Profile:         profileName,
				Probe:           groupSpec.Probe,
				Digest:          groupProbeDigest(profileName, groupName, groupSpec, groupProfile, probeDigest),
				ProviderDigests: map[string]string{},
				Nodes:           map[string]GroupNodeState{},
			}
			for _, candidate := range candidates {
				groupState.ProviderDigests[candidate.Provider] = candidate.SubscriptionDigest
				providerState, ok := state.Providers[candidate.Provider]
				if !ok || providerState.SubscriptionDigest != candidate.SubscriptionDigest {
					continue
				}
				nodeState, ok := providerState.Nodes[candidate.Proxy.Name]
				if !ok || nodeState.Services == nil {
					continue
				}
				serviceState, ok := nodeState.Services[groupSpec.Probe]
				if !ok || !serviceState.OK || serviceState.ProbeDigest != probeDigest {
					continue
				}
				groupState.Nodes[candidate.Proxy.Name] = GroupNodeState{
					Provider: candidate.Provider,
					NodeName: candidate.Proxy.Name,
					Service:  serviceState,
				}
			}
			state.Groups[groupStateKey(profileName, groupName)] = groupState
		}
	}
}

func profileConfigPath(configDir, profile string) string {
	return filepath.Join(configDir, profile, "mihomo.yaml")
}

func LoadConfigString(content string) (Config, error) {
	var config Config
	if err := yaml.Unmarshal([]byte(content), &config); err != nil {
		return nil, err
	}
	if config == nil {
		config = Config{}
	}
	return config, nil
}

func assertStringSlice(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("slice len = %d, want %d (%v vs %v)", len(got), len(want), got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("slice[%d] = %q, want %q", index, got[index], want[index])
		}
	}
}

func assertGroupProxyNames(t *testing.T, config Config, want []string) {
	t.Helper()
	groups, ok := config["proxy-groups"].([]any)
	if !ok || len(groups) == 0 {
		t.Fatalf("proxy-groups = %#v", config["proxy-groups"])
	}
	group, ok := asMap(groups[0])
	if !ok {
		t.Fatalf("group type = %T", groups[0])
	}
	rawProxies, ok := group["proxies"].([]any)
	if !ok {
		t.Fatalf("group proxies type = %T", group["proxies"])
	}
	got := make([]string, 0, len(rawProxies))
	for _, item := range rawProxies {
		name, ok := item.(string)
		if !ok {
			t.Fatalf("group proxy item type = %T", item)
		}
		got = append(got, name)
	}
	assertStringSlice(t, got, want)
}

func assertAllGroupProxyNames(t *testing.T, config Config, want map[string][]string) {
	t.Helper()
	groups, ok := config["proxy-groups"].([]any)
	if !ok || len(groups) == 0 {
		t.Fatalf("proxy-groups = %#v", config["proxy-groups"])
	}

	gotByName := make(map[string][]string, len(groups))
	for _, item := range groups {
		group, ok := asMap(item)
		if !ok {
			t.Fatalf("group type = %T", item)
		}
		name, ok := group["name"].(string)
		if !ok {
			t.Fatalf("group name type = %T", group["name"])
		}
		rawProxies, ok := group["proxies"].([]any)
		if !ok {
			t.Fatalf("group proxies type = %T", group["proxies"])
		}
		names := make([]string, 0, len(rawProxies))
		for _, proxy := range rawProxies {
			proxyName, ok := proxy.(string)
			if !ok {
				t.Fatalf("group proxy item type = %T", proxy)
			}
			names = append(names, proxyName)
		}
		gotByName[name] = names
	}

	if len(gotByName) != len(want) {
		t.Fatalf("group count = %d, want %d", len(gotByName), len(want))
	}
	for groupName, expected := range want {
		got, ok := gotByName[groupName]
		if !ok {
			t.Fatalf("missing group %q", groupName)
		}
		assertStringSlice(t, got, expected)
	}
}

func assertAllGroupProxyNamesIgnoreOrder(t *testing.T, config Config, want map[string][]string) {
	t.Helper()
	groups, ok := config["proxy-groups"].([]any)
	if !ok || len(groups) == 0 {
		t.Fatalf("proxy-groups = %#v", config["proxy-groups"])
	}

	gotByName := make(map[string][]string, len(groups))
	for _, item := range groups {
		group, ok := asMap(item)
		if !ok {
			t.Fatalf("group type = %T", item)
		}
		name, ok := group["name"].(string)
		if !ok {
			t.Fatalf("group name type = %T", group["name"])
		}
		rawProxies, ok := group["proxies"].([]any)
		if !ok {
			t.Fatalf("group proxies type = %T", group["proxies"])
		}
		names := make([]string, 0, len(rawProxies))
		for _, proxy := range rawProxies {
			proxyName, ok := proxy.(string)
			if !ok {
				t.Fatalf("group proxy item type = %T", proxy)
			}
			names = append(names, proxyName)
		}
		sort.Strings(names)
		gotByName[name] = names
	}

	if len(gotByName) != len(want) {
		t.Fatalf("group count = %d, want %d", len(gotByName), len(want))
	}
	for groupName, expected := range want {
		got, ok := gotByName[groupName]
		if !ok {
			t.Fatalf("missing group %q", groupName)
		}
		sortedExpected := append([]string(nil), expected...)
		sort.Strings(sortedExpected)
		assertStringSlice(t, got, sortedExpected)
	}
}

func assertTopLevelProxyNames(t *testing.T, config Config, want []string) {
	t.Helper()
	raw, ok := config["proxies"].([]any)
	if !ok {
		t.Fatalf("proxies type = %T", config["proxies"])
	}
	got := make([]string, 0, len(raw))
	for _, item := range raw {
		proxy, ok := asMap(item)
		if !ok {
			t.Fatalf("proxy type = %T", item)
		}
		name, ok := proxy["name"].(string)
		if !ok {
			t.Fatalf("proxy name type = %T", proxy["name"])
		}
		got = append(got, name)
	}
	assertStringSlice(t, got, want)
}

func assertTopLevelProxyNamesIgnoreOrder(t *testing.T, config Config, want []string) {
	t.Helper()
	raw, ok := config["proxies"].([]any)
	if !ok {
		t.Fatalf("proxies type = %T", config["proxies"])
	}
	got := make([]string, 0, len(raw))
	for _, item := range raw {
		proxy, ok := asMap(item)
		if !ok {
			t.Fatalf("proxy type = %T", item)
		}
		name, ok := proxy["name"].(string)
		if !ok {
			t.Fatalf("proxy name type = %T", proxy["name"])
		}
		got = append(got, name)
	}
	sort.Strings(got)
	sortedWant := append([]string(nil), want...)
	sort.Strings(sortedWant)
	assertStringSlice(t, got, sortedWant)
}
