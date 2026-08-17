/**
 * 知识库治理提案：payload 解析与二审决策路由（纯函数）。
 * 事实段 conflict 必须 keep_old / keep_new，禁止裸 applied。
 */
export type GovernanceDecision = 'applied' | 'rejected' | 'keep_old' | 'keep_new';

export function parseProposalPayload(json: string): Record<string, string> {
  const rawText = json.trim();
  if (!rawText) return {};
  try {
    const raw: unknown = JSON.parse(rawText);
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return {};
    const out: Record<string, string> = {};
    for (const [key, value] of Object.entries(raw as Record<string, unknown>)) {
      if (value == null) continue;
      out[key] = typeof value === 'string' ? value : String(value);
    }
    return out;
  } catch {
    return {};
  }
}

export function isFactConflict(kind: string, payload: Record<string, string>): boolean {
  return kind === 'conflict' && (payload.target_fact_id ?? '').trim() !== '';
}

export function decisionsForProposal(kind: string, payload: Record<string, string>): GovernanceDecision[] {
  if (isFactConflict(kind, payload)) return ['keep_old', 'keep_new', 'rejected'];
  return ['applied', 'rejected'];
}

export function proposalSummary(kind: string, payload: Record<string, string>): string {
  if (isFactConflict(kind, payload)) {
    return payload.new_statement || payload.reason || payload.rel_path || payload.target_fact_id;
  }
  if (kind === 'orphan') return payload.rel_path || payload.doc_id;
  if (kind === 'conflict') {
    const src = payload.doc_id;
    const dst = payload.target_doc_id;
    if (src && dst) return `${src} → ${dst}`;
  }
  return payload.dedup_key || kind;
}
