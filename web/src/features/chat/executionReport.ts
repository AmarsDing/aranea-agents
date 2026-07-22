// web/src/features/chat/executionReport.ts
/**
 * B.10.17 任务执行总结报告 — Step.Content JSON 信封解析。
 *
 * Backend: internal/service/spirit_synthesis.go executionReportEnvelope
 * (Step Kind=notice, NoticeType="synthesis_completed", Content=JSON envelope).
 * Parsing is defensive: any shape mismatch returns null so callers fall back
 * to plain notice rendering (fault-tolerant per design B.10.17.5).
 */

export type ExecutionReportFinalStatus = 'completed' | 'partial_failure' | 'failed';

export interface ExecutionReportOverview {
  query: string;
  finalStatus: ExecutionReportFinalStatus;
  durationMs: number;
  totalUnits: number;
  completedUnits: number;
  failedUnits: number;
  tokenIn: number;
  tokenOut: number;
}

export interface ExecutionReportTeamResult {
  teamId: string;
  teamName: string;
  taskName: string;
  status: string;
  summary: string;
  keyFindings: string;
  durationMs?: number;
  errorMessage?: string;
}

export interface ExecutionReportDeliverable {
  nodeId: string;
  unitName: string;
  summary: string;
  type: string;
  format: string;
  sizeChars: number;
}

export interface ExecutionReportEnvelope {
  version: number;
  kind: 'execution_report';
  /** LLM 结论 markdown；degraded 时为空。 */
  content: string;
  strategy: string;
  degraded: boolean;
  overview: ExecutionReportOverview | null;
  teamResults: ExecutionReportTeamResult[];
  deliverables: ExecutionReportDeliverable[];
  synthesizedAt: string;
}

const EXECUTION_REPORT_KIND = 'execution_report';

/** NoticeType written by the backend synthesis publisher. */
export const SYNTHESIS_COMPLETED_NOTICE_TYPE = 'synthesis_completed';

/**
 * Parse Step.Content into an ExecutionReportEnvelope.
 * Returns null when content is not a valid execution_report envelope.
 */
export function parseExecutionReport(content: string): ExecutionReportEnvelope | null {
  const raw = (content ?? '').trim();
  if (!raw.startsWith('{')) return null;
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return null;
  }
  if (!parsed || typeof parsed !== 'object') return null;
  const env = parsed as Record<string, unknown>;
  if (env.kind !== EXECUTION_REPORT_KIND) return null;
  return {
    version: Number(env.version ?? 1),
    kind: EXECUTION_REPORT_KIND,
    content: String(env.content ?? ''),
    strategy: String(env.strategy ?? ''),
    degraded: env.degraded === true,
    overview: parseOverview(env.overview),
    teamResults: parseTeamResults(env.team_results),
    deliverables: parseDeliverables(env.deliverables),
    synthesizedAt: String(env.synthesized_at ?? ''),
  };
}

function parseOverview(raw: unknown): ExecutionReportOverview | null {
  if (!raw || typeof raw !== 'object') return null;
  const o = raw as Record<string, unknown>;
  return {
    query: String(o.query ?? ''),
    finalStatus: parseFinalStatus(o.final_status),
    durationMs: Number(o.duration_ms ?? 0),
    totalUnits: Number(o.total_units ?? 0),
    completedUnits: Number(o.completed_units ?? 0),
    failedUnits: Number(o.failed_units ?? 0),
    tokenIn: Number(o.token_in ?? 0),
    tokenOut: Number(o.token_out ?? 0),
  };
}

function parseFinalStatus(raw: unknown): ExecutionReportFinalStatus {
  return raw === 'partial_failure' || raw === 'failed' ? raw : 'completed';
}

function parseTeamResults(raw: unknown): ExecutionReportTeamResult[] {
  if (!Array.isArray(raw)) return [];
  return raw.map((item) => {
    const r = (item ?? {}) as Record<string, unknown>;
    const out: ExecutionReportTeamResult = {
      teamId: String(r.team_id ?? ''),
      teamName: String(r.team_name ?? ''),
      taskName: String(r.task_name ?? ''),
      status: String(r.status ?? ''),
      summary: String(r.summary ?? ''),
      keyFindings: String(r.key_findings ?? ''),
    };
    if (typeof r.duration_ms === 'number' && r.duration_ms > 0) out.durationMs = r.duration_ms;
    const errMsg = String(r.error_message ?? '');
    if (errMsg) out.errorMessage = errMsg;
    return out;
  });
}

function parseDeliverables(raw: unknown): ExecutionReportDeliverable[] {
  if (!Array.isArray(raw)) return [];
  return raw.map((item) => {
    const d = (item ?? {}) as Record<string, unknown>;
    return {
      nodeId: String(d.node_id ?? ''),
      unitName: String(d.unit_name ?? ''),
      summary: String(d.summary ?? ''),
      type: String(d.type ?? ''),
      format: String(d.format ?? ''),
      sizeChars: Number(d.size_chars ?? 0),
    };
  });
}
