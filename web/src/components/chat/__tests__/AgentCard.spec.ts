import { describe, it, expect, vi, afterEach } from 'vitest';
import { createApp, nextTick, type App as VueApp } from 'vue';
import { createI18n } from 'vue-i18n';
import zhCN from '../../../i18n/locales/zh-CN';
import type { SessionStageEvent } from '../../../features/chat/streamEventTypes';
import type { RunStatusValue } from '../../../features/chat/types';

// Mock listActivities to avoid loading session/api module side effects
// (transitively imported via useActivityTimeline.activityToStreamEvent).
vi.mock('../../../features/session/api', () => ({
  listActivities: vi.fn().mockResolvedValue([]),
}));

import AgentCard from '../AgentCard.vue';

/** Build a minimal SessionStageEvent with sensible defaults. */
function makeSessionEvent(
  overrides: Partial<SessionStageEvent> & Pick<SessionStageEvent, 'id'>,
): SessionStageEvent {
  return {
    id: overrides.id,
    kind: 'session',
    status: 'running',
    title: 'agent task',
    childSessionId: 'sess-agent-1',
    agentKey: 'agent_analyst',
    agentName: 'Analyst',
    timestamp: '2026-06-01T10:00:00Z',
    ...overrides,
  };
}

interface MountOptions {
  runStatus?: RunStatusValue;
  /** When provided, AgentCard emits are routed to these handlers. */
  onPauseAgent?: (sessionId: string) => void;
  onResumeAgent?: (sessionId: string) => void;
  onInjectAgent?: (payload: { sessionId: string; message: string }) => void;
}

/** Mount AgentCard with the given activity prop, returning the DOM container. */
function mountAgentCard(
  activity: SessionStageEvent,
  opts: MountOptions = {},
): { container: HTMLElement; app: VueApp } {
  const i18n = createI18n({
    legacy: false,
    locale: 'zh-CN',
    messages: { 'zh-CN': zhCN },
  });
  const container = document.createElement('div');
  document.body.appendChild(container);
  const app = createApp(AgentCard, {
    activity,
    runStatus: opts.runStatus ?? 'running',
    // Vue 3 supports passing emit listeners as onXxx props.
    onPauseAgent: opts.onPauseAgent,
    onResumeAgent: opts.onResumeAgent,
    onInjectAgent: opts.onInjectAgent,
  });
  app.use(i18n);
  app.mount(container);
  return { container, app };
}

/** Captured emit events recorded in order. */
type CapturedEvent =
  | { name: 'pause-agent'; args: [string] }
  | { name: 'resume-agent'; args: [string] }
  | { name: 'inject-agent'; args: [{ sessionId: string; message: string }] };

/** Mount AgentCard and capture all emits via onXxx prop listeners. */
function captureEmit(activity: SessionStageEvent, opts: MountOptions = {}): {
  events: CapturedEvent[];
  pauseBtn: HTMLElement | null;
  resumeBtn: HTMLElement | null;
  injectInput: HTMLInputElement | null;
  injectSend: HTMLButtonElement | null;
} {
  const events: CapturedEvent[] = [];
  const { container } = mountAgentCard(activity, {
    ...opts,
    onPauseAgent: (sessionId: string) => events.push({ name: 'pause-agent', args: [sessionId] }),
    onResumeAgent: (sessionId: string) => events.push({ name: 'resume-agent', args: [sessionId] }),
    onInjectAgent: (payload: { sessionId: string; message: string }) =>
      events.push({ name: 'inject-agent', args: [payload] }),
  });
  return {
    events,
    pauseBtn: container.querySelector('.agent-card__action--pause'),
    resumeBtn: container.querySelector('.agent-card__action--resume'),
    injectInput: container.querySelector('.agent-card__inject-input'),
    injectSend: container.querySelector('.agent-card__inject-send'),
  };
}

afterEach(() => {
  document.body.innerHTML = '';
});

