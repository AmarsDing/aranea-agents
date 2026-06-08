import type { SpiritTeamStatus, SpiritTeamMode } from './types';
import type { SessionStatus } from '../session/types';

/**
 * Maps SpiritTeamStatus to SessionStatus for SessionStatusBadge display.
 * Shared by TeamTaskCard and TaskExecutionPanel.
 */
export function mapSpiritStatusToSession(status: SpiritTeamStatus): SessionStatus {
  const mapping: Record<SpiritTeamStatus, SessionStatus> = {
    pending: 'idle',
    running: 'running',
    completed: 'completed',
    failed: 'interrupted',
    cancelled: 'interrupted',
    interrupted: 'interrupted',
    archived: 'completed',
  };
  return mapping[status] ?? 'idle';
}

/**
 * Returns a Chinese label for a SpiritTeamMode value.
 * Shared by TeamTaskCard and TeamAssemblyCard.
 */
export function spiritModeLabel(mode: SpiritTeamMode | string): string {
  if (!mode) return '';
  const labels: Record<string, string> = {
    coordinator: '协调者',
    sequential: '顺序',
    parallel: '并行',
    critic_loop: '批判循环',
    swarm: '蜂群',
    adaptive: '自适应',
  };
  return labels[mode] ?? mode;
}

/** 7 aggregate display labels for AgentNode status */
export type AgentNodeStatusLabel =
  | 'queued'
  | 'active'
  | 'suspended'
  | 'done'
  | 'failed'
  | 'skipped'
  | 'cancelled';

/** Maps 17 backend AgentNodeStatus values to 7 display labels */
export const AGENT_NODE_STATUS_MAP: Record<string, AgentNodeStatusLabel> = {
  // Queued
  idle: 'queued',
  queued: 'queued',
  scheduled: 'queued',
  // Active
  running: 'active',
  thinking: 'active',
  tool_running: 'active',
  transferring: 'active',
  retrying: 'active',
  // Suspended
  waiting_input: 'suspended',
  waiting_review: 'suspended',
  waiting_assign: 'suspended',
  blocked: 'suspended',
  // Done
  success: 'done',
  // Failed
  failed: 'failed',
  timed_out: 'failed',
  // Skipped
  skipped: 'skipped',
  // Cancelled
  cancelled: 'cancelled',
};

/** Display config for each status label */
export const STATUS_LABEL_CONFIG: Record<
  AgentNodeStatusLabel,
  { text: string; color: string; icon: string; animated: boolean }
> = {
  queued: { text: '排队中', color: 'var(--color-text-tertiary)', icon: 'circle', animated: false },
  active: { text: '执行中', color: 'var(--color-accent)', icon: 'bolt', animated: true },
  suspended: { text: '等待中', color: 'var(--color-warning)', icon: 'pause', animated: false },
  done: { text: '已完成', color: 'var(--color-success)', icon: 'check_circle', animated: false },
  failed: { text: '失败', color: 'var(--color-danger)', icon: 'error', animated: false },
  skipped: { text: '已跳过', color: 'var(--color-text-tertiary)', icon: 'remove_circle', animated: false },
  cancelled: { text: '已取消', color: 'var(--color-text-tertiary)', icon: 'cancel', animated: false },
};

/** Maps any AgentNode status string to an aggregate display label */
export function agentNodeStatusToLabel(status: string): AgentNodeStatusLabel {
  return AGENT_NODE_STATUS_MAP[status] ?? 'queued';
}

/** Maps SpiritMember.status (idle/running/error) to aggregate display label */
export function spiritMemberStatusToLabel(status: string): AgentNodeStatusLabel {
  const mapping: Record<string, AgentNodeStatusLabel> = {
    idle: 'queued',
    running: 'active',
    error: 'failed',
  };
  return mapping[status] ?? 'queued';
}

/** Maps SpiritTeamStatus to aggregate display label for TeamTaskCard */
export function spiritTeamStatusToLabel(status: SpiritTeamStatus): AgentNodeStatusLabel {
  const mapping: Record<SpiritTeamStatus, AgentNodeStatusLabel> = {
    pending: 'queued',
    running: 'active',
    completed: 'done',
    failed: 'failed',
    cancelled: 'cancelled',
    interrupted: 'suspended',
    archived: 'done',
  };
  return mapping[status] ?? 'queued';
}
