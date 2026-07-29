import type { MonitorEvent } from '../../realtime/monitorEvent';
import type { MonitorLogLine, MonitorTrace } from './types';
import { parseJSON } from './utils';

export type FlowSeverity = 'ok' | 'info' | 'warn' | 'error' | 'critical';

function str(v: unknown): string {
  return v == null ? '' : String(v);
}

export function flowSeverityToLevel(severity: string): MonitorLogLine['level'] {
  switch (severity) {
    case 'critical':
    case 'error':
      return 'ERROR';
    case 'warn':
      return 'WARN';
    case 'info':
    case 'ok':
    default:
      return 'INFO';
  }
}

/** Map WS flow_log MonitorEvent → Monitor log line for LogStream. */
export function monitorLogLineFromFlowEvent(ev: MonitorEvent): MonitorLogLine | null {
  const m = ev.metadata ?? {};
  const severity = str(m.severity || 'info') as FlowSeverity;
  const title = str(m.title);
  const message = str(m.message);
  const stepId = str(m.step_id);
  const phase = str(m.flow_phase);
  const display = (ev.message ?? '').trim() || [title, message].filter(Boolean).join(' — ') || `${stepId}.${phase}`;

  return {
    id: str(m.flow_id || ev.id),
    time: ev.timestamp,
    level: flowSeverityToLevel(severity),
    message: display,
    source: str(m.agent_key || ev.source || 'flow'),
    created_at: ev.timestamp,
    kind: 'flow',
    severity,
    title,
    step_id: stepId,
    trace_id: str(m.trace_id),
    run_id: str(m.run_id),
    session_id: str(m.session_id || ev.session_id),
    hint: str(m.hint),
  };
}

/** Correlation extracted from a MonitorTrace row. */
export function traceCorrelationFromTraceRow(row: MonitorTrace): {
  traceId: string;
  runId: string;
  sessionId: string;
} {
  const meta = parseJSON(row.metadata_json || '');
  return {
    traceId: str(meta.trace_id) || row.key,
    runId: row.run_id || str(meta.run_id),
    sessionId: row.session_id || str(meta.session_id),
  };
}

/** @deprecated Use traceCorrelationFromTraceRow instead. */
export const traceCorrelationFromUsageRow = traceCorrelationFromTraceRow;

/** Whether a live flow log line belongs to the open trace detail. */
export function flowLogMatchesTrace(
  line: MonitorLogLine,
  opts: { traceId?: string; runId?: string; sessionId?: string },
): boolean {
  if (line.kind !== 'flow') return false;
  const traceId = (opts.traceId || '').trim();
  const runId = (opts.runId || '').trim();
  const sessionId = (opts.sessionId || '').trim();
  if (traceId && line.trace_id && line.trace_id !== traceId) return false;
  if (runId && line.run_id && line.run_id !== runId) return false;
  if (sessionId && line.session_id && line.session_id !== sessionId) return false;
  if (!traceId && !runId && !sessionId) return true;
  if (traceId && !line.trace_id) return true;
  return true;
}

export function sortFlowLogLines(lines: MonitorLogLine[]): MonitorLogLine[] {
  return [...lines].sort((a, b) => String(a.time).localeCompare(String(b.time)));
}

export type FlowLogExportEntry = {
  schema_version: string;
  id: string;
  timestamp: string;
  correlation: {
    trace_id: string;
    session_id: string;
    run_id: string;
    domain: string;
    agent_key: string;
  };
  step: { id: string; phase: string };
  severity: string;
  title: string;
  message: string;
  hint?: string;
};

export function flowLogExportEntryFromLine(line: MonitorLogLine): FlowLogExportEntry {
  return {
    schema_version: 'flow_log/v1',
    id: line.id,
    timestamp: line.time,
    correlation: {
      trace_id: line.trace_id || '',
      session_id: line.session_id || '',
      run_id: line.run_id || '',
      domain: 'chat',
      agent_key: line.source || '',
    },
    step: { id: line.step_id || '', phase: '' },
    severity: line.severity || 'info',
    title: line.title || '',
    message: line.message,
    hint: line.hint || undefined,
  };
}

/** Build AI-friendly JSONL (bundle header + one entry per line). */
export function buildFlowDiagnosticJsonl(traceId: string, lines: MonitorLogLine[]): string {
  const sorted = sortFlowLogLines(lines.filter((l) => l.kind === 'flow'));
  const header = {
    type: 'flow_diagnostic_bundle',
    schema_version: 'flow_log/v1',
    trace_id: traceId,
    exported_at: new Date().toISOString(),
    entry_count: sorted.length,
  };
  const body = sorted.map((line) => flowLogExportEntryFromLine(line));
  return `${JSON.stringify(header)}\n${body.map((row) => JSON.stringify(row)).join('\n')}\n`;
}

export function downloadFlowDiagnosticJsonl(traceId: string, lines: MonitorLogLine[]): void {
  const safeId = (traceId || 'unknown').replace(/[^\w.-]+/g, '_').slice(0, 64);
  const blob = new Blob([buildFlowDiagnosticJsonl(traceId, lines)], { type: 'application/x-ndjson;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = `flow-diagnostic-${safeId}.jsonl`;
  anchor.click();
  URL.revokeObjectURL(url);
}
