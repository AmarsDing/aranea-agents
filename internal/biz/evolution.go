package biz

import (
	"context"
	"strings"
	"time"

	kerrors "github.com/go-kratos/kratos/v2/errors"
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
}

func NewEvolutionUsecase(
	metricsRepo EvolutionMetricsRepo,
	suggestionRepo EvolutionSuggestionRepo,
	agents AgentRepository,
) *EvolutionUsecase {
	return &EvolutionUsecase{
		metricsRepo:    metricsRepo,
		suggestionRepo: suggestionRepo,
		agents:         agents,
	}
}

func (uc *EvolutionUsecase) GetEvolutionMetrics(ctx context.Context, agentID string, timeRange string) (EvolutionMetrics, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return EvolutionMetrics{}, kerrors.BadRequest("EVOLUTION", "agent_id is required")
	}
	since := timeRangeToSince(timeRange)
	toolRate, toolSeries, _ := uc.metricsRepo.GetToolSuccessRate(ctx, agentID, since)
	retrievalRate, retrievalSeries, _ := uc.metricsRepo.GetRetrievalQuality(ctx, agentID, since)
	episodes, _ := uc.metricsRepo.GetEpisodeCount(ctx, agentID, since)
	negFeedback, _ := uc.metricsRepo.GetNegativeFeedbackCount(ctx, agentID, since)
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
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, kerrors.BadRequest("EVOLUTION", "agent_id is required")
	}
	return uc.suggestionRepo.ListByAgent(ctx, agentID, status)
}

func (uc *EvolutionUsecase) GetSuggestionByID(ctx context.Context, id string) (EvolutionSuggestion, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return EvolutionSuggestion{}, kerrors.BadRequest("EVOLUTION", "id is required")
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
		for i, f := range files {
			if f.Name == "SOUL.md" {
				files[i].Body = s.Content
				break
			}
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
