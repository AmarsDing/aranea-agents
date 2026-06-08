/**
 * OBS-05: Sidebar status pulse composable.
 *
 * Manages pulse animations for team cards in the ChatEntitySidebar.
 * When a team's status changes, a brief highlight animation is triggered.
 * Pulses are suppressed during WS replay to avoid flicker.
 */
import { onUnmounted, ref, type Ref } from 'vue';
import { PULSE_COLOR_MAP, PULSE_DURATION_MAP } from '../../../features/spirit/observabilityConstants';

export type PulseState = {
  /** CSS color for the pulse animation. */
  color: string;
  /** Whether the pulse is currently active. */
  active: boolean;
  /** Duration of the pulse animation in milliseconds. */
  durationMs: number;
};

export function useStatusPulse(isReplaying: Ref<boolean>) {
  /** Map of teamId → current pulse state. */
  const pulseStates = ref<Map<string, PulseState>>(new Map());

  /** Active timeout IDs for cleanup. */
  const activeTimeouts = new Map<string, ReturnType<typeof setTimeout>>();

  /**
   * Call when a team's status changes.
   * Triggers a pulse animation for the team card.
   */
  function onTeamStatusChanged(teamId: string, newStatus: string): void {
    if (isReplaying.value) return;

    const pulseColor = PULSE_COLOR_MAP[newStatus];
    if (!pulseColor) return;

    const durationMs = PULSE_DURATION_MAP[newStatus] ?? 1500;

    // Clear any existing timeout for this team
    const existingTimeout = activeTimeouts.get(teamId);
    if (existingTimeout !== undefined) {
      clearTimeout(existingTimeout);
    }

    // Set the pulse state
    pulseStates.value.set(teamId, { color: pulseColor, active: true, durationMs });

    // Auto-clear after duration
    const timeout = setTimeout(() => {
      pulseStates.value.delete(teamId);
      activeTimeouts.delete(teamId);
    }, durationMs);

    activeTimeouts.set(teamId, timeout);
  }

  /**
   * Check if a team currently has an active pulse.
   */
  function isPulsing(teamId: string): boolean {
    return pulseStates.value.get(teamId)?.active ?? false;
  }

  /**
   * Get the pulse color for a team.
   */
  function pulseColor(teamId: string): string {
    return pulseStates.value.get(teamId)?.color ?? '';
  }

  /**
   * Clean up all active timeouts (e.g., on unmount).
   */
  function cleanup(): void {
    for (const timeout of activeTimeouts.values()) {
      clearTimeout(timeout);
    }
    activeTimeouts.clear();
    pulseStates.value.clear();
  }

  // Auto-cleanup on component unmount
  onUnmounted(() => cleanup());

  return {
    pulseStates,
    onTeamStatusChanged,
    isPulsing,
    pulseColor,
    cleanup,
  };
}
