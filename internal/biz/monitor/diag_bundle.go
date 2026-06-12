package monitor

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"

	"github.com/google/uuid"
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

// DiagBundleGenerator generates diagnostic bundles for self-heal and RCA.
type DiagBundleGenerator struct {
	eventRepo     EventRepo
	traceRepo     TraceRepo
	engine        *RootCauseEngine
	selfCheckRepo SelfCheckReportRepo
}

func NewDiagBundleGenerator(eventRepo EventRepo, traceRepo TraceRepo, engine *RootCauseEngine) *DiagBundleGenerator {
	if eventRepo == nil || traceRepo == nil || engine == nil {
		return nil
	}
	return &DiagBundleGenerator{eventRepo: eventRepo, traceRepo: traceRepo, engine: engine}
}

// SetSelfCheckRepo injects the self-check report repo for diagnostic snapshots.
func (g *DiagBundleGenerator) SetSelfCheckRepo(repo SelfCheckReportRepo) {
	if g != nil {
		g.selfCheckRepo = repo
	}
}

func (g *DiagBundleGenerator) Generate(ctx context.Context, traceID, sessionID, runID, stepID, triggerType string, contextMinutes int32) (*DiagBundle, error) {
	if g == nil {
		return nil, apierror.Internal(apierror.DomainMonitor, "DiagBundleGenerator is nil")
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

	// Query runner.completion and alert events with EventType filter to avoid full table scan.
	var flowEntries []map[string]any
	var alertEntries []map[string]any
	var triggerMetadata map[string]any
	total := 0

	if sessionID != "" || traceID != "" {
		// Fetch runner.completion events with SQL-level filtering
		completionEvents, err := g.eventRepo.ListMonitorEvents(ctx, EventsQuery{
			Limit:     500,
			Offset:    0,
			EventType: "runner.completion",
			SessionID: sessionID,
			TraceID:   traceID,
		})
		if err == nil && completionEvents.Items != nil {
			for _, row := range completionEvents.Items {
				m := map[string]any{
					"id": row.ID, "name": row.Name, "status": row.Status,
					"created_at": row.CreatedAt, "metadata_json": row.MetadataJSON,
				}
				flowEntries = append(flowEntries, m)
				total++
				if triggerMetadata == nil && stepID != "" && strings.Contains(row.Key, stepID) {
					var parsed map[string]any
					if json.Unmarshal([]byte(row.MetadataJSON), &parsed) == nil {
						triggerMetadata = parsed
					}
				}
			}
		}
		// Fetch alert events with SQL-level filtering
		alertEvents, err := g.eventRepo.ListMonitorEvents(ctx, EventsQuery{
			Limit:     200,
			Offset:    0,
			EventType: "alert",
			SessionID: sessionID,
			TraceID:   traceID,
		})
		if err == nil && alertEvents.Items != nil {
			for _, row := range alertEvents.Items {
				m := map[string]any{
					"id": row.ID, "name": row.Name, "status": row.Status,
					"created_at": row.CreatedAt, "metadata_json": row.MetadataJSON,
				}
				alertEntries = append(alertEntries, m)
				flowEntries = append(flowEntries, m)
				total++
			}
		}
	}

	var traceData map[string]any
	spanCount := 0
	if traceID != "" {
		traceRow, err := g.traceRepo.GetMonitorTrace(ctx, traceID)
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
	usageEvents, err := g.eventRepo.ListMonitorEvents(ctx, EventsQuery{
		Limit:     50,
		Offset:    0,
		EventType: "usage",
		SessionID: sessionID,
		TraceID:   traceID,
	})
	if err == nil && usageEvents.Items != nil {
		for _, row := range usageEvents.Items {
			if !strings.HasPrefix(row.Key, "usage") {
				continue
			}
			usageRows = append(usageRows, map[string]any{
				"id": row.ID, "name": row.Name, "status": row.Status,
				"created_at": row.CreatedAt, "metadata_json": row.MetadataJSON,
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

	// Parse auto-heal metadata from flow entries for self-heal summary
	var autoHealCount, healSuccessCount, healFailCount int
	for _, entry := range flowEntries {
		if m, ok := entry["metadata_json"].(string); ok {
			var parsed map[string]any
			if json.Unmarshal([]byte(m), &parsed) == nil {
				if v, ok := parsed["auto_healed"].(bool); ok && v {
					autoHealCount++
					if s, ok := parsed["heal_success"].(bool); ok && s {
						healSuccessCount++
					} else {
						healFailCount++
					}
				}
			}
		}
	}
	if autoHealCount > 0 {
		manifest["self_heal_summary"] = map[string]any{
			"auto_heal_count":    autoHealCount,
			"heal_success_count": healSuccessCount,
			"heal_fail_count":    healFailCount,
		}
	}

	// Self-check snapshot: include the most recent self-check report
	var selfCheckSnapshot map[string]any
	if g.selfCheckRepo != nil {
		reports, _, _ := g.selfCheckRepo.ListSelfCheckReports(ctx, 1, 0)
		if len(reports) > 0 {
			latest := reports[0]
			selfCheckSnapshot = map[string]any{
				"id":             latest.ID,
				"overall_status": string(latest.OverallStatus),
				"started_at":     latest.StartedAt.Format(time.RFC3339),
				"finished_at":    latest.FinishedAt.Format(time.RFC3339),
				"duration_ms":    latest.DurationMs,
				"check_count":    len(latest.CheckResults),
			}
			manifest["files"].(map[string]any)["self_check_snapshot"] = map[string]any{"report_id": latest.ID}
		}
	}

	var rootCauseResults []RootCauseResult
	if g.engine != nil && stepID != "" {
		rootCauseResults = g.engine.Evaluate(ctx, stepID, "error", triggerMetadata)
	}

	if selfCheckSnapshot != nil {
		manifest["self_check_snapshot"] = selfCheckSnapshot
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

// metadataMatchesID checks if a JSON metadata string contains an exact match
// for the given traceID or sessionID in the corresponding JSON fields.
// Returns true if at least one specified ID matches its field exactly.
// Returns false if both IDs are empty (no filter criteria).
// This avoids false positives from strings.Contains which could match partial
// strings in unrelated JSON fields (e.g., traceID "abc" matching "xyz_abc_field").
func metadataMatchesID(metadataJSON, traceID, sessionID string) bool {
	if traceID == "" && sessionID == "" {
		return false
	}
	parsed := parseMetadataJSON(metadataJSON)
	if parsed == nil {
		return false
	}
	if traceID != "" {
		if v, ok := parsed["trace_id"].(string); ok && v == traceID {
			return true
		}
	}
	if sessionID != "" {
		if v, ok := parsed["session_id"].(string); ok && v == sessionID {
			return true
		}
	}
	return false
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
