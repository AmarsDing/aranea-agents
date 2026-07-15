package graph

import (
	"fmt"
)

// StateKeyControlCommand is the graph state key where AfterNode stores the
// latest ControlCommand when a replan decision is applied. Downstream
// observers and future executor hooks can detect retry/fallback without
// mistaking soft string recovery for successful node content.
const StateKeyControlCommand = "__aranea_control_command"

// ControlCommand is a structured replan control signal produced by the
// RuntimeReplanner AfterNode adapter (C-23).
//
// Framework note: trpc-agent-go AfterNode can recover a failed node by
// returning (customResult, nil), which clears nodeErr and continues the
// graph. True node re-invocation is not exposed from AfterNode (retries
// must be configured via RetryPolicy before the final failure). Returning
// a ControlCommand (instead of a "[recovered]" string) stops pretending
// soft string recovery is successful agent content — consumers MUST check
// IsControlCommand / AsControlCommand before treating a result as output.
type ControlCommand struct {
	// Action is the replan type (retry, insert_fallback, …).
	Action ReplanType `json:"action"`
	// NodeID is the failed node that triggered the control signal.
	NodeID string `json:"node_id"`
	// Cause is the original node error message (best-effort).
	Cause string `json:"cause,omitempty"`
	// FallbackAgent is set for insert_fallback when a concrete agent key
	// is known (from ReplanAction.NewNodes or node FallbackAgent).
	FallbackAgent string `json:"fallback_agent,omitempty"`
	// AttemptAllowed is true when the replanner still had budget for this
	// decision (max attempts not exceeded at decision time).
	AttemptAllowed bool `json:"attempt_allowed"`
}

// IsControlCommand reports whether v is a ControlCommand (value or pointer).
func IsControlCommand(v any) bool {
	_, ok := AsControlCommand(v)
	return ok
}

// AsControlCommand extracts a ControlCommand from an AfterNode result.
func AsControlCommand(v any) (ControlCommand, bool) {
	switch c := v.(type) {
	case ControlCommand:
		return c, true
	case *ControlCommand:
		if c == nil {
			return ControlCommand{}, false
		}
		return *c, true
	default:
		return ControlCommand{}, false
	}
}

// String implements fmt.Stringer for logs (never use as synthesized content).
func (c ControlCommand) String() string {
	return fmt.Sprintf("ControlCommand{action=%s node=%s fallback=%s allowed=%v}",
		c.Action, c.NodeID, c.FallbackAgent, c.AttemptAllowed)
}

// NewControlCommand builds a ControlCommand from a ReplanAction and failure context.
func NewControlCommand(action *ReplanAction, nodeID string, cause error) ControlCommand {
	cmd := ControlCommand{
		NodeID:         nodeID,
		AttemptAllowed: true,
	}
	if action != nil {
		cmd.Action = action.Type
		cmd.FallbackAgent = fallbackAgentFromAction(action)
	}
	if cause != nil {
		cmd.Cause = cause.Error()
	}
	return cmd
}

func fallbackAgentFromAction(action *ReplanAction) string {
	if action == nil {
		return ""
	}
	for _, n := range action.NewNodes {
		if n.AgentName != "" {
			return n.AgentName
		}
		if n.FallbackAgent != "" {
			return n.FallbackAgent
		}
	}
	return ""
}
