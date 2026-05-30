package modelregistry

import "time"

type MigratePhase struct {
	backend MigrationWriter
}

func NewMigratePhase(backend MigrationWriter) *MigratePhase {
	return &MigratePhase{backend: backend}
}

func (p *MigratePhase) Name() string         { return "migrate" }
func (p *MigratePhase) Timeout() time.Duration { return 300 * time.Second }

func (p *MigratePhase) Run(pc *PhaseContext) PhaseResult {
	checkpoint, cpErr := pc.Store.LoadMigrationCheckpoint()
	if cpErr != nil {
		return PhaseResult{PhaseName: "migrate", Status: PhaseFailed, Errors: []string{"load checkpoint: " + cpErr.Error()}}
	}
	skipRules := checkpoint.CompletedRules

	result := p.backend.BatchMigrateProviderBindings(pc.Ctx, ListProviderMigrationRules(), skipRules)

	if saveErr := pc.Store.SaveMigrationCheckpoint(*NewCheckpoint(result.CompletedRules)); saveErr != nil {
		result.Errors = append(result.Errors, "save checkpoint: "+saveErr.Error())
	}

	status := PhaseSucceeded
	if len(result.FailedRules) > 0 && len(result.CompletedRules) == 0 {
		status = PhaseFailed
	}
	if len(result.Errors) > 0 && len(result.CompletedRules) == 0 {
		status = PhaseFailed
	}
	return PhaseResult{
		PhaseName:  "migrate",
		Status:     status,
		Stats:      map[string]int{"agents": result.Stats.Agents, "sessions": result.Stats.Sessions, "eval": result.Stats.Eval, "runtime_settings": result.Stats.RuntimeSettings, "skills": result.Stats.Skills, "knowledge_embed": result.Stats.KnowledgeEmbed, "web_research": result.Stats.WebResearch},
		Errors:     result.Errors,
		Checkpoint: NewCheckpoint(result.CompletedRules),
	}
}

func NewCheckpoint(completedRules []string) *MigrationCheckpoint {
	return &MigrationCheckpoint{CompletedRules: completedRules}
}
