package service

import (
	"context"
	"strings"

	adkr "arenea/backend/internal/conversation/adapters/adkruntime"
	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

type PluginService struct {
	repo repository.Store
}

func NewPluginService(repo repository.Store) *PluginService {
	return &PluginService{repo: repo}
}

func (s *PluginService) SyncBuiltins() error {
	for _, def := range adkr.BuiltinPluginDefinitions() {
		_, err := s.repo.UpsertPlugin(domain.Plugin{
			ID:                "plugin_" + strings.ReplaceAll(def.Key, "-", "_"),
			Key:               def.Key,
			Name:              def.Name,
			Description:       def.Description,
			Category:          def.Category,
			RiskLevel:         def.RiskLevel,
			Enabled:           false,
			Scope:             "global",
			CallbackPoints:    def.CallbackPoints,
			SortOrder:         def.SortOrder,
			ConfigSchemaJSON:  firstNonEmptyString(def.ConfigSchemaJSON, "{}"),
			ConfigJSON:        firstNonEmptyString(def.DefaultConfigJSON, "{}"),
			DefaultConfigJSON: firstNonEmptyString(def.DefaultConfigJSON, "{}"),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *PluginService) List(query domain.PluginListQuery) (domain.PluginListResult, error) {
	return s.repo.SearchPlugins(query)
}

func (s *PluginService) ToggleEnabled(id string, enabled bool) (domain.Plugin, error) {
	if strings.TrimSpace(id) == "" {
		return domain.Plugin{}, validationError("plugin id is required")
	}
	return s.repo.UpdatePluginEnabled(id, enabled)
}

func (s *PluginService) UpdateConfig(id string, configJSON string) (domain.Plugin, error) {
	if strings.TrimSpace(id) == "" {
		return domain.Plugin{}, validationError("plugin id is required")
	}
	return s.repo.UpdatePluginConfig(id, configJSON)
}

func (s *PluginService) EnabledPluginKeys(_ context.Context) ([]string, error) {
	return s.repo.ListEnabledPluginKeys()
}

func (s *PluginService) EnabledPluginConfigs(_ context.Context) ([]adkr.PluginRuntimeConfig, error) {
	result, err := s.repo.SearchPlugins(domain.PluginListQuery{Enabled: "true", Limit: 100})
	if err != nil {
		return nil, err
	}
	items := make([]adkr.PluginRuntimeConfig, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, adkr.PluginRuntimeConfig{Key: item.Key, ConfigJSON: item.ConfigJSON})
	}
	return items, nil
}
