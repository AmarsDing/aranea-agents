package modelcatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ApplyRow is a minimal llm_provider_models row for catalog sync.
type ApplyRow struct {
	ID           string
	Key          string
	Name         string
	Provider     string
	Model        string
	Enabled      bool
	ConfigJSON   string
	MetadataJSON string
}

// ApplyMigrationStats counts binding migrations.
type ApplyMigrationStats struct {
	Agents           int
	Sessions         int
	Eval             int
	RuntimeSettings  int
	Skills           int
	KnowledgeEmbed   int
	WebResearch      int
}

// ApplyBackend persists catalog apply + migration side effects.
type ApplyBackend interface {
	ListProviderModels(ctx context.Context) ([]ApplyRow, error)
	SaveProviderModel(ctx context.Context, row ApplyRow) error
	UpsertModelPricing(ctx context.Context, provider, model string, micro MicroPricing, source string) error
	CountProviderBindings(ctx context.Context, provider string) (ApplyMigrationStats, error)
	MigrateProviderBindings(ctx context.Context, from, to string) (ApplyMigrationStats, error)
}

// ApplyResult summarizes one catalog apply pass.
type ApplyResult struct {
	LLMRowsUpdated      int
	LLMRowsDisabled     int
	PricingRulesUpdated int
	Migration           ApplyMigrationStats
	Errors              []string
}

// Applier merges catalog into DB rows according to auto_apply policy.
type Applier struct {
	backend ApplyBackend
}

func NewApplier(backend ApplyBackend) *Applier {
	return &Applier{backend: backend}
}

func (a *Applier) Apply(ctx context.Context, cat Catalog, autoApply string) ApplyResult {
	if a == nil || a.backend == nil || len(cat) == 0 {
		return ApplyResult{}
	}
	mode := strings.ToLower(strings.TrimSpace(autoApply))
	if mode == "" || mode == "none" {
		return ApplyResult{}
	}

	var res ApplyResult
	stats, errs := RunProviderMigrations(ctx, a.backend)
	res.Migration = stats
	res.Errors = append(res.Errors, errs...)

	rows, err := a.backend.ListProviderModels(ctx)
	if err != nil {
		res.Errors = append(res.Errors, err.Error())
		return res
	}

	for _, row := range rows {
		var cfgProbe map[string]any
		_ = json.Unmarshal([]byte(row.ConfigJSON), &cfgProbe)
		if shouldSkipCatalogApply(cfgProbe, row.MetadataJSON) {
			continue
		}

		providerID := MigrateProviderCode(row.Provider)
		prov, ok := cat[providerID]
		if !ok {
			continue
		}
		model, ok := prov.Models[row.Model]
		if !ok {
			continue
		}

		patch := row
		backfillChanged := false
		if backfilled, ok := BackfillCostFromMicro(row.ConfigJSON); ok {
			patch.ConfigJSON = backfilled
			row.ConfigJSON = backfilled
			backfillChanged = true
		}
		if providerID != row.Provider {
			patch.Provider = providerID
			patch.Key = providerID + ":" + row.Model
		}

		baseURL := extractAPIBaseURL(row.ConfigJSON)
		cfg, cfgChanged := mergeCatalogIntoConfig(row.ConfigJSON, prov, model, mode, baseURL)
		meta, metaChanged := mergeCatalogMetadata(row.MetadataJSON, prov, model)
		if cfgChanged {
			patch.ConfigJSON = cfg
		}
		if metaChanged {
			patch.MetadataJSON = meta
		}

		if strings.EqualFold(model.Status, "deprecated") && isCatalogManaged(patch.ConfigJSON) {
			if patch.Enabled {
				patch.Enabled = false
				res.LLMRowsDisabled++
			}
		}

		if providerID != row.Provider || cfgChanged || metaChanged || backfillChanged || patch.Enabled != row.Enabled {
			if err := a.backend.SaveProviderModel(ctx, patch); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("save %s/%s: %v", patch.Provider, patch.Model, err))
				continue
			}
			res.LLMRowsUpdated++
		}

		pricingJSON := patch.ConfigJSON
		var wrap struct {
			Cost CostUSDPer1M `json:"cost"`
		}
		_ = json.Unmarshal([]byte(pricingJSON), &wrap)
		micro := MicroPricingFromCostBlock(wrap.Cost)
		if micro.Input == 0 && micro.Output == 0 && model.Cost != nil {
			_, micro = MicroPricingFromModelCost(model.Cost)
		}
		if micro.Input > 0 || micro.Output > 0 || micro.CacheRead > 0 || micro.CacheWrite > 0 || micro.Reasoning > 0 || micro.Embedding > 0 {
			if err := a.backend.UpsertModelPricing(ctx, patch.Provider, patch.Model, micro, "models.dev-sync"); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("pricing %s/%s: %v", patch.Provider, patch.Model, err))
			} else {
				res.PricingRulesUpdated++
			}
		}
	}
	return res
}

func isCatalogManaged(configJSON string) bool {
	var cfg struct {
		CatalogManaged bool   `json:"catalog_managed"`
		CatalogSource  string `json:"catalog_source"`
	}
	_ = json.Unmarshal([]byte(configJSON), &cfg)
	return cfg.CatalogManaged || strings.EqualFold(cfg.CatalogSource, "models.dev")
}
