import { describe, it, expect, vi, afterEach } from 'vitest';
import { createApp, type App as VueApp } from 'vue';
import { createI18n } from 'vue-i18n';
import type { Activity } from '../../../features/chat/activityTypes';

// Mock listActivities to avoid loading session/api module side effects
// (transitively imported via useActivityTimeline.activityToStreamEvent).
vi.mock('../../../features/session/api', () => ({
  listActivities: vi.fn().mockResolvedValue([]),
}));

import ActivityStream from '../ActivityStream.vue';

/** Build a minimal valid Activity with sensible defaults. */
function makeActivity(
  overrides: Partial<Activity> & Pick<Activity, 'id' | 'kind'>,
): Activity {
  return {
    status: 'completed',
    sessionId: 's1',
    turnId: 't1',
    parentActivityId: null,
    timestamp: '2026-01-01T00:00:00Z',
    durationMs: null,
    collapsed: false,
    ...overrides,
  };
}

/** Mount ActivityStream with the given activities prop, returning the DOM container. */
function mountActivityStream(activities: Activity[]): { container: HTMLElement; app: VueApp } {
  const i18n = createI18n({
    legacy: false,
    locale: 'en',
    messages: { en: {} },
  });
  const container = document.createElement('div');
  document.body.appendChild(container);
  const app = createApp(ActivityStream, { activities });
  app.use(i18n);
  app.mount(container);
  return { container, app };
}

afterEach(() => {
  document.body.innerHTML = '';
});

describe('ActivityStream', () => {
  it('renders task activity as UserMessageBubble', () => {
    const activities: Activity[] = [
      makeActivity({ id: 't1', kind: 'task', content: 'hello world' }),
    ];
    const { container } = mountActivityStream(activities);
    expect(container.querySelector('.user-message-bubble')).toBeTruthy();
    expect(container.textContent).toContain('hello world');
  });

  it('does not render UserMessageBubble for task.failed', () => {
    const activities: Activity[] = [
      makeActivity({ id: 't1', kind: 'task', status: 'failed', content: 'failed task' }),
    ];
    const { container } = mountActivityStream(activities);
    expect(container.querySelector('.user-message-bubble')).toBeNull();
  });

  it('renders thinking and reply activities', () => {
    const activities: Activity[] = [
      makeActivity({
        id: 'th1',
        kind: 'thinking',
        parentActivityId: 't1',
        content: 'thinking content',
      }),
      makeActivity({
        id: 'r1',
        kind: 'reply',
        parentActivityId: 't1',
        content: 'reply content',
      }),
    ];
    const { container } = mountActivityStream(activities);
    expect(container.querySelector('.thinking-block')).toBeTruthy();
    expect(container.querySelector('.reply-block')).toBeTruthy();
  });
});
