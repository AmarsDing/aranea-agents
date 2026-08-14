import { describe, it, expect, vi, beforeEach } from 'vitest';
import { ref, type Ref } from 'vue';
import { useChatComposerActions } from '../useChatComposerActions';
import type { ComposerActionDeps } from '../useChatComposerActions';
import type { Message } from '../../types';

function makeMessage(overrides: Partial<Message> & Pick<Message, 'id' | 'role'>): Message {
  return {
    session_id: 'sid-1',
    parent_message_id: '',
    turn_id: 't1',
    turn_number: 1,
    seq_in_turn: 1,
    content_markdown: '',
    model_name: '',
    token_in: 0,
    token_out: 0,
    latency_ms: 0,
    status: 'ok',
    attachments_count: 0,
    options_json: '',
    error_message: '',
    created_at: '2026-06-21T00:00:00Z',
    ...overrides,
  } as Message;
}

function makeDeps(overrides?: Partial<ComposerActionDeps>): ComposerActionDeps {
  return {
    sessionStore: { entityKind: 'agent' } as unknown as ComposerActionDeps['sessionStore'],
    messageStore: {
      getMessages: vi.fn().mockReturnValue([]),
      setMessages: vi.fn(),
    } as unknown as ComposerActionDeps['messageStore'],
    runtimeStore: {} as unknown as ComposerActionDeps['runtimeStore'],
    streamManager: { cancelActiveStream: vi.fn() } as unknown as ComposerActionDeps['streamManager'],
    sender: {
      stopStreaming: vi.fn(),
      sendAgentUserContent: vi.fn(),
      sendTeamMessage: vi.fn(),
    } as unknown as ComposerActionDeps['sender'],
    runStatus: ref('idle') as Ref<string>,
    selectedSessionId: ref('sid-1') as Ref<string | undefined>,
    notify: vi.fn(),
    t: vi.fn((key: string, arg?: string | Record<string, unknown>) =>
      typeof arg === 'object' && arg !== null ? `请加载并使用 skill: ${String(arg.slug)}` : (arg ?? key),
    ),
    sessionDrafts: new Map(),
    inputText: ref('') as Ref<string>,
    selectedSkillSlugs: ref([]) as Ref<string[]>,
    ...overrides,
  } as ComposerActionDeps;
}

