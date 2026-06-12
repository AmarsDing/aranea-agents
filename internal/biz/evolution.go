package biz

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// Evolution suggestion status constants.
const (
	EvolutionStatusPending    = "pending"
	EvolutionStatusApplied    = "applied"
	EvolutionStatusRejected   = "rejected"
	EvolutionStatusRolledBack = "rolled_back"
)

type EvolutionMetrics struct {
	AgentID                string
	TimeRange              string
	ToolSuccessRate        float64
	RetrievalQuality       float64
	TotalEpisodes          int
	NegativeFeedback       int
	ToolSuccessSeries      []MetricDataPoint
	RetrievalQualitySeries []MetricDataPoint
	// S-05 fix: indicate when metrics data is incomplete due to partial query failures.
	Partial bool
	// S-08 fix: record which sub-queries failed for observability.
	PartialErrors []string
}

type MetricDataPoint struct {
	Date  string
	Value float64
}

type EvolutionSuggestion struct {
	ID               string
	AgentID          string
	Type             string
	Title            string
	Content          string
	Status           string
	DiffPreview      string
	PreApplySnapshot string // JSON-encoded map[filename]content of files before apply
	CreatedAt        string
	AppliedAt        string
}

// Stability:stable
type EvolutionMetricsRepo interface {
	GetToolSuccessRate(ctx context.Context, agentID string, since time.Time) (float64, []MetricDataPoint, error)
	GetRetrievalQuality(ctx context.Context, agentID string, since time.Time) (float64, []MetricDataPoint, error)
	GetEpisodeCount(ctx context.Context, agentID string, since time.Time) (int, error)
	GetNegativeFeedbackCount(ctx context.Context, agentID string, since time.Time) (int, error)
}

// Stability:stable
type EvolutionSuggestionRepo interface {
	ListByAgent(ctx context.Context, agentID string, status string) ([]EvolutionSuggestion, error)
	GetByID(ctx context.Context, id string) (EvolutionSuggestion, error)
	Create(ctx context.Context, s EvolutionSuggestion) (EvolutionSuggestion, error)
	UpdateStatus(ctx context.Context, id string, status string) (EvolutionSuggestion, error)
	UpdateSnapshot(ctx context.Context, id string, snapshot string) error
}

type EvolutionUsecase struct {
	metricsRepo      EvolutionMetricsRepo
	suggestionRepo   EvolutionSuggestionRepo
	agents           AgentRepository
	coordinator      *EvolutionCoordinator
	orchestrator     *SkillEvolutionOrchestrator
	orchestratorOnce sync.Once
	lg               loggateway.Logger
	evolutionSM      *EvolutionStateMachine
}

func NewEvolutionUsecase(
	metricsRepo EvolutionMetricsRepo,
	suggestionRepo EvolutionSuggestionRepo,
	agents AgentRepository,
	lg loggateway.Logger,
) *EvolutionUsecase {
	return &EvolutionUsecase{
		metricsRepo:    metricsRepo,
		suggestionRepo: suggestionRepo,
		agents:         agents,
		lg:             lg,
		evolutionSM:    NewEvolutionStateMachine(),
	}
}

// SetCoordinator sets the evolution coordinator for cross-pipeline dedup.
// NOTE: Must only be called during initialization, before any concurrent access.
//
// Deprecated: Use SetOrchestrator instead.
func (uc *EvolutionUsecase) SetCoordinator(c *EvolutionCoordinator) {
	uc.coordinator = c
}

// SetOrchestrator sets the unified evolution orchestrator for cross-pipeline dedup.
// When set, ScanAgent delegates to the orchestrator for pending checks.
// Protected by sync.Once to prevent concurrent initialization races.
func (uc *EvolutionUsecase) SetOrchestrator(o *SkillEvolutionOrchestrator) {
	uc.orchestratorOnce.Do(func() {
		uc.orchestrator = o
	})
}

