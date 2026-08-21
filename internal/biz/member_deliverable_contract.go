package biz

import (
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz/shared"

	"golang.org/x/text/unicode/norm"
)

// NormalizeDeliverableTopic 归一化 C3 topic 命名空间键：去首尾空白 + NFC +
// 全角 ASCII 变体（U+FF01–U+FF5E）折叠为对应半角。LLM 依据契约文本转写
// topic 时可能产生码点差异（全角 Ａ vs 半角 A、组合字符序列、padding 空白），
// 写入侧（set_deliverable）、读取侧（get/ack_deliverable）与契约条目若不做
// 同规则归一，写键与读键错位导致 topic 过滤静默失效（2026-08-21 诗歌会话
// 根因）。不做大小写折叠——topic 是大小写敏感标识符，折叠会把契约声明的
// 不同 topic 静默合并。
func NormalizeDeliverableTopic(topic string) string {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return ""
	}
	topic = norm.NFC.String(topic)
	return strings.Map(func(r rune) rune {
		if r >= 0xFF01 && r <= 0xFF5E {
			return r - 0xFEE0
		}
		return r
	}, topic)
}

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

// MemberEntriesFromDeliverableContracts derives MDC entries from a team's
// inter-team deliverable contracts (F5, Phase 11). The topic maps 1:1 to the
// contract name — no slugging or re-mapping — so members (set_deliverable)
// and the spirit (get_deliverable / get_team_deliverable) can never disagree
// on topic names (12:33 root cause: spirit guessed the wrong topic).
//
// Entries are Required (completion-time advisory) and inherit the contract's
// SchemaJSON for write-time content validation; required_keys derive from the
// schema's top-level "required" array. Empty-name or schema-less contracts
// still produce entries (advisory-only when no schema). Contracts named after
// the reserved state keys ("summary"/"cognition") are skipped: set_deliverable
// rejects writes under reserved keys, so such entries would be unsatisfiable
// and only generate perpetual false warnings (TS9-BUG-4).
func MemberEntriesFromDeliverableContracts(contracts []DeliverableContract) []MemberDeliverableEntry {
	entries := make([]MemberDeliverableEntry, 0, len(contracts))
	for _, c := range contracts {
		name := strings.TrimSpace(c.Name)
		if name == "" || name == deliverableReservedKeySummary || name == deliverableReservedKeyCognition {
			continue
		}
		entries = append(entries, MemberDeliverableEntry{
			Topic:        name,
			Description:  strings.TrimSpace(c.Description),
			Required:     true,
			RequiredKeys: RequiredKeysFromSchema(c.SchemaJSON),
			SchemaJSON:   strings.TrimSpace(c.SchemaJSON),
		})
	}
	return entries
}

// RequiredKeysFromSchema extracts the top-level "required" array from a JSON
// Schema document. Returns nil for empty or invalid schemas (advisory-only
// path — never fail definition generation on a bad schema).
func RequiredKeysFromSchema(schemaJSON string) []string {
	schemaJSON = strings.TrimSpace(schemaJSON)
	if schemaJSON == "" {
		return nil
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
		return nil
	}
	out := make([]string, 0, len(schema.Required))
	for _, key := range schema.Required {
		if key = strings.TrimSpace(key); key != "" {
			out = append(out, key)
		}
	}
	return out
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
	// 契约条目 topic 与工具读写同规则归一：EntryForTopic/ValidateTopicData
	// 是精确匹配，条目不归一会让归一化后的写入绕过契约校验。
	for i := range c.Entries {
		c.Entries[i].Topic = NormalizeDeliverableTopic(c.Entries[i].Topic)
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
