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
