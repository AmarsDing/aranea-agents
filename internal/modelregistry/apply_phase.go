package modelregistry

import (
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
)

type ApplyPhase struct {
	reader ApplyReader
	writer ApplyWriter
	lg     loggateway.Logger
}

func NewApplyPhase(reader ApplyReader, writer ApplyWriter, lg loggateway.Logger) *ApplyPhase {
	return &ApplyPhase{reader: reader, writer: writer, lg: lg}
}

func (p *ApplyPhase) Name() string           { return "apply" }
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
		if err := json.Unmarshal([]byte(row.ConfigJSON), &cfgProbe); err != nil {
			p.lg.Warn("解析 provider model config 失败", loggateway.StepID("modelregistry.apply_phase"), loggateway.Err(err))
		}
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

		baseURL := extractAPIBaseURL(p.lg, row.ConfigJSON)
		cfg, cfgChanged := mergeCatalogIntoConfig(p.lg, row.ConfigJSON, prov, model, mode, baseURL)
		meta, metaChanged := mergeCatalogMetadata(p.lg, row.MetadataJSON, prov, model)
		if cfgChanged {
			patch.ConfigJSON = cfg
		}
		if metaChanged {
			patch.MetadataJSON = meta
		}

		if strings.EqualFold(model.Status, "deprecated") && isDirectoryManaged(p.lg, patch.ConfigJSON) {
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
		if err := json.Unmarshal([]byte(pricingJSON), &wrap); err != nil {
			p.lg.Warn("解析 pricing config 失败", loggateway.StepID("modelregistry.apply_phase.pricing"), loggateway.Err(err))
		}
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
