package configgen

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type ParsedProxy struct {
	Provider string
	Name     string
	Config   map[string]any
}

type ProviderSnapshot struct {
	Name    string
	Digest  string
	Raw     []byte
	Proxies []ParsedProxy
}

type ProviderCatalog struct {
	Providers map[string]ProviderSnapshot
}

func LoadProviderCatalog(repoRoot string, specs map[string]ProxyProviderSpec) (*ProviderCatalog, error) {
	catalog := &ProviderCatalog{Providers: make(map[string]ProviderSnapshot, len(specs))}
	for name, spec := range specs {
		path := spec.Path
		if path == "" {
			continue
		}
		absPath := filepath.Join(repoRoot, strings.TrimPrefix(path, "./"))
		snapshot, err := loadProviderSnapshot(name, absPath)
		if err != nil {
			return nil, err
		}
		catalog.Providers[name] = snapshot
	}
	return catalog, nil
}

func loadProviderSnapshot(name, path string) (ProviderSnapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ProviderSnapshot{}, fmt.Errorf("read provider %s: %w", path, err)
	}
	digest := sha256.Sum256(data)

	proxies, err := parseProviderBytes(name, data)
	if err != nil {
		return ProviderSnapshot{}, fmt.Errorf("parse provider %s: %w", path, err)
	}
	return ProviderSnapshot{
		Name:    name,
		Digest:  fmt.Sprintf("%x", digest[:]),
		Raw:     append([]byte(nil), data...),
		Proxies: proxies,
	}, nil
}

func parseProviderBytes(provider string, data []byte) ([]ParsedProxy, error) {
	var decoded any
	if err := yaml.Unmarshal(data, &decoded); err == nil {
		switch typed := decoded.(type) {
		case map[string]any:
			if raw, ok := typed["proxies"].([]any); ok {
				return parseYAMLProxies(provider, raw)
			}
		case []any:
			return parseYAMLProxies(provider, typed)
		}
	}

	trimmed := strings.TrimSpace(string(data))
	payload, err := decodeSubscriptionPayload(trimmed)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(payload, "\n")
	proxies := make([]ParsedProxy, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parsed, err := parseSubscriptionLine(provider, line)
		if err != nil {
			return nil, err
		}
		proxies = append(proxies, parsed)
	}
	return proxies, nil
}

func parseYAMLProxies(provider string, raw []any) ([]ParsedProxy, error) {
	proxies := make([]ParsedProxy, 0, len(raw))
	for _, item := range raw {
		proxy, ok := asMap(item)
		if !ok {
			continue
		}
		name, _ := proxy["name"].(string)
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("provider %s contains proxy without name", provider)
		}
		proxies = append(proxies, ParsedProxy{
			Provider: provider,
			Name:     name,
			Config:   cloneMap(proxy),
		})
	}
	return proxies, nil
}

func decodeSubscriptionPayload(value string) (string, error) {
	normalized := strings.TrimSpace(value)
	normalized = strings.ReplaceAll(normalized, "\n", "")
	decoded, err := base64.StdEncoding.DecodeString(normalized + strings.Repeat("=", (4-len(normalized)%4)%4))
	if err != nil {
		return "", fmt.Errorf("decode base64 subscription: %w", err)
	}
	return string(decoded), nil
}

func parseSubscriptionLine(provider, line string) (ParsedProxy, error) {
	switch {
	case strings.HasPrefix(line, "ss://"):
		return parseSSProxy(provider, line)
	case strings.HasPrefix(line, "vmess://"):
		return parseVMessProxy(provider, line)
	case strings.HasPrefix(line, "vless://"):
		return parseVLESSProxy(provider, line)
	case strings.HasPrefix(line, "hysteria2://"):
		return parseHysteria2Proxy(provider, line)
	default:
		return ParsedProxy{}, fmt.Errorf("provider %s contains unsupported subscription scheme", provider)
	}
}

func parseSSProxy(provider, raw string) (ParsedProxy, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return ParsedProxy{}, err
	}
	name := decodeFragment(u.Fragment)
	if beforeAt, _, ok := strings.Cut(name, "@"); ok {
		name = beforeAt
	}

	method, password, server, portValue, err := parseSSUserInfo(u)
	if err != nil {
		return ParsedProxy{}, err
	}
	if server == "" {
		server = u.Hostname()
	}
	if portValue == 0 {
		portValue, err = strconv.Atoi(u.Port())
		if err != nil {
			return ParsedProxy{}, fmt.Errorf("invalid ss port in %q", raw)
		}
	}

	cfg := map[string]any{
		"name":     name,
		"type":     "ss",
		"server":   server,
		"port":     portValue,
		"cipher":   method,
		"password": password,
		"udp":      true,
	}
	applySSPluginOptions(cfg, u.Query().Get("plugin"))
	return ParsedProxy{Provider: provider, Name: name, Config: cfg}, nil
}

