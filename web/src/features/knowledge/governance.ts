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
      // F14 终审加固：数组 JSON.stringify 保形（String() 逗号压平会让元素内
      // 逗号与分隔逗号不可区分——成员 rel_path 含逗号即被切碎成幻影成员）。
      out[key] = Array.isArray(value) ? JSON.stringify(value) : typeof value === 'string' ? value : String(value);
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
  // G-34 MOC 簇：标题是治理巡检 LLM 摘要产物，缺失回退 hub 页路径。
  if (kind === 'moc_emerge') return payload.suggested_title || payload.hub_rel_path || payload.dedup_key || kind;
  // G-37 实体消解：「keeper ← mergee」直陈合并方向，缺失回退 dedup_key。
  if (kind === 'entity_merge') {
    const keeper = payload.keeper_name;
    const mergee = payload.mergee_name;
    if (keeper && mergee) return `${keeper} ← ${mergee}`;
    return payload.dedup_key || kind;
  }
  return payload.dedup_key || kind;
}

/**
 * G-34 MOC 簇成员清单：保序反解 payload.members。
 * F14 终审加固：新格式（parseProposalPayload 数组 JSON.stringify 保形）优先
 * JSON.parse——成员 rel_path 内逗号不误切；旧格式（String() 逗号串接落库
 * 数据）回退逗号反解兼容。
 */
export function mocMembers(payload: Record<string, string>): string[] {
  const raw = (payload.members ?? '').trim();
  if (raw === '') return [];
  if (raw.startsWith('[')) {
    try {
      const arr: unknown = JSON.parse(raw);
      if (Array.isArray(arr)) {
        return arr
          .filter((m): m is string => typeof m === 'string')
          .map((m) => m.trim())
          .filter((m) => m !== '');
      }
    } catch {
      // 旧格式回退：逗号反解
    }
  }
  return raw
    .split(',')
    .map((m) => m.trim())
    .filter((m) => m !== '');
}
