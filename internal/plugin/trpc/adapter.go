// Package plugintrpc bridges biz.Plugin DB rows to trpc-agent-go plugin.Plugin
// instances. Only plugins with known built-in keys are instantiated; unknown
// keys are silently skipped so that database experiments do not break the runner.
package plugintrpc

import (
	"strings"

	"aranea-agents/internal/biz"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
)

// builtin returns a concrete plugin.Plugin for known built-in plugin keys.
// Returns nil if the key has no matching implementation.
func builtin(p biz.Plugin) trpcplugin.Plugin {
	key := strings.ToLower(strings.TrimSpace(p.Key))
	switch key {
	case "audit_log", "audit-log", "auditlog":
		return &AuditLogPlugin{name: p.Key}
	default:
		return nil
	}
}

// adapt converts a biz.Plugin to a trpcplugin.Plugin.
// Returns nil when the plugin is disabled or has no built-in implementation.
func adapt(p biz.Plugin) trpcplugin.Plugin {
	if !p.Enabled {
		return nil
	}
	return builtin(p)
}
