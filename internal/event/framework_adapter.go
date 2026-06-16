package event

import (
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/event/contract"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
)

// FrameworkEventMeta carries turn-scoped metadata that supplements framework event fields
// when constructing an Envelope. This decouples the conversion function from
// turn-scoped state (ProjectMeta) while allowing callers to inject context.
type FrameworkEventMeta struct {
	SessionID          string
	RequestID          string
	InvocationID       string
	ParentInvocationID string
	TeamID             string
	Branch             string
	FilterKey          string
	Source             string
}

// FromFrameworkEvent creates an Envelope from a framework *trpcevent.Event,
// extracting all common fields (ID, Author, InvocationID, Branch, FilterKey,
// Tag, Version, Timestamp, Extensions, Actions) and overlaying turn-scoped
// metadata from meta.
//
// This is the single source of truth for framework Event → Envelope field
// mapping. Both EventProjector and Graph EventBridge should use this function
// instead of manually extracting fields.
func FromFrameworkEvent(ev *trpcevent.Event, meta FrameworkEventMeta, typ contract.EnvelopeType) contract.Envelope {
	env := contract.NewEnvelope(typ, ev.Author, meta.SessionID)

	// Framework event ID takes precedence (stable UUID).
	if ev.ID != "" {
		env.ID = ev.ID
	}

	// RequestID: meta provides turn-level default, framework event overrides.
	env.RequestID = meta.RequestID
	if ev.RequestID != "" {
		env.RequestID = ev.RequestID
	}

	// InvocationID: same overlay pattern.
	env.InvocationID = ev.InvocationID
	if meta.InvocationID != "" {
		env.InvocationID = meta.InvocationID
	}

	env.ParentInvocationID = ev.ParentInvocationID
	if meta.ParentInvocationID != "" {
		env.ParentInvocationID = meta.ParentInvocationID
	}

	// Branch/FilterKey: prefer non-empty value from either source.
	env.Branch = coalesceStr(ev.Branch, meta.Branch)
	env.FilterKey = coalesceStr(ev.FilterKey, meta.FilterKey)

	env.Tag = ev.Tag
	env.TeamID = meta.TeamID
	env.Version = ev.Version

	if !ev.Timestamp.IsZero() {
		env.Timestamp = ev.Timestamp.UTC().Format(time.RFC3339Nano)
	}

	// Extensions: framework uses map[string]json.RawMessage, project uses map[string]string.
	if len(ev.Extensions) > 0 {
		env.Extensions = make(map[string]string, len(ev.Extensions))
		for k, v := range ev.Extensions {
			// If the raw JSON is a simple string (e.g. `"foo"`), strip quotes.
			// Otherwise store the raw JSON as-is for downstream consumers to parse.
			if s := strings.Trim(string(v), `"`); len(s) != len(v)-2 || !isJSONString(v) {
				env.Extensions[k] = string(v)
			} else {
				env.Extensions[k] = s
			}
		}
	}

	// Actions: map framework EventActions to EnvelopeActions.
	if ev.Actions != nil {
		env.Actions = &contract.EnvelopeActions{
			SkipSummarization: ev.Actions.SkipSummarization,
		}
	}

	// Source from meta (project-specific, not in framework event).
	if src := strings.TrimSpace(meta.Source); src != "" {
		env.Source = src
	}

	return env
}

// isJSONString checks if raw JSON is a quoted string literal.
func isJSONString(raw json.RawMessage) bool {
	if len(raw) < 2 {
		return false
	}
	return raw[0] == '"' && raw[len(raw)-1] == '"'
}

// coalesceStr returns the first non-empty string.
func coalesceStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
