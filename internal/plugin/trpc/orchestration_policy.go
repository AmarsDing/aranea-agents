package plugintrpc

import (
	"strings"

	"aranea-agents/internal/biz"
)

type PluginOrchestrationPath string

const (
	OrchestrationRunner PluginOrchestrationPath = "runner"
)

func ResolvePluginOrchestration(_ biz.Plugin) PluginOrchestrationPath {
	return OrchestrationRunner
}

func pluginDeclaresOnEvent(p biz.Plugin) bool {
	for _, pt := range p.CallbackPoints {
		if strings.EqualFold(strings.TrimSpace(pt), "on_event") {
			return true
		}
	}
	pts := BuiltinCallbackPoints(p.Key)
	for _, pt := range pts {
		if pt == "on_event" {
			return true
		}
	}
	return false
}
