package intent

import (
	"encoding/json"
	"fmt"
	"strings"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

const intentContextHeader = "Derived intent (align your plan and tools to this JSON):"

// SystemContextMessage builds a system message for injected turn context.
func SystemContextMessage(art *Artifact) trpcmodel.Message {
	if art == nil {
		return trpcmodel.Message{}
	}
	b, err := json.Marshal(art)
	if err != nil {
		return trpcmodel.Message{}
	}
	return trpcmodel.NewSystemMessage(fmt.Sprintf("%s\n%s", intentContextHeader, string(b)))
}

// RunOptionInject returns a RunOption that injects the intent artifact as system context.
func RunOptionInject(art *Artifact) trpcagent.RunOption {
	msg := SystemContextMessage(art)
	if msg.Role == "" {
		return func(*trpcagent.RunOptions) {}
	}
	return trpcagent.WithInjectedContextMessages([]trpcmodel.Message{msg})
}

// IsIntentContextContent reports whether text looks like injected intent JSON context.
func IsIntentContextContent(text string) bool {
	return strings.Contains(text, intentContextHeader)
}
