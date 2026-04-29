package service

import (
	"time"

	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

type UsageService struct {
	repo repository.Store
	now  func() time.Time
}

func NewUsageService(repo repository.Store) *UsageService {
	return &UsageService{
		repo: repo,
		now:  func() time.Time { return time.Now().UTC() },
	}
}

func (s *UsageService) Overview(query domain.ModelUsageQuery) (domain.ModelUsageOverview, error) {
	now := s.now()
	rangeQuery := s.normalizeQuery(query, now)
	todayQuery := query
	todayQuery.StartDate = dateKey(now)
	todayQuery.EndDate = dateKey(now)
	yesterdayQuery := query
	yesterday := now.AddDate(0, 0, -1)
	yesterdayQuery.StartDate = dateKey(yesterday)
	yesterdayQuery.EndDate = dateKey(yesterday)
	monthQuery := query
	monthQuery.StartDate = dateKey(time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC))
	monthQuery.EndDate = dateKey(now)

	today, err := s.repo.GetModelUsageSummary(todayQuery)
	if err != nil {
		return domain.ModelUsageOverview{}, err
	}
	yesterdaySummary, err := s.repo.GetModelUsageSummary(yesterdayQuery)
	if err != nil {
		return domain.ModelUsageOverview{}, err
	}
	month, err := s.repo.GetModelUsageSummary(monthQuery)
	if err != nil {
		return domain.ModelUsageOverview{}, err
	}
	rangeSummary, err := s.repo.GetModelUsageSummary(rangeQuery)
	if err != nil {
		return domain.ModelUsageOverview{}, err
	}
	trends, err := s.repo.ListModelUsageTrends(rangeQuery)
	if err != nil {
		return domain.ModelUsageOverview{}, err
	}
	topModels, err := s.repo.ListTopModelUsage(withLimit(rangeQuery, 8))
	if err != nil {
		return domain.ModelUsageOverview{}, err
	}
	topAgents, err := s.repo.ListTopAgentUsage(withLimit(rangeQuery, 8))
	if err != nil {
		return domain.ModelUsageOverview{}, err
	}
	anomalyQuery := withLimit(rangeQuery, 12)
	anomalyQuery.Status = "abnormal"
	anomalies, err := s.repo.ListModelUsageEvents(anomalyQuery)
	if err != nil {
		return domain.ModelUsageOverview{}, err
	}

	return domain.ModelUsageOverview{
		Today:     today,
		Yesterday: yesterdaySummary,
		Month:     month,
		Range:     rangeSummary,
		Trends:    trends,
		TopModels: topModels,
		TopAgents: topAgents,
		Anomalies: anomalies,
	}, nil
}

func (s *UsageService) Summary(query domain.ModelUsageQuery) (domain.ModelUsageSummary, error) {
	return s.repo.GetModelUsageSummary(s.normalizeQuery(query, s.now()))
}

func (s *UsageService) Trends(query domain.ModelUsageQuery) ([]domain.ModelUsageTrendPoint, error) {
	return s.repo.ListModelUsageTrends(s.normalizeQuery(query, s.now()))
}

func (s *UsageService) TopModels(query domain.ModelUsageQuery) ([]domain.ModelUsageBreakdownRow, error) {
	return s.repo.ListTopModelUsage(s.normalizeQuery(query, s.now()))
}

func (s *UsageService) TopAgents(query domain.ModelUsageQuery) ([]domain.ModelUsageBreakdownRow, error) {
	return s.repo.ListTopAgentUsage(s.normalizeQuery(query, s.now()))
}

func (s *UsageService) Events(query domain.ModelUsageQuery) ([]domain.ModelTokenUsageEvent, error) {
	return s.repo.ListModelUsageEvents(s.normalizeQuery(query, s.now()))
}

func (s *UsageService) normalizeQuery(query domain.ModelUsageQuery, now time.Time) domain.ModelUsageQuery {
	if query.StartDate != "" && query.EndDate != "" {
		return query
	}
	end := dateKey(now)
	start := now.AddDate(0, 0, -29)
	switch query.Range {
	case "today":
		start = now
	case "7d":
		start = now.AddDate(0, 0, -6)
	case "month":
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	case "30d", "":
		start = now.AddDate(0, 0, -29)
	default:
		start = now.AddDate(0, 0, -29)
	}
	if query.StartDate == "" {
		query.StartDate = dateKey(start)
	}
	if query.EndDate == "" {
		query.EndDate = end
	}
	return query
}

func withLimit(query domain.ModelUsageQuery, limit int) domain.ModelUsageQuery {
	query.Limit = limit
	return query
}

func dateKey(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}
