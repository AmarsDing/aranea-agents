package compress

import (
	"encoding/json"
	"errors"
	"strings"
)

const MemoryExtractPromptVersion = "v1"

const MemoryExtractSystemPrompt = `You extract durable user-specific facts from chat messages for a long-term memory store.
Output JSON only with this schema:
{"facts":[{"statement":"...","topics":["optional-tag"]}]}

Rules:
- Include only stable preferences, identity, constraints, and confirmed facts about the user.
- Do not store secrets, passwords, API keys, or ephemeral one-off task details.
- Each statement must be self-contained and written in third person about the user when possible.
- Return at most 8 facts. Use {"facts":[]} when nothing is worth remembering.
`

const MemoryExtractPromptV2Version = "v2"

const MemoryExtractSystemPromptV2 = `You extract durable user-specific facts from chat messages for a long-term memory store.

## Output Format
Call the provided function "extract_memory_facts" with your results.
If the model does not support function calling, output JSON with this schema:
{"facts":[{"statement":"...","subject_type":"person|preference|event|concept","scope":"user|agent","confidence":0.9,"topics":["tag"],"is_pii_sensitive":false}],"no_facts_reason":""}

## Rules
- Include only stable preferences, identity, constraints, and confirmed facts about the user.
- Do NOT store secrets, passwords, API keys, or ephemeral one-off task details.
- Each statement must be self-contained and written in third person about the user when possible.
- Return at most 8 facts.
- Set "is_pii_sensitive" to true if the statement contains or implies personal identifiable information.
- Set "no_facts_reason" when returning zero facts to explain why (e.g. "only_greetings", "only_task_context", "already_known").
- "subject_type" categorizes the fact: person, preference, event, concept, or other.
- "scope" is "user" for cross-session facts, "agent" for agent-specific behavior.
- "confidence" ranges 0.0–1.0 reflecting how certain the fact is.
`

const ExtractMemoryFactsFunctionName = "extract_memory_facts"

var ExtractMemoryFactsFunctionSchema = map[string]any{
	"name":        ExtractMemoryFactsFunctionName,
	"description": "Extract durable user-specific facts from the conversation for long-term memory storage.",
	"parameters": map[string]any{
		"type": "object",
		"properties": map[string]any{
			"facts": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"statement":        map[string]any{"type": "string", "description": "The fact statement, self-contained, third person about the user"},
						"subject_type":     map[string]any{"type": "string", "enum": []string{"person", "preference", "event", "concept", "other"}, "description": "Category of the fact"},
						"scope":            map[string]any{"type": "string", "enum": []string{"user", "agent"}, "description": "Whether this fact applies to the user globally or only within this agent"},
						"confidence":       map[string]any{"type": "number", "description": "0.0-1.0 confidence in this fact"},
						"topics":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional tags"},
						"is_pii_sensitive": map[string]any{"type": "boolean", "description": "True if the statement contains or implies PII"},
					},
					"required": []string{"statement", "subject_type", "confidence"},
				},
			},
			"no_facts_reason": map[string]any{"type": "string", "description": "If no facts found, explain why: only_greetings, only_task_context, already_known, other"},
		},
		"required": []string{"facts"},
	},
}

var ErrEmptyMemoryTranscript = errors.New("memory extract: empty transcript")

type MemoryExtractFact struct {
	Statement      string   `json:"statement"`
	SubjectType    string   `json:"subject_type"`
	Scope          string   `json:"scope"`
	Confidence     float64  `json:"confidence"`
	Topics         []string `json:"topics"`
	IsPIISensitive bool     `json:"is_pii_sensitive"`
}

type memoryExtractPayload struct {
	Facts          []MemoryExtractFact `json:"facts"`
	NoFactsReason  string              `json:"no_facts_reason"`
}

func ParseMemoryExtractFunctionCallArgs(raw string) ([]MemoryExtractFact, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, "", nil
	}
	raw = stripJSONFence(raw)
	var payload memoryExtractPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, "", err
	}
	out := make([]MemoryExtractFact, 0, len(payload.Facts))
	for _, f := range payload.Facts {
		stmt := strings.TrimSpace(f.Statement)
		if stmt == "" {
			continue
		}
		if f.Confidence <= 0 {
			f.Confidence = 0.7
		}
		if f.SubjectType == "" {
			f.SubjectType = "other"
		}
		if f.Scope == "" {
			f.Scope = "user"
		}
		out = append(out, f)
	}
	return out, strings.TrimSpace(payload.NoFactsReason), nil
}

// ParseMemoryExtractJSON parses LLM output into fact statements (tolerates fenced JSON).
func ParseMemoryExtractJSON(raw string) ([]MemoryExtractFact, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	raw = stripJSONFence(raw)
	var payload memoryExtractPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}
	out := make([]MemoryExtractFact, 0, len(payload.Facts))
	for _, f := range payload.Facts {
		stmt := strings.TrimSpace(f.Statement)
		if stmt == "" {
			continue
		}
		if f.Confidence <= 0 {
			f.Confidence = 0.7
		}
		if f.SubjectType == "" {
			f.SubjectType = "other"
		}
		if f.Scope == "" {
			f.Scope = "user"
		}
		out = append(out, f)
	}
	return out, nil
}

func stripJSONFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```JSON")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	return strings.TrimSpace(s)
}

// BuildMemoryExtractTranscript formats user/assistant lines for extraction.
func BuildMemoryExtractTranscript(messages []struct{ Role, Content string }) string {
	var b strings.Builder
	for _, m := range messages {
		role := strings.ToUpper(strings.TrimSpace(m.Role))
		if role != "USER" && role != "ASSISTANT" {
			continue
		}
		content := strings.TrimSpace(m.Content)
		if content == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(role)
		b.WriteString(": ")
		b.WriteString(content)
	}
	return b.String()
}
