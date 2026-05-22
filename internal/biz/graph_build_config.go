package biz

// GraphDefinitionFromBuildConfig materializes a graph definition view for observability projection.
func GraphDefinitionFromBuildConfig(cfg GraphBuildConfig, id, name string) *GraphDefinition {
	if name == "" {
		name = id
	}
	return &GraphDefinition{
		ID:               id,
		Name:             name,
		Nodes:            append([]NodeDef(nil), cfg.Nodes...),
		Edges:            append([]EdgeDef(nil), cfg.Edges...),
		ConditionalEdges: append([]ConditionalEdgeDef(nil), cfg.ConditionalEdges...),
		Subgraphs:        append([]SubgraphDef(nil), cfg.Subgraphs...),
		StateFields:      append([]StateFieldDef(nil), cfg.StateFields...),
		EntryPoint:       cfg.EntryPoint,
		FinishPoint:      cfg.FinishPoint,
		EnableCheckpoint: cfg.EnableCheckpoint,
		ExecutionEngine:  cfg.ExecutionEngine,
		InterruptBefore:  append([]string(nil), cfg.InterruptBefore...),
		InterruptAfter:   append([]string(nil), cfg.InterruptAfter...),
	}
}
