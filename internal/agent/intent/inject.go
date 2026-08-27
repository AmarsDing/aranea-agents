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

// inputRiskNoticeHeader 是降级注入风险提示的消息头（S3-2）。
const inputRiskNoticeHeader = "Input safety notice (deterministic pre-scan):"

// RunOptionInjectInputRisk returns a RunOption that injects a caution notice when
// the intent pass produced no artifact (failed / timed out / skipped) but the
// deterministic input scan flagged risk (2026-08-28 方案② S3-2 降级注入)。
// 语义：本 turn 无 LLM 意图产物可参考，提醒主 LLM 对潜在破坏性操作先确认再执行；
// 不改变流程、不挂起（硬拦截保持在 L3 ParamRuleGate）。flags 为空时不注入。
func RunOptionInjectInputRisk(flags []string) trpcagent.RunOption {
	if len(flags) == 0 {
		return func(*trpcagent.RunOptions) {}
	}
	msg := trpcmodel.NewSystemMessage(fmt.Sprintf(
		"%s\nA deterministic pre-scan flagged this request as potentially destructive/irreversible (flags: %s). The intent-recognition pass produced no artifact this turn. Exercise extra caution: confirm with the user before executing dangerous or irreversible operations.",
		inputRiskNoticeHeader, strings.Join(flags, ",")))
	return trpcagent.WithInjectedContextMessages([]trpcmodel.Message{msg})
}
