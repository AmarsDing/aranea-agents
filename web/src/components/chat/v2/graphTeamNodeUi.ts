// web/src/components/chat/v2/graphTeamNodeUi.ts
//
// GraphTeamNode 的尺寸常量与纯函数（布局算法 heightOf 依赖，必须与组件样式保持一致）。
// 视觉对齐参考图：头部（标题+状态）/ 成员行（状态点+名称+状态文本）/ 底部进度条。

import type { MemberSession } from '../../../features/chat/v2Types';

export const GTN_WIDTH = 240;
export const GTN_PAD_Y = 10;
export const GTN_HEADER_H = 20;
export const GTN_HEADER_GAP = 8;
export const GTN_ROW_H = 24;
export const GTN_ROW_GAP = 6;
export const GTN_PROGRESS_GAP = 10;
export const GTN_PROGRESS_H = 6;
/** 状态行（当前动作 / 错误摘要）行高：与成员行一致，布局算入固定一行。 */
export const GTN_STATUS_ROW_H = GTN_ROW_H;

/** 成员数为 memberCount 时的卡片总高度（成员为 0 时渲染 1 行占位）。 */
export function graphTeamNodeHeight(memberCount: number): number {
  const rows = Math.max(memberCount, 1);
  return (
    GTN_PAD_Y * 2 +
    GTN_HEADER_H +
    GTN_HEADER_GAP +
    rows * GTN_ROW_H +
    (rows - 1) * GTN_ROW_GAP +
    // members → status-row 间距（.gtn-members margin-bottom: 10px）
    GTN_PROGRESS_GAP +
    GTN_STATUS_ROW_H +
    // status-row → progress 间距（.gtn-status-row margin-bottom: 6px）
    GTN_ROW_GAP +
    GTN_PROGRESS_H
  );
}

/** 成员耗时文案：<60s 保留 1 位小数（参考图 "12.4s"）；否则 XmSSs。 */
export function formatMemberDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return '';
  const totalSec = ms / 1000;
  const rounded = Math.round(totalSec * 10) / 10;
  if (rounded < 60) return `${rounded.toFixed(1)}s`;
  const min = Math.floor(totalSec / 60);
  const sec = Math.round(totalSec % 60);
  return `${min}m${String(sec).padStart(2, '0')}s`;
}

/** 成员/节点状态 → 色调（dot / 边框 / 状态文本共用）。 */
export type GraphTeamNodeTone = 'accent' | 'success' | 'danger' | 'warning' | 'muted';

export function graphTeamNodeTone(status: string): GraphTeamNodeTone {
  switch (status) {
    case 'running':
      return 'accent';
    case 'completed':
      return 'success';
    case 'failed':
      return 'danger';
    case 'paused':
    case 'interrupted':
    case 'waiting_human':
      return 'warning';
    default:
      // pending / cancelled / skipped / 未知
      return 'muted';
  }
}

export interface GraphTeamNodeStatusStep {
  Kind: string;
  Status: string;
  ToolName?: string;
}

/**
 * 状态行（动作/错误）纯函数：failed 成员 Error 优先，其次 running/paused 成员最新 step，
 * 兜底用成员自身状态文本。返回 null 表示无活跃成员且无失败（状态行渲染空白占位）。
 */
export function graphTeamNodeStatusText(
  members: readonly MemberSession[],
  latestStepOf: (ms: MemberSession) => GraphTeamNodeStatusStep | null,
  labels: { running: string; paused: string; failed: string },
): { kind: 'action' | 'error'; icon: string; text: string; title?: string } | null {
  const failed = members.find((m) => m.Status === 'failed');
  if (failed) {
    const text = failed.Error?.trim() ? failed.Error.trim() : labels.failed;
    return { kind: 'error', icon: 'error', text, title: failed.Error?.trim() || undefined };
  }
  const active = members.find((m) => m.Status === 'running' || m.Status === 'paused');
  if (!active) return null;
  const step = latestStepOf(active);
  if (step) {
    switch (step.Kind) {
      case 'action':
        return { kind: 'action', icon: 'build', text: step.ToolName?.trim() || labels.running };
      case 'thinking':
        return { kind: 'action', icon: 'psychology', text: labels.running };
      case 'reply':
        return { kind: 'action', icon: 'chat', text: labels.running };
      default:
        break;
    }
  }
  return { kind: 'action', icon: 'bolt', text: active.Status === 'paused' ? labels.paused : labels.running };
}
