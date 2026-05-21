package agent

import (
	"strings"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
)

// BizAgentRegistryOptions registers pre-built agents on the Runner lookup table
// (instances before factory fallback). Keys must match agent Info().Name / agent_key.
func BizAgentRegistryOptions(agents map[string]trpcagent.Agent) []trpcrunner.Option {
	if len(agents) == 0 {
		return nil
	}
	var opts []trpcrunner.Option
	for name, ag := range agents {
		name = strings.TrimSpace(name)
		if name == "" || ag == nil {
			continue
		}
		opts = append(opts, trpcrunner.WithAgent(name, ag))
	}
	return opts
}
