package data

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

// RunMonitorTraceInterruptedBackfillMigration 修复历史假 "interrupted" monitor traces。
//
// 背景：RecordRunnerCompletion 曾长期无生产调用方（event_bus_runner_handler 删除后
// 断链），runner.completion 事件缺失导致清扫器把超时 running 的 trace 一律标记为
// interrupted（生产库 599 条）。实际上这些 trace 的 span 数据完整持久化于
// monitor_trace_spans，可据此重建真实终态：
//
//  1. span 聚合回填 duration_ms / span_count / error_count（所有 interrupted 行）；
//  2. 有 error span → "error"；末 span 为完成标志（chat.turn.execute /
//     chat.assistant_msg_persist / team.run.finish）→ "ok"；
//  3. 中段截断的行经 session_turns（session_id + 时间窗）确认 completed/failed
//     → ok/error，并回填 turn 级 tokens/provider/model；
//  4. usage events（metadata.trace_id 直配）回填 tokens/cost/provider/model；
//  5. 无任何佐证的行保持 interrupted（真实中断，不粉饰）。
//
// 仅 Postgres 执行（生产唯一驱动）；SQLite（遗留 CLI）直接记录跳过。
func RunMonitorTraceInterruptedBackfillMigration(ctx context.Context, client *ent.Client, d Dialect, lg loggateway.Logger) error {
	if client == nil {
		return fmt.Errorf("monitor trace interrupted backfill migration: ent client required")
	}
	applied, err := isMigrationApplied(ctx, client, MigrationMonitorTraceInterruptedBackfill, lg)
	if err != nil {
		return fmt.Errorf("monitor trace interrupted backfill migration: check gate: %w", err)
	}
	if applied {
		return nil
	}
	record := func() error {
		return recordMigrationApplied(ctx, client, d, MigrationMonitorTraceInterruptedBackfill, migrationNameMonitorTraceInterruptedBackfill, lg)
	}
	if !d.IsPostgres() {
		return record()
	}
	for _, table := range []string{"monitor_traces", "monitor_trace_spans"} {
		has, err := tableExistsWithDialect(ctx, client, lg, table, d)
		if err != nil {
			return fmt.Errorf("monitor trace interrupted backfill migration: check %s: %w", table, err)
		}
		if !has {
			return record()
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Step 1: span 聚合回填 duration/span_count/error_count（全部 interrupted 行）。
	if _, err := client.ExecContext(ctx, `
		UPDATE monitor_traces t
		SET duration_ms = sa.duration_ms,
		    span_count = sa.span_count,
		    error_count = sa.error_count,
		    updated_at = $1
		FROM (
		  SELECT s.trace_id,
		         COUNT(*) AS span_count,
		         COUNT(*) FILTER (WHERE s.status = 'error') AS error_count,
		         GREATEST(MAX(s.ended_at) - MIN(s.started_at), 0) AS duration_ms
		  FROM monitor_trace_spans s
		  GROUP BY s.trace_id
		) sa
		WHERE t.id = sa.trace_id AND t.status = 'interrupted' AND t.deleted_at = ''`, now); err != nil {
		return fmt.Errorf("monitor trace interrupted backfill migration: span metrics: %w", err)
	}

	// Step 2: span 证据重分类 —— error span 优先，其次完成标志末 span。
	if _, err := client.ExecContext(ctx, `
		UPDATE monitor_traces t
		SET status = CASE WHEN sa.error_count > 0 THEN 'error' ELSE 'ok' END,
		    updated_at = $1
		FROM (
		  SELECT s.trace_id,
		         COUNT(*) FILTER (WHERE s.status = 'error') AS error_count,
		         (ARRAY_AGG(s.name ORDER BY s.ended_at DESC, s.started_at DESC))[1] AS last_span
		  FROM monitor_trace_spans s
		  GROUP BY s.trace_id
		) sa
		WHERE t.id = sa.trace_id AND t.status = 'interrupted' AND t.deleted_at = ''
		  AND (sa.error_count > 0
		       OR sa.last_span IN ('chat.turn.execute', 'chat.assistant_msg_persist', 'team.run.finish'))`, now); err != nil {
		return fmt.Errorf("monitor trace interrupted backfill migration: span reclassify: %w", err)
	}

	// Step 3: usage events（metadata.trace_id 直配）回填 tokens/cost/provider/model。
	// 与迁移 20261114 表达式索引 / AggregateUsageByTrace 同一 Dialect.JSONExtract
	// 表达式，保证规划器匹配；NULLIF/COALESCE 容忍空串与非法 JSON 行。
	traceIDExpr := d.JSONExtract("u.metadata_json", "trace_id")
	if hasUsage, err := tableExistsWithDialect(ctx, client, lg, "model_token_usage_events", d); err != nil {
		return fmt.Errorf("monitor trace interrupted backfill migration: check model_token_usage_events: %w", err)
	} else if hasUsage {
		if _, err := client.ExecContext(ctx, fmt.Sprintf(`
			UPDATE monitor_traces t
			SET total_tokens = ua.tokens,
			    total_cost_usd = ua.cost_usd,
			    provider = CASE WHEN t.provider = '' AND ua.provider != '' THEN ua.provider ELSE t.provider END,
			    model = CASE WHEN t.model = '' AND ua.model != '' THEN ua.model ELSE t.model END,
			    updated_at = $1
			FROM (
			  SELECT %s AS trace_id,
			         SUM(u.total_tokens) AS tokens,
			         SUM(u.total_cost_micro_usd) / 1e6 AS cost_usd,
			         COALESCE((ARRAY_AGG(u.provider_code ORDER BY u.occurred_at DESC) FILTER (WHERE u.provider_code <> ''))[1], '') AS provider,
			         COALESCE((ARRAY_AGG(u.model_api_id ORDER BY u.occurred_at DESC) FILTER (WHERE u.model_api_id <> ''))[1], '') AS model
			  FROM model_token_usage_events u
			  WHERE u.metadata_json <> '' AND u.metadata_json <> '{}'
			  GROUP BY 1
			) ua
			WHERE t.id = ua.trace_id AND t.deleted_at = ''
			  AND t.status IN ('ok', 'error') AND t.total_tokens = 0`, traceIDExpr), now); err != nil {
			return fmt.Errorf("monitor trace interrupted backfill migration: usage aggregate: %w", err)
		}
	}

	// Step 4: session_turns 佐证确认中段截断的行（session_id + 时间窗内最早 turn）。
	// 窗口收紧到 +2m：真实匹配的 turn 与 trace 创建时间差 <5s 占 74%（生产实测），
	// 宽窗口会把「崩溃后用户快速重发」的下一条 turn 误判为本 trace 的终态。
	if hasTurns, err := tableExistsWithDialect(ctx, client, lg, "session_turns", d); err != nil {
		return fmt.Errorf("monitor trace interrupted backfill migration: check session_turns: %w", err)
	} else if hasTurns {
		if _, err := client.ExecContext(ctx, `
			UPDATE monitor_traces t
			SET status = CASE WHEN m.turn_status = 'completed' THEN 'ok' ELSE 'error' END,
			    total_tokens = CASE WHEN t.total_tokens = 0 THEN m.turn_tokens ELSE t.total_tokens END,
			    provider = CASE WHEN t.provider = '' THEN m.final_provider ELSE t.provider END,
			    model = CASE WHEN t.model = '' THEN m.final_model ELSE t.model END,
			    updated_at = $1
			FROM monitor_traces t2
			JOIN LATERAL (
			  SELECT st.status AS turn_status, st.total_tokens AS turn_tokens,
			         st.final_provider, st.final_model
			  FROM session_turns st
			  WHERE st.session_id = t2.session_id
			    AND st.started_at <> ''
			    AND st.started_at::timestamptz >= t2.created_at::timestamptz - interval '1 minute'
			    AND st.started_at::timestamptz <= t2.created_at::timestamptz + interval '2 minutes'
			    AND st.status IN ('completed', 'failed')
			  ORDER BY st.started_at ASC
			  LIMIT 1
			) m ON true
			WHERE t2.id = t.id AND t.status = 'interrupted' AND t.deleted_at = '' AND t.session_id <> ''`, now); err != nil {
			return fmt.Errorf("monitor trace interrupted backfill migration: session_turns confirm: %w", err)
		}
	}

	if err := record(); err != nil {
		return fmt.Errorf("monitor trace interrupted backfill migration: record: %w", err)
	}
	lg.Info("monitor trace interrupted backfill migration: done",
		loggateway.StepID("migration.monitor_trace_interrupted_backfill"))
	return nil
}
