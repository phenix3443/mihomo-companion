package configgen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var darwinExternalUIDirFunc = defaultDarwinExternalUIDir

type Service struct {
	Paths Paths
}

type GeneratedArtifact struct {
	Label string
	Path  string
}

type GenerateResult struct {
	Artifacts []GeneratedArtifact
}

func NewService(repoRoot string) *Service {
	return &Service{
		Paths: Paths{
			RepoRoot:       repoRoot,
			TemplateConfig: filepath.Join(repoRoot, "config", "mihomo.yaml.tmpl"),
			ValuesConfig:   filepath.Join(repoRoot, "config", "values.yaml"),
		},
	}
}

func (s *Service) Generate(options GenerateOptions) (*GenerateResult, error) {
	cfg, err := LoadGenerationConfig(s.Paths.ValuesConfig)
	if err != nil {
		return nil, err
	}
	catalog, err := LoadProviderCatalog(s.Paths.RepoRoot, cfg.ProxyProviders)
	if err != nil {
		return nil, err
	}

	result := &GenerateResult{
		Artifacts: []GeneratedArtifact{},
	}
	for _, profile := range profilesForGeneration(cfg) {
		outputPath := s.Paths.OutputForProfile(profile.Name)
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			return nil, err
		}
		if err := s.generateProfile(profile.Name, profile.Platform, outputPath, options, cfg, catalog); err != nil {
			return nil, err
		}
		result.Artifacts = append(result.Artifacts, GeneratedArtifact{
			Label: profile.Name,
			Path:  outputPath,
		})
	}

	return result, nil
}

func (s *Service) generateProfile(profile, platform, outputPath string, options GenerateOptions, cfg *GenerationConfig, catalog *ProviderCatalog) error {
	renderData, err := s.buildRenderData(profile, platform, options, cfg, catalog)
	if err != nil {
		return err
	}
	rendered, err := RenderTemplate(s.Paths.TemplateConfig, renderData)
	if err != nil {
		return err
	}
	var parsed Config
	if err := yaml.Unmarshal([]byte(rendered), &parsed); err != nil {
		return fmt.Errorf("decode rendered %s config: %w", profile, err)
	}
	if err := Validate(parsed); err != nil {
		return err
	}
	return os.WriteFile(outputPath, []byte(rendered), 0o644)
}

