import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { ref } from 'vue';

// Mock onUnmounted since we're not in a component context
vi.mock('vue', async () => {
  const actual = await vi.importActual<typeof import('vue')>('vue');
  return {
    ...actual,
    onUnmounted: vi.fn(),
  };
});

import { useStatusPulse } from '../useStatusPulse';

describe('useStatusPulse', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('initially no teams are pulsing', () => {
    const isReplaying = ref(false);
    const { isPulsing, pulseColor } = useStatusPulse(isReplaying);

    expect(isPulsing('team-1')).toBe(false);
    expect(pulseColor('team-1')).toBe('');
  });

  it('onTeamStatusChanged(teamId, "running") triggers accent pulse', () => {
    const isReplaying = ref(false);
    const { onTeamStatusChanged, isPulsing, pulseColor } = useStatusPulse(isReplaying);

    onTeamStatusChanged('team-1', 'running');

    expect(isPulsing('team-1')).toBe(true);
    expect(pulseColor('team-1')).toBe('var(--color-accent)');
  });

  it('onTeamStatusChanged(teamId, "completed") triggers success pulse', () => {
    const isReplaying = ref(false);
    const { onTeamStatusChanged, isPulsing, pulseColor } = useStatusPulse(isReplaying);

    onTeamStatusChanged('team-1', 'completed');

    expect(isPulsing('team-1')).toBe(true);
    expect(pulseColor('team-1')).toBe('var(--color-success)');
  });

  it('onTeamStatusChanged(teamId, "failed") triggers danger pulse', () => {
    const isReplaying = ref(false);
    const { onTeamStatusChanged, isPulsing, pulseColor } = useStatusPulse(isReplaying);

    onTeamStatusChanged('team-1', 'failed');

    expect(isPulsing('team-1')).toBe(true);
    expect(pulseColor('team-1')).toBe('var(--color-danger)');
  });

  it('onTeamStatusChanged(teamId, "interrupted") triggers warning pulse', () => {
    const isReplaying = ref(false);
    const { onTeamStatusChanged, isPulsing, pulseColor } = useStatusPulse(isReplaying);

    onTeamStatusChanged('team-1', 'interrupted');

    expect(isPulsing('team-1')).toBe(true);
    expect(pulseColor('team-1')).toBe('var(--color-warning)');
  });

  it('unknown status does nothing', () => {
    const isReplaying = ref(false);
    const { onTeamStatusChanged, isPulsing, pulseColor } = useStatusPulse(isReplaying);

    onTeamStatusChanged('team-1', 'unknown_status');

    expect(isPulsing('team-1')).toBe(false);
    expect(pulseColor('team-1')).toBe('');
  });

  // D10-F5（P8）：PULSE_COLOR_MAP 补 partial_failure/cancelled/paused/pending 四键，
  // SpiritTeamStatus 十值中除 archived（纯展示折叠）外其余转移均产生确定性脉冲。
  it('partial_failure triggers warning pulse for 2000ms（失败先行）', () => {
    const isReplaying = ref(false);
    const { onTeamStatusChanged, isPulsing, pulseColor, pulseStates } = useStatusPulse(isReplaying);

    onTeamStatusChanged('team-1', 'partial_failure');

    expect(isPulsing('team-1')).toBe(true);
    expect(pulseColor('team-1')).toBe('var(--color-warning)');
    expect(pulseStates.value.get('team-1')?.durationMs).toBe(2000);

    vi.advanceTimersByTime(2000);
    expect(isPulsing('team-1')).toBe(false);
  });

  it('cancelled triggers tertiary pulse for 1500ms', () => {
    const isReplaying = ref(false);
    const { onTeamStatusChanged, isPulsing, pulseColor, pulseStates } = useStatusPulse(isReplaying);

    onTeamStatusChanged('team-1', 'cancelled');

    expect(pulseColor('team-1')).toBe('var(--color-text-tertiary)');
    expect(pulseStates.value.get('team-1')?.durationMs).toBe(1500);

    vi.advanceTimersByTime(1500);
    expect(isPulsing('team-1')).toBe(false);
  });

  it('paused triggers warning pulse for 1000ms', () => {
    const isReplaying = ref(false);
    const { onTeamStatusChanged, isPulsing, pulseColor, pulseStates } = useStatusPulse(isReplaying);

    onTeamStatusChanged('team-1', 'paused');

    expect(pulseColor('team-1')).toBe('var(--color-warning)');
    expect(pulseStates.value.get('team-1')?.durationMs).toBe(1000);

    vi.advanceTimersByTime(1000);
    expect(isPulsing('team-1')).toBe(false);
  });

  it('pending triggers tertiary pulse for 800ms（团队首次出现的弱提示）', () => {
    const isReplaying = ref(false);
    const { onTeamStatusChanged, isPulsing, pulseColor, pulseStates } = useStatusPulse(isReplaying);

    onTeamStatusChanged('team-1', 'pending');

    expect(pulseColor('team-1')).toBe('var(--color-text-tertiary)');
    expect(pulseStates.value.get('team-1')?.durationMs).toBe(800);

    vi.advanceTimersByTime(800);
    expect(isPulsing('team-1')).toBe(false);
  });

  it('四旧键行为与时长回归不变', () => {
    const isReplaying = ref(false);
    const { onTeamStatusChanged, pulseColor, pulseStates } = useStatusPulse(isReplaying);

    onTeamStatusChanged('t-running', 'running');
    onTeamStatusChanged('t-completed', 'completed');
    onTeamStatusChanged('t-failed', 'failed');
    onTeamStatusChanged('t-interrupted', 'interrupted');

    expect(pulseColor('t-running')).toBe('var(--color-accent)');
    expect(pulseStates.value.get('t-running')?.durationMs).toBe(1000);
    expect(pulseColor('t-completed')).toBe('var(--color-success)');
    expect(pulseStates.value.get('t-completed')?.durationMs).toBe(1500);
    expect(pulseColor('t-failed')).toBe('var(--color-danger)');
    expect(pulseStates.value.get('t-failed')?.durationMs).toBe(2000);
    expect(pulseColor('t-interrupted')).toBe('var(--color-warning)');
    expect(pulseStates.value.get('t-interrupted')?.durationMs).toBe(1500);
  });

  it('isPulsing(teamId) returns true during active pulse', () => {
    const isReplaying = ref(false);
    const { onTeamStatusChanged, isPulsing } = useStatusPulse(isReplaying);

    onTeamStatusChanged('team-1', 'running');

    expect(isPulsing('team-1')).toBe(true);
  });

  it('isPulsing(teamId) returns false after pulse duration expires', () => {
    const isReplaying = ref(false);
    const { onTeamStatusChanged, isPulsing } = useStatusPulse(isReplaying);

    onTeamStatusChanged('team-1', 'running');
    expect(isPulsing('team-1')).toBe(true);

    // running pulse duration is 1000ms
    vi.advanceTimersByTime(1000);

    expect(isPulsing('team-1')).toBe(false);
  });

  it('pulseColor(teamId) returns correct color', () => {
    const isReplaying = ref(false);
    const { onTeamStatusChanged, pulseColor } = useStatusPulse(isReplaying);

    onTeamStatusChanged('team-1', 'failed');
    expect(pulseColor('team-1')).toBe('var(--color-danger)');

    onTeamStatusChanged('team-2', 'completed');
    expect(pulseColor('team-2')).toBe('var(--color-success)');
  });

  it('isReplaying=true suppresses pulse', () => {
    const isReplaying = ref(true);
    const { onTeamStatusChanged, isPulsing, pulseColor } = useStatusPulse(isReplaying);

    onTeamStatusChanged('team-1', 'running');

    expect(isPulsing('team-1')).toBe(false);
    expect(pulseColor('team-1')).toBe('');
  });

  it('cleanup() clears all pulses', () => {
    const isReplaying = ref(false);
    const { onTeamStatusChanged, isPulsing, cleanup } = useStatusPulse(isReplaying);

    onTeamStatusChanged('team-1', 'running');
    onTeamStatusChanged('team-2', 'completed');

    expect(isPulsing('team-1')).toBe(true);
    expect(isPulsing('team-2')).toBe(true);

    cleanup();

    expect(isPulsing('team-1')).toBe(false);
    expect(isPulsing('team-2')).toBe(false);
  });

  it('cleanup() prevents pending timeouts from firing', () => {
    const isReplaying = ref(false);
    const { onTeamStatusChanged, isPulsing, cleanup } = useStatusPulse(isReplaying);

    onTeamStatusChanged('team-1', 'running');
    cleanup();

    // Advance past the original timeout — should not cause issues
    vi.advanceTimersByTime(2000);

    expect(isPulsing('team-1')).toBe(false);
  });

  it('new pulse for same team replaces previous pulse', () => {
    const isReplaying = ref(false);
    const { onTeamStatusChanged, isPulsing, pulseColor } = useStatusPulse(isReplaying);

    onTeamStatusChanged('team-1', 'running');
    expect(pulseColor('team-1')).toBe('var(--color-accent)');

    onTeamStatusChanged('team-1', 'completed');
    expect(pulseColor('team-1')).toBe('var(--color-success)');
    expect(isPulsing('team-1')).toBe(true);
  });

  it('different teams pulse independently', () => {
    const isReplaying = ref(false);
    const { onTeamStatusChanged, isPulsing, pulseColor } = useStatusPulse(isReplaying);

    onTeamStatusChanged('team-1', 'running');
    onTeamStatusChanged('team-2', 'failed');

    expect(pulseColor('team-1')).toBe('var(--color-accent)');
    expect(pulseColor('team-2')).toBe('var(--color-danger)');

    // running duration = 1000ms, failed duration = 2000ms
    vi.advanceTimersByTime(1000);

    expect(isPulsing('team-1')).toBe(false);
    expect(isPulsing('team-2')).toBe(true);

    vi.advanceTimersByTime(1000);

    expect(isPulsing('team-2')).toBe(false);
  });
});
