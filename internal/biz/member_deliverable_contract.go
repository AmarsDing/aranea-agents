package biz

import (
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz/shared"
)

// Member-level deliverable contract (MDC): intra-team counterpart of the
// inter-team DeliverableContract. Declared in team Definition JSON under
// "deliverable_contract"; enforced inside set_deliverable at the topic level
// (LLM-actionable, auto-correct) and advisory-checked at team completion.
// Philosophy mirrors P1: advisory, never blocking the run.

// MemberDeliverableEntry is one topic-scoped contract entry.
type MemberDeliverableEntry struct {
	Topic        string   `json:"topic"`                   // C3 topic namespace the entry governs
	Description  string   `json:"description,omitempty"`   // human/LLM-facing explanation
	Required     bool     `json:"required,omitempty"`      // completion-time: warn when the topic was never written
	RequiredKeys []string `json:"required_keys,omitempty"` // write-time: data must contain these keys
	SchemaJSON   string   `json:"schema_json,omitempty"`   // write-time: optional JSON Schema for data content
}

// MemberDeliverableContract is the team-level container.
type MemberDeliverableContract struct {
	Entries []MemberDeliverableEntry `json:"entries"`
}

// Member contract violation kinds produced by ValidateTopicData.
const (
	MemberContractViolationMissingKey = "missing_key"
	MemberContractViolationSchema     = "schema_violation"
)

// MemberContractViolation is a single structured violation, LLM-actionable.
type MemberContractViolation struct {
	Topic  string `json:"topic"`
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

// MemberContractViolationError is returned by set_deliverable when the
// written topic data violates the declared contract entry. The message lists
// every violation so the calling agent can correct and retry in one pass.
type MemberContractViolationError struct {
	Violations []MemberContractViolation
}

// Error renders an LLM-actionable Chinese message.
func (e *MemberContractViolationError) Error() string {
	var sb strings.Builder
	sb.WriteString("成员交付物契约校验未通过 —")
	for i, v := range e.Violations {
		if i > 0 {
			sb.WriteString("; ")
		}
		fmt.Fprintf(&sb, "topic %q: %s", v.Topic, v.Detail)
	}
	sb.WriteString("。请修正 data 后重试 set_deliverable")
	return sb.String()
}

// ParseMemberDeliverableContract parses the Definition JSON fragment; empty
// input yields nil (no contract). Invalid JSON is a hard error (fail-fast at
// team load, consistent with ParseDeliverableContracts).
func ParseMemberDeliverableContract(raw string) (*MemberDeliverableContract, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var c MemberDeliverableContract
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return nil, err
	}
	if len(c.Entries) == 0 {
		return nil, nil
	}
	return &c, nil
}

// EntryForTopic resolves the entry governing a topic, nil when undeclared.
func (c *MemberDeliverableContract) EntryForTopic(topic string) *MemberDeliverableEntry {
	if c == nil {
		return nil
	}
	for i := range c.Entries {
		if c.Entries[i].Topic == topic {
			return &c.Entries[i]
		}
	}
	return nil
}

// ValidateTopicData validates topic-written data against the matching entry.
// Returns nil when no entry governs the topic (legacy/uncontracted topics
// stay advisory). Schema validation reuses the C2 document validator and is
// skipped when SchemaJSON is empty.
func (c *MemberDeliverableContract) ValidateTopicData(topic string, data map[string]any) []MemberContractViolation {
	entry := c.EntryForTopic(topic)
	if entry == nil {
		return nil
	}
	var out []MemberContractViolation
	for _, key := range entry.RequiredKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := data[key]; !ok {
			out = append(out, MemberContractViolation{
				Topic:  topic,
				Kind:   MemberContractViolationMissingKey,
				Detail: fmt.Sprintf("缺少必需键 %q", key),
			})
		}
	}
	if strings.TrimSpace(entry.SchemaJSON) != "" {
		docJSON, err := json.Marshal(data)
		if err == nil {
			if verr := shared.ValidateDocumentAgainstSchema("member_deliverable", entry.SchemaJSON, string(docJSON)); verr != nil {
				out = append(out, MemberContractViolation{
					Topic:  topic,
					Kind:   MemberContractViolationSchema,
					Detail: fmt.Sprintf("内容不满足 schema 约束（%v）", verr),
				})
			}
		}
	}
	return out
}

// RequiredTopicsMissing lists entries marked Required whose topic is absent
// from the final deliverable map. Used by WriteDeliverablesToSession for the
// completion-time advisory warning (covers the "never called the tool"
// bypass without blocking the run).
func (c *MemberDeliverableContract) RequiredTopicsMissing(deliverable map[string]any) []string {
	if c == nil {
		return nil
	}
	var out []string
	for _, e := range c.Entries {
		if !e.Required {
			continue
		}
		if _, ok := deliverable[e.Topic]; !ok {
			out = append(out, e.Topic)
		}
	}
	return out
}