func (s *Service) buildRenderData(profile, platform string, options GenerateOptions, cfg *GenerationConfig, catalog *ProviderCatalog) (RenderData, error) {
	externalUI := ""
	if platform == "linux" {
		externalUI = strings.TrimSpace(cfg.Template.ExternalUI["linux"])
	} else if platform == "macos" && profile == "local" {
		externalUI = darwinExternalUIDirFunc()
	}

	manualProxyNames := map[string]bool{}
	manualProxies := make([]any, 0, len(cfg.ManualProxies))
	seenProxyProviders := map[string]string{}
	feilian := map[string]any{}
	for _, spec := range cfg.ManualProxies {
		cloned := cloneMap(map[string]any(spec))
		name := stringValue(cloned["name"])
		if name != "" {
			manualProxyNames[name] = true
			seenProxyProviders[name] = "manual"
			if name == "feilian-proxy" {
				feilian = cloned
			}
		}
		manualProxies = append(manualProxies, cloned)
	}
	tunOptions := options
	if profile == "clash-verge" {
		tunOptions.EnableMacOSTUN = false
	}
	tun, iface, err := buildPlatformTun(platform, tunOptions, cfg.Tun, feilian)
	if err != nil {
		return RenderData{}, err
	}
	if iface != "" {
		feilian["interface-name"] = iface
	}

	groupConfigs := make([]OrderedMap, 0, len(cfg.GroupOrder))
	selectedProviderNodes := map[string]map[string]map[string]any{}
	for _, groupName := range cfg.GroupOrder {
		groupSpec, ok := cfg.ServiceGroups[groupName]
		if !ok {
			continue
		}
		groupProfile, ok := resolveServiceGroupProfile(groupSpec, profile)
		if !ok {
			continue
		}
		groupType := nonEmpty(groupSpec.Type, "url-test")
		groupConfigValues := map[string]any{
			"name":      groupName,
			"type":      groupType,
			"interval":  groupSpec.Interval,
			"tolerance": groupSpec.Tolerance,
			"lazy":      groupSpec.Lazy,
		}
		groupConfigKeys := []string{"name", "type", "interval", "tolerance", "lazy"}
		useProviders, filterPattern, excludePattern, err := buildRuntimeGroupProvidersAndFilters(groupProfile, groupSpec, cfg)
		if err != nil {
			return RenderData{}, fmt.Errorf("build runtime group %s: %w", groupName, err)
		}
		if len(useProviders) == 0 {
			return RenderData{}, fmt.Errorf("service group %s has no eligible providers on %s", groupName, profile)
		}
		groupConfigValues["use"] = toAnySlice(useProviders)
		groupConfigKeys = append(groupConfigKeys, "use")
		if filterPattern != "" {
			groupConfigValues["filter"] = filterPattern
			groupConfigKeys = append(groupConfigKeys, "filter")
		}
		excludeFilter, err := groupExcludeFilter(cfg, groupSpec, excludePattern)
		if err != nil {
			return RenderData{}, fmt.Errorf("build exclude-filter for group %s: %w", groupName, err)
		}
		if excludeFilter != "" {
			groupConfigValues["exclude-filter"] = excludeFilter
			groupConfigKeys = append(groupConfigKeys, "exclude-filter")
		}
		if groupType == "url-test" {
			groupURL := resolveServiceGroupURL(groupSpec)
			groupConfigValues["url"] = groupURL
			groupConfigKeys = append(groupConfigKeys, "url")
		}
		groupConfigs = append(groupConfigs, OrderedMap{
			Keys:   groupConfigKeys,
			Values: groupConfigValues,
		})
	}

	proxies := append([]any(nil), manualProxies...)
	for _, providerName := range cfg.ProviderOrder {
		nodes := selectedProviderNodes[providerName]
		if len(nodes) == 0 {
			continue
		}
		names := make([]string, 0, len(nodes))
		for name := range nodes {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			node := cloneMap(nodes[name])
			delete(node, "_provider")
			proxies = append(proxies, node)
		}
	}

	allowedNames := allowedProxyNamesFromGroups(toAnyOrderedGroups(groupConfigs))
	filteredProxies := make([]any, 0, len(proxies))
	for _, proxy := range proxies {
		proxyMap, ok := asMap(proxy)
		if !ok {
			filteredProxies = append(filteredProxies, proxy)
			continue
		}
		name, _ := proxyMap["name"].(string)
		if name == "" || allowedNames[name] || manualProxyNames[name] {
			filteredProxies = append(filteredProxies, proxy)
		}
	}
	proxies = filteredProxies

	filteredProviders := orderedProxyProviders(cfg)
	ruleProviders := orderedRuleProviders(cfg)
	groupConfigsAny := toAnyOrderedGroups(groupConfigs)
	proxyGroupNames := proxyGroupNameSet(groupConfigsAny)
	rules := filterRulesForProfile(cfg.Rules, groupConfigsAny)
	return RenderData{
		ExternalUI:      externalUI,
		Tun:             tun,
		Template:        cfg.Template,
		Proxies:         proxies,
		ProxyGroups:     OrderedList{Items: groupConfigs},
		ProxyGroupNames: proxyGroupNames,
		ProxyProviders:  filteredProviders,
		RuleProviders:   ruleProviders,
		Rules:           rules,
	}, nil
}

