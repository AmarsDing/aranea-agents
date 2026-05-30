package modelregistry

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

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

type ApplyMigrationStats struct {
	Agents          int
	Sessions        int
	Eval            int
	RuntimeSettings int
	Skills          int
	KnowledgeEmbed  int
	WebResearch     int
}

type PricingUpsert struct {
	Provider string
	Model    string
	Micro    MicroPricing
	Source   string
}

type BatchMigrationResult struct {
	CompletedRules []string
	FailedRules    []string
	Stats          ApplyMigrationStats
	Errors         []string
}

type BatchApplyResult struct {
	RowsUpdated    int
	PricingUpdated int
	Errors         []string
}

type ApplyReader interface {
	ListProviderModels(ctx context.Context) ([]ApplyRow, error)
	CountProviderBindings(ctx context.Context, provider string) (ApplyMigrationStats, error)
}

type ApplyWriter interface {
	SaveProviderModel(ctx context.Context, row ApplyRow) error
	UpsertModelPricing(ctx context.Context, provider, model string, micro MicroPricing, source string) error
	BatchApply(ctx context.Context, patches []ApplyRow, pricing []PricingUpsert) BatchApplyResult
}

type MigrationWriter interface {
	MigrateProviderBindings(ctx context.Context, from, to string) (ApplyMigrationStats, error)
	BatchMigrateProviderBindings(ctx context.Context, rules []ProviderMigrationRule, skipRules []string) BatchMigrationResult
}

type ApplyBackend interface {
	ApplyReader
	ApplyWriter
	MigrationWriter
}

type ApplyResult struct {
	LLMRowsUpdated      int
	LLMRowsDisabled     int
	PricingRulesUpdated int
	Migration           ApplyMigrationStats
	Errors              []string
}

type Applier struct {
	backend ApplyBackend
}

func NewApplier(backend ApplyBackend) *Applier {
	return &Applier{backend: backend}
}

func (a *Applier) Apply(ctx context.Context, cat Directory, autoApply string) ApplyResult {
	if a == nil || a.backend == nil || len(cat) == 0 {
		return ApplyResult{}
	}
	mode := strings.ToLower(strings.TrimSpace(autoApply))
	if mode == "" || mode == "none" {
		return ApplyResult{}
	}

	var res ApplyResult
	rows, err := a.backend.ListProviderModels(ctx)
	if err != nil {
		res.Errors = append(res.Errors, err.Error())
		return res
	}

	for _, row := range rows {
		var cfgProbe map[string]any
		_ = json.Unmarshal([]byte(row.ConfigJSON), &cfgProbe)
		if shouldSkipDirectoryApply(cfgProbe, row.MetadataJSON) {
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

		if strings.EqualFold(model.Status, "deprecated") && isDirectoryManaged(patch.ConfigJSON) {
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

func (a *Applier) ApplyWithMigration(ctx context.Context, cat Directory, autoApply string) ApplyResult {
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

	mergeRes := a.Apply(ctx, cat, autoApply)
	res.LLMRowsUpdated = mergeRes.LLMRowsUpdated
	res.LLMRowsDisabled = mergeRes.LLMRowsDisabled
	res.PricingRulesUpdated = mergeRes.PricingRulesUpdated
	res.Errors = append(res.Errors, mergeRes.Errors...)
	return res
}

func isDirectoryManaged(configJSON string) bool {
	var cfg struct {
		CatalogManaged bool   `json:"catalog_managed"`
		CatalogSource  string `json:"catalog_source"`
	}
	_ = json.Unmarshal([]byte(configJSON), &cfg)
	return cfg.CatalogManaged || strings.EqualFold(cfg.CatalogSource, "models.dev")
}
