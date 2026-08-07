package evaluation

import (
	"encoding/json"
	"strings"

	"aranea-agents/pkg/loggateway"
)

// EvalTurn is one message in a multi-turn eval case (metadata_json.turns).
type EvalTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ExpectedTool is one expected tool call in metadata_json.expected_tools.
type ExpectedTool struct {
	Name      string `json:"name"`
	Arguments any    `json:"arguments,omitempty"`
	Result    any    `json:"result,omitempty"`
}

// CaseMetadata is parsed from eval_cases.metadata_json.
type CaseMetadata struct {
	ExpectedTools     []string         `json:"expected_tools"`
	ExpectedToolCalls []ExpectedTool   `json:"expected_tool_calls"`
	Turns             []EvalTurn       `json:"turns"`
	UserSimulation    *UserSimMetadata `json:"user_simulation"`
	// SessionUserID overrides the default SessionInput.UserID ("eval").
	SessionUserID string `json:"session_user_id"`
	// SessionState seeds SessionInput.State (initial session state).
	SessionState map[string]any `json:"session_state"`
}

// UserSimMetadata configures user simulation (scripted or LLM-driven).
type UserSimMetadata struct {
	Script           []string `json:"script"`
	UseLLM           bool     `json:"use_llm"`
	MaxInvocations   int      `json:"max_invocations"`
	ConversationPlan string   `json:"conversation_plan"`
}

// ParseCaseMetadata unmarshals metadata_json; invalid JSON yields zero value.
func ParseCaseMetadata(raw string, lg loggateway.Logger) CaseMetadata {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return CaseMetadata{}
	}
	var m CaseMetadata
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		lg.Warn("解析 eval case metadata 失败", loggateway.StepID("evaluation.case_metadata"), loggateway.Err(err))
	}
	return m
}

// HasMultiTurn reports whether the case defines explicit conversation turns.
func (m CaseMetadata) HasMultiTurn() bool {
	return len(m.Turns) > 0
}

func (m CaseMetadata) HasScriptedSimulation() bool {
	return m.UserSimulation != nil && len(m.UserSimulation.Script) > 0
}

func (m CaseMetadata) HasLLMSimulation() bool {
	if m.UserSimulation == nil {
		return false
	}
	if m.UserSimulation.UseLLM {
		return true
	}
	return len(m.UserSimulation.Script) == 0 && strings.TrimSpace(m.UserSimulation.ConversationPlan) != ""
}

func (m CaseMetadata) UserSimulationMaxInvocations() int {
	if m.UserSimulation == nil || m.UserSimulation.MaxInvocations <= 0 {
		return 5
	}
	return m.UserSimulation.MaxInvocations
}

func (m CaseMetadata) HasUserSimulation() bool {
	return m.HasScriptedSimulation() || m.HasLLMSimulation()
}

func (m CaseMetadata) expectedToolEntries() []ExpectedTool {
	if len(m.ExpectedToolCalls) > 0 {
		return m.ExpectedToolCalls
	}
	out := make([]ExpectedTool, 0, len(m.ExpectedTools))
	for _, name := range m.ExpectedTools {
		out = append(out, ExpectedTool{Name: name})
	}
	return out
}
