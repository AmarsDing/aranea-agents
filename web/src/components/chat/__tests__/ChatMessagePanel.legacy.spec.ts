import { describe, it, expect, afterEach, vi } from 'vitest';
import { createApp, defineComponent, type App as VueApp } from 'vue';
import { createI18n } from 'vue-i18n';

// Mock the session/api module to prevent transitive side effects via composables.
vi.mock('../../../features/session/api', () => ({
  listActivities: vi.fn().mockResolvedValue([]),
  listSessions: vi.fn().mockResolvedValue([]),
  getSessionTree: vi.fn().mockResolvedValue({ session: {}, children: [] }),
}));

import ChatMessagePanel from '../ChatMessagePanel.vue';

// Lightweight stubs for heavy child components — we only care about whether
// the legacy TaskExecutionPanel / MemberReadOnlyPanel root nodes are rendered.
const stubs = {
  ChatMessageList: defineComponent({ template: '<div class="chat-message-list-stub" />' }),
  ChatComposer: defineComponent({ template: '<div class="chat-composer-stub" />' }),
  ChatHeaderUsagePanel: defineComponent({ template: '<div />' }),
  ChatHeaderPromptBar: defineComponent({ template: '<div />' }),
  ChatReasoningDrawer: defineComponent({ template: '<div />' }),
  ChatRunnerStatus: defineComponent({ template: '<div />' }),
  ChatTeamMemberStrip: defineComponent({ template: '<div />' }),
  TodoKanbanBoard: defineComponent({ template: '<div />' }),
  UiConfigToggle: defineComponent({ template: '<div />' }),
  UnifiedExecutionPanel: defineComponent({ template: '<div />' }),
  ContextIndicator: defineComponent({ template: '<div />' }),
  SpiritStatusBar: defineComponent({ template: '<div />' }),
};

const i18n = createI18n({
  legacy: false,
  locale: 'en',
  messages: { en: {} },
});

function mountPanel(props: Record<string, unknown>): { container: HTMLElement; app: VueApp } {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const app = createApp(ChatMessagePanel, {
    modelValue: '',
    messages: [],
    attachments: [],
    dialogMode: 'chat',
    modelProvider: '',
    modeOptions: [],
    providerOptions: [],
    sessionTitle: 'Test',
    isDark: false,
    activityTree: [],
    ...props,
  });
  app.use(i18n);
  for (const [name, stub] of Object.entries(stubs)) {
    app.component(name, stub);
  }
  app.mount(container);
  return { container, app };
}

afterEach(() => {
  document.body.innerHTML = '';
});

describe('ChatMessagePanel legacy path removal (Phase B-5)', () => {
  it('does not render TaskExecutionPanel in team mode', () => {
    const { container } = mountPanel({
      panelMode: 'team',
      spiritTeam: { id: 't1', teamName: 'Team 1', teamSessionId: 's1', members: [] } as any,
    });
    expect(container.querySelector('.task-execution-panel')).toBeNull();
  });

  it('does not render MemberReadOnlyPanel in member mode', () => {
    const { container } = mountPanel({
      panelMode: 'member',
      spiritTeam: { id: 't1', teamName: 'Team 1', teamSessionId: 's1', members: [] } as any,
      activeMember: {
        agentId: 'a1',
        agentKey: 'k1',
        displayName: 'M',
        role: 'r',
        status: 'running',
        avatarUrl: '',
      } as any,
    });
    expect(container.querySelector('.member-readonly-panel')).toBeNull();
  });
});
