package compress

import (
	"encoding/json"
	"strings"
)

// EnhancedExtractionResult holds the output of a combined Episode + Entity + Relation extraction.
type EnhancedExtractionResult struct {
	Episode   EnhancedEpisodeData `json:"episode"`
	Entities  []ExtractedEntity   `json:"entities"`
	Relations []ExtractedRelation `json:"relations"`
}

// EnhancedEpisodeData holds the LLM-enhanced episode fields.
type EnhancedEpisodeData struct {
	Title          string   `json:"title"`
	Goal           string   `json:"goal"`
	Outcome        string   `json:"outcome"`
	OutcomeSummary string   `json:"outcome_summary"`
	KeyDecisions   []string `json:"key_decisions"`
	KeyArtifacts   []string `json:"key_artifacts"`
	Importance     float64  `json:"importance"`
	Confidence     float64  `json:"confidence"`
}

// ExtractedEntity represents a named entity extracted from conversation.
type ExtractedEntity struct {
	Name        string  `json:"name"`
	EntityType  string  `json:"entity_type"`
	Description string  `json:"description"`
	Confidence  float64 `json:"confidence"`
}

// ExtractedRelation represents a relationship between two entities.
type ExtractedRelation struct {
	SourceEntity string  `json:"source_entity"`
	TargetEntity string  `json:"target_entity"`
	RelationType string  `json:"relation_type"`
	Confidence   float64 `json:"confidence"`
}

// EnhancedExtractFunctionName is the function calling tool name for enhanced extraction.
const EnhancedExtractFunctionName = "extract_enhanced_memory"

// EnhancedExtractSystemPrompt is the system prompt for enhanced extraction.
const EnhancedExtractSystemPrompt = `You analyze a conversation transcript and simultaneously extract three kinds of structured data: an episode summary, named entities, and relations between entities.

## Output Format
Call the provided function "extract_enhanced_memory" with your results.
If the model does not support function calling, output JSON with the same schema as the function parameters.

## Episode Extraction Rules
- Extract a concise title summarizing the conversation episode.
- Identify the user's goal (what they were trying to accomplish).
- Describe the outcome (what actually happened).
- Provide a short outcome_summary (one sentence).
- List key_decisions: important choices or turning points made during the conversation.
- List key_artifacts: files, URLs, code snippets, or references produced or discussed.
- Rate importance 0.0–1.0: how significant is this episode for long-term memory?
- Rate confidence 0.0–1.0: how certain are you about the extracted data?

## Entity Extraction Rules
- Extract named entities: person names, user preferences, concepts, projects, tools, or technologies.
- entity_type must be one of: "person", "preference", "concept", "project".
- Provide a brief description for each entity.
- Rate confidence 0.0–1.0 for each entity.
- Do NOT extract entities that are only mentioned in passing with no significance.
- Return at most 10 entities.

## Relation Extraction Rules
- Extract relations between the entities you identified.
- relation_type must be one of:
  - "depends_on": A depends on B (structural/code relation)
  - "implements": A implements B (structural/code relation)
  - "references": A references B (structural/code relation)
  - "contains": A contains B (structural/code relation)
  - "knows_as": A is known as B (user profile relation, compatible with L4 entity graph)
  - "prefers": A prefers B (user profile relation, compatible with L4 entity graph)
- Source and target must match entity names you extracted.
- Rate confidence 0.0–1.0 for each relation.
- Return at most 15 relations.

## General Rules
- Do NOT store secrets, passwords, API keys, or ephemeral one-off details.
- Write descriptions and outcomes in third person when possible.
- If nothing is worth extracting, return empty arrays and low importance.
`

