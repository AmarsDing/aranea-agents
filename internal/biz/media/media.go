// Package media defines biz-level ports for media generation provider
// configurations (text-to-image / text-to-video / image-to-video). The media
// provider system is independent of the LLM provider catalog: media
// generation is an asynchronous long-running task (submit → poll → fetch),
// configured via the media_providers table.
package media

import (
	"context"
	"encoding/json"
	"strings"
)

// Capability identifies a media generation capability used to match providers.
type Capability string

const (
	CapabilityImage       Capability = "image"
	CapabilityVideo       Capability = "video"
	CapabilityImageToVideo Capability = "image_to_video"
)

// ProviderConfig is one row of the media_providers catalog.
type ProviderConfig struct {
	ID           string
	Name         string
	ProviderType string // "qwen" / "comfyui_local" / "kling"
	BaseURL      string
	APIKey       string
	ConfigJSON   string
	Capabilities []string
	Status       string
}

// Supports reports whether the provider declares the given capability.
func (c ProviderConfig) Supports(cap Capability) bool {
	for _, v := range c.Capabilities {
		if strings.EqualFold(strings.TrimSpace(v), string(cap)) {
			return true
		}
	}
	return false
}

// Extra parses ConfigJSON into a map for provider-specific options.
// Returns an empty map for empty or invalid JSON (best-effort).
func (c ProviderConfig) Extra() map[string]any {
	raw := strings.TrimSpace(c.ConfigJSON)
	if raw == "" {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil || m == nil {
		return map[string]any{}
	}
	return m
}

// ProviderReader resolves active media provider configs by capability.
// Stability:evolving
type ProviderReader interface {
	// ActiveProviderFor returns the first active provider (by created_at)
	// supporting the capability. Returns a NotFound apierror when none match.
	ActiveProviderFor(ctx context.Context, cap Capability) (ProviderConfig, error)
}
