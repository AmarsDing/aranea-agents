package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent/agenttemplate"

	entsql "entgo.io/ent/dialect/sql"
)

type agentTemplateRepo struct {
	data *Data
}

func NewAgentTemplateRepo(d *Data) biz.AgentTemplateRepo {
	return &agentTemplateRepo{data: d}
}

func (r *agentTemplateRepo) ListAgentTemplates(ctx context.Context) ([]biz.AgentTemplate, error) {
	rows, err := r.data.ReadClient(ctx).AgentTemplate.Query().
		Where(agenttemplate.DeletedAtEQ("")).
		Order(agenttemplate.BySortOrder(entsql.OrderAsc())).
		All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]biz.AgentTemplate, 0, len(rows))
	for _, row := range rows {
		result = append(result, biz.AgentTemplate{
			Key:         row.TemplateKey,
			Label:       row.Label,
			Icon:        row.Icon,
			Description: row.Description,
			DisplayName: row.DisplayName,
			Provider:    row.Provider,
			Model:       row.Model,
		})
	}
	return result, nil
}
