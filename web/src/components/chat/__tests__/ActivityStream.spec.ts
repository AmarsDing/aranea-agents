import { describe, it, expect, vi, afterEach } from 'vitest';
import { createApp, type App as VueApp } from 'vue';
import { createI18n } from 'vue-i18n';
import type { Activity, ActivityTreeNode } from '../../../features/chat/activityTypes';

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

/** Wrap an Activity as a leaf ActivityTreeNode (no children). B-04 / Phase A:
 * ActivityStream now consumes `activityTree: ActivityTreeNode[]` and recurses
 * over each node's `children` to render nested Activities. */
function asTreeNode(activity: Activity, children: ActivityTreeNode[] = []): ActivityTreeNode {
  return { ...activity, children };
}

/** Mount ActivityStream with the given activity tree prop, returning the DOM container. */
function mountActivityStream(activityTree: ActivityTreeNode[]): { container: HTMLElement; app: VueApp } {
  const i18n = createI18n({
    legacy: false,
    locale: 'en',
    messages: { en: {} },
  });
  const container = document.createElement('div');
  document.body.appendChild(container);
  const app = createApp(ActivityStream, { activityTree });
  app.use(i18n);
  app.mount(container);
  return { container, app };
}

afterEach(() => {
  document.body.innerHTML = '';
});

describe('ActivityStream', () => {
  it('renders task activity as UserMessageBubble', () => {
    const activityTree: ActivityTreeNode[] = [
      asTreeNode(makeActivity({ id: 't1', kind: 'task', content: 'hello world' })),
    ];
    const { container } = mountActivityStream(activityTree);
    expect(container.querySelector('.user-message-bubble')).toBeTruthy();
    expect(container.textContent).toContain('hello world');
  });

  it('does not render UserMessageBubble for task.failed', () => {
    const activityTree: ActivityTreeNode[] = [
      asTreeNode(makeActivity({ id: 't1', kind: 'task', status: 'failed', content: 'failed task' })),
    ];
    const { container } = mountActivityStream(activityTree);
    expect(container.querySelector('.user-message-bubble')).toBeNull();
  });

  it('renders thinking and reply activities', () => {
    const activityTree: ActivityTreeNode[] = [
      asTreeNode(
        makeActivity({
          id: 'th1',
          kind: 'thinking',
          parentActivityId: 't1',
          content: 'thinking content',
        }),
      ),
      asTreeNode(
        makeActivity({
          id: 'r1',
          kind: 'reply',
          parentActivityId: 't1',
          content: 'reply content',
        }),
      ),
    ];
    const { container } = mountActivityStream(activityTree);
    expect(container.querySelector('.thinking-block')).toBeTruthy();
    expect(container.querySelector('.reply-block')).toBeTruthy();
  });

  it('B-04: renders nested children in an indented container', () => {
    // Parent plan with two child activities (thinking + reply).
    const activityTree: ActivityTreeNode[] = [
      asTreeNode(
        makeActivity({ id: 'p1', kind: 'plan', content: 'plan' }),
        [
          asTreeNode(
            makeActivity({ id: 'th1', kind: 'thinking', parentActivityId: 'p1', content: 'child thinking' }),
          ),
          asTreeNode(
            makeActivity({ id: 'r1', kind: 'reply', parentActivityId: 'p1', content: 'child reply' }),
          ),
        ],
      ),
    ];
    const { container } = mountActivityStream(activityTree);
    // The plan block itself should render.
    expect(container.querySelector('.plan-block')).toBeTruthy();
    // The nested children container should exist with the proper class.
    const childrenContainer = container.querySelector('.event-stream__children');
    expect(childrenContainer).toBeTruthy();
    // Child blocks should render inside the children container.
    expect(childrenContainer!.querySelector('.thinking-block')).toBeTruthy();
    expect(childrenContainer!.querySelector('.reply-block')).toBeTruthy();
  });
});
