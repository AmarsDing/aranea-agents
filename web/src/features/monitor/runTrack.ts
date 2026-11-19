/**
 * runTrack：把一条运行的全部 span 归一成「一维时间轨迹」的纯函数。
 *
 * 与 TraceSpanTree / TraceWaterfall 的分工：
 * - Tree 保留调用层级，回答「谁调了谁」；
 * - Waterfall 是层级甘特图，缩进即父子关系；
 * - RunTrack 按时间拍平，回答「这条运行先跑了什么、死在哪一步」，
 *   并给出 firstFailedId 供弹窗一键定位，不再靠人眼在瀑布里找红色块。
 */
import { normalizeTraceSpans, type TraceSpanNode, type SpanStatusTone } from './spans';

/** 步骤宽度可见下限（百分比）：极短 step 仍要能在轨上看见。 */
export const RUN_TRACK_MIN_WIDTH_PCT = 0.6;

export type RunTrackStep = {
  id: string;
  name: string;
  kind: string;
  tone: SpanStatusTone;
  /** 调用层级（0 = 根）；仅用于左缩进，排序仍按时间 */
  depth: number;
  startMs: number;
  durationMs: number;
  endMs: number;
  /** 相对整条运行起点的百分比 0-100 */
  offsetPct: number;
  /** 宽度百分比（含可见下限；未关闭步骤铺满剩余区间） */
  widthPct: number;
  error: string;
  /** 自身失败（tone=error），区别于「后代失败上卷」 */
  failed: boolean;
  /** 自身或任一后代失败 */
  hasError: boolean;
};

export type RunTrack = {
  steps: RunTrackStep[];
  /** 整条运行时长（ms）；空轨迹回退 1 避免除零 */
  totalMs: number;
  failedCount: number;
  /** 耗时最长的步骤 id（并列取先出现）；空轨迹为 '' */
  slowestId: string;
  /** 时间顺序上第一个自身失败的步骤 id；无失败为 '' */
  firstFailedId: string;
};

function round2(n: number): number {
  return Math.round(n * 100) / 100;
}

/** 深度优先拍平（保留 depth），随后按开始时间全局排序。 */
function flattenByTime(roots: TraceSpanNode[]): TraceSpanNode[] {
  const flat: TraceSpanNode[] = [];
  const walk = (nodes: TraceSpanNode[]) => {
    for (const node of nodes) {
      flat.push(node);
      walk(node.children);
    }
  };
  walk(roots);
  return flat.sort((a, b) => a.startMs - b.startMs || a.name.localeCompare(b.name));
}

export function buildRunTrack(input: unknown[]): RunTrack {
  const nodes = flattenByTime(normalizeTraceSpans(input));
  if (nodes.length === 0) {
    return { steps: [], totalMs: 1, failedCount: 0, slowestId: '', firstFailedId: '' };
  }

  let totalMs = 0;
  for (const node of nodes) {
    const end = node.startMs + node.durationMs;
    if (end > totalMs) totalMs = end;
  }
  if (totalMs <= 0) totalMs = 1;

  const steps: RunTrackStep[] = nodes.map((node) => {
    const offsetPct = round2(Math.min(100, Math.max(0, (node.startMs / totalMs) * 100)));
    // duration=0 表示 span 未关闭：按「到运行结束」铺满，而不是画成 0 宽
    const rawWidth = node.durationMs > 0 ? (node.durationMs / totalMs) * 100 : 100 - offsetPct;
    return {
      id: node.id,
      name: node.name,
      kind: node.kind,
      tone: node.tone,
      depth: node.depth,
      startMs: node.startMs,
      durationMs: node.durationMs,
      endMs: node.startMs + node.durationMs,
      offsetPct,
      widthPct: round2(Math.max(RUN_TRACK_MIN_WIDTH_PCT, Math.min(100 - offsetPct, rawWidth))),
      error: node.error,
      failed: node.tone === 'error',
      hasError: node.hasError,
    };
  });

  let slowestId = '';
  let slowestMs = 0;
  for (const step of steps) {
    if (step.durationMs > slowestMs) {
      slowestMs = step.durationMs;
      slowestId = step.id;
    }
  }

  return {
    steps,
    totalMs,
    failedCount: steps.filter((s) => s.failed).length,
    slowestId,
    firstFailedId: steps.find((s) => s.failed)?.id ?? '',
  };
}
