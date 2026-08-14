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

// 75：运行中的 turn 含 computer_use 工具会话时内嵌 CuStepStream（实时 + 急停）；
// 历史 turn 同样内嵌，但 readonly（急停隐藏，审计回放走 ListComputerUseSteps）。
describe('TurnContainer — CuStepStream embedding', () => {
  beforeEach(() => setActivePinia(createPinia()));

  const cuActionStep = (over: Partial<Step> = {}) =>
    mkStep({
      ID: 'cu1',
      Kind: 'action',
      ToolName: 'computer_use_act',
      ToolResult: { session_id: 'cu-sess-1', result: 'ok' },
      ...over,
    });

  function mountWithStream(turn: Turn) {
    return mount(TurnContainer, {
      props: { turn },
      global: { stubs: { CuStepStream: { template: '<div class="cu-stream-stub" :data-session="sessionId" :data-readonly="String(readonly)" />', props: ['sessionId', 'readonly'] } } },
    });
  }

  it('embeds CuStepStream with session id when turn is running', () => {
    const store = useChatActivityStore();
    store.upsertStep(cuActionStep());
    const wrapper = mountWithStream(mkTurn({ Status: 'running' }));
    const stub = wrapper.find('.cu-stream-stub');
    expect(stub.exists()).toBe(true);
    expect(stub.attributes('data-session')).toBe('cu-sess-1');
    expect(stub.attributes('data-readonly')).toBe('false');
  });

  it('embeds readonly CuStepStream for completed turns (historical replay)', () => {
    const store = useChatActivityStore();
    store.upsertStep(cuActionStep());
    const wrapper = mountWithStream(mkTurn({ Status: 'completed' }));
    const stub = wrapper.find('.cu-stream-stub');
    expect(stub.exists()).toBe(true);
    expect(stub.attributes('data-session')).toBe('cu-sess-1');
    expect(stub.attributes('data-readonly')).toBe('true');
  });

  it('does not embed when no computer_use session exists in steps', () => {
    const store = useChatActivityStore();
    store.upsertStep(mkStep({ ID: 'a1', Kind: 'action', ToolName: 'web_search', ToolResult: { session_id: 'x' } }));
    const wrapper = mountWithStream(mkTurn({ Status: 'running' }));
    expect(wrapper.find('.cu-stream-stub').exists()).toBe(false);
  });
});