describe('useChatComposerActions', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe('regenerateMessage', () => {
    it('resends the selected user message directly', () => {
      const messages: Message[] = [
        makeMessage({ id: 'u1', role: 'user', content_markdown: 'hello' }),
        makeMessage({ id: 'a1', role: 'assistant', content_markdown: 'hi' }),
      ];
      const deps = makeDeps({
        messageStore: {
          getMessages: vi.fn().mockReturnValue(messages),
          setMessages: vi.fn(),
        } as unknown as ComposerActionDeps['messageStore'],
      });
      const { regenerateMessage } = useChatComposerActions(deps);

      regenerateMessage({ id: 'u1', role: 'user', content_markdown: 'hello' });

      expect(deps.sender.sendAgentUserContent).toHaveBeenCalledWith('hello');
      expect(deps.sender.sendTeamMessage).not.toHaveBeenCalled();
    });

    it('for an assistant message, resends the user message in the same turn', () => {
      const messages: Message[] = [
        makeMessage({ id: 'u1', role: 'user', content_markdown: 'hello' }),
        makeMessage({ id: 'a1', role: 'assistant', content_markdown: 'hi' }),
      ];
      const deps = makeDeps({
        messageStore: {
          getMessages: vi.fn().mockReturnValue(messages),
          setMessages: vi.fn(),
        } as unknown as ComposerActionDeps['messageStore'],
      });
      const { regenerateMessage } = useChatComposerActions(deps);

      regenerateMessage({ id: 'a1', role: 'assistant', content_markdown: 'hi' });

      expect(deps.sender.sendAgentUserContent).toHaveBeenCalledWith('hello');
    });

    it('resends the first user message when it is the only message', () => {
      const messages: Message[] = [makeMessage({ id: 'u1', role: 'user', content_markdown: 'first question' })];
      const deps = makeDeps({
        messageStore: {
          getMessages: vi.fn().mockReturnValue(messages),
          setMessages: vi.fn(),
        } as unknown as ComposerActionDeps['messageStore'],
      });
      const { regenerateMessage } = useChatComposerActions(deps);

      regenerateMessage({ id: 'u1', role: 'user', content_markdown: 'first question' });

      expect(deps.sender.sendAgentUserContent).toHaveBeenCalledWith('first question');
    });

    it('uses sendTeamMessage in team mode', () => {
      const messages: Message[] = [
        makeMessage({ id: 'u1', role: 'user', content_markdown: 'team task' }),
        makeMessage({ id: 'a1', role: 'assistant', content_markdown: 'ok' }),
      ];
      const deps = makeDeps({
        sessionStore: { entityKind: 'team' } as unknown as ComposerActionDeps['sessionStore'],
        messageStore: {
          getMessages: vi.fn().mockReturnValue(messages),
          setMessages: vi.fn(),
        } as unknown as ComposerActionDeps['messageStore'],
      });
      const { regenerateMessage } = useChatComposerActions(deps);

      regenerateMessage({ id: 'a1', role: 'assistant', content_markdown: 'ok' });

      expect(deps.sender.sendTeamMessage).toHaveBeenCalledWith('team task');
      expect(deps.sender.sendAgentUserContent).not.toHaveBeenCalled();
    });

    it('cancels an active stream before regenerating', () => {
      const messages: Message[] = [makeMessage({ id: 'u1', role: 'user', content_markdown: 'hello' })];
      const deps = makeDeps({
        runStatus: ref('running') as Ref<string>,
        messageStore: {
          getMessages: vi.fn().mockReturnValue(messages),
          setMessages: vi.fn(),
        } as unknown as ComposerActionDeps['messageStore'],
      });
      const { regenerateMessage } = useChatComposerActions(deps);

      regenerateMessage({ id: 'u1', role: 'user', content_markdown: 'hello' });

      expect(deps.streamManager.cancelActiveStream).toHaveBeenCalled();
      expect(deps.sender.stopStreaming).toHaveBeenCalledWith('sid-1');
      expect(deps.sender.sendAgentUserContent).toHaveBeenCalledWith('hello');
    });
  });

  describe('onSend with selected skills', () => {
    function makeSendDeps(opts: { inputText?: string; selected?: string[]; acceptSend?: boolean }) {
      const inputText = ref(opts.inputText ?? '');
      const selectedSkillSlugs = ref(opts.selected ?? []);
      const onSend = vi.fn(async () => {
        // 模拟 sender.onSend 主路径：发送被接受时清空输入框
        if (opts.acceptSend !== false) inputText.value = '';
      });
      const deps = makeDeps({
        inputText: inputText as Ref<string>,
        selectedSkillSlugs: selectedSkillSlugs as Ref<string[]>,
        sender: {
          stopStreaming: vi.fn(),
          sendAgentUserContent: vi.fn(),
          sendTeamMessage: vi.fn(),
          onSend,
        } as unknown as ComposerActionDeps['sender'],
      });
      return { deps, inputText, selectedSkillSlugs, onSend };
    }

    it('无选中技能：inputText 原样发送，不拼接提示', async () => {
      const { deps, inputText, onSend } = makeSendDeps({ inputText: '查一下告警' });
      const { onSend: send } = useChatComposerActions(deps);

      await send();

      expect(onSend).toHaveBeenCalled();
      // sender.onSend 直接消费 inputText，内容不应被改写
      expect(inputText.value).toBe('');
    });

    it('有选中技能：发送前把加载提示拼到 inputText 尾部', async () => {
      const inputText = ref('处理这条告警');
      const selectedSkillSlugs = ref(['alert-diagnosis', 'cms-alert']);
      let seenBySender = '';
      const onSend = vi.fn(async () => {
        seenBySender = inputText.value;
        inputText.value = '';
      });
      const deps = makeDeps({
        inputText: inputText as Ref<string>,
        selectedSkillSlugs: selectedSkillSlugs as Ref<string[]>,
        sender: {
          stopStreaming: vi.fn(),
          sendAgentUserContent: vi.fn(),
          sendTeamMessage: vi.fn(),
          onSend,
        } as unknown as ComposerActionDeps['sender'],
      });
      const { onSend: send } = useChatComposerActions(deps);

      await send();

      expect(seenBySender).toBe('处理这条告警\n请加载并使用 skill: alert-diagnosis, cms-alert');
    });

    it('仅选技能无文本：内容为纯提示语也能发送', async () => {
      const inputText = ref('');
      const selectedSkillSlugs = ref(['alert-diagnosis']);
      let seenBySender = '';
      const onSend = vi.fn(async () => {
        seenBySender = inputText.value;
        inputText.value = '';
      });
      const deps = makeDeps({
        inputText: inputText as Ref<string>,
        selectedSkillSlugs: selectedSkillSlugs as Ref<string[]>,
        sender: {
          stopStreaming: vi.fn(),
          sendAgentUserContent: vi.fn(),
          sendTeamMessage: vi.fn(),
          onSend,
        } as unknown as ComposerActionDeps['sender'],
      });
      const { onSend: send } = useChatComposerActions(deps);

      await send();

      expect(seenBySender).toBe('请加载并使用 skill: alert-diagnosis');
    });

    it('发送被接受（输入框清空）后清空选中技能', async () => {
      const { deps, selectedSkillSlugs } = makeSendDeps({ inputText: 'hi', selected: ['alert-diagnosis'] });
      const { onSend: send } = useChatComposerActions(deps);

      await send();

      expect(selectedSkillSlugs.value).toEqual([]);
    });

    it('发送未被接受（输入框未清）保留选中技能', async () => {
      const { deps, inputText, selectedSkillSlugs } = makeSendDeps({
        inputText: 'hi',
        selected: ['alert-diagnosis'],
        acceptSend: false,
      });
      const { onSend: send } = useChatComposerActions(deps);

      await send();

      expect(selectedSkillSlugs.value).toEqual(['alert-diagnosis']);
      // 注入文本保留在输入框中，用户可编辑后重发
      expect(inputText.value).toContain('请加载并使用 skill: alert-diagnosis');
    });

    it('inputText 已含相同提示时不重复拼接', async () => {
      const { deps, inputText } = makeSendDeps({
        inputText: 'hi\n请加载并使用 skill: alert-diagnosis',
        selected: ['alert-diagnosis'],
        acceptSend: false,
      });
      const { onSend: send } = useChatComposerActions(deps);

      await send();

      const occurrences = inputText.value.split('请加载并使用 skill: alert-diagnosis').length - 1;
      expect(occurrences).toBe(1);
    });
  });
});
