package configgen

import (
	"encoding/base64"
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

func TestLoadProviderCatalogParsesJMSStyleSSSubscriptions(t *testing.T) {
	dir := t.TempDir()
	jmsPath := filepath.Join(dir, "providers", "jms.yaml")
	if err := os.MkdirAll(filepath.Dir(jmsPath), 0o755); err != nil {
		t.Fatal(err)
	}

	jmsSSLine := "ss://YWVzLTI1Ni1nY206ZGphb1NScWZ0djlINlNnVUAxNDQuMzQuMTY0LjEyODoxNzIxNQ#JMS-1404950@c69s1.portablesubmarines.com:17215"
	if err := os.WriteFile(jmsPath, []byte(base64.StdEncoding.EncodeToString([]byte(jmsSSLine+"\n"))), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog, err := LoadProviderCatalog(dir, map[string]ProxyProviderSpec{
		"jms": {Path: "./providers/jms.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	jms := catalog.Providers["jms"]
	if len(jms.Proxies) != 1 {
		t.Fatalf("jms proxy count = %d, want 1", len(jms.Proxies))
	}
	if got := stringValue(jms.Proxies[0].Config["type"]); got != "ss" {
		t.Fatalf("jms type = %q", got)
	}
	if got := stringValue(jms.Proxies[0].Config["name"]); got != "🇺🇸 美国-Los Angeles c69s1丨1x US" {
		t.Fatalf("jms name = %q", got)
	}
	if got := stringValue(jms.Proxies[0].Config["server"]); got != "144.34.164.128" {
		t.Fatalf("jms server = %q", got)
	}
	if got, ok := intValue(jms.Proxies[0].Config["port"]); !ok || got != 17215 {
		t.Fatalf("jms port = %v, ok=%v", jms.Proxies[0].Config["port"], ok)
	}
}

func TestLoadProviderCatalogParsesVMessSubscriptions(t *testing.T) {
	dir := t.TempDir()
	vmessPath := filepath.Join(dir, "providers", "vmess.yaml")
	if err := os.MkdirAll(filepath.Dir(vmessPath), 0o755); err != nil {
		t.Fatal(err)
	}

	vmessPayload := `{"ps":"JMS-1404950@c69s3.portablesubmarines.com:17215","port":"17215","id":"08980dda-c246-49df-8da4-66b9ccc6de13","aid":0,"net":"tcp","type":"none","tls":"none","add":"104.243.26.96"}`
	vmessLine := "vmess://" + base64.StdEncoding.EncodeToString([]byte(vmessPayload))
	if err := os.WriteFile(vmessPath, []byte(base64.StdEncoding.EncodeToString([]byte(vmessLine+"\n"))), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog, err := LoadProviderCatalog(dir, map[string]ProxyProviderSpec{
		"jms": {Path: "./providers/vmess.yaml"},
	})
	if err != nil {
		t.Fatal(err)
	}

	vmess := catalog.Providers["jms"]
	if len(vmess.Proxies) != 1 {
		t.Fatalf("vmess proxy count = %d, want 1", len(vmess.Proxies))
	}
	if got := stringValue(vmess.Proxies[0].Config["type"]); got != "vmess" {
		t.Fatalf("vmess type = %q", got)
	}
	if got := stringValue(vmess.Proxies[0].Config["name"]); got != "🇺🇸 美国-Los Angeles c69s3丨1x US" {
		t.Fatalf("vmess name = %q", got)
	}
	if got := stringValue(vmess.Proxies[0].Config["server"]); got != "104.243.26.96" {
		t.Fatalf("vmess server = %q", got)
	}
	if got, ok := intValue(vmess.Proxies[0].Config["port"]); !ok || got != 17215 {
		t.Fatalf("vmess port = %v, ok=%v", vmess.Proxies[0].Config["port"], ok)
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
		useProviders, ok := group["use"].([]any)
		if !ok || len(useProviders) != 1 {
			t.Fatalf("group use = %#v", group["use"])
		}
		if got, _ := useProviders[0].(string); got != "bywave" {
			t.Fatalf("group use[0] = %q, want bywave", got)
		}
		if _, exists := group["proxies"]; exists {
			t.Fatalf("runtime group should not render static proxies: %#v", group["proxies"])
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
    url: https://github.com/
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
	assertStringSlice(t, []string{group["use"].([]any)[0].(string), group["use"].([]any)[1].(string)}, []string{"bywave", "tag"})
	if got := stringValue(group["filter"]); got != "(香港|香港家宽|澳门|新加坡|日本|台湾|HK|MO|SG|JP|TW)" {
		t.Fatalf("group filter = %q", got)
	}
	if !strings.Contains(stringValue(group["exclude-filter"]), "(美国|英国|德国|法国|印度|韩国|土耳其|US|UK|DE|FR|IN|KR|TU)") {
		t.Fatalf("group exclude-filter = %q", stringValue(group["exclude-filter"]))
	}
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
	linuxUse, ok := linuxGroup["use"].([]any)
	if !ok || len(linuxUse) != 1 {
		t.Fatalf("linux group use = %#v", linuxGroup["use"])
	}
	if got, _ := linuxUse[0].(string); got != "bywave" {
		t.Fatalf("linux group use = %q, want bywave", got)
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
	macosUse, ok := macosGroup["use"].([]any)
	if !ok || len(macosUse) != 1 {
		t.Fatalf("macos group use = %#v", macosGroup["use"])
	}
	if got, _ := macosUse[0].(string); got != "tag" {
		t.Fatalf("macos group use = %q, want tag", got)
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
	useProviders, ok := group["use"].([]any)
	if !ok || len(useProviders) != 1 {
		t.Fatalf("group use = %#v", group["use"])
	}
	if got, _ := useProviders[0].(string); got != "jisu" {
		t.Fatalf("group use = %q, want jisu", got)
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

func TestGenerateRuntimeGroupDoesNotRequireProbeConfigOrState(t *testing.T) {
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
proxy-providers:
  bywave:
    type: http
    url: https://example.com/sub
    interval: 60
    path: ./providers/bywave.yaml
service-groups:
  stable:
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
		t.Fatalf("runtime group should not render static proxies: %#v", group["proxies"])
	}
	if got := stringValue(group["url"]); got != "https://connectivitycheck.gstatic.com/generate_204" {
		t.Fatalf("group url = %q, want connectivity check default", got)
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
proxy-providers:
  bywave:
    type: http
    url: https://example.com/sub
    interval: 60
    path: ./providers/bywave.yaml
service-groups:
  auto:
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
	for _, item := range groups {
		group, ok := asMap(item)
		if !ok {
			t.Fatalf("group type = %T", item)
		}
		got := stringValue(group["exclude-filter"])
		if !strings.Contains(got, "(?i)(?:^|[^0-9.])(?:x3|3x)(?:[^0-9.]|$)") {
			t.Fatalf("exclude-filter = %q, want x3 pattern", got)
		}
		if !strings.Contains(got, "(?i)(?:^|[^0-9.])(?:x5|5x)(?:[^0-9.]|$)") {
			t.Fatalf("exclude-filter = %q, want x5 pattern", got)
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
proxy-providers:
  bywave:
    type: http
    url: https://example.com/sub
    interval: 60
    path: ./providers/bywave.yaml
service-groups:
  auto:
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

	service := newTestService(repoRoot, configDir)
	if _, err := service.Generate(GenerateOptions{}); err != nil {
		t.Fatal(err)
	}

	filtered, err := LoadConfig(profileConfigPath(configDir, "local"))
	if err != nil {
		t.Fatal(err)
	}
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
proxy-providers:
  bywave:
    type: http
    url: https://example.com/sub
    interval: 60
    path: ./providers/bywave.yaml
service-groups:
  auto:
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

	service := newTestService(repoRoot, configDir)
	if _, err := service.Generate(GenerateOptions{}); err != nil {
		t.Fatal(err)
	}

	filtered, err := LoadConfig(profileConfigPath(configDir, "local"))
	if err != nil {
		t.Fatal(err)
	}
	groups, ok := filtered["proxy-groups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("proxy-groups = %#v", filtered["proxy-groups"])
	}
	group, ok := asMap(groups[0])
	if !ok {
		t.Fatalf("group type = %T", groups[0])
	}
	ef := stringValue(group["exclude-filter"])
	if strings.Contains(ef, "(?i)(?:^|[^0-9.])(?:x2|2x)(?:[^0-9.]|$)") {
		t.Fatalf("exclude-filter = %q, should not exclude supported x2", ef)
	}
	if !strings.Contains(ef, "(?i)(?:^|[^0-9.])(?:x3|3x)(?:[^0-9.]|$)") {
		t.Fatalf("exclude-filter = %q, want x3 pattern", ef)
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

func TestResolveServiceGroupURLUsesExplicitGroupURL(t *testing.T) {
	got := resolveServiceGroupURL(ServiceGroupSpec{URL: "https://github.com/"})
	if got != "https://github.com/" {
		t.Fatalf("group url = %q, want https://github.com/", got)
	}
}

func TestResolveServiceGroupURLFallsBackToDefaultLatencyURL(t *testing.T) {
	got := resolveServiceGroupURL(ServiceGroupSpec{})
	if got != "https://connectivitycheck.gstatic.com/generate_204" {
		t.Fatalf("group url = %q, want connectivity check default", got)
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