func defaultDarwinExternalUIDir() string {
	brewPath, err := exec.LookPath("brew")
	if err == nil {
		cmd := exec.Command(brewPath, "--prefix")
		output, outputErr := cmd.Output()
		if outputErr == nil {
			prefix := strings.TrimSpace(string(output))
			if prefix != "" {
				return filepath.Join(prefix, "etc", "mihomo", "ui")
			}
		}
	}

	configHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME"))
	if configHome == "" {
		homeDir, homeErr := os.UserHomeDir()
		if homeErr == nil {
			configHome = filepath.Join(homeDir, ".config")
		}
	}
	if configHome == "" {
		configHome = ".config"
	}
	return filepath.Join(configHome, "mihomo", "ui")
}

func allowedProxyNamesFromGroups(groups []any) map[string]bool {
	allowed := make(map[string]bool)
	for _, item := range groups {
		group, ok := asMap(item)
		if !ok {
			continue
		}
		proxies, ok := group["proxies"].([]any)
		if !ok {
			continue
		}
		for _, proxy := range proxies {
			name, ok := proxy.(string)
			if ok {
				allowed[name] = true
			}
		}
	}
	return allowed
}

func toAnyOrderedGroups(groups []OrderedMap) []any {
	items := make([]any, 0, len(groups))
	for _, group := range groups {
		items = append(items, group.Values)
	}
	return items
}

func toAnySlice(values []string) []any {
	items := make([]any, 0, len(values))
	for _, value := range values {
		items = append(items, value)
	}
	return items
}

func filterReservedProxyGroupMembers(values []any) []any {
	filtered := make([]any, 0, len(values))
	for _, value := range values {
		name, ok := value.(string)
		if ok && strings.EqualFold(strings.TrimSpace(name), "DIRECT") {
			continue
		}
		filtered = append(filtered, value)
	}
	return filtered
}

func buildRuntimeGroupProvidersAndFilters(groupProfile ServiceGroupProfileSpec, groupSpec ServiceGroupSpec, cfg *GenerationConfig) ([]string, string, string, error) {
	providers := make([]string, 0, len(cfg.ProviderOrder))
	for _, providerName := range cfg.ProviderOrder {
		if _, ok := cfg.ProxyProviders[providerName]; !ok {
			continue
		}
		if len(groupProfile.Providers) > 0 && !containsString(groupProfile.Providers, providerName) {
			continue
		}
		providers = append(providers, providerName)
	}
	filterValues := append([]string(nil), groupSpec.Match...)
	excludeValues := append([]string(nil), groupSpec.Exclude...)
	for _, providerName := range providers {
		if groupProfile.ProviderMatch != nil {
			filterValues = append(filterValues, groupProfile.ProviderMatch[providerName]...)
		}
		if groupProfile.ProviderExclude != nil {
			excludeValues = append(excludeValues, groupProfile.ProviderExclude[providerName]...)
		}
	}
	if _, err := compileRegexps(filterValues); err != nil {
		return nil, "", "", fmt.Errorf("compile runtime filter regex: %w", err)
	}
	if _, err := compileRegexps(excludeValues); err != nil {
		return nil, "", "", fmt.Errorf("compile runtime exclude regex: %w", err)
	}
	return providers, combineRegexPatterns(filterValues), combineRegexPatterns(excludeValues), nil
}

func combineRegexPatterns(values []string) string {
	seen := map[string]struct{}{}
	ordered := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		ordered = append(ordered, value)
	}
	if len(ordered) == 0 {
		return ""
	}
	if len(ordered) == 1 {
		return ordered[0]
	}
	parts := make([]string, 0, len(ordered))
	for _, value := range ordered {
		parts = append(parts, "(?:"+value+")")
	}
	return strings.Join(parts, "|")
}

func combineExcludeFilters(values ...string) string {
	return combineRegexPatterns(values)
}

