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
    t: vi.fn((key: string, fallback?: string) => fallback ?? key),
    sessionDrafts: new Map(),
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
      const messages: Message[] = [
        makeMessage({ id: 'u1', role: 'user', content_markdown: 'first question' }),
      ];
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
      const messages: Message[] = [
        makeMessage({ id: 'u1', role: 'user', content_markdown: 'hello' }),
      ];
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
});