func (uc *EvolutionUsecase) GetEvolutionMetrics(ctx context.Context, agentID string, timeRange string) (EvolutionMetrics, error) {
	agentID, err := requireNonEmpty(agentID, "EVOLUTION", "agent_id")
	if err != nil {
		return EvolutionMetrics{}, err
	}
	since := timeRangeToSince(timeRange)
	var partial bool
	var partialErrors []string
	toolRate, toolSeries, err := uc.metricsRepo.GetToolSuccessRate(ctx, agentID, since)
	if err != nil {
		partial = true
		partialErrors = append(partialErrors, "GetToolSuccessRate: "+err.Error())
		uc.lg.Warn("GetToolSuccessRate failed", loggateway.StepID("evolution.get_tool_success_rate"), loggateway.Err(err))
	}
	retrievalRate, retrievalSeries, err := uc.metricsRepo.GetRetrievalQuality(ctx, agentID, since)
	if err != nil {
		partial = true
		partialErrors = append(partialErrors, "GetRetrievalQuality: "+err.Error())
		uc.lg.Warn("GetRetrievalQuality failed", loggateway.StepID("evolution.get_retrieval_quality"), loggateway.Err(err))
	}
	episodes, err := uc.metricsRepo.GetEpisodeCount(ctx, agentID, since)
	if err != nil {
		partial = true
		partialErrors = append(partialErrors, "GetEpisodeCount: "+err.Error())
		uc.lg.Warn("GetEpisodeCount failed", loggateway.StepID("evolution.get_episode_count"), loggateway.Err(err))
	}
	negFeedback, err := uc.metricsRepo.GetNegativeFeedbackCount(ctx, agentID, since)
	if err != nil {
		partial = true
		partialErrors = append(partialErrors, "GetNegativeFeedbackCount: "+err.Error())
		uc.lg.Warn("GetNegativeFeedbackCount failed", loggateway.StepID("evolution.get_negative_feedback_count"), loggateway.Err(err))
	}
	return EvolutionMetrics{
		AgentID:                agentID,
		TimeRange:              timeRange,
		ToolSuccessRate:        toolRate,
		RetrievalQuality:       retrievalRate,
		TotalEpisodes:          episodes,
		NegativeFeedback:       negFeedback,
		ToolSuccessSeries:      toolSeries,
		RetrievalQualitySeries: retrievalSeries,
		Partial:                partial,
		PartialErrors:          partialErrors,
	}, nil
}

func (uc *EvolutionUsecase) GetEvolutionSuggestions(ctx context.Context, agentID string, status string) ([]EvolutionSuggestion, error) {
	agentID, err := requireNonEmpty(agentID, "EVOLUTION", "agent_id")
	if err != nil {
		return nil, err
	}
	return uc.suggestionRepo.ListByAgent(ctx, agentID, status)
}

func (uc *EvolutionUsecase) GetSuggestionByID(ctx context.Context, id string) (EvolutionSuggestion, error) {
	id, err := requireNonEmpty(id, "EVOLUTION", "id")
	if err != nil {
		return EvolutionSuggestion{}, err
	}
	return uc.suggestionRepo.GetByID(ctx, id)
}

