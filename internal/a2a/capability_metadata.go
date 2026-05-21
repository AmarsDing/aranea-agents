package a2a

import "strings"

// CapabilityMetadata returns A2A message metadata carrying the requested capability name.
func CapabilityMetadata(capability string) map[string]any {
	capability = strings.TrimSpace(capability)
	if capability == "" {
		return nil
	}
	return map[string]any{MetadataKeyCapability: capability}
}
