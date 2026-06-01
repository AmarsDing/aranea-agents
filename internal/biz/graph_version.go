package biz

import (
	"encoding/json"
	"time"

	"aranea-agents/pkg/loggateway"
)

const (
	GraphMetadataVersionKey        = "_version"
	GraphMetadataVersionHistoryKey = "_version_history"
	GraphMetadataUserTemplateKey   = "user_template"
	graphLayoutMetadataKey         = "layout"
	GraphMaxVersionHistory         = 50
	UserTemplateIDPrefix           = "user:"
)

type GraphVersionEntry struct {
	Version  int              `json:"version"`
	SavedAt  time.Time        `json:"saved_at"`
	Name     string           `json:"name"`
	Snapshot *GraphDefinition `json:"snapshot"`
}

type UserTemplateMeta struct {
	TemplateID  string `json:"template_id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
}

func GraphVersion(def *GraphDefinition) int {
	if def == nil {
		return 1
	}
	if def.Version > 0 {
		return def.Version
	}
	return versionFromMetadata(def.Metadata)
}

func versionFromMetadata(meta map[string]any) int {
	if meta == nil {
		return 1
	}
	switch v := meta[GraphMetadataVersionKey].(type) {
	case float64:
		if int(v) > 0 {
			return int(v)
		}
	case int:
		if v > 0 {
			return v
		}
	case json.Number:
		if n, err := v.Int64(); err == nil && n > 0 {
			return int(n)
		}
	}
	return 1
}

func syncVersionMetadata(def *GraphDefinition) {
	if def == nil {
		return
	}
	if def.Metadata == nil {
		def.Metadata = map[string]any{}
	}
	if def.Version <= 0 {
		def.Version = versionFromMetadata(def.Metadata)
	}
	def.Metadata[GraphMetadataVersionKey] = def.Version
}

func cloneGraphDefinition(def *GraphDefinition, lg loggateway.Logger) *GraphDefinition {
	if def == nil {
		return nil
	}
	raw, _ := json.Marshal(def)
	var copy GraphDefinition
	if err := json.Unmarshal(raw, &copy); err != nil {
		lg.Warn("克隆 GraphDefinition 失败", loggateway.StepID("graph_version.clone"), loggateway.Err(err))
	}
	return &copy
}

func snapshotForVersion(def *GraphDefinition, lg loggateway.Logger) *GraphDefinition {
	snap := cloneGraphDefinition(def, lg)
	if snap == nil {
		return nil
	}
	snap.Version = GraphVersion(snap)
	snap.Nodes = compactNodesForVersion(snap.Nodes)
	if snap.Metadata != nil {
		layout := snap.Metadata[graphLayoutMetadataKey]
		snap.Metadata = map[string]any{}
		if layout != nil {
			snap.Metadata[graphLayoutMetadataKey] = layout
		}
	}
	return snap
}

func compactNodesForVersion(nodes []NodeDef) []NodeDef {
	if len(nodes) == 0 {
		return nodes
	}
	out := make([]NodeDef, len(nodes))
	for i, n := range nodes {
		out[i] = n
		out[i].Description = ""
		out[i].Instruction = ""
		out[i].InputMapperJSON = ""
		out[i].OutputMapperJSON = ""
		out[i].ReviewRules = ""
	}
	return out
}

func appendVersionHistory(def *GraphDefinition, previous *GraphDefinition, lg loggateway.Logger) {
	if def == nil || previous == nil {
		return
	}
	if def.Metadata == nil {
		def.Metadata = map[string]any{}
	}
	history := readVersionHistory(def.Metadata)
	entry := GraphVersionEntry{
		Version:  GraphVersion(previous),
		SavedAt:  previous.UpdatedAt,
		Name:     previous.Name,
		Snapshot: snapshotForVersion(previous, lg),
	}
	if entry.SavedAt.IsZero() {
		entry.SavedAt = time.Now()
	}
	history = append(history, entry)
	if len(history) > GraphMaxVersionHistory {
		history = history[len(history)-GraphMaxVersionHistory:]
	}
	def.Metadata[GraphMetadataVersionHistoryKey] = history
	def.Version = GraphVersion(previous) + 1
	syncVersionMetadata(def)
}

func readVersionHistory(meta map[string]any) []GraphVersionEntry {
	if meta == nil {
		return nil
	}
	raw, ok := meta[GraphMetadataVersionHistoryKey]
	if !ok || raw == nil {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var history []GraphVersionEntry
	if err := json.Unmarshal(data, &history); err != nil {
		return nil
	}
	return history
}

func ListGraphVersionEntries(def *GraphDefinition) []GraphVersionEntry {
	if def == nil {
		return nil
	}
	return readVersionHistory(def.Metadata)
}

func FindGraphVersionSnapshot(def *GraphDefinition, version int, lg loggateway.Logger) *GraphDefinition {
	for _, entry := range ListGraphVersionEntries(def) {
		if entry.Version == version && entry.Snapshot != nil {
			return cloneGraphDefinition(entry.Snapshot, lg)
		}
	}
	return nil
}

func ReadUserTemplateMeta(def *GraphDefinition) *UserTemplateMeta {
	if def == nil || def.Metadata == nil {
		return nil
	}
	raw, ok := def.Metadata[GraphMetadataUserTemplateKey]
	if !ok || raw == nil {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var meta UserTemplateMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil
	}
	if meta.TemplateID == "" {
		return nil
	}
	return &meta
}

func WriteUserTemplateMeta(def *GraphDefinition, meta UserTemplateMeta) {
	if def == nil {
		return
	}
	if def.Metadata == nil {
		def.Metadata = map[string]any{}
	}
	def.Metadata[GraphMetadataUserTemplateKey] = meta
}
