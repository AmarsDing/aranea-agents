/**
 * Trace Span 树构建（纯函数，locale 中立）。
 * 线协议两种形态统一归一为树：
 *  1. 持久化 monitor_trace_spans（GetMonitorTrace）：扁平数组，parent_id 链接；
 *  2. 旧 metadata.spans：嵌套 children。
 * 消费方：TraceSpanTree / TraceWaterfall（components/monitor）。
 */

/** Span 状态展示色调（与语言无关）。 */
export type SpanStatusTone = 'ok' | 'running' | 'error' | 'warn' | 'idle';

export type TraceSpanNode = {
  id: string;
  parentId: string;
  kind: string;
  name: string;
  rawStatus: string;
  tone: SpanStatusTone;
  /** 相对最早 span 的偏移毫秒（后端已归一化；旧协议缺省 0） */
  startMs: number;
  /** 0 = 仍未关闭（运行中） */
  durationMs: number;
  attributes: Record<string, unknown>;
  error: string;
  depth: number;
  /** 自身或任一后代为 error 态（子树错误上卷） */
  hasError: boolean;
  children: TraceSpanNode[];
};

/** 状态字符串 → 展示色调。timeout/interrupted 归为 warn，与列表状态配色一致。 */
export function spanStatusTone(status?: string): SpanStatusTone {
  const s = String(status ?? '')
    .trim()
    .toLowerCase();
  if (s === 'ok' || s === 'success') return 'ok';
  if (s === 'running') return 'running';
  if (s === 'error' || s === 'failed') return 'error';
  if (s === 'timeout' || s === 'interrupted') return 'warn';
  return 'idle';
}

type RawSpan = Record<string, unknown>;

function str(v: unknown): string {
  return typeof v === 'string' ? v : v == null ? '' : String(v);
}

function num(v: unknown): number {
  const n = Number(v);
  return Number.isFinite(n) && n > 0 ? n : 0;
}

function attrsOf(raw: RawSpan): Record<string, unknown> {
  const a = raw.attributes;
  return a && typeof a === 'object' && !Array.isArray(a) ? (a as Record<string, unknown>) : {};
}

function errorOf(raw: RawSpan): string {
  const e = raw.error;
  if (e == null) return '';
  if (typeof e === 'string') return e;
  if (typeof e === 'object' && !Array.isArray(e)) {
    const msg = (e as Record<string, unknown>).message;
    return str(msg) || JSON.stringify(e);
  }
  return str(e);
}

/**
 * 归一化任意线协议 span 列表为树。
 * 先递归拍平（嵌套 children 补 parentId），再按 parentId 链接；
 * parentId 悬空/缺失的提升为根；子节点按 startMs 再按名称排序；
 * 最后计算 depth 与 hasError（后序上卷）。
 */
