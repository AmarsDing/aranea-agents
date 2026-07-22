package data

import (
	"context"
	"encoding/json"

	bizmedia "aranea-agents/internal/biz/media"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/mediaprovider"
	"aranea-agents/pkg/apierror"

	entsql "entgo.io/ent/dialect/sql"
)

type mediaProviderRepo struct {
	data *Data
}

var _ bizmedia.ProviderReader = (*mediaProviderRepo)(nil)

// NewMediaProviderRepo exposes the media_providers catalog as a biz reader.
func NewMediaProviderRepo(d *Data) bizmedia.ProviderReader {
	return &mediaProviderRepo{data: d}
}

func entToBizMediaProvider(e *ent.MediaProvider) bizmedia.ProviderConfig {
	if e == nil {
		return bizmedia.ProviderConfig{}
	}
	var caps []string
	// Capabilities is a JSON array string; malformed rows simply yield no caps.
	_ = json.Unmarshal([]byte(e.Capabilities), &caps)
	return bizmedia.ProviderConfig{
		ID:           e.ID,
		Name:         e.Name,
		ProviderType: e.ProviderType,
		BaseURL:      e.BaseURL,
		APIKey:       e.APIKey,
		ConfigJSON:   e.ConfigJSON,
		Capabilities: caps,
		Status:       e.Status,
	}
}

func (r *mediaProviderRepo) ActiveProviderFor(ctx context.Context, cap bizmedia.Capability) (bizmedia.ProviderConfig, error) {
	rows, err := r.data.RW().Read(ctx).MediaProvider.Query().
		Where(mediaprovider.StatusEQ("active")).
		Order(mediaprovider.ByCreatedAt(entsql.OrderAsc())).
		All(ctx)
	if err != nil {
		return bizmedia.ProviderConfig{}, entErrToBizErr(err, apierror.DomainTool)
	}
	for _, row := range rows {
		cfg := entToBizMediaProvider(row)
		if cfg.Supports(cap) {
			return cfg, nil
		}
	}
	return bizmedia.ProviderConfig{}, apierror.NotFound(apierror.DomainTool, "no active media provider for capability %q", string(cap))
}
