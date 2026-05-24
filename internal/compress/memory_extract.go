package compress

import (
	"encoding/json"
	"errors"
	"strings"
)

// MemoryExtractPromptVersion labels the built-in memory extraction prompt.
const MemoryExtractPromptVersion = "v1"

// MemoryExtractSystemPrompt instructs the model to emit JSON facts only.
const MemoryExtractSystemPrompt = `You extract durable user-specific facts from chat messages for a long-term memory store.
Output JSON only with this schema:
{"facts":[{"statement":"...","topics":["optional-tag"]}]}

Rules:
- Include only stable preferences, identity, constraints, and confirmed facts about the user.
- Do not store secrets, passwords, API keys, or ephemeral one-off task details.
- Each statement must be self-contained and written in third person about the user when possible.
- Return at most 8 facts. Use {"facts":[]} when nothing is worth remembering.
`

var ErrEmptyMemoryTranscript = errors.New("memory extract: empty transcript")

type memoryExtractPayload struct {
	Facts []memoryExtractFact `json:"facts"`
}

type memoryExtractFact struct {
	Statement string   `json:"statement"`
	Topics    []string `json:"topics"`
}

// ParseMemoryExtractJSON parses LLM output into fact statements (tolerates fenced JSON).
func ParseMemoryExtractJSON(raw string) ([]memoryExtractFact, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	raw = stripJSONFence(raw)
	var payload memoryExtractPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}
	out := make([]memoryExtractFact, 0, len(payload.Facts))
	for _, f := range payload.Facts {
		stmt := strings.TrimSpace(f.Statement)
		if stmt == "" {
			continue
		}
		out = append(out, memoryExtractFact{Statement: stmt, Topics: f.Topics})
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
