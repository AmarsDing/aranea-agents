package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/glebarez/go-sqlite/compat"
)

func main() {
	dbPath := "data/aranea.db"
	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	db.SetMaxOpenConns(1)

	ctx := context.Background()

	metricsDiff := checkSessionMetrics(ctx, db)
	runtimeDiff := checkSessionRuntime(ctx, db)

	if metricsDiff == 0 && runtimeDiff == 0 {
		fmt.Println("No differences found between sessions and session_metrics/session_runtime tables")
	} else {
		fmt.Fprintf(os.Stderr, "Found %d metrics diffs, %d runtime diffs\n", metricsDiff, runtimeDiff)
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// session_metrics
// ---------------------------------------------------------------------------

type metricsRow struct {
	sessionID          string
	sMessageCount      int
	mMessageCount      int
	sRunCount          int
	mRunCount          int
	sModelCallCount    int
	mModelCallCount    int
	sToolCallCount     int
	mToolCallCount     int
	sSkillCallCount    int
	mSkillCallCount    int
	sMcpCallCount      int
	mMcpCallCount      int
	sInputTokens       int
	mInputTokens       int
	sOutputTokens      int
	mOutputTokens      int
	sTotalTokens       int
	mTotalTokens       int
	sTotalCostMicroUsd int64
	mTotalCostMicroUsd int64
	sAvgLatencyMs      float64
	mAvgLatencyMs      float64
	sErrorCount        int
	mErrorCount        int
	sContextUsedTokens int
	mContextUsedTokens int
	sContextUsedRatio  float64
	mContextUsedRatio  float64
	sMaxContextRatio   float64
	mMaxContextRatio   float64
	sContextStatus     string
	mContextStatus     string
	sLastMessageAt     string
	mLastMessageAt     string
}

func checkSessionMetrics(ctx context.Context, db *sql.DB) int {
	const query = `
		SELECT
			s.id,
			s.message_count,   m.message_count,
			s.run_count,       m.run_count,
			s.model_call_count,m.model_call_count,
			s.tool_call_count, m.tool_call_count,
			s.skill_call_count,m.skill_call_count,
			s.mcp_call_count,  m.mcp_call_count,
			s.input_tokens,    m.input_tokens,
			s.output_tokens,   m.output_tokens,
			s.total_tokens,    m.total_tokens,
			s.total_cost_micro_usd, m.total_cost_micro_usd,
			s.avg_latency_ms,  m.avg_latency_ms,
			s.error_count,     m.error_count,
			s.context_used_tokens, m.context_used_tokens,
			s.context_used_ratio,  m.context_used_ratio,
			s.max_context_used_ratio, m.max_context_used_ratio,
			s.context_status,  m.context_status,
			s.last_message_at, m.last_message_at
		FROM sessions s
		INNER JOIN session_metrics m ON s.id = m.session_id
		WHERE (s.deleted_at = '' OR s.deleted_at IS NULL)
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "query session_metrics: %v\n", err)
		return -1
	}
	defer rows.Close()

	diffCount := 0
	for rows.Next() {
		var r metricsRow
		err := rows.Scan(
			&r.sessionID,
			&r.sMessageCount, &r.mMessageCount,
			&r.sRunCount, &r.mRunCount,
			&r.sModelCallCount, &r.mModelCallCount,
			&r.sToolCallCount, &r.mToolCallCount,
			&r.sSkillCallCount, &r.mSkillCallCount,
			&r.sMcpCallCount, &r.mMcpCallCount,
			&r.sInputTokens, &r.mInputTokens,
			&r.sOutputTokens, &r.mOutputTokens,
			&r.sTotalTokens, &r.mTotalTokens,
			&r.sTotalCostMicroUsd, &r.mTotalCostMicroUsd,
			&r.sAvgLatencyMs, &r.mAvgLatencyMs,
			&r.sErrorCount, &r.mErrorCount,
			&r.sContextUsedTokens, &r.mContextUsedTokens,
			&r.sContextUsedRatio, &r.mContextUsedRatio,
			&r.sMaxContextRatio, &r.mMaxContextRatio,
			&r.sContextStatus, &r.mContextStatus,
			&r.sLastMessageAt, &r.mLastMessageAt,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "scan metrics row: %v\n", err)
			continue
		}

		diffs := compareMetrics(r)
		if len(diffs) > 0 {
			diffCount++
			fmt.Printf("[metrics] session %s: %s\n", r.sessionID, strings.Join(diffs, ", "))
		}
	}

	// Check sessions that exist in sessions but not in session_metrics
	missingMetrics(ctx, db)

	// Check session_metrics that exist but not in sessions
	orphanMetrics(ctx, db)

	fmt.Printf("[metrics] total diffs: %d\n", diffCount)
	return diffCount
}

func compareMetrics(r metricsRow) []string {
	var diffs []string
	add := func(name string, sVal, mVal interface{}) {
		diffs = append(diffs, fmt.Sprintf("%s: sessions=%v metrics=%v", name, sVal, mVal))
	}
	if r.sMessageCount != r.mMessageCount {
		add("message_count", r.sMessageCount, r.mMessageCount)
	}
	if r.sRunCount != r.mRunCount {
		add("run_count", r.sRunCount, r.mRunCount)
	}
	if r.sModelCallCount != r.mModelCallCount {
		add("model_call_count", r.sModelCallCount, r.mModelCallCount)
	}
	if r.sToolCallCount != r.mToolCallCount {
		add("tool_call_count", r.sToolCallCount, r.mToolCallCount)
	}
	if r.sSkillCallCount != r.mSkillCallCount {
		add("skill_call_count", r.sSkillCallCount, r.mSkillCallCount)
	}
	if r.sMcpCallCount != r.mMcpCallCount {
		add("mcp_call_count", r.sMcpCallCount, r.mMcpCallCount)
	}
	if r.sInputTokens != r.mInputTokens {
		add("input_tokens", r.sInputTokens, r.mInputTokens)
	}
	if r.sOutputTokens != r.mOutputTokens {
		add("output_tokens", r.sOutputTokens, r.mOutputTokens)
	}
	if r.sTotalTokens != r.mTotalTokens {
		add("total_tokens", r.sTotalTokens, r.mTotalTokens)
	}
	if r.sTotalCostMicroUsd != r.mTotalCostMicroUsd {
		add("total_cost_micro_usd", r.sTotalCostMicroUsd, r.mTotalCostMicroUsd)
	}
	if r.sAvgLatencyMs != r.mAvgLatencyMs {
		add("avg_latency_ms", r.sAvgLatencyMs, r.mAvgLatencyMs)
	}
	if r.sErrorCount != r.mErrorCount {
		add("error_count", r.sErrorCount, r.mErrorCount)
	}
	if r.sContextUsedTokens != r.mContextUsedTokens {
		add("context_used_tokens", r.sContextUsedTokens, r.mContextUsedTokens)
	}
	if r.sContextUsedRatio != r.mContextUsedRatio {
		add("context_used_ratio", r.sContextUsedRatio, r.mContextUsedRatio)
	}
	if r.sMaxContextRatio != r.mMaxContextRatio {
		add("max_context_used_ratio", r.sMaxContextRatio, r.mMaxContextRatio)
	}
	if r.sContextStatus != r.mContextStatus {
		add("context_status", r.sContextStatus, r.mContextStatus)
	}
	if r.sLastMessageAt != r.mLastMessageAt {
		add("last_message_at", r.sLastMessageAt, r.mLastMessageAt)
	}
	return diffs
}

func missingMetrics(ctx context.Context, db *sql.DB) {
	const query = `
		SELECT s.id FROM sessions s
		LEFT JOIN session_metrics m ON s.id = m.session_id
		WHERE m.session_id IS NULL
		  AND (s.deleted_at = '' OR s.deleted_at IS NULL)
	`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "query missing metrics: %v\n", err)
		return
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) > 0 {
		fmt.Printf("[metrics] sessions missing from session_metrics (%d): %s\n", len(ids), strings.Join(ids, ", "))
	}
}

func orphanMetrics(ctx context.Context, db *sql.DB) {
	const query = `
		SELECT m.session_id FROM session_metrics m
		LEFT JOIN sessions s ON m.session_id = s.id
		WHERE s.id IS NULL
	`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "query orphan metrics: %v\n", err)
		return
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) > 0 {
		fmt.Printf("[metrics] orphan session_metrics without sessions (%d): %s\n", len(ids), strings.Join(ids, ", "))
	}
}

// ---------------------------------------------------------------------------
// session_runtime
// ---------------------------------------------------------------------------

type runtimeRow struct {
	sessionID           string
	sSessionRevision    int64
	rSessionRevision    int64
	sStateJSON          string
	rStateJSON          string
	sRunnerSnapshotJSON string
	rRunnerSnapshotJSON string
	sMetadataJSON       string
	rMetadataJSON       string
	sCompressVersion    int64
	rCompressVersion    int64
}

func checkSessionRuntime(ctx context.Context, db *sql.DB) int {
	const query = `
		SELECT
			s.id,
			s.session_revision,  r.session_revision,
			s.state_json,        r.state_json,
			s.runner_snapshot_json, r.runner_snapshot_json,
			s.metadata_json,     r.metadata_json,
			s.compress_version,  r.compress_version
		FROM sessions s
		INNER JOIN session_runtime r ON s.id = r.session_id
		WHERE (s.deleted_at = '' OR s.deleted_at IS NULL)
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "query session_runtime: %v\n", err)
		return -1
	}
	defer rows.Close()

	diffCount := 0
	for rows.Next() {
		var r runtimeRow
		err := rows.Scan(
			&r.sessionID,
			&r.sSessionRevision, &r.rSessionRevision,
			&r.sStateJSON, &r.rStateJSON,
			&r.sRunnerSnapshotJSON, &r.rRunnerSnapshotJSON,
			&r.sMetadataJSON, &r.rMetadataJSON,
			&r.sCompressVersion, &r.rCompressVersion,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "scan runtime row: %v\n", err)
			continue
		}

		diffs := compareRuntime(r)
		if len(diffs) > 0 {
			diffCount++
			fmt.Printf("[runtime] session %s: %s\n", r.sessionID, strings.Join(diffs, ", "))
		}
	}

	missingRuntime(ctx, db)
	orphanRuntime(ctx, db)

	fmt.Printf("[runtime] total diffs: %d\n", diffCount)
	return diffCount
}

