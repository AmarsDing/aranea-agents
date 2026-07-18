package media

import "fmt"

// ProviderConfig holds the configuration for a media provider instance.
type ProviderConfig struct {
	Name         string
	ProviderType string // "comfyui_local" / "qwen" / "kling"
	BaseURL      string
	APIKey       string
	Extra        map[string]any
}

// MediaProviderConstructor creates a MediaProvider from config.
type MediaProviderConstructor func(cfg ProviderConfig) (MediaProvider, error)

var providers = map[string]MediaProviderConstructor{}

// Register adds a provider constructor to the registry.
// Must be called during application startup (Wire provider or init).
func Register(name string, c MediaProviderConstructor) {
	providers[name] = c
}

// Get returns a MediaProvider by name.
func Get(name string, cfg ProviderConfig) (MediaProvider, error) {
	c, ok := providers[name]
	if !ok {
		return nil, fmt.Errorf("media provider %q not registered", name)
	}
	return c(cfg)
}

// RegisteredNames returns all registered provider names.
func RegisteredNames() []string {
	names := make([]string, 0, len(providers))
	for n := range providers {
		names = append(names, n)
	}
	return names
}
