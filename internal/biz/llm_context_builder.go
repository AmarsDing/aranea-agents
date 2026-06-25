package biz

import "context"

// LLMMessage is the minimal message shape required by LLM provider APIs.
// It is produced by BuildLLMContext from Activity records, replacing the
// legacy Message-table-based context construction.
type LLMMessage struct {
	Role       string // user / assistant / tool / system
	Content    string
	ToolCallID string // Role=tool only
	ToolName   string // Role=tool only
	// Name identifies the speaker for assistant messages (e.g. team member
	// agent_key). OpenAI/Anthropic APIs support a `name` field on assistant
	// messages; providers that don't support it silently ignore it.
	Name string
}

// BuildLLMContext reconstructs the LLM conversation context from Activity
// records for the given session+turn. It replaces the legacy Message-table
// query path, allowing the Chat module to depend solely on Activity.
//
// Role mapping (LLM APIs only accept user/assistant/tool/system):
//   - task    → user
//   - reply   → assistant (team member replies included; agent_key → Name)
//   - action  → tool      (ToolCallID + ToolName + ToolResult)
//   - notice  → system
//
// Activities are returned by ListBySessionTurn in emission order (seq, timestamp),
// which preserves the original conversation flow. Activities in terminal states
// (failed/cancelled) are still included so the LLM can see tool errors and
// reason about retry behavior; callers may filter them out if needed.
//
// This function performs no I/O of its own beyond the supplied Reader; it is
// safe to call from biz/usecase layers.
func BuildLLMContext(ctx context.Context, repo ActivityReader, sessionID, turnID string) ([]LLMMessage, error) {
	if repo == nil {
		return nil, nil
	}
	activities, err := repo.ListBySessionTurn(ctx, sessionID, turnID)
	if err != nil {
		return nil, err
	}

	messages := make([]LLMMessage, 0, len(activities))
	for _, a := range activities {
		switch a.Kind {
		case ActivityKindTask:
			if a.Content == "" {
				continue
			}
			messages = append(messages, LLMMessage{Role: "user", Content: a.Content})
		case ActivityKindReply:
			if a.Content == "" {
				continue
			}
			messages = append(messages, LLMMessage{
				Role:    "assistant",
				Content: a.Content,
				Name:    a.AgentKey,
			})
		case ActivityKindAction:
			// Skip actions without a tool call ID — they cannot be correlated
			// to an assistant tool_call by the LLM API.
			if a.ToolCallID == "" {
				continue
			}
			messages = append(messages, LLMMessage{
				Role:       "tool",
				Content:    a.ToolResult,
				ToolCallID: a.ToolCallID,
				ToolName:   a.ToolName,
			})
		case ActivityKindNotice:
			if a.Content == "" {
				continue
			}
			messages = append(messages, LLMMessage{Role: "system", Content: a.Content})
		}
	}
	return messages, nil
}
