package browser

// PlaywrightMCPConfig configures the Playwright MCP Server connection and
// the SSRF protection policy applied to browser navigation tools.
type PlaywrightMCPConfig struct {
	Command    string
	Args       []string
	Transport  string
	ServerURL  string
	TimeoutSec int
	Headless   *bool
	Vision     *bool
	Isolated   *bool

	// SSRF protection policy. When non-nil, the assembled browser ToolSet
	// is wrapped with a NavigationGuardedToolSet that validates URLs in
	// browser_navigate/browser_navigate_back/browser_navigate_forward
	// calls before forwarding them to the Playwright MCP Server.
	//
	// Defaults are security-first: loopback/private network/file URLs
	// are all blocked. Set the corresponding Allow* flags to relax.
	Navigation *NavigationPolicy

	// EnabledSubGroups restricts which browser sub-tool groups are exposed
	// to the agent. When empty (default), all sub-tools are enabled.
	// Valid values: "navigate", "interact", "observe", "tabs", "other".
	//
	// Example: to allow read-only browser automation (no interaction):
	//   EnabledSubGroups: []string{"navigate", "observe"}
	//
	// This filtering is applied at the ToolSet level, after SSRF validation
	// but before the framework's ToolDecorator. Filtered-out tools are never
	// visible to the agent.
	EnabledSubGroups []string
}

func BoolPtr(b bool) *bool { return &b }

func DefaultPlaywrightMCPConfig() PlaywrightMCPConfig {
	return PlaywrightMCPConfig{
		Command:   "npx",
		Args:      []string{"--yes", "@playwright/mcp@latest"},
		Transport: "stdio",
		Headless:  BoolPtr(true),
		Vision:    BoolPtr(false),
		Isolated:  BoolPtr(true),
	}
}

func (c PlaywrightMCPConfig) EffectiveHeadless() bool {
	if c.Headless == nil {
		return true
	}
	return *c.Headless
}

func (c PlaywrightMCPConfig) EffectiveVision() bool {
	if c.Vision == nil {
		return false
	}
	return *c.Vision
}

func (c PlaywrightMCPConfig) EffectiveIsolated() bool {
	if c.Isolated == nil {
		return true
	}
	return *c.Isolated
}

func (c PlaywrightMCPConfig) BuildArgs() []string {
	args := make([]string, 0, len(c.Args)+4)
	args = append(args, c.Args...)
	if c.EffectiveHeadless() {
		args = append(args, "--headless")
	}
	if c.EffectiveIsolated() {
		args = append(args, "--isolated")
	}
	if c.EffectiveVision() {
		args = append(args, "--caps", "vision")
	}
	return args
}

// EffectiveNavigationPolicy returns the navigation policy with normalized
// domains. When Navigation is nil, a zero-value policy is returned which
// blocks loopback/private network/file URLs by default (security-first).
func (c PlaywrightMCPConfig) EffectiveNavigationPolicy() NavigationPolicy {
	if c.Navigation == nil {
		return NavigationPolicy{}
	}
	return NavigationPolicy{
		AllowedDomains:  normalizeDomains(c.Navigation.AllowedDomains),
		BlockedDomains:  normalizeDomains(c.Navigation.BlockedDomains),
		AllowLoopback:   c.Navigation.AllowLoopback,
		AllowPrivateNet: c.Navigation.AllowPrivateNet,
		AllowFileURLs:   c.Navigation.AllowFileURLs,
	}
}