// EnhancedExtractFunctionSchema is the function calling schema for enhanced extraction.
var EnhancedExtractFunctionSchema = map[string]any{
	"name":        EnhancedExtractFunctionName,
	"description": "Extract a structured episode, named entities, and entity relations from a conversation for long-term memory storage.",
	"parameters": map[string]any{
		"type": "object",
		"properties": map[string]any{
			"episode": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title":           map[string]any{"type": "string", "description": "Concise title summarizing the conversation episode"},
					"goal":            map[string]any{"type": "string", "description": "What the user was trying to accomplish"},
					"outcome":         map[string]any{"type": "string", "description": "What actually happened as a result"},
					"outcome_summary": map[string]any{"type": "string", "description": "One-sentence summary of the outcome"},
					"key_decisions":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Important choices or turning points"},
					"key_artifacts":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Files, URLs, code snippets, or references produced"},
					"importance":      map[string]any{"type": "number", "description": "0.0-1.0 how significant this episode is for long-term memory"},
					"confidence":      map[string]any{"type": "number", "description": "0.0-1.0 confidence in the extracted data"},
				},
				"required": []string{"title", "outcome_summary", "importance", "confidence"},
			},
			"entities": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":        map[string]any{"type": "string", "description": "Entity name or identifier"},
						"entity_type": map[string]any{"type": "string", "enum": []string{"person", "preference", "concept", "project"}, "description": "Category of the entity"},
						"description": map[string]any{"type": "string", "description": "Brief description of the entity"},
						"confidence":  map[string]any{"type": "number", "description": "0.0-1.0 confidence in this entity"},
					},
					"required": []string{"name", "entity_type", "confidence"},
				},
			},
			"relations": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"source_entity": map[string]any{"type": "string", "description": "Name of the source entity"},
						"target_entity": map[string]any{"type": "string", "description": "Name of the target entity"},
						"relation_type": map[string]any{"type": "string", "enum": []string{"depends_on", "implements", "references", "contains", "knows_as", "prefers"}, "description": "Type of relationship"},
						"confidence":    map[string]any{"type": "number", "description": "0.0-1.0 confidence in this relation"},
					},
					"required": []string{"source_entity", "target_entity", "relation_type", "confidence"},
				},
			},
		},
		"required": []string{"episode", "entities", "relations"},
	},
}

// ParseEnhancedExtractionResult parses the LLM response into EnhancedExtractionResult.
// It handles both function call arguments and raw JSON output.
func ParseEnhancedExtractionResult(raw string) (*EnhancedExtractionResult, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	raw = stripJSONFence(raw)
	var result EnhancedExtractionResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, err
	}
	// Apply defaults for missing fields.
	if result.Episode.Importance <= 0 {
		result.Episode.Importance = 0.5
	}
	if result.Episode.Confidence <= 0 {
		result.Episode.Confidence = 0.6
	}
	if result.Episode.KeyDecisions == nil {
		result.Episode.KeyDecisions = []string{}
	}
	if result.Episode.KeyArtifacts == nil {
		result.Episode.KeyArtifacts = []string{}
	}
	if result.Entities == nil {
		result.Entities = []ExtractedEntity{}
	}
	if result.Relations == nil {
		result.Relations = []ExtractedRelation{}
	}
	// Validate and default entity confidence and type.
	validEntityTypes := map[string]bool{"person": true, "preference": true, "concept": true, "project": true}
	for i := range result.Entities {
		if result.Entities[i].Confidence <= 0 {
			result.Entities[i].Confidence = 0.7
		}
		if !validEntityTypes[result.Entities[i].EntityType] {
			result.Entities[i].EntityType = "concept"
		}
	}
	// Validate and default relation confidence and type.
	validRelationTypes := map[string]bool{
		"depends_on": true, "implements": true, "references": true,
		"contains": true, "knows_as": true, "prefers": true,
	}
	validRelations := result.Relations[:0]
	for _, r := range result.Relations {
		if r.Confidence <= 0 {
			r.Confidence = 0.7
		}
		if !validRelationTypes[r.RelationType] {
			continue // skip invalid relation types
		}
		validRelations = append(validRelations, r)
	}
	result.Relations = validRelations
	return &result, nil
}