func parseSSUserInfo(u *url.URL) (string, string, string, int, error) {
	if u.User == nil && u.Host != "" && u.Port() == "" {
		decoded, err := decodeBase64URLValue(u.Host)
		if err == nil {
			method, password, server, port, err := parseSSDecodedPayload(decoded)
			if err == nil {
				return method, password, server, port, nil
			}
		}
	}
	if u.User == nil {
		return "", "", "", 0, fmt.Errorf("ss uri missing user info")
	}
	if password, ok := u.User.Password(); ok {
		return u.User.Username(), password, "", 0, nil
	}
	decoded, err := decodeBase64URLValue(u.User.Username())
	if err != nil {
		return "", "", "", 0, fmt.Errorf("decode ss userinfo: %w", err)
	}
	return parseSSDecodedPayload(decoded)
}

func parseSSDecodedPayload(decoded string) (string, string, string, int, error) {
	if credentials, endpoint, ok := strings.Cut(decoded, "@"); ok {
		parts := strings.SplitN(credentials, ":", 2)
		if len(parts) != 2 {
			return "", "", "", 0, fmt.Errorf("invalid ss userinfo payload")
		}
		host, portText, err := net.SplitHostPort(endpoint)
		if err != nil {
			return "", "", "", 0, fmt.Errorf("invalid ss endpoint in userinfo")
		}
		portValue, err := strconv.Atoi(portText)
		if err != nil {
			return "", "", "", 0, fmt.Errorf("invalid ss port in userinfo")
		}
		return parts[0], parts[1], host, portValue, nil
	}

	parts := strings.SplitN(decoded, ":", 2)
	if len(parts) != 2 {
		return "", "", "", 0, fmt.Errorf("invalid ss userinfo payload")
	}
	return parts[0], parts[1], "", 0, nil
}

func applySSPluginOptions(cfg map[string]any, raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}
	decoded, err := url.QueryUnescape(raw)
	if err != nil {
		decoded = raw
	}
	parts := strings.Split(decoded, ";")
	if len(parts) == 0 {
		return
	}
	cfg["plugin"] = normalizeSSPluginName(parts[0])
	pluginOpts := map[string]any{}
	for _, part := range parts[1:] {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		switch key {
		case "obfs":
			pluginOpts["mode"] = value
		case "obfs-host":
			pluginOpts["host"] = value
		case "tfo":
			cfg["tfo"] = value == "1" || strings.EqualFold(value, "true")
		default:
			pluginOpts[key] = value
		}
	}
	if len(pluginOpts) > 0 {
		cfg["plugin-opts"] = pluginOpts
	}
}

func normalizeSSPluginName(name string) string {
	switch strings.TrimSpace(name) {
	case "obfs-local":
		return "obfs"
	default:
		return name
	}
}

func parseVMessProxy(provider, raw string) (ParsedProxy, error) {
	payload := strings.TrimPrefix(strings.TrimSpace(raw), "vmess://")
	decoded, err := decodeBase64URLValue(payload)
	if err != nil {
		return ParsedProxy{}, fmt.Errorf("decode vmess payload: %w", err)
	}

	var node struct {
		Name     string `json:"ps"`
		Server   string `json:"add"`
		Port     string `json:"port"`
		UUID     string `json:"id"`
		AlterID  int    `json:"aid"`
		Network  string `json:"net"`
		Type     string `json:"type"`
		TLS      string `json:"tls"`
		Host     string `json:"host"`
		Path     string `json:"path"`
		SNI      string `json:"sni"`
		ServerNI string `json:"servername"`
		Cipher   string `json:"scy"`
	}
	if err := json.Unmarshal([]byte(decoded), &node); err != nil {
		return ParsedProxy{}, fmt.Errorf("decode vmess json: %w", err)
	}

	portValue, err := strconv.Atoi(strings.TrimSpace(node.Port))
	if err != nil {
		return ParsedProxy{}, fmt.Errorf("invalid vmess port in %q", raw)
	}

	name := node.Name
	if beforeAt, _, ok := strings.Cut(name, "@"); ok {
		name = beforeAt
	}

	cfg := map[string]any{
		"name":    name,
		"type":    "vmess",
		"server":  strings.TrimSpace(node.Server),
		"port":    portValue,
		"uuid":    strings.TrimSpace(node.UUID),
		"alterId": node.AlterID,
		"cipher":  nonEmpty(strings.TrimSpace(node.Cipher), "auto"),
		"udp":     true,
	}
	if network := strings.TrimSpace(node.Network); network != "" && network != "tcp" {
		cfg["network"] = network
	}
	if strings.EqualFold(strings.TrimSpace(node.TLS), "tls") {
		cfg["tls"] = true
	}
	if serverName := nonEmpty(strings.TrimSpace(node.SNI), strings.TrimSpace(node.ServerNI)); serverName != "" {
		cfg["servername"] = serverName
	}
	if network := strings.TrimSpace(node.Network); network == "ws" {
		wsOpts := map[string]any{}
		if path := strings.TrimSpace(node.Path); path != "" {
			wsOpts["path"] = path
		}
		if host := strings.TrimSpace(node.Host); host != "" {
			wsOpts["headers"] = map[string]any{"Host": host}
		}
		if len(wsOpts) > 0 {
			cfg["ws-opts"] = wsOpts
		}
	}

	return ParsedProxy{Provider: provider, Name: name, Config: cfg}, nil
}

