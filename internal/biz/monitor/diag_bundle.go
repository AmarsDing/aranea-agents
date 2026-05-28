package monitor

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type DiagBundle struct {
	BundleID    string
	Manifest    map[string]any
	FlowJSONL   string
	TraceJSON   string
	UsageJSON   string
	AlertsJSONL string
	RootCauses  []RootCauseResult
	Total       int
}

type DiagBundleGenerator struct {
	repo   Repo
	engine *RootCauseEngine
}

func NewDiagBundleGenerator(repo Repo) *DiagBundleGenerator {
	if repo == nil {
		return nil
	}
	return &DiagBundleGenerator{repo: repo, engine: NewRootCauseEngine()}
}

func (g *DiagBundleGenerator) Generate(ctx context.Context, traceID, sessionID, runID, stepID, triggerType string, contextMinutes int32) (*DiagBundle, error) {
	if g == nil {
		return nil, kerrors.InternalServer("MONITOR", "DiagBundleGenerator is nil")
	}
	if contextMinutes <= 0 {
		contextMinutes = 5
	}
	bundleID := uuid.NewString()
	now := time.Now().UTC()
	scopeStart := now.Add(-time.Duration(contextMinutes) * time.Minute)

	manifest := map[string]any{
		"schema_version": "diag_bundle/v1",
		"bundle_id":      bundleID,
		"created_at":     now.Format(time.RFC3339),
		"trigger": map[string]any{
			"type":       triggerType,
			"trace_id":   traceID,
			"session_id": sessionID,
			"run_id":     runID,
			"step_id":    stepID,
		},
		"scope": map[string]any{
			"time_range":  []string{scopeStart.Format(time.RFC3339), now.Format(time.RFC3339)},
			"trace_ids":   nonEmpty(traceID),
			"session_ids": nonEmpty(sessionID),
			"run_ids":     nonEmpty(runID),
		},
	}

	var flowEntries []map[string]any
	var alertEntries []map[string]any
	var triggerMetadata map[string]any
	total := 0

	if sessionID != "" || traceID != "" {
		events, err := g.repo.ListMonitorEvents(ctx, EventsQuery{
			Limit:  500,
			Offset: 0,
			Status: "",
		})
		if err == nil && events.Items != nil {
			for _, row := range events.Items {
				mj := row.MetadataJSON
				matched := false
				if traceID != "" && strings.Contains(mj, traceID) {
					matched = true
				}
				if !matched && sessionID != "" && strings.Contains(mj, sessionID) {
					matched = true
				}
				if !matched {
					continue
				}
				m := map[string]any{
					"id": row.ID, "name": row.Name, "status": row.Status,
					"created_at": row.CreatedAt, "metadata_json": row.MetadataJSON,
				}
				flowEntries = append(flowEntries, m)
				total++
				if strings.HasPrefix(row.Key, "alert.") {
					alertEntries = append(alertEntries, m)
				}
				if triggerMetadata == nil && stepID != "" && strings.Contains(row.Key, stepID) {
					var parsed map[string]any
					if json.Unmarshal([]byte(mj), &parsed) == nil {
						triggerMetadata = parsed
					}
				}
			}
		}
	}

	var traceData map[string]any
	spanCount := 0
	if traceID != "" {
		traceRow, err := g.repo.GetMonitorTrace(ctx, traceID)
		if err == nil {
			traceData = map[string]any{
				"id": traceRow.ID, "name": traceRow.Name, "status": traceRow.Status,
				"created_at": traceRow.CreatedAt, "metadata_json": traceRow.MetadataJSON,
			}
			total++
			if cfg := parseMetadataJSON(traceRow.MetadataJSON); cfg != nil {
				if sc, ok := cfg["span_count"]; ok {
					switch v := sc.(type) {
					case float64:
						spanCount = int(v)
					case int:
						spanCount = v
					}
				}
			}
		}
	}

	var usageData map[string]any
	var usageRows []map[string]any
	usageEvents, err := g.repo.ListMonitorEvents(ctx, EventsQuery{
		Limit:  50,
		Offset: 0,
		Status: "",
	})
	if err == nil && usageEvents.Items != nil {
		for _, row := range usageEvents.Items {
			if !strings.HasPrefix(row.Key, "usage") {
				continue
			}
			mj := row.MetadataJSON
			if traceID != "" && !strings.Contains(mj, traceID) {
				if sessionID != "" && !strings.Contains(mj, sessionID) {
					continue
				}
			}
			usageRows = append(usageRows, map[string]any{
				"id": row.ID, "name": row.Name, "status": row.Status,
				"created_at": row.CreatedAt, "metadata_json": mj,
			})
		}
		if len(usageRows) > 0 {
			usageData = map[string]any{"records": usageRows}
			total += len(usageRows)
		}
	}

	flowJSONL, _ := json.Marshal(flowEntries)
	alertsJSONL, _ := json.Marshal(alertEntries)
	traceJSON, _ := json.Marshal(traceData)
	usageJSON, _ := json.Marshal(usageData)

	manifest["files"] = map[string]any{
		"flow.jsonl":   map[string]any{"entries": len(flowEntries)},
		"trace.json":   map[string]any{"spans": spanCount},
		"usage.json":   map[string]any{"records": len(usageRows)},
		"alerts.jsonl": map[string]any{"entries": len(alertEntries)},
	}

	var rootCauseResults []RootCauseResult
	if g.engine != nil && stepID != "" {
		rootCauseResults = g.engine.Evaluate(ctx, stepID, "error", triggerMetadata)
	}

	return &DiagBundle{
		BundleID:    bundleID,
		Manifest:    manifest,
		FlowJSONL:   string(flowJSONL),
		TraceJSON:   string(traceJSON),
		UsageJSON:   string(usageJSON),
		AlertsJSONL: string(alertsJSONL),
		RootCauses:  rootCauseResults,
		Total:       total,
	}, nil
}

func parseMetadataJSON(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return nil
	}
	return m
}

func nonEmpty(ss ...string) []string {
	var out []string
	for _, s := range ss {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}
