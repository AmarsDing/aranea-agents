import type { MonitorTrace, PlatformResource } from './types';
import { parseJSON } from './utils';

export type RunnerCompletionMeta = {
  schema_version?: string;
  session_id?: string;
  trace_id?: string;
  usage_event_id?: string;
  invocation_id?: string;
  run_id?: string;
  request_id?: string;
  agent_id?: string;
  agent_key?: string;
  agent_display_name?: string;
  run_kind?: string;
  status?: string;
  duration_ms?: number;
  usage?: { prompt_tokens?: number; completion_tokens?: number; total_tokens?: number };
  error?: { message?: string; type?: string };
};

export function parseRunnerCompletionMeta(raw: unknown): RunnerCompletionMeta {
  if (!raw || typeof raw !== 'object') return {};
  return raw as RunnerCompletionMeta;
}

export function runnerCompletionMetaFromRow(row: PlatformResource): RunnerCompletionMeta {
  const cfg = parseJSON(row.config_json);
  const fromCfg = parseRunnerCompletionMeta(cfg);
  const fromMeta = parseRunnerCompletionMeta(parseJSON(row.metadata_json));
  return { ...fromCfg, ...fromMeta };
}

export function isRunnerCompletionRow(row: PlatformResource): boolean {
  const key = String(row.key || '').trim();
  const cfg = parseJSON(row.config_json);
  const type = String(cfg.type || '').trim();
  return key === 'runner.completion' || type === 'runner.completion' || type === 'runner_completion';
}

/** Plan C: hide persisted Chat completion when Runs already has the truth row. */
export function shouldHideCompletionInEvents(meta: RunnerCompletionMeta, traces: MonitorTrace[] = []): boolean {
  if (String(meta.usage_event_id || '').trim()) return true;
  const traceId = String(meta.trace_id || '').trim();
  if (!traceId) return false;
  return traces.some((row) => {
    const rowMeta = parseJSON(row.metadata_json || '');
    return String(rowMeta.trace_id || '').trim() === traceId;
  });
}

/** True when Runs list has a row we can open for this completion metadata. */
export function completionCanOpenInRuns(meta: RunnerCompletionMeta, traces: MonitorTrace[] = []): boolean {
  const usageId = String(meta.usage_event_id || '').trim();
  if (usageId && findRunByUsageEventId(traces, usageId)) return true;
  const traceId = String(meta.trace_id || '').trim();
  if (traceId && findRunByTraceId(traces, traceId)) return true;
  return false;
}

/** WS runner_completion duplicates persisted monitor row for chat turns. */
export function shouldHideWsRunnerCompletion(type: string): boolean {
  return String(type || '').trim() === 'runner_completion';
}

export function findRunByUsageEventId(
  traces: MonitorTrace[],
  usageEventId: string,
): MonitorTrace | undefined {
  const id = usageEventId.trim();
  if (!id) return undefined;
  return traces.find((row) => String(row.id || '').trim() === id);
}

export function findRunByTraceId(traces: MonitorTrace[], traceId: string): MonitorTrace | undefined {
  const tid = traceId.trim();
  if (!tid) return undefined;
  return traces.find((row) => {
    const meta = parseJSON(row.metadata_json || '');
    return String(meta.trace_id || '').trim() === tid;
  });
}

export function completionFallbackTitle(meta: RunnerCompletionMeta, rowName?: string): string {
  if (rowName?.trim()) return rowName.trim();
  if (meta.status === 'error') return '对话失败';
  return '对话完成';
}

export function completionFallbackSubtitle(meta: RunnerCompletionMeta, rowDesc?: string): string {
  if (rowDesc?.trim()) return rowDesc.trim();
  const parts: string[] = [];
  if (meta.agent_display_name) parts.push(`Agent ${meta.agent_display_name}`);
  else if (meta.agent_key) parts.push(`Agent ${meta.agent_key}`);
  if (meta.duration_ms && meta.duration_ms > 0) {
    parts.push(meta.duration_ms >= 1000 ? `${(meta.duration_ms / 1000).toFixed(1)} s` : `${meta.duration_ms} ms`);
  }
  const total = meta.usage?.total_tokens;
  if (total && total > 0) parts.push(`${total} tokens`);
  if (meta.session_id) parts.push(`会话 ${meta.session_id.slice(0, 8)}…`);
  return parts.length ? parts.join(' · ') : '运行完成';
}
