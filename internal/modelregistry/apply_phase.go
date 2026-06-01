package modelregistry

import (
	"encoding/json"
	"strings"
	"time"
)

type ApplyPhase struct {
	reader ApplyReader
	writer ApplyWriter
}

func NewApplyPhase(reader ApplyReader, writer ApplyWriter) *ApplyPhase {
	return &ApplyPhase{reader: reader, writer: writer}
}

func (p *ApplyPhase) Name() string         { return "apply" }
func (p *ApplyPhase) Timeout() time.Duration { return 300 * time.Second }

func (p *ApplyPhase) Run(pc *PhaseContext) PhaseResult {
	if pc.Directory == nil || len(pc.Directory) == 0 {
		return PhaseResult{PhaseName: "apply", Status: PhaseSkipped}
	}
	mode := strings.ToLower(strings.TrimSpace(pc.Policy.AutoApply))
	if mode == "" || mode == "none" {
		return PhaseResult{PhaseName: "apply", Status: PhaseSkipped}
	}

	rows, err := p.reader.ListProviderModels(pc.Ctx)
	if err != nil {
		return PhaseResult{PhaseName: "apply", Status: PhaseFailed, Errors: []string{err.Error()}}
	}

	var patches []ApplyRow
	var pricingUpserts []PricingUpsert
	llmRowsUpdated := 0
	llmRowsDisabled := 0

	for _, row := range rows {
		var cfgProbe map[string]any
		_ = json.Unmarshal([]byte(row.ConfigJSON), &cfgProbe)
		if shouldSkipDirectoryApply(cfgProbe, row.MetadataJSON) {
			continue
		}
		providerID := MigrateProviderCode(row.Provider)
		prov, ok := pc.Directory[providerID]
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
				llmRowsDisabled++
			}
		}

		if providerID != row.Provider || cfgChanged || metaChanged || backfillChanged || patch.Enabled != row.Enabled {
			patches = append(patches, patch)
			llmRowsUpdated++
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
			pricingUpserts = append(pricingUpserts, PricingUpsert{
				Provider: patch.Provider,
				Model:    patch.Model,
				Micro:    micro,
				Source:   "models.dev-sync",
			})
		}
	}

	result := p.writer.BatchApply(pc.Ctx, patches, pricingUpserts)
	return PhaseResult{
		PhaseName: "apply",
		Status:    PhaseSucceeded,
		Stats:     map[string]int{"rows_updated": llmRowsUpdated, "rows_disabled": llmRowsDisabled, "pricing_updated": result.PricingUpdated},
		Errors:    result.Errors,
	}
}
