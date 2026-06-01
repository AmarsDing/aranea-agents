package biz

import (
	"context"
	"strings"
	"time"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	"aranea-agents/pkg/loggateway"
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
}

type MetricDataPoint struct {
	Date  string
	Value float64
}

type EvolutionSuggestion struct {
	ID          string
	AgentID     string
	Type        string
	Title       string
	Content     string
	Status      string
	DiffPreview string
	CreatedAt   string
	AppliedAt   string
}

type EvolutionMetricsRepo interface {
	GetToolSuccessRate(ctx context.Context, agentID string, since time.Time) (float64, []MetricDataPoint, error)
	GetRetrievalQuality(ctx context.Context, agentID string, since time.Time) (float64, []MetricDataPoint, error)
	GetEpisodeCount(ctx context.Context, agentID string, since time.Time) (int, error)
	GetNegativeFeedbackCount(ctx context.Context, agentID string, since time.Time) (int, error)
}

type EvolutionSuggestionRepo interface {
	ListByAgent(ctx context.Context, agentID string, status string) ([]EvolutionSuggestion, error)
	GetByID(ctx context.Context, id string) (EvolutionSuggestion, error)
	Create(ctx context.Context, s EvolutionSuggestion) (EvolutionSuggestion, error)
	UpdateStatus(ctx context.Context, id string, status string) (EvolutionSuggestion, error)
}

type EvolutionUsecase struct {
	metricsRepo    EvolutionMetricsRepo
	suggestionRepo EvolutionSuggestionRepo
	agents         AgentRepository
	lg             loggateway.Logger
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
	}
}

func (uc *EvolutionUsecase) GetEvolutionMetrics(ctx context.Context, agentID string, timeRange string) (EvolutionMetrics, error) {
	agentID, err := requireNonEmpty(agentID, "EVOLUTION", "agent_id")
	if err != nil {
		return EvolutionMetrics{}, err
	}
	since := timeRangeToSince(timeRange)
	toolRate, toolSeries, err := uc.metricsRepo.GetToolSuccessRate(ctx, agentID, since)
	if err != nil {
		uc.lg.Warn("GetToolSuccessRate failed", loggateway.StepID("evolution.get_tool_success_rate"), loggateway.Err(err))
	}
	retrievalRate, retrievalSeries, err := uc.metricsRepo.GetRetrievalQuality(ctx, agentID, since)
	if err != nil {
		uc.lg.Warn("GetRetrievalQuality failed", loggateway.StepID("evolution.get_retrieval_quality"), loggateway.Err(err))
	}
	episodes, err := uc.metricsRepo.GetEpisodeCount(ctx, agentID, since)
	if err != nil {
		uc.lg.Warn("GetEpisodeCount failed", loggateway.StepID("evolution.get_episode_count"), loggateway.Err(err))
	}
	negFeedback, err := uc.metricsRepo.GetNegativeFeedbackCount(ctx, agentID, since)
	if err != nil {
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
		return EvolutionSuggestion{}, kerrors.BadRequest("EVOLUTION", "agent_id and suggestion_id are required")
	}
	s, err := uc.suggestionRepo.GetByID(ctx, suggestionID)
	if err != nil {
		return EvolutionSuggestion{}, err
	}
	if s.AgentID != agentID {
		return EvolutionSuggestion{}, kerrors.NotFound("EVOLUTION", "suggestion not found for this agent")
	}
	if s.Status != "pending" {
		return EvolutionSuggestion{}, kerrors.BadRequest("EVOLUTION", "only pending suggestions can be applied")
	}
	switch s.Type {
	case "persona":
		files, err := uc.agents.ListAgentPromptFiles(ctx, agentID)
		if err != nil {
			return EvolutionSuggestion{}, err
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
			files[0].Body = s.Content
		}
		if _, err := uc.agents.ReplaceAgentPromptFiles(ctx, agentID, files); err != nil {
			return EvolutionSuggestion{}, err
		}
	}
	updated, err := uc.suggestionRepo.UpdateStatus(ctx, suggestionID, "applied")
	if err != nil {
		return EvolutionSuggestion{}, err
	}
	return updated, nil
}

func (uc *EvolutionUsecase) RejectSuggestion(ctx context.Context, agentID string, suggestionID string) (EvolutionSuggestion, error) {
	agentID = strings.TrimSpace(agentID)
	suggestionID = strings.TrimSpace(suggestionID)
	if agentID == "" || suggestionID == "" {
		return EvolutionSuggestion{}, kerrors.BadRequest("EVOLUTION", "agent_id and suggestion_id are required")
	}
	s, err := uc.suggestionRepo.GetByID(ctx, suggestionID)
	if err != nil {
		return EvolutionSuggestion{}, err
	}
	if s.AgentID != agentID {
		return EvolutionSuggestion{}, kerrors.NotFound("EVOLUTION", "suggestion not found for this agent")
	}
	if s.Status != "pending" {
		return EvolutionSuggestion{}, kerrors.BadRequest("EVOLUTION", "only pending suggestions can be rejected")
	}
	return uc.suggestionRepo.UpdateStatus(ctx, suggestionID, "rejected")
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
