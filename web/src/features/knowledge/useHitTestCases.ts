// U1-2 hit-test 用例：把排查 query + 参数固化为命名用例，localStorage 持久化
// （隐私模式/写失败降级为会话内存态），支持删除/导入导出与回归历史
// （每用例保留最近 20 次三策略对比运行的 topScores，用于前后分数 delta 对比）。
import { ref } from 'vue';

export type HitTestCaseParams = {
  topK: number;
  useRerank: boolean;
  pathPrefix: string;
  filterJson?: string;
  minScore?: number;
};

export type HitTestRunRecord = {
  /** ISO 时间戳 */
  at: string;
  /** 各策略实际执行模式去重后以 / 连接（如 "rrf" 或 "rrf/dense"） */
  modeUsed: string;
  reranked: boolean;
  /** 本次对比墙钟耗时（各策略 elapsed 的最大值） */
  durationMs: number;
  /** dense/sparse/rrf 顺序，各策略 top1..3 score 平铺 */
  topScores: number[];
};

export type HitTestCase = {
  id: string;
  name: string;
  query: string;
  params: HitTestCaseParams;
  /** 最新在前，上限 HIT_TEST_MAX_RUNS */
  runs: HitTestRunRecord[];
};

export type HitTestCaseInput = {
  name: string;
  query: string;
  params?: Partial<HitTestCaseParams>;
};

export type ImportCaseResult = { ok: true; case: HitTestCase } | { ok: false; error: string };

export const HIT_TEST_CASES_STORAGE_KEY = 'kb.hitTest.cases.v1';
export const HIT_TEST_MAX_RUNS = 20;

/** 最小存储接口（便于测试注入 mock；默认实现走 localStorage，失败降级内存）。 */
export type StorageLike = {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
};

export type HitTestCasesDeps = {
  storage?: StorageLike;
  idGen?: () => string;
};

type StoreShape = { collections: Record<string, HitTestCase[]> };

function defaultId(): string {
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
}

/** localStorage 包装：任何读写抛错（隐私模式/配额）都降级到会话内存 Map。 */
function createDefaultStorage(): StorageLike {
  const memory = new Map<string, string>();
  return {
    getItem(key) {
      try {
        return localStorage.getItem(key);
      } catch {
        return memory.has(key) ? (memory.get(key) as string) : null;
      }
    },
    setItem(key, value) {
      try {
        localStorage.setItem(key, value);
      } catch {
        memory.set(key, value);
      }
    },
  };
}

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

function isRunRecord(v: unknown): v is HitTestRunRecord {
  if (!isRecord(v)) return false;
  return (
    typeof v.at === 'string' &&
    typeof v.modeUsed === 'string' &&
    typeof v.reranked === 'boolean' &&
    typeof v.durationMs === 'number' &&
    Number.isFinite(v.durationMs) &&
    Array.isArray(v.topScores) &&
    (v.topScores as unknown[]).every((s) => typeof s === 'number' && Number.isFinite(s))
  );
}

function isHitTestCase(v: unknown): v is HitTestCase {
  if (!isRecord(v)) return false;
  if (typeof v.id !== 'string' || typeof v.name !== 'string' || typeof v.query !== 'string') return false;
  if (!isRecord(v.params)) return false;
  const p = v.params;
  if (typeof p.topK !== 'number' || typeof p.useRerank !== 'boolean' || typeof p.pathPrefix !== 'string') return false;
  if (v.runs !== undefined && !(Array.isArray(v.runs) && (v.runs as unknown[]).every(isRunRecord))) return false;
  return true;
}

/** 参数归一化：topK<=0/缺失回退 10；其余给缺省值。 */
export function normalizeParams(p?: Partial<HitTestCaseParams>): HitTestCaseParams {
  const topK = Number(p?.topK);
  const out: HitTestCaseParams = {
    topK: Number.isFinite(topK) && topK > 0 ? Math.floor(topK) : 10,
    useRerank: Boolean(p?.useRerank),
    pathPrefix: typeof p?.pathPrefix === 'string' ? p.pathPrefix : '',
  };
  if (typeof p?.filterJson === 'string' && p.filterJson) out.filterJson = p.filterJson;
  if (typeof p?.minScore === 'number' && Number.isFinite(p.minScore)) out.minScore = p.minScore;
  return out;
}