describe('AgentCard', () => {
  it('running + parent running: shows pause + cancel buttons + inject dialog', () => {
    const { container } = mountAgentCard(
      makeSessionEvent({ id: 'a1', status: 'running' }),
      { runStatus: 'running' },
    );
    // Pause button visible
    expect(container.querySelector('.agent-card__action--pause')).toBeTruthy();
    // Cancel button visible
    expect(container.querySelector('.agent-card__action--cancel')).toBeTruthy();
    // Retry button hidden
    expect(container.querySelector('.agent-card__action--retry')).toBeNull();
    // Inject dialog visible
    expect(container.querySelector('.agent-card__inject')).toBeTruthy();
  });

  it('paused: shows resume + cancel buttons + inject dialog', () => {
    const { container } = mountAgentCard(
      makeSessionEvent({ id: 'a2', status: 'paused' }),
      { runStatus: 'running' },
    );
    // Resume button visible
    expect(container.querySelector('.agent-card__action--resume')).toBeTruthy();
    // Cancel button visible
    expect(container.querySelector('.agent-card__action--cancel')).toBeTruthy();
    // Pause button hidden (replaced by resume)
    expect(container.querySelector('.agent-card__action--pause')).toBeNull();
    // Inject dialog visible
    expect(container.querySelector('.agent-card__inject')).toBeTruthy();
    // Status badge should be orange
    expect(container.querySelector('.agent-card__status-badge--orange')).toBeTruthy();
  });

  it('failed: shows retry button only (no inject dialog)', () => {
    const { container } = mountAgentCard(
      makeSessionEvent({ id: 'a3', status: 'failed' }),
    );
    expect(container.querySelector('.agent-card__action--retry')).toBeTruthy();
    // Pause/resume/cancel all hidden
    expect(container.querySelector('.agent-card__action--pause')).toBeNull();
    expect(container.querySelector('.agent-card__action--resume')).toBeNull();
    expect(container.querySelector('.agent-card__action--cancel')).toBeNull();
    // Inject dialog hidden
    expect(container.querySelector('.agent-card__inject')).toBeNull();
  });

  it('completed: hides all action buttons and inject dialog', () => {
    const { container } = mountAgentCard(
      makeSessionEvent({ id: 'a4', status: 'completed' }),
    );
    expect(container.querySelector('.agent-card__action--pause')).toBeNull();
    expect(container.querySelector('.agent-card__action--resume')).toBeNull();
    expect(container.querySelector('.agent-card__action--cancel')).toBeNull();
    expect(container.querySelector('.agent-card__action--retry')).toBeNull();
    expect(container.querySelector('.agent-card__inject')).toBeNull();
  });

  it('cancelled: hides all action buttons', () => {
    const { container } = mountAgentCard(
      makeSessionEvent({ id: 'a5', status: 'cancelled' }),
    );
    expect(container.querySelector('.agent-card__action--pause')).toBeNull();
    expect(container.querySelector('.agent-card__action--resume')).toBeNull();
    expect(container.querySelector('.agent-card__action--cancel')).toBeNull();
    expect(container.querySelector('.agent-card__action--retry')).toBeNull();
  });

  it('system agent (__spirit__): hides all action buttons even when running', () => {
    const { container } = mountAgentCard(
      makeSessionEvent({
        id: 'a6',
        status: 'running',
        agentKey: '__spirit__',
        agentName: 'System',
      }),
      { runStatus: 'running' },
    );
    expect(container.querySelector('.agent-card__action--pause')).toBeNull();
    expect(container.querySelector('.agent-card__action--resume')).toBeNull();
    expect(container.querySelector('.agent-card__action--cancel')).toBeNull();
    expect(container.querySelector('.agent-card__action--retry')).toBeNull();
    expect(container.querySelector('.agent-card__inject')).toBeNull();
  });

  it('pause button click emits pause-agent with childSessionId', async () => {
    const captured = captureEmit(
      makeSessionEvent({ id: 'a7', status: 'running', childSessionId: 'sess-pause-target' }),
      { runStatus: 'running' },
    );
    expect(captured.pauseBtn).toBeTruthy();
    captured.pauseBtn!.click();
    await nextTick();
    expect(captured.events).toContainEqual({
      name: 'pause-agent',
      args: ['sess-pause-target'],
    });
  });

  it('inject send button emits inject-agent with sessionId + message', async () => {
    const captured = captureEmit(
      makeSessionEvent({ id: 'a8', status: 'running', childSessionId: 'sess-inject-target' }),
      { runStatus: 'running' },
    );
    const input = captured.injectInput;
    const sendBtn = captured.injectSend;
    expect(input).toBeTruthy();
    expect(sendBtn).toBeTruthy();

    // Type a message — v-model on input listens to the `input` event.
    input!.value = 'revise the second step';
    input!.dispatchEvent(new Event('input', { bubbles: true }));
    // Flush Vue's reactivity before clicking send so injectMessage ref is updated.
    await nextTick();

    sendBtn!.click();
    await nextTick();

    expect(captured.events).toContainEqual({
      name: 'inject-agent',
      args: [{ sessionId: 'sess-inject-target', message: 'revise the second step' }],
    });
  });
});