func groupExcludeFilter(cfg *GenerationConfig, groupSpec ServiceGroupSpec, excludePattern string) (string, error) {
	unsupportedPatterns, err := unsupportedHighMultiplierPatterns(cfg, groupSpec)
	if err != nil {
		return "", err
	}
	if len(unsupportedPatterns) == 0 {
		return excludePattern, nil
	}
	values := make([]string, 0, len(unsupportedPatterns)+1)
	values = append(values, excludePattern)
	values = append(values, unsupportedPatterns...)
	return combineExcludeFilters(values...), nil
}

func unsupportedHighMultiplierPatterns(cfg *GenerationConfig, groupSpec ServiceGroupSpec) ([]string, error) {
	_ = cfg
	if len(groupSpec.MultiplierFilters) == 0 {
		if len(groupSpec.SupportedHighMultipliers) > 0 {
			return nil, fmt.Errorf("supported-high-multipliers requires multiplier-filters")
		}
		return nil, nil
	}
	supported := make(map[string]struct{}, len(groupSpec.SupportedHighMultipliers))
	for _, name := range groupSpec.SupportedHighMultipliers {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		pattern, ok := groupSpec.MultiplierFilters[name]
		if !ok {
			return nil, fmt.Errorf("unknown supported high multiplier %q", name)
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return nil, fmt.Errorf("compile high multiplier option %q: %w", name, err)
		}
		supported[name] = struct{}{}
	}

	optionNames := make([]string, 0, len(groupSpec.MultiplierFilters))
	for name := range groupSpec.MultiplierFilters {
		optionNames = append(optionNames, name)
	}
	if len(optionNames) == 0 {
		return nil, nil
	}
	sort.Strings(optionNames)

	patterns := make([]string, 0, len(optionNames))
	for _, name := range optionNames {
		pattern, ok := groupSpec.MultiplierFilters[name]
		if !ok {
			continue
		}
		if _, skip := supported[name]; skip {
			continue
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return nil, fmt.Errorf("compile high multiplier option %q: %w", name, err)
		}
		patterns = append(patterns, pattern)
	}
	return patterns, nil
}

func resolveServiceGroupURL(groupSpec ServiceGroupSpec) string {
	if groupSpec.URL != "" {
		return groupSpec.URL
	}
	return "https://connectivitycheck.gstatic.com/generate_204"
}

func providerRegexps(groupProfile ServiceGroupProfileSpec, providerName string) ([]*regexp.Regexp, []*regexp.Regexp, error) {
	var matchValues []string
	if groupProfile.ProviderMatch != nil {
		matchValues = groupProfile.ProviderMatch[providerName]
	}
	var excludeValues []string
	if groupProfile.ProviderExclude != nil {
		excludeValues = groupProfile.ProviderExclude[providerName]
	}
	matchers, err := compileRegexps(matchValues)
	if err != nil {
		return nil, nil, err
	}
	excluders, err := compileRegexps(excludeValues)
	if err != nil {
		return nil, nil, err
	}
	return matchers, excluders, nil
}

func matchesProxyName(name string, matchers, excluders []*regexp.Regexp) bool {
	if len(matchers) > 0 {
		matched := false
		for _, matcher := range matchers {
			if matcher.MatchString(name) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, matcher := range excluders {
		if matcher.MatchString(name) {
			return false
		}
	}
	return true
}

func compileRegexps(values []string) ([]*regexp.Regexp, error) {
	out := make([]*regexp.Regexp, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		compiled, err := regexp.Compile(value)
		if err != nil {
			return nil, err
		}
		out = append(out, compiled)
	}
	return out, nil
}

func asMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case Config:
		return map[string]any(typed), true
	default:
		return nil, false
	}
}

