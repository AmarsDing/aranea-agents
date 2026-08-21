/**
 * 伴侣确认卡队列（M74 V2-T5，设计 §7.2「确认卡队列」）。
 *
 * 数据流：activityV2Store.steps（WS 权威源）→ 纯函数派生 pending 队列 →
 * HoloConfirmCard 渲染队首。队列不在 companion store 冗余存储（单一数据源），
 * 状态推进完全依赖 WS step 更新（tool_blocked → completed/cancelled）。
 *
 * ISSUE-007：超过确认窗口的卡片在展示层自动滤除（超时自动关闭）。
 * 后端 defaultToolConfirmationTimeout 到期后确认已失效，继续渲染只会误导用户
 * 点击一个必然失败的按钮；WS 状态推进可能缺失（页面快照早于超时推进），
 * 因此队列派生时按本地时钟过滤过期项。
 */

import { computed, onScopeDispose, ref, type ComputedRef } from 'vue';

import { useChatActivityStore } from '../../stores/chat/activityV2Store';
import type { Step } from '../chat/v2Types';
import type { ConfirmCardModel } from './types';

/** 确认窗口秒数：与后端 defaultToolConfirmationTimeout（internal/agent/tool_confirmation.go）一致。 */
export const CONFIRM_WINDOW_S = 5 * 60;

/** 判断确认卡是否已过确认窗口（纯函数，可单测）。startedAt 无效时视为未过期（无法判断，保持现状）。 */
export function isConfirmExpired(startedAt: string, nowMs: number): boolean {
  const started = Date.parse(startedAt);
  if (!Number.isFinite(started)) return false;
  return nowMs - started >= CONFIRM_WINDOW_S * 1000;
}

/** Step → 确认卡视图模型（纯函数，可单测）。 */
export function toConfirmCardModel(step: Step): ConfirmCardModel {
  return {
    id: step.ID,
    sessionId: step.SessionID,
    toolName: step.ToolName,
    target: extractTarget(step.ToolArgs),
    description: step.Content,
    argsJson: prettyArgs(step.ToolArgs),
    startedAt: step.StartedAt,
  };
}

/** 从 ToolArgs 提取操作目标：优先 target，其次 url；非字符串/缺失 → ''。 */
function extractTarget(args: unknown): string {
  if (args === null || typeof args !== 'object' || Array.isArray(args)) return '';
  const rec = args as Record<string, unknown>;
  for (const key of ['target', 'url']) {
    const v = rec[key];
    if (typeof v === 'string' && v.trim() !== '') return v.trim();
  }
  return '';
}

function prettyArgs(args: unknown): string {
  if (args === null || args === undefined) return '';
  if (typeof args === 'string') return args;
  try {
    return JSON.stringify(args, null, 2);
  } catch {
    return '';
  }
}

/**
 * 过滤会话内待决议确认步骤（kind=confirm + tool_blocked），按发起时间 FIFO。
 * 纯函数：接受任意 Step 可迭代对象，便于单测。
 */
export function pendingConfirmSteps(steps: Iterable<Step>, sessionId: string): Step[] {
  const out: Step[] = [];
  for (const s of steps) {
    if (s.Kind !== 'confirm' || s.Status !== 'tool_blocked') continue;
    if (s.SessionID !== sessionId && s.SpiritSessionID !== sessionId) continue;
    out.push(s);
  }
  out.sort((a, b) => a.StartedAt.localeCompare(b.StartedAt));
  return out;
}

export type UseCompanionConfirmsReturn = {
  /** 当前会话待决议确认队列（FIFO，队首为当前应展示的卡）。 */
  queue: ComputedRef<ConfirmCardModel[]>;
  /** 队首确认卡（无待决议时为 null）。 */
  active: ComputedRef<ConfirmCardModel | null>;
};

/** 派生当前会话的确认卡队列（CompanionPage 使用；sessionId 可响应式切换）。 */
export function useCompanionConfirms(sessionId: () => string | null): UseCompanionConfirmsReturn {
  const activityStore = useChatActivityStore();
  // 本地时钟 tick：驱动过期卡片自动出队（ISSUE-007），不污染 WS 权威 steps。
  const nowMs = ref(Date.now());
  const tickTimer = setInterval(() => {
    nowMs.value = Date.now();
  }, 1000);
  onScopeDispose(() => clearInterval(tickTimer));

  const queue = computed(() => {
    const sid = sessionId();
    if (!sid) return [];
    return pendingConfirmSteps(activityStore.steps.values(), sid)
      .filter((s) => !isConfirmExpired(s.StartedAt, nowMs.value))
      .map(toConfirmCardModel);
  });
  const active = computed(() => queue.value[0] ?? null);
  return { queue, active };
}
