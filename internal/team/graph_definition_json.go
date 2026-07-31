package team

import (
	"encoding/json"
	"strings"
)

// LinkedGraphIDFromDefinition reads linked_graph_id from team definition JSON.
func LinkedGraphIDFromDefinition(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var body struct {
		LinkedGraphID string `json:"linked_graph_id"`
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return ""
	}
	return strings.TrimSpace(body.LinkedGraphID)
}

// ResolveLinkedGraphID returns the team's materialized graph asset ID:
// the linked_graph_id column when set, else the definition_json fallback.
// Empty for legacy teams without a materialized graph asset.
func ResolveLinkedGraphID(column, definitionJSON string) string {
	if id := strings.TrimSpace(column); id != "" {
		return id
	}
	return LinkedGraphIDFromDefinition(definitionJSON)
}

// MergeLinkedGraphID writes linked_graph_id into definition JSON without dropping other keys.
func MergeLinkedGraphID(raw, linkedGraphID string) (string, error) {
	linkedGraphID = strings.TrimSpace(linkedGraphID)
	if raw == "" {
		raw = `{"version":1,"mode":"sequential","members":[]}`
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return "", err
	}
	if linkedGraphID == "" {
		delete(body, "linked_graph_id")
	} else {
		body["linked_graph_id"] = linkedGraphID
	}
	out, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