func (uc *EvolutionUsecase) ApplySuggestion(ctx context.Context, agentID string, suggestionID string) (EvolutionSuggestion, error) {
	agentID = strings.TrimSpace(agentID)
	suggestionID = strings.TrimSpace(suggestionID)
	if agentID == "" || suggestionID == "" {
		return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "agent_id and suggestion_id are required")
	}
	s, err := uc.suggestionRepo.GetByID(ctx, suggestionID)
	if err != nil {
		return EvolutionSuggestion{}, err
	}
	if s.AgentID != agentID {
		return EvolutionSuggestion{}, apierror.NotFound("EVOLUTION", "suggestion not found for this agent")
	}
	if s.Status != EvolutionStatusPending {
		return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "only pending suggestions can be applied")
	}
	// AS-FSM-01: Validate state transition via state machine.
	if _, err := uc.evolutionSM.Transition(ParseEvolutionState(s.Status), EvolutionEventApply); err != nil {
		return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "invalid status transition from "+s.Status+" to applied")
	}
	switch s.Type {
	case "persona":
		files, err := uc.agents.ListAgentPromptFiles(ctx, agentID)
		if err != nil {
			return EvolutionSuggestion{}, err
		}
		// Save pre-apply snapshot for rollback support.
		if snapErr := uc.savePreApplySnapshot(ctx, suggestionID, files); snapErr != nil {
			uc.lg.Warn("failed to save pre-apply snapshot", loggateway.StepID("evolution.apply"), loggateway.Err(snapErr))
		}
		// PGO-1-BIZ-06: Write persona suggestions into IDENTITY.md's ## Persona
		// section instead of the now-deprecated SOUL.md. Fall back to SOUL.md
		// for legacy agents that still carry that file.
		applied := false
		for i, f := range files {
			if f.Name == "IDENTITY.md" {
				files[i].Body = replaceOrAppendPersona(f.Body, s.Content)
				applied = true
				break
			}
		}
		if !applied {
			for i, f := range files {
				if f.Name == "SOUL.md" {
					files[i].Body = s.Content
					applied = true
					break
				}
			}
		}
		// If neither file exists, append a new IDENTITY.md.
		if !applied {
			files = append(files, AgentPromptFile{
				AgentID:   agentID,
				Name:      "IDENTITY.md",
				Body:      "# IDENTITY\n\n## Persona\n\n" + s.Content,
				SortOrder: 30,
			})
		}
		if _, err := uc.agents.ReplaceAgentPromptFiles(ctx, agentID, files); err != nil {
			return EvolutionSuggestion{}, err
		}
	case "prompt":
		files, err := uc.agents.ListAgentPromptFiles(ctx, agentID)
		if err != nil {
			return EvolutionSuggestion{}, err
		}
		// Save pre-apply snapshot for rollback support.
		if snapErr := uc.savePreApplySnapshot(ctx, suggestionID, files); snapErr != nil {
			uc.lg.Warn("failed to save pre-apply snapshot", loggateway.StepID("evolution.apply"), loggateway.Err(snapErr))
		}
		applied := false
		for i, f := range files {
			name := strings.TrimSpace(f.Name)
			if name == "AGENTS_CORE.md" || name == "AGENTS_TASK.md" || strings.HasPrefix(name, "AGENTS") {
				files[i].Body = s.Content
				applied = true
				break
			}
		}
		if !applied && len(files) > 0 {
			// No AGENTS*.md file found — refuse to apply rather than
			// overwriting an unrelated file (SOUL.md, IDENTITY.md, etc.).
			return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "no AGENTS*.md prompt file found; create one before applying prompt suggestions")
		}
		if _, err := uc.agents.ReplaceAgentPromptFiles(ctx, agentID, files); err != nil {
			return EvolutionSuggestion{}, err
		}
	}
	updated, err := uc.suggestionRepo.UpdateStatus(ctx, suggestionID, EvolutionStatusApplied)
	if err != nil {
		return EvolutionSuggestion{}, err
	}
	return updated, nil
}

func (uc *EvolutionUsecase) RejectSuggestion(ctx context.Context, agentID string, suggestionID string) (EvolutionSuggestion, error) {
	agentID = strings.TrimSpace(agentID)
	suggestionID = strings.TrimSpace(suggestionID)
	if agentID == "" || suggestionID == "" {
		return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "agent_id and suggestion_id are required")
	}
	s, err := uc.suggestionRepo.GetByID(ctx, suggestionID)
	if err != nil {
		return EvolutionSuggestion{}, err
	}
	if s.AgentID != agentID {
		return EvolutionSuggestion{}, apierror.NotFound("EVOLUTION", "suggestion not found for this agent")
	}
	if s.Status != EvolutionStatusPending {
		return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "only pending suggestions can be rejected")
	}
	// AS-FSM-01: Validate state transition via state machine.
	if _, err := uc.evolutionSM.Transition(ParseEvolutionState(s.Status), EvolutionEventReject); err != nil {
		return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "invalid status transition from "+s.Status+" to rejected")
	}
	return uc.suggestionRepo.UpdateStatus(ctx, suggestionID, EvolutionStatusRejected)
}

