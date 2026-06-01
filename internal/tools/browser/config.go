package browser

type PlaywrightMCPConfig struct {
	Command    string
	Args       []string
	Transport  string
	ServerURL  string
	TimeoutSec int
	Headless   *bool
	Vision     *bool
	Isolated   *bool
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
