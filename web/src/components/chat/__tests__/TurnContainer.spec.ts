// web/src/components/chat/__tests__/TurnContainer.spec.ts
import { describe, it, expect, beforeEach } from 'vitest';
import { mount } from '@vue/test-utils';
import { setActivePinia, createPinia } from 'pinia';
import TurnContainer from '../v2/TurnContainer.vue';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { Turn, Step } from '../../../features/chat/v2Types';

function mkTurn(over: Partial<Turn> = {}): Turn {
  return {
    ID: 'turn-1',
    TaskID: 'task-1',
    SessionID: 'sess-1',
    SpiritSessionID: 'spirit-1',
    ParentTurnID: '',
    AgentKey: 'agent-1',
    TeamID: '',
    TeamStageID: '',
    Seq: 1,
    Version: 1,
    Status: 'running',
    StartedAt: '',
    CompletedAt: null,
    ...over,
  };
}

function mkStep(over: Partial<Step> = {}): Step {
  return {
    ID: 's1',
    TurnID: 'turn-1',
    TaskID: 'task-1',
    SessionID: 'sess-1',
    SpiritSessionID: 'spirit-1',
    Kind: 'reply',
    AuthorAgentKey: 'agent-1',
    Seq: 1,
    Version: 1,
    Content: '',
    Reasoning: '',
    ToolName: '',
    ToolCallID: '',
    ToolArgs: null,
    ToolResult: null,
    ToolDurationMs: 0,
    ToolErrorCode: '',
    NoticeType: '',
    Status: 'completed',
    IsFinal: false,
    StartedAt: '',
    CompletedAt: null,
    ...over,
  };
}

describe('TurnContainer.visibleSteps — empty reply filtering', () => {
  beforeEach(() => setActivePinia(createPinia()));

  it('filters empty completed reply step', () => {
    const store = useChatActivityStore();
    store.upsertStep(mkStep({ ID: 'r1', Kind: 'reply', Content: '', Status: 'completed' }));
    const wrapper = mount(TurnContainer, { props: { turn: mkTurn() } });
    // No ReplyBlock should render — its root has class "reply-block"
    expect(wrapper.findAll('.reply-block')).toHaveLength(0);
  });

  it('filters empty cancelled reply step', () => {
    const store = useChatActivityStore();
    store.upsertStep(mkStep({ ID: 'r1', Kind: 'reply', Content: '', Status: 'cancelled' }));
    const wrapper = mount(TurnContainer, { props: { turn: mkTurn() } });
    expect(wrapper.findAll('.reply-block')).toHaveLength(0);
  });

  it('filters whitespace-only completed reply step', () => {
    const store = useChatActivityStore();
    store.upsertStep(mkStep({ ID: 'r1', Kind: 'reply', Content: '   \n  ', Status: 'completed' }));
    const wrapper = mount(TurnContainer, { props: { turn: mkTurn() } });
    expect(wrapper.findAll('.reply-block')).toHaveLength(0);
  });

  it('keeps running empty reply step (streaming in progress)', () => {
    const store = useChatActivityStore();
    store.upsertStep(mkStep({ ID: 'r1', Kind: 'reply', Content: '', Status: 'running' }));
    const wrapper = mount(TurnContainer, { props: { turn: mkTurn() } });
    expect(wrapper.findAll('.reply-block')).toHaveLength(1);
  });

  it('keeps non-empty reply step', () => {
    const store = useChatActivityStore();
    store.upsertStep(mkStep({ ID: 'r1', Kind: 'reply', Content: 'Hi', Status: 'completed', IsFinal: true }));
    const wrapper = mount(TurnContainer, { props: { turn: mkTurn() } });
    expect(wrapper.findAll('.reply-block')).toHaveLength(1);
  });

  it('does not affect thinking steps', () => {
    const store = useChatActivityStore();
    store.upsertStep(mkStep({ ID: 't1', Kind: 'thinking', Reasoning: 'think', Content: '', Status: 'completed' }));
    const wrapper = mount(TurnContainer, { props: { turn: mkTurn() } });
    // ThinkingBlock renders with class "thinking-block" (verified in ThinkingBlock.vue)
    expect(wrapper.findAll('.thinking-block')).toHaveLength(1);
  });

  it('renders mixed: empty reply filtered + non-empty reply kept', () => {
    const store = useChatActivityStore();
    store.upsertStep(mkStep({ ID: 'r1', Kind: 'reply', Content: '', Status: 'cancelled', Seq: 1 }));
    store.upsertStep(mkStep({ ID: 'r2', Kind: 'reply', Content: 'real', Status: 'completed', Seq: 2, IsFinal: true }));
    const wrapper = mount(TurnContainer, { props: { turn: mkTurn() } });
    expect(wrapper.findAll('.reply-block')).toHaveLength(1);
  });
});
