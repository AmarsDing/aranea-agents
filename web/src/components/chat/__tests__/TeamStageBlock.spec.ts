import { describe, it, expect, afterEach } from 'vitest';
import { createApp, h, nextTick, ref, type App as VueApp } from 'vue';
import { createI18n } from 'vue-i18n';
import TeamStageBlock from '../TeamStageBlock.vue';
import type { TeamStageEvent } from '../../../features/chat/streamEventTypes';

const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  messages: { 'zh-CN': { chat: { teamStage: { expandMember: '展开 {name} 的会话' } } } },
});

const baseActivity: TeamStageEvent = {
  id: 'ts-1',
  kind: 'team_stage',
  status: 'running',
  sessionId: 'team-1',
  timestamp: new Date().toISOString(),
  durationMs: null,
  stage: 'executing',
  members: [
    { agentKey: 'agent-key-A', agentName: 'Agent A', status: 'running' },
    { agentKey: 'agent-key-B', agentName: 'Agent B', status: 'pending' },
  ],
} as unknown as TeamStageEvent;

type ExpandMemberPayload = { agentKey: string; agentName?: string };

/**
 * Mount TeamStageBlock inside a parent that captures `expand-member` emits.
 * Uses createApp (project convention) instead of @vue/test-utils, which is
 * not a project dependency. Emits are observed via the onExpandMember prop.
 */
function mountTeamStage(activity: TeamStageEvent): {
  container: HTMLElement;
  captured: ReturnType<typeof ref<ExpandMemberPayload | null>>;
} {
  const captured = ref<ExpandMemberPayload | null>(null);
  const Parent = {
    setup() {
      return () =>
        h(TeamStageBlock, {
          activity,
          onExpandMember: (p: ExpandMemberPayload) => {
            captured.value = p;
          },
        });
    },
  };
  const container = document.createElement('div');
  document.body.appendChild(container);
  const app: VueApp = createApp(Parent);
  app.use(i18n);
  app.mount(container);
  return { container, captured };
}

afterEach(() => {
  document.body.innerHTML = '';
});

describe('TeamStageBlock', () => {
  it('emits expand-member with agentKey when a member row is clicked', async () => {
    const { container, captured } = mountTeamStage(baseActivity);
    const rows = container.querySelectorAll('.team-stage-block__member');
    expect(rows).toHaveLength(2);
    rows[0].dispatchEvent(new MouseEvent('click', { bubbles: true }));
    await nextTick();
    expect(captured.value).toEqual({ agentKey: 'agent-key-A', agentName: 'Agent A' });
  });

  it('emits expand-member with undefined agentName when missing', async () => {
    const activityNoName = {
      ...baseActivity,
      members: [{ agentKey: 'agent-key-C', status: 'pending' }],
    } as unknown as TeamStageEvent;
    const { container, captured } = mountTeamStage(activityNoName);
    const row = container.querySelector('.team-stage-block__member') as HTMLElement;
    row.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    await nextTick();
    expect(captured.value).toEqual({ agentKey: 'agent-key-C', agentName: undefined });
  });
});