/** 导入定义校验：返回 null 表示通过，否则为具体错误原因。 */
export function validateCaseDefinition(rec: unknown): string | null {
  if (!isRecord(rec)) return 'not an object';
  if (typeof rec.name !== 'string' || !rec.name.trim()) return 'missing name';
  if (typeof rec.query !== 'string' || !rec.query.trim()) return 'missing query';
  const p = rec.params;
  if (p === undefined || p === null) return null;
  if (!isRecord(p)) return 'invalid params';
  if (p.topK !== undefined && (typeof p.topK !== 'number' || !Number.isFinite(p.topK) || p.topK <= 0))
    return 'invalid params.topK';
  if (p.useRerank !== undefined && typeof p.useRerank !== 'boolean') return 'invalid params.useRerank';
  if (p.pathPrefix !== undefined && typeof p.pathPrefix !== 'string') return 'invalid params.pathPrefix';
  if (p.filterJson !== undefined && typeof p.filterJson !== 'string') return 'invalid params.filterJson';
  if (p.minScore !== undefined && (typeof p.minScore !== 'number' || !Number.isFinite(p.minScore)))
    return 'invalid params.minScore';
  return null;
}

export function useHitTestCases(deps: HitTestCasesDeps = {}) {
  const idGen = deps.idGen ?? defaultId;
  const storage = deps.storage ?? createDefaultStorage();

  function load(): Record<string, HitTestCase[]> {
    try {
      const raw = storage.getItem(HIT_TEST_CASES_STORAGE_KEY);
      if (!raw) return {};
      const parsed: unknown = JSON.parse(raw);
      if (!isRecord(parsed) || !isRecord(parsed.collections)) return {};
      const out: Record<string, HitTestCase[]> = {};
      for (const [key, list] of Object.entries(parsed.collections)) {
        out[key] = Array.isArray(list) ? list.filter(isHitTestCase).map((c) => ({ ...c, runs: c.runs ?? [] })) : [];
      }
      return out;
    } catch {
      return {};
    }
  }

  const byCollection = ref<Record<string, HitTestCase[]>>(load());

  function save() {
    try {
      const shape: StoreShape = { collections: byCollection.value };
      storage.setItem(HIT_TEST_CASES_STORAGE_KEY, JSON.stringify(shape));
    } catch {
      /* 持久化失败不影响会话内功能 */
    }
  }

  function listCases(collectionId: string): HitTestCase[] {
    return byCollection.value[collectionId] ?? [];
  }

  function getCase(collectionId: string, caseId: string): HitTestCase | null {
    return listCases(collectionId).find((c) => c.id === caseId) ?? null;
  }

  function addCase(collectionId: string, input: HitTestCaseInput): HitTestCase {
    const name = input.name.trim();
    if (!name) throw new Error('hit-test case name required');
    if (!input.query.trim()) throw new Error('hit-test case query required');
    const created: HitTestCase = {
      id: idGen(),
      name,
      query: input.query,
      params: normalizeParams(input.params),
      runs: [],
    };
    byCollection.value = { ...byCollection.value, [collectionId]: [...listCases(collectionId), created] };
    save();
    return created;
  }

  function deleteCase(collectionId: string, caseId: string): boolean {
    const list = listCases(collectionId);
    const next = list.filter((c) => c.id !== caseId);
    if (next.length === list.length) return false;
    byCollection.value = { ...byCollection.value, [collectionId]: next };
    save();
    return true;
  }

  /** 追加一次运行记录（最新在前，超出上限截断尾部）。 */
  function addRun(collectionId: string, caseId: string, run: HitTestRunRecord): boolean {
    const list = listCases(collectionId);
    const idx = list.findIndex((c) => c.id === caseId);
    if (idx < 0) return false;
    const target = list[idx];
    const updated: HitTestCase = { ...target, runs: [run, ...target.runs].slice(0, HIT_TEST_MAX_RUNS) };
    const next = [...list];
    next[idx] = updated;
    byCollection.value = { ...byCollection.value, [collectionId]: next };
    save();
    return true;
  }

  /** 导出用例定义（不含运行历史）为 JSON 字符串。 */
  function exportCase(collectionId: string, caseId: string): string | null {
    const c = getCase(collectionId, caseId);
    if (!c) return null;
    return JSON.stringify({ version: 1, case: { name: c.name, query: c.query, params: c.params } }, null, 2);
  }

  /** 导入用例：兼容导出的 {version,case} 包裹与裸 case 定义；校验通过则新建（新 id、空历史）。 */
  function importCase(collectionId: string, json: string): ImportCaseResult {
    let parsed: unknown;
    try {
      parsed = JSON.parse(json);
    } catch {
      return { ok: false, error: 'invalid json' };
    }
    const candidate = isRecord(parsed) && isRecord(parsed.case) ? parsed.case : parsed;
    const err = validateCaseDefinition(candidate);
    if (err) return { ok: false, error: err };
    const def = candidate as { name: string; query: string; params?: Partial<HitTestCaseParams> };
    const created = addCase(collectionId, { name: def.name, query: def.query, params: def.params });
    return { ok: true, case: created };
  }

  return { byCollection, listCases, getCase, addCase, deleteCase, addRun, exportCase, importCase };
}
