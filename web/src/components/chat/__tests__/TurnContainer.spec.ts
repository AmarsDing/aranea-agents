// web/src/components/chat/__tests__/TurnContainer.spec.ts
import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { mount } from '@vue/test-utils';
import { setActivePinia, createPinia } from 'pinia';
import TurnContainer from '../v2/TurnContainer.vue';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import { useUiConfigStore } from '../../../stores/uiConfig';
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

// 2026-08-21 全链路审查 R2：showToolCalls 开关贯通 action steps。
// 此前只管 TodoKanban，ActionBlock 无条件渲染，开关名不副实。
describe('TurnContainer.visibleSteps — showToolCalls action filtering', () => {
  beforeEach(() => {
    // uiConfig store 在创建时读 localStorage；先清键再建 pinia，保证默认 true。
    localStorage.removeItem('chat.ui.showToolCalls');
    setActivePinia(createPinia());
  });
  afterEach(() => localStorage.removeItem('chat.ui.showToolCalls'));

  it('renders action step when showToolCalls is true (default)', () => {
    const store = useChatActivityStore();
    store.upsertStep(mkStep({ ID: 'a1', Kind: 'action', ToolName: 'exec_command' }));
    const wrapper = mount(TurnContainer, { props: { turn: mkTurn() } });
    expect(wrapper.findAll('.act-activity')).toHaveLength(1);
  });

  it('hides action step when showToolCalls is false, keeps reply', () => {
    const store = useChatActivityStore();
    store.upsertStep(mkStep({ ID: 'a1', Kind: 'action', ToolName: 'exec_command', Seq: 1 }));
    store.upsertStep(mkStep({ ID: 'r1', Kind: 'reply', Content: 'done', Status: 'completed', Seq: 2, IsFinal: true }));
    const uiConfig = useUiConfigStore();
    uiConfig.setShowToolCalls(false);
    const wrapper = mount(TurnContainer, { props: { turn: mkTurn() } });
    expect(wrapper.findAll('.act-activity')).toHaveLength(0);
    expect(wrapper.findAll('.reply-block')).toHaveLength(1);
  });

  it('keeps plan_and_execute action visible when showToolCalls is false', () => {
    const store = useChatActivityStore();
    store.upsertStep(mkStep({ ID: 'a1', Kind: 'action', ToolName: 'plan_and_execute', Seq: 1 }));
    const uiConfig = useUiConfigStore();
    uiConfig.setShowToolCalls(false);
    const wrapper = mount(TurnContainer, { props: { turn: mkTurn() } });
    expect(wrapper.findAll('.act-activity')).toHaveLength(1);
  });

  it('re-renders action step when toggled back on (reactive)', async () => {
    const store = useChatActivityStore();
    store.upsertStep(mkStep({ ID: 'a1', Kind: 'action', ToolName: 'exec_command' }));
    const uiConfig = useUiConfigStore();
    uiConfig.setShowToolCalls(false);
    const wrapper = mount(TurnContainer, { props: { turn: mkTurn() } });
    expect(wrapper.findAll('.act-activity')).toHaveLength(0);
    uiConfig.setShowToolCalls(true);
    await wrapper.vm.$nextTick();
    expect(wrapper.findAll('.act-activity')).toHaveLength(1);
  });
});