func parseVLESSProxy(provider, raw string) (ParsedProxy, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return ParsedProxy{}, err
	}
	portValue, err := strconv.Atoi(u.Port())
	if err != nil {
		return ParsedProxy{}, fmt.Errorf("invalid vless port in %q", raw)
	}
	q := u.Query()
	name := decodeFragment(u.Fragment)
	cfg := map[string]any{
		"name":   name,
		"type":   "vless",
		"server": u.Hostname(),
		"port":   portValue,
		"uuid":   u.User.Username(),
		"udp":    true,
	}
	if flow := q.Get("flow"); flow != "" {
		cfg["flow"] = flow
	}
	if encryption := q.Get("encryption"); encryption != "" {
		cfg["encryption"] = encryption
	}
	if network := q.Get("type"); network != "" {
		cfg["network"] = network
	}
	if security := q.Get("security"); security == "tls" || security == "reality" {
		cfg["tls"] = true
	}
	if serverName := nonEmpty(q.Get("sni"), q.Get("servername")); serverName != "" {
		cfg["servername"] = serverName
	}
	if fp := nonEmpty(q.Get("fp"), q.Get("fingerprint")); fp != "" {
		cfg["client-fingerprint"] = fp
	}
	if q.Get("allowInsecure") == "1" || strings.EqualFold(q.Get("allowInsecure"), "true") || q.Get("insecure") == "1" {
		cfg["skip-cert-verify"] = true
	}
	if q.Get("security") == "reality" {
		cfg["reality-opts"] = map[string]any{
			"public-key": q.Get("pbk"),
			"short-id":   q.Get("sid"),
		}
	}
	if network := q.Get("type"); network == "ws" {
		wsOpts := map[string]any{}
		if path := q.Get("path"); path != "" {
			wsOpts["path"] = path
		}
		if host := q.Get("host"); host != "" {
			wsOpts["headers"] = map[string]any{"Host": host}
		}
		if len(wsOpts) > 0 {
			cfg["ws-opts"] = wsOpts
		}
	}
	return ParsedProxy{Provider: provider, Name: name, Config: cfg}, nil
}

func parseHysteria2Proxy(provider, raw string) (ParsedProxy, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return ParsedProxy{}, err
	}
	portValue, err := strconv.Atoi(u.Port())
	if err != nil {
		return ParsedProxy{}, fmt.Errorf("invalid hysteria2 port in %q", raw)
	}
	q := u.Query()
	name := decodeFragment(u.Fragment)
	cfg := map[string]any{
		"name":     name,
		"type":     "hysteria2",
		"server":   u.Hostname(),
		"port":     portValue,
		"password": u.User.Username(),
		"udp":      true,
	}
	if ports := q.Get("mport"); ports != "" {
		cfg["ports"] = ports
	}
	if sni := q.Get("sni"); sni != "" {
		cfg["sni"] = sni
	}
	if q.Get("insecure") == "1" || strings.EqualFold(q.Get("insecure"), "true") {
		cfg["skip-cert-verify"] = true
	}
	if obfs := q.Get("obfs"); obfs != "" {
		cfg["obfs"] = obfs
	}
	if obfsPassword := q.Get("obfs-password"); obfsPassword != "" {
		cfg["obfs-password"] = obfsPassword
	}
	if fp := nonEmpty(q.Get("fp"), q.Get("fingerprint")); fp != "" {
		cfg["fingerprint"] = fp
	}
	return ParsedProxy{Provider: provider, Name: name, Config: cfg}, nil
}

func decodeFragment(value string) string {
	decoded, err := url.QueryUnescape(value)
	if err != nil {
		return value
	}
	return decoded
}

func decodeBase64URLValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "//")
	for _, encoding := range []*base64.Encoding{base64.RawURLEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.StdEncoding} {
		if decoded, err := encoding.DecodeString(value); err == nil {
			return string(decoded), nil
		}
	}
	return "", fmt.Errorf("invalid base64 value")
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func nonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizeYAMLBytes(value []byte) []byte {
	return bytes.TrimSpace(value)
}

func sortUniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func splitHostPortDefault(addr string, defaultPort int) (string, int, error) {
	host, port, err := net.SplitHostPort(addr)
	if err == nil {
		intPort, convErr := strconv.Atoi(port)
		if convErr != nil {
			return "", 0, convErr
		}
		return host, intPort, nil
	}
	return addr, defaultPort, nil
}