export function normalizeTraceSpans(input: unknown[]): TraceSpanNode[] {
  const flat: TraceSpanNode[] = [];
  const usedIds = new Set<string>();

  const walk = (raw: unknown, inheritedParentId: string, indexPath: string) => {
    if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return;
    const row = raw as RawSpan;
    let id = str(row.id ?? row.span_id) || `${indexPath}`;
    if (usedIds.has(id)) id = `${id}#${flat.length}`;
    usedIds.add(id);
    const node: TraceSpanNode = {
      id,
      parentId: str(row.parent_id ?? row.parentId) || inheritedParentId,
      kind: str(row.kind ?? row.type),
      name: str(row.name ?? row.type ?? row.kind) || 'span',
      rawStatus: str(row.status),
      tone: spanStatusTone(str(row.status)),
      startMs: num(row.start_ms ?? row.startMs),
      durationMs: num(row.duration_ms ?? row.durationMs),
      attributes: attrsOf(row),
      error: errorOf(row),
      depth: 0,
      hasError: false,
      children: [],
    };
    flat.push(node);
    const children = row.children;
    if (Array.isArray(children)) {
      children.forEach((child, i) => walk(child, node.id, `${indexPath}.${i}`));
    }
  };

  input.forEach((raw, i) => walk(raw, '', `span-${i}`));

  const byId = new Map(flat.map((n) => [n.id, n]));
  const roots: TraceSpanNode[] = [];
  for (const node of flat) {
    const parent = node.parentId ? byId.get(node.parentId) : undefined;
    if (parent && parent !== node) {
      parent.children.push(node);
    } else {
      node.parentId = '';
      roots.push(node);
    }
  }

  const sortRec = (nodes: TraceSpanNode[]) => {
    nodes.sort((x, y) => x.startMs - y.startMs || x.name.localeCompare(y.name));
    nodes.forEach((n) => sortRec(n.children));
  };
  sortRec(roots);

  const finalize = (node: TraceSpanNode, depth: number): boolean => {
    node.depth = depth;
    let subtreeError = node.tone === 'error' || Boolean(node.error);
    for (const child of node.children) {
      if (finalize(child, depth + 1)) subtreeError = true;
    }
    node.hasError = subtreeError;
    return subtreeError;
  };
  roots.forEach((r) => finalize(r, 0));
  return roots;
}

/** 先序拍平可见行；collapsed 中的 id 不展开其子树。 */
export function flattenSpanTree(nodes: TraceSpanNode[], collapsed: ReadonlySet<string>): TraceSpanNode[] {
  const out: TraceSpanNode[] = [];
  const walk = (list: TraceSpanNode[]) => {
    for (const node of list) {
      out.push(node);
      if (!collapsed.has(node.id)) walk(node.children);
    }
  };
  walk(nodes);
  return out;
}

/**
 * 关键字过滤（name/kind/status 及 attributes 的 tool_name/model），
 * 命中节点保留其祖先链；空关键字原样返回（不克隆）。
 */
export function filterSpanTree(nodes: TraceSpanNode[], keyword: string): TraceSpanNode[] {
  const kw = keyword.trim().toLowerCase();
  if (!kw) return nodes;
  const matches = (n: TraceSpanNode): boolean => {
    if (n.name.toLowerCase().includes(kw)) return true;
    if (n.kind.toLowerCase().includes(kw)) return true;
    if (n.rawStatus.toLowerCase().includes(kw)) return true;
    const tool = str(n.attributes.tool_name ?? n.attributes.tool);
    if (tool.toLowerCase().includes(kw)) return true;
    const model = str(n.attributes.model);
    return model.toLowerCase().includes(kw);
  };
  const prune = (list: TraceSpanNode[]): TraceSpanNode[] => {
    const out: TraceSpanNode[] = [];
    for (const node of list) {
      const children = prune(node.children);
      if (matches(node) || children.length) {
        out.push(children.length === node.children.length ? node : { ...node, children });
      }
    }
    return out;
  };
  return prune(nodes);
}

/** 时间轴右端（ms）：所有 span end 的最大值；空树回退 1 避免除零。 */
export function spanTimelineMaxEnd(nodes: TraceSpanNode[]): number {
  let max = 0;
  const walk = (list: TraceSpanNode[]) => {
    for (const n of list) {
      const end = n.startMs + n.durationMs;
      if (end > max) max = end;
      walk(n.children);
    }
  };
  walk(nodes);
  return max > 0 ? max : 1;
}

/** 节点总数（含全部层级）。 */
export function countSpanNodes(nodes: TraceSpanNode[]): number {
  let count = 0;
  const walk = (list: TraceSpanNode[]) => {
    for (const n of list) {
      count += 1;
      walk(n.children);
    }
  };
  walk(nodes);
  return count;
}

/** 时长格式化：<1000 → "42ms"；≥1000 → "53.2s"。 */
export function formatSpanDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return '0ms';
  if (ms < 1000) return `${Math.round(ms)}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}
