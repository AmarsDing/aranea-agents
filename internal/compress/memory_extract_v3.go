package compress

import (
	"encoding/json"
	"strings"
)

// V3 extraction prompt (P1-3): subject_type vocabulary aligned with the
// unified write pipeline's kind whitelist (preference/profile/goal/
// constraint/decision/relationship). Ephemeral event/concept kinds are no
// longer offered — "what the user did" belongs to L2 episodes, not L3 facts.
const MemoryExtractPromptV3Version = "v3"

const MemoryExtractSystemPromptV3 = `You extract durable user-specific facts from chat messages for a long-term memory store.

## Output Format
Call the provided function "extract_memory_facts" with your results.
If the model does not support function calling, output JSON with this schema:
{"facts":[{"statement":"...","subject_type":"person|preference|goal","scope":"user|agent","confidence":0.9,"topics":["tag"],"is_pii_sensitive":false}],"no_facts_reason":""}

## Rules
- Include only durable facts worth governing: preferences, identity/profile, goals, constraints, decisions, and relationships.
- Do NOT extract ephemeral events, one-off task details, or generic world knowledge — those are not durable memories.
- Do NOT store secrets, passwords, API keys.
- Each statement must be self-contained and written in third person about the user when possible.
- Return at most 8 facts.
- Set "is_pii_sensitive" to true if the statement contains or implies personal identifiable information.
- Set "no_facts_reason" when returning zero facts to explain why (e.g. "only_greetings", "only_task_context", "already_known").
- "subject_type" categorizes the fact:
  - "person": identity/profile attributes (name, role, location).
  - "preference": likes, dislikes, style and format preferences.
  - "constraint": negative constraints or standing work requirements (e.g. "never use tool X").
  - "goal": objectives the user is pursuing (e.g. "wants to migrate the system to Postgres").
  - "decision": decisions the user has made (e.g. "chose Postgres over MySQL").
  - "relationship": relationships between the user and other people/entities.
  - "other": only when none of the above fit; such facts are usually discarded, so prefer not to emit them.
- "scope" is "user" for cross-session facts, "agent" for agent-specific behavior.
- "confidence" ranges 0.0–1.0. Facts below 0.6 are discarded by the store — do not emit low-confidence guesses.
`

// ExtractMemoryFactsFunctionSchemaV3 is the V3 function-calling schema with
// the whitelist subject_type vocabulary.
var ExtractMemoryFactsFunctionSchemaV3 = map[string]any{
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
						"subject_type":     map[string]any{"type": "string", "enum": []string{"person", "preference", "constraint", "goal", "decision", "relationship", "other"}, "description": "Category of the fact (durable kinds only)"},
						"scope":            map[string]any{"type": "string", "enum": []string{"user", "agent"}, "description": "Whether this fact applies to the user globally or only within this agent"},
						"confidence":       map[string]any{"type": "number", "description": "0.0-1.0 confidence in this fact; below 0.6 the fact is discarded"},
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

// --- Fact write adjudication (P1-3 operation semantics) ---

// FactAdjudicationSystemPrompt instructs the LLM to verdict each candidate
// fact ADD/UPDATE/DELETE/NOOP against its existing neighbors (Mem0-style
// operation semantics).
const FactAdjudicationSystemPrompt = `You arbitrate NEW memory facts against EXISTING memories of the same user.

For each NEW fact, decide exactly one operation:
- "add": the fact is new information not covered by any existing memory.
- "update": the fact supersedes an existing memory about the same subject (the situation changed, e.g. preference moved from coffee to tea). Set "target_id" to the superseded existing memory id.
- "delete": the fact states something is no longer true, without a replacement. Set "target_id" to the existing memory id to invalidate.
- "noop": the fact merely restates an existing memory in other words — nothing to write.

Rules:
- Verdicts must reference existing memories only by their given ids.
- When unsure between "add" and "update", prefer "add" (a wrong update loses history).
- Output ONLY a JSON object: {"verdicts":[{"statement":"<exact new fact statement>","operation":"add|update|delete|noop","target_id":"<id or empty>"}]}
- Include one verdict per NEW fact, copying the statement verbatim.`

// FactAdjudicationPromptNeighbor is one existing memory shown beside a
// candidate in the adjudication prompt.
type FactAdjudicationPromptNeighbor struct {
	ID        string `json:"id"`
	Statement string `json:"statement"`
	Kind      string `json:"kind"`
}

// FactAdjudicationPromptItem is one contested candidate with its neighbors.
type FactAdjudicationPromptItem struct {
	Statement string                           `json:"statement"`
	Kind      string                           `json:"kind"`
	Neighbors []FactAdjudicationPromptNeighbor `json:"neighbors"`
}

// BuildFactAdjudicationPrompt renders the user message for the adjudication
// call: candidates plus their existing neighbors as JSON.
func BuildFactAdjudicationPrompt(items []FactAdjudicationPromptItem) string {
	b, _ := json.Marshal(map[string]any{"new_facts": items})
	return "Arbitrate these NEW facts against their EXISTING neighbors:\n" + string(b)
}

// FactAdjudicationVerdictRaw is one parsed adjudication verdict (pre-mapping
// to the biz type to keep this package dependency-free).
type FactAdjudicationVerdictRaw struct {
	Statement string `json:"statement"`
	Operation string `json:"operation"`
	TargetID  string `json:"target_id"`
}

// ParseFactAdjudicationResponse parses the adjudicator LLM output. Verdicts
// with empty statements or unknown operations are dropped; fenced JSON is
// tolerated.
func ParseFactAdjudicationResponse(raw string) ([]FactAdjudicationVerdictRaw, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	raw = stripJSONFence(raw)
	var payload struct {
		Verdicts []FactAdjudicationVerdictRaw `json:"verdicts"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}
	out := make([]FactAdjudicationVerdictRaw, 0, len(payload.Verdicts))
	for _, v := range payload.Verdicts {
		v.Statement = strings.TrimSpace(v.Statement)
		v.TargetID = strings.TrimSpace(v.TargetID)
		if v.Statement == "" {
			continue
		}
		switch v.Operation {
		case "add", "update", "delete", "noop":
		default:
			continue
		}
		out = append(out, v)
	}
	return out, nil
}