func buildPlatformTun(platform string, options GenerateOptions, tunSpec TunSpec, feilian map[string]any) (map[string]any, string, error) {
	switch platform {
	case "linux":
		if !options.EnableLinuxTUN {
			return nil, "", nil
		}
		tun := cloneMap(map[string]any(tunSpec.Linux))
		if len(tun) == 0 {
			return nil, "", fmt.Errorf("missing tun.linux config")
		}
		return tun, "", nil
	case "macos":
		if !options.EnableMacOSTUN {
			return nil, "", nil
		}
		tun := cloneMap(map[string]any(tunSpec.MacOS))
		if len(tun) == 0 {
			return nil, "", fmt.Errorf("missing tun.macos config")
		}
		iface := ""
		server := stringValue(feilian["server"])
		if strings.HasPrefix(server, "100.") {
			if detected, err := detectTailscaleInterface(server); err == nil {
				iface = detected
			}
		}
		exclude := anySliceValue(tun["exclude-interface"])
		if iface != "" && !containsAnyString(exclude, iface) {
			exclude = append(exclude, iface)
		}
		tun["exclude-interface"] = exclude
		return tun, iface, nil
	default:
		return nil, "", fmt.Errorf("unsupported platform: %s", platform)
	}
}

func anySliceValue(value any) []any {
	items, ok := value.([]any)
	if !ok {
		return []any{}
	}
	cloned := make([]any, 0, len(items))
	cloned = append(cloned, items...)
	return cloned
}

func containsAnyString(values []any, target string) bool {
	for _, value := range values {
		text, ok := value.(string)
		if ok && text == target {
			return true
		}
	}
	return false
}

func orderedProxyProviders(cfg *GenerationConfig) OrderedMap {
	values := map[string]any{}
	keys := []string{}
	for _, providerName := range cfg.ProviderOrder {
		spec, ok := cfg.ProxyProviders[providerName]
		if !ok {
			continue
		}
		keys = append(keys, providerName)
		values[providerName] = map[string]any{
			"type":     spec.Type,
			"url":      spec.URL,
			"interval": spec.Interval,
			"path":     spec.Path,
		}
	}
	return OrderedMap{Keys: keys, Values: values}
}

func orderedRuleProviders(cfg *GenerationConfig) OrderedMap {
	values := map[string]any{}
	keys := []string{}
	for _, providerName := range cfg.RuleProviderOrder {
		spec, ok := cfg.RuleProviders[providerName]
		if !ok {
			continue
		}
		keys = append(keys, providerName)
		values[providerName] = map[string]any{
			"type":     spec.Type,
			"behavior": spec.Behavior,
			"format":   spec.Format,
			"url":      spec.URL,
			"path":     spec.Path,
			"interval": spec.Interval,
		}
	}
	return OrderedMap{Keys: keys, Values: values}
}

func filterRulesForProfile(rules []string, groupConfigs []any) []string {
	availableGroups := proxyGroupNameSet(groupConfigs)

	filtered := make([]string, 0, len(rules))
	for _, rule := range rules {
		if shouldSkipRuleForMissingGroup(rule, availableGroups) {
			continue
		}
		filtered = append(filtered, rule)
	}
	return filtered
}

func proxyGroupNameSet(groupConfigs []any) map[string]bool {
	availableGroups := map[string]bool{}
	for _, raw := range groupConfigs {
		group, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := strings.TrimSpace(stringValue(group["name"]))
		if name == "" {
			continue
		}
		availableGroups[name] = true
	}
	return availableGroups
}

func shouldSkipRuleForMissingGroup(rule string, availableGroups map[string]bool) bool {
	parts := strings.Split(rule, ",")
	if len(parts) < 3 {
		return false
	}
	if strings.TrimSpace(parts[0]) != "RULE-SET" {
		return false
	}
	groupName := strings.TrimSpace(parts[2])
	if groupName == "" {
		return false
	}
	if groupName == "DIRECT" || groupName == "REJECT" || groupName == "GLOBAL" {
		return false
	}
	return !availableGroups[groupName]
}