// RollbackSuggestion restores the agent's prompt files to the state captured
// in the pre-apply snapshot and updates the suggestion status to "rolled_back".
func (uc *EvolutionUsecase) RollbackSuggestion(ctx context.Context, agentID string, suggestionID string) (EvolutionSuggestion, error) {
	agentID = strings.TrimSpace(agentID)
	suggestionID = strings.TrimSpace(suggestionID)
	if agentID == "" || suggestionID == "" {
		return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "agent_id and suggestion_id are required")
	}
	s, err := uc.suggestionRepo.GetByID(ctx, suggestionID)
	if err != nil {
		return EvolutionSuggestion{}, err
	}
	if s.AgentID != agentID {
		return EvolutionSuggestion{}, apierror.NotFound("EVOLUTION", "suggestion not found for this agent")
	}
	if s.Status != EvolutionStatusApplied {
		return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "only applied suggestions can be rolled back")
	}
	// AS-FSM-01: Validate state transition via state machine.
	if _, err := uc.evolutionSM.Transition(ParseEvolutionState(s.Status), EvolutionEventRollback); err != nil {
		return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "invalid status transition from "+s.Status+" to rolled_back")
	}
	if s.PreApplySnapshot == "" {
		return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "no pre-apply snapshot available for rollback")
	}
	// Decode the snapshot: map[filename]content
	var snapshot map[string]string
	if err := json.Unmarshal([]byte(s.PreApplySnapshot), &snapshot); err != nil {
		return EvolutionSuggestion{}, apierror.BadRequest("EVOLUTION", "invalid pre-apply snapshot data")
	}
	// Load current files and restore from snapshot.
	files, err := uc.agents.ListAgentPromptFiles(ctx, agentID)
	if err != nil {
		return EvolutionSuggestion{}, err
	}
	for i, f := range files {
		if content, ok := snapshot[f.Name]; ok {
			files[i].Body = content
		}
	}
	if _, err := uc.agents.ReplaceAgentPromptFiles(ctx, agentID, files); err != nil {
		return EvolutionSuggestion{}, err
	}
	updated, err := uc.suggestionRepo.UpdateStatus(ctx, suggestionID, EvolutionStatusRolledBack)
	if err != nil {
		return EvolutionSuggestion{}, err
	}
	return updated, nil
}

// savePreApplySnapshot captures the current content of all prompt files as a
// JSON-encoded map[filename]content and persists it via the suggestion repo.
func (uc *EvolutionUsecase) savePreApplySnapshot(ctx context.Context, suggestionID string, files []AgentPromptFile) error {
	snapshot := make(map[string]string, len(files))
	for _, f := range files {
		snapshot[f.Name] = f.Body
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	return uc.suggestionRepo.UpdateSnapshot(ctx, suggestionID, string(data))
}

// replaceOrAppendPersona writes personaContent into the "## Persona" section
// of an IDENTITY.md body. If the section exists it replaces everything from
// the ## Persona heading to the next same-level heading (or EOF). If the
// section does not exist it is appended. PGO-1-BIZ-06.
func replaceOrAppendPersona(body, personaContent string) string {
	const anchor = "## Persona"
	idx := strings.Index(body, anchor)
	if idx == -1 {
		// No ## Persona section — append it.
		trimmed := strings.TrimRight(body, "\n ")
		return trimmed + "\n\n" + anchor + "\n\n" + strings.TrimSpace(personaContent)
	}
	// Find the end of the ## Persona section: next "## " heading or EOF.
	after := body[idx+len(anchor):]
	nextH2 := strings.Index(after, "\n## ")
	var tail string
	if nextH2 == -1 {
		tail = ""
	} else {
		tail = after[nextH2:]
	}
	prefix := strings.TrimRight(body[:idx], "\n ")
	return prefix + "\n\n" + anchor + "\n\n" + strings.TrimSpace(personaContent) + tail
}

func timeRangeToSince(tr string) time.Time {
	now := time.Now()
	switch tr {
	case "7d":
		return now.AddDate(0, 0, -7)
	case "30d":
		return now.AddDate(0, 0, -30)
	case "90d":
		return now.AddDate(0, 0, -90)
	default:
		return now.AddDate(0, 0, -30)
	}
}
