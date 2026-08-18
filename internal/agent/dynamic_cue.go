package agent

import trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"

// dynamicCueToolName is a local sentinel on trailing dynamic-cue messages.
// Consecutive-message merger skips any message with ToolName set, so a
// user-role cue that follows the real user turn is not concatenated into
// that turn's content (which would diverge the cacheable prefix on the
// next model call). The OpenAI/DeepSeek converter does not send ToolName
// on role=user, so the sentinel never leaves the process.
const dynamicCueToolName = "aranea.dynamic_cue"

// asDynamicCue builds a trailing per-turn cue as role=user so DeepSeek's
// system-prefix cache does not treat it as part of the prompt prefix.
func asDynamicCue(content string) trpcmodel.Message {
	msg := trpcmodel.NewUserMessage(content)
	msg.ToolName = dynamicCueToolName
	return msg
}

// appendDynamicCue appends a per-turn cue at the end of the message list.
func appendDynamicCue(msgs []trpcmodel.Message, content string) []trpcmodel.Message {
	return append(msgs, asDynamicCue(content))
}

// isDynamicCueMessage reports whether msg was injected by appendDynamicCue
// (or converted to a cue by the intent reorder hook).
func isDynamicCueMessage(msg trpcmodel.Message) bool {
	return msg.ToolName == dynamicCueToolName
}

// isPromptFixedMessage reports messages that compression must not evict as
// conversation history: the static system head and trailing dynamic cues.
func isPromptFixedMessage(msg trpcmodel.Message) bool {
	return msg.Role == trpcmodel.RoleSystem || isDynamicCueMessage(msg)
}