func compareRuntime(r runtimeRow) []string {
	var diffs []string
	add := func(name string, sVal, rVal interface{}) {
		diffs = append(diffs, fmt.Sprintf("%s: sessions=%v runtime=%v", name, sVal, rVal))
	}
	if r.sSessionRevision != r.rSessionRevision {
		add("session_revision", r.sSessionRevision, r.rSessionRevision)
	}
	if r.sStateJSON != r.rStateJSON {
		add("state_json", summarizeStr(r.sStateJSON), summarizeStr(r.rStateJSON))
	}
	if r.sRunnerSnapshotJSON != r.rRunnerSnapshotJSON {
		add("runner_snapshot_json", summarizeStr(r.sRunnerSnapshotJSON), summarizeStr(r.rRunnerSnapshotJSON))
	}
	if r.sMetadataJSON != r.rMetadataJSON {
		add("metadata_json", summarizeStr(r.sMetadataJSON), summarizeStr(r.rMetadataJSON))
	}
	if r.sCompressVersion != r.rCompressVersion {
		add("compress_version", r.sCompressVersion, r.rCompressVersion)
	}
	return diffs
}

func missingRuntime(ctx context.Context, db *sql.DB) {
	const query = `
		SELECT s.id FROM sessions s
		LEFT JOIN session_runtime r ON s.id = r.session_id
		WHERE r.session_id IS NULL
		  AND (s.deleted_at = '' OR s.deleted_at IS NULL)
	`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "query missing runtime: %v\n", err)
		return
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) > 0 {
		fmt.Printf("[runtime] sessions missing from session_runtime (%d): %s\n", len(ids), strings.Join(ids, ", "))
	}
}

func orphanRuntime(ctx context.Context, db *sql.DB) {
	const query = `
		SELECT r.session_id FROM session_runtime r
		LEFT JOIN sessions s ON r.session_id = s.id
		WHERE s.id IS NULL
	`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "query orphan runtime: %v\n", err)
		return
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) > 0 {
		fmt.Printf("[runtime] orphan session_runtime without sessions (%d): %s\n", len(ids), strings.Join(ids, ", "))
	}
}

// summarizeStr returns a truncated summary of a JSON string for display.
func summarizeStr(s string) string {
	const maxLen = 80
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