func resolveServiceGroupProfile(groupSpec ServiceGroupSpec, profile string) (ServiceGroupProfileSpec, bool) {
	if len(groupSpec.Profiles) == 0 {
		return ServiceGroupProfileSpec{}, true
	}
	spec, ok := groupSpec.Profiles[profile]
	return spec, ok
}

type generationProfile struct {
	Name     string
	Platform string
}

func profilesForGeneration(cfg *GenerationConfig) []generationProfile {
	if len(cfg.ProfileOrder) == 0 {
		return []generationProfile{
			{Name: "linux", Platform: "linux"},
			{Name: "macos", Platform: "macos"},
		}
	}

	profiles := make([]generationProfile, 0, len(cfg.ProfileOrder))
	for _, profileName := range cfg.ProfileOrder {
		spec, ok := cfg.Profiles[profileName]
		if !ok {
			continue
		}
		profiles = append(profiles, generationProfile{
			Name:     profileName,
			Platform: spec.OS,
		})
	}
	return profiles
}

type OrderedMap struct {
	Keys   []string
	Values map[string]any
}

type OrderedList struct {
	Items []OrderedMap
}

func (o OrderedMap) MarshalYAML() (any, error) {
	node := &yaml.Node{Kind: yaml.MappingNode}
	for _, key := range o.Keys {
		value := o.Values[key]
		valueNode := &yaml.Node{}
		if err := valueNode.Encode(value); err != nil {
			return nil, err
		}
		node.Content = append(node.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!str",
			Value: key,
		}, valueNode)
	}
	return node, nil
}

func (o OrderedList) MarshalYAML() (any, error) {
	node := &yaml.Node{Kind: yaml.SequenceNode}
	for _, item := range o.Items {
		itemNodeAny, err := item.MarshalYAML()
		if err != nil {
			return nil, err
		}
		itemNode, ok := itemNodeAny.(*yaml.Node)
		if !ok {
			return nil, fmt.Errorf("ordered list item marshal returned %T, want *yaml.Node", itemNodeAny)
		}
		node.Content = append(node.Content, itemNode)
	}
	return node, nil
}

func DetectRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "cmd", "mihctl", "main.go")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("repository root not found from %s", dir)
		}
		dir = parent
	}
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func detectTailscaleInterface(serverIP string) (string, error) {
	if _, err := exec.LookPath("ifconfig"); err != nil {
		return "", err
	}

	var routeInterface string
	if serverIP != "" {
		if _, err := exec.LookPath("route"); err == nil {
			cmd := exec.Command("route", "-n", "get", serverIP)
			if output, err := cmd.Output(); err == nil {
				for _, line := range strings.Split(string(output), "\n") {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "interface:") {
						routeInterface = strings.TrimSpace(strings.TrimPrefix(line, "interface:"))
						break
					}
				}
			}
		}
	}

	output, err := exec.Command("ifconfig").Output()
	if err != nil {
		return "", err
	}
	return selectTailscaleInterfaceFromIfconfig(serverIP, routeInterface, string(output))
}

func selectTailscaleInterfaceFromIfconfig(serverIP, routeInterface, output string) (string, error) {
	type ifaceBlock struct {
		name   string
		has100 bool
	}

	var blocks []ifaceBlock
	var current *ifaceBlock
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, " ") {
			name := strings.TrimSuffix(strings.Split(line, ":")[0], ":")
			blocks = append(blocks, ifaceBlock{name: name})
			current = &blocks[len(blocks)-1]
			continue
		}
		if current == nil {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "inet 100.") {
			current.has100 = true
		}
	}

	for _, block := range blocks {
		if block.name == routeInterface && block.has100 {
			return block.name, nil
		}
	}
	for _, block := range blocks {
		if strings.HasPrefix(block.name, "utun") && block.has100 {
			return block.name, nil
		}
	}
	if routeInterface != "" {
		return "", fmt.Errorf("tailscale interface not found for %s via route interface %s", serverIP, routeInterface)
	}
	return "", fmt.Errorf("tailscale interface not found")
}
