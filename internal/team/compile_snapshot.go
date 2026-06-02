package team

import (
	"encoding/json"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// CompileSnapshot is a backend compile result for Observatory / API responses.
type CompileSnapshot struct {
	TemplateID       string
	Mode             string
	EntryPoint       string
	FinishPoint      string
	GraphJSON        string
	Valid            bool
	CompileError     string
	Nodes            []biz.NodeDef
	Edges            []biz.EdgeDef
	ConditionalEdges []biz.ConditionalEdgeDef
	TaskMeta         map[string]biz.NodeTaskMeta
}

// BuildCompileSnapshot compiles OrchestrationSpec JSON to graph topology (embedded graph aware).
func BuildCompileSnapshot(def Definition, rawDefinitionJSON string, agentKey CompileAgentKey, lg loggateway.Logger) CompileSnapshot {
	mode := normalizeCompileMode(def.Mode)
	snap := CompileSnapshot{
		TemplateID: CompileTemplateID(def.Mode),
		Mode:       mode,
	}
	cfg, err := compileToGraphBuildConfig(def, rawDefinitionJSON, agentKey, lg)
	if err != nil {
		snap.Valid = false
		snap.CompileError = err.Error()
		return snap
	}
	snap.Valid = true
	snap.EntryPoint = cfg.EntryPoint
	snap.FinishPoint = cfg.FinishPoint
	snap.Nodes = append([]biz.NodeDef(nil), cfg.Nodes...)
	snap.Edges = append([]biz.EdgeDef(nil), cfg.Edges...)
	snap.ConditionalEdges = append([]biz.ConditionalEdgeDef(nil), cfg.ConditionalEdges...)
	snap.TaskMeta = cfg.TaskMeta
	if b, merr := json.Marshal(cfg); merr == nil {
		snap.GraphJSON = string(b)
	}
	return snap
}
