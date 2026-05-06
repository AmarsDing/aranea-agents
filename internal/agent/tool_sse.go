package agent

import (
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"

	"google.golang.org/adk/session"
)

// ChatToolUseSSE is the JSON shape for chat SSE event "tool_event" (see web/src/features/chat/api.ts).
type ChatToolUseSSE struct {
	ID         string         `json:"id"`
	Phase      string         `json:"phase,omitempty"`
	Status     string         `json:"status"`
	AgentID    string         `json:"agent_id"`
	AgentKey   string         `json:"agent_key"`
	AgentName  string         `json:"agent_name"`
	AgentIcon  string         `json:"agent_icon"`
	ToolName   string         `json:"tool_name"`
	ToolLabel  string         `json:"tool_label"`
	Arguments  map[string]any `json:"arguments,omitempty"`
	Result     map[string]any `json:"result,omitempty"`
	Error      string         `json:"error,omitempty"`
	OccurredAt string         `json:"occurred_at"`
	DurationMs int64          `json:"duration_ms,omitempty"`
}

// ChatToolSSERelay emits tool start/finish events on a chat stream.
type ChatToolSSERelay struct {
	stream    StreamEmitter
	startedAt map[string]time.Time
}

// NewChatToolSSERelay returns nil when stream is nil.
func NewChatToolSSERelay(stream StreamEmitter) *ChatToolSSERelay {
	if stream == nil {
		return nil
	}
	return &ChatToolSSERelay{
		stream:    stream,
		startedAt: make(map[string]time.Time),
	}
}

// SessionHasFunctionCalls reports whether ev carries model-issued function calls.
func SessionHasFunctionCalls(ev *session.Event) bool {
	if ev == nil || ev.LLMResponse.Content == nil {
		return false
	}
	for _, p := range ev.LLMResponse.Content.Parts {
		if p != nil && p.FunctionCall != nil {
			return true
		}
	}
	return false
}

// SessionHasFunctionResponses reports whether ev carries tool results.
func SessionHasFunctionResponses(ev *session.Event) bool {
	if ev == nil || ev.LLMResponse.Content == nil {
		return false
	}
	for _, p := range ev.LLMResponse.Content.Parts {
		if p != nil && p.FunctionResponse != nil {
			return true
		}
	}
	return false
}

func fillChatToolAgentMeta(p *ChatToolUseSSE, ag biz.Agent) {
	p.AgentID = ag.ID
	p.AgentKey = strings.TrimSpace(ag.AgentKey)
	p.AgentName = strings.TrimSpace(ag.DisplayName)
	if p.AgentName == "" {
		p.AgentName = p.AgentKey
	}
	p.AgentIcon = strings.TrimSpace(ag.Icon)
}

func toolResultErrMsg(resp map[string]any) string {
	if resp == nil {
		return ""
	}
	e, ok := resp["error"]
	if !ok || e == nil {
		return ""
	}
	switch v := e.(type) {
	case string:
		return strings.TrimSpace(v)
	case error:
		return strings.TrimSpace(v.Error())
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
}

// AgentForToolSSELabel maps an ADK event author to a biz.Agent row for UI metadata (id/name/icon).
func AgentForToolSSELabel(author string, root biz.Agent) biz.Agent {
	k := strings.TrimSpace(author)
	if k == "" || strings.EqualFold(k, "user") {
		return root
	}
	if strings.EqualFold(k, strings.TrimSpace(root.AgentKey)) {
		return root
	}
	return biz.Agent{
		AgentKey:    k,
		DisplayName: k,
	}
}

// EmitToolCalls emits one "running" tool_event per new function-call id.
func (r *ChatToolSSERelay) EmitToolCalls(ag biz.Agent, ev *session.Event) {
	if r == nil || r.stream == nil || ev == nil || ev.LLMResponse.Content == nil {
		return
	}
	for _, p := range ev.LLMResponse.Content.Parts {
		if p == nil || p.FunctionCall == nil {
			continue
		}
		fc := p.FunctionCall
		id := strings.TrimSpace(fc.ID)
		if id == "" {
			continue
		}
		if _, exists := r.startedAt[id]; exists {
			continue
		}
		r.startedAt[id] = time.Now()
		args := map[string]any{}
		if fc.Args != nil {
			for k, v := range fc.Args {
				args[k] = v
			}
		}
		payload := ChatToolUseSSE{
			ID:         id,
			Phase:      "before",
			Status:     "running",
			ToolName:   fc.Name,
			ToolLabel:  fc.Name,
			Arguments:  args,
			OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
		}
		fillChatToolAgentMeta(&payload, ag)
		_ = r.stream.Emit("tool_event", payload)
	}
}

// EmitToolResponses emits one success/failed tool_event per function response part.
func (r *ChatToolSSERelay) EmitToolResponses(ag biz.Agent, ev *session.Event) {
	if r == nil || r.stream == nil || ev == nil || ev.LLMResponse.Content == nil {
		return
	}
	for _, p := range ev.LLMResponse.Content.Parts {
		if p == nil || p.FunctionResponse == nil {
			continue
		}
		fr := p.FunctionResponse
		id := strings.TrimSpace(fr.ID)
		var d time.Duration
		if id != "" {
			if t0, ok := r.startedAt[id]; ok {
				d = time.Since(t0)
				delete(r.startedAt, id)
			}
		}
		st := "success"
		errMsg := toolResultErrMsg(fr.Response)
		if errMsg != "" {
			st = "failed"
		}
		payload := ChatToolUseSSE{
			ID:         id,
			Phase:      "after",
			Status:     st,
			ToolName:   fr.Name,
			ToolLabel:  fr.Name,
			Result:     fr.Response,
			Error:      errMsg,
			OccurredAt: time.Now().UTC().Format(time.RFC3339Nano),
			DurationMs: d.Milliseconds(),
		}
		fillChatToolAgentMeta(&payload, ag)
		_ = r.stream.Emit("tool_event", payload)
	}
}
