import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { computed, effectScope, reactive, ref } from 'vue';
import { flushPromises } from '@vue/test-utils';
import { useChatSender, type SenderDeps } from '../useChatSender';
import { useChatDrafts } from '../useChatDrafts';
import type { ChatAttachment } from '../../../../components/chat/types';

const mocks = vi.hoisted(() => ({
  notify: vi.fn(),
  enqueue: vi.fn(),
  stop: vi.fn(),
  upsertTask: vi.fn(),
  removeTask: vi.fn(),
  fetchSessionHistory: vi.fn(async () => undefined),
  adoptOptimisticTask: vi.fn(),
  sendCommand: vi.fn(),
  health: vi.fn(),
  alive: { value: true },
}));
vi.mock('quasar', () => ({ useQuasar: () => ({ notify: mocks.notify }) }));
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }));
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }));
vi.mock('../../../../stores/chat/runtimeStore', () => ({ useChatRuntimeStore: () => mocks }));
vi.mock('../../../../stores/chat/activityV2Store', () => ({ useChatActivityStore: () => mocks }));
vi.mock('../../../../stores/auth', () => ({ useAuthStore: () => ({ user: null, sessionChecked: true }) }));
vi.mock('../../../heartbeat/useServerHeartbeat', () => ({
  getServerHeartbeatState: () => ({ isAlive: mocks.alive }),
  checkBackendHealth: mocks.health,
}));
vi.mock('../../../../realtime/command_channel', () => ({ sendCommand: mocks.sendCommand }));
vi.mock('../../modelCapabilities', () => ({ shouldBlockAttachmentsForModel: () => null }));
vi.mock('../../api', () => ({ isChatQueueFullError: () => false }));

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}
const cleanups: Array<() => void> = [];
afterEach(() => {
  cleanups.splice(0).forEach((stop) => stop());
});
beforeEach(() => {
  vi.clearAllMocks();
  mocks.alive.value = true;
  mocks.health.mockResolvedValue(true);
  mocks.enqueue.mockResolvedValue({ accepted: true, queued: true });
  mocks.sendCommand.mockResolvedValue({});
});

function setup(status = 'idle') {
  const scope = effectScope();
  const session = reactive({
    entityKind: 'agent',
    selectedSession: { id: 'A', title: 'A' },
    teamSelectedSessionId: null,
    selectedTeamId: null,
    renameSessionByKind: vi.fn(),
  });
  const input = ref('');
  const files = ref<ChatAttachment[]>([{ id: 'file-A', name: 'A.png', mime_type: 'image/png', progress: 1 }]);
  const mode = ref('mode-A');
  const model = ref({ provider: 'provider-A', model: 'model-A' });
  const runStatus = ref(status);
  const drafts = scope.run(() =>
    useChatDrafts(
      input,
      () => 'agent',
      () => session.selectedSession?.id,
    ),
  )!;
  input.value = 'send A';
  const deps = {
    appStore: { selectedAgent: { id: 'agent', agent_key: 'agent-key' } },
    sessionStore: session,
    messageStore: { getMessages: () => [], setMessages: vi.fn() },
    inputText: input,
    dialogMode: mode,
    attachments: files,
    runStatus,
    isAwaitingUser: ref(false),
    awaitingRunId: ref(''),
    awaitKind: ref(''),
    selectedProviderModel: computed(() => model.value),
    ensureChatStream: vi.fn(),
    ensureTeamStream: vi.fn(),
    sendChatViaWs: vi.fn(),
    onNewSession: vi.fn(),
    makeSessionTitle: (s: string) => s,
    refreshRunStatus: vi.fn(),
    reconcileRunStatus: vi.fn(),
    setRunStatus: (s: string) => {
      runStatus.value = s;
    },
    submitAwaitingReply: vi.fn(),
    submitToolConfirm: vi.fn(),
    captureDraft: drafts.capture,
  } as unknown as SenderDeps;
  const sender = scope.run(() => useChatSender(deps))!;
  cleanups.push(() => {
    sender.markSendingDone();
    scope.stop();
  });
  return { deps, sender, input, files, session, mode, model, drafts };
}

describe('send snapshots and receipts', () => {
  it('uses the returned new session ID and preserves edits made while creating it', async () => {
    const c = setup();
    c.deps.sessionStore.selectedSession = null;
    c.input.value = 'unsaved instruction';
    c.files.value = [];
    c.deps.onNewSession = () =>
      c.drafts.createSession(async () => {
        c.session.selectedSession = { id: 'created', title: 'created' };
        c.input.value = 'new draft';
        return 'created';
      });
    await c.sender.onSend();
    expect(c.deps.sendChatViaWs).toHaveBeenCalledWith(
      undefined,
      expect.objectContaining({
        payload: expect.objectContaining({ session_id: 'created', content: 'unsaved instruction' }),
      }),
    );
    expect(c.input.value).toBe('new draft');
  });

  it('does not send a newly created session instruction into a separately selected session', async () => {
    const c = setup();
    c.deps.sessionStore.selectedSession = null;
    c.input.value = 'unsaved instruction';
    c.files.value = [];
    c.deps.onNewSession = () =>
      c.drafts.createSession(async () => {
        c.session.selectedSession = { id: 'B', title: 'B' };
        c.input.value = 'draft B';
        return 'created-A';
      });
    await c.sender.onSend();
    expect(c.deps.sendChatViaWs).not.toHaveBeenCalled();
    expect(c.input.value).toBe('draft B');
    c.session.selectedSession = { id: 'created-A', title: 'created-A' };
    expect(c.input.value).toBe('unsaved instruction');
  });

  it('freezes model, mode and files before asynchronous preflight, preserving later edits', async () => {
    const health = deferred<boolean>();
    mocks.alive.value = false;
    mocks.health.mockReturnValueOnce(health.promise);
    const c = setup();
    const pending = c.sender.onSend();
    await flushPromises();
    c.input.value = 'next draft';
    c.mode.value = 'mode-B';
    c.model.value = { provider: 'provider-B', model: 'model-B' };
    c.files.value.push({ id: 'new-file', name: 'new.png', progress: 1 });
    health.resolve(true);
    await pending;
    expect(c.deps.sendChatViaWs).toHaveBeenCalledWith(
      undefined,
      expect.objectContaining({
        payload: expect.objectContaining({
          session_id: 'A',
          content: 'send A',
          options: {
            dialog_mode: 'mode-A',
            provider: 'provider-A',
            model: 'model-A',
            attachments: [{ id: 'file-A' }],
          },
        }),
      }),
    );
    expect(c.input.value).toBe('next draft');
    expect(c.files.value.map((file) => file.id)).toEqual(['new-file']);
  });

  it('does not dispatch into B when selection changes during preflight', async () => {
    const health = deferred<boolean>();
    mocks.alive.value = false;
    mocks.health.mockReturnValueOnce(health.promise);
    const c = setup();
    const pending = c.sender.onSend();
    await flushPromises();
    c.session.selectedSession = { id: 'B', title: 'B' };
    c.input.value = 'draft B';
    health.resolve(true);
    await pending;
    expect(c.deps.sendChatViaWs).not.toHaveBeenCalled();
    expect(mocks.sendCommand).not.toHaveBeenCalled();
    expect(c.input.value).toBe('draft B');
    c.session.selectedSession = { id: 'A', title: 'A' };
    expect(c.input.value).toBe('send A');
  });

  it('late HTTP success consumes the original draft and files, leaving B untouched', async () => {
    const ack = deferred<object>();
    mocks.sendCommand.mockReturnValueOnce(ack.promise);
    const c = setup();
    vi.mocked(c.deps.sendChatViaWs).mockImplementation(() => {
      throw new Error('WS unavailable');
    });
    const originalFiles = c.files.value;
    const pending = c.sender.onSend();
    await flushPromises();
    c.session.selectedSession = { id: 'B', title: 'B' };
    c.input.value = 'draft B';
    c.files.value = [{ id: 'file-B', name: 'B.png', progress: 1 }];
    ack.resolve({});
    await pending;
    expect(mocks.sendCommand).toHaveBeenCalledWith(
      expect.objectContaining({
        session_id: 'A',
        options: expect.objectContaining({ attachments: [{ id: 'file-A' }] }),
      }),
    );
    expect(c.input.value).toBe('draft B');
    expect(c.files.value[0]?.id).toBe('file-B');
    expect(originalFiles).toHaveLength(0);
    c.session.selectedSession = { id: 'A', title: 'A' };
    expect(c.input.value).toBe('');
  });

  it('keeps the draft and attachments if dispatch fails', async () => {
    const c = setup();
    vi.mocked(c.deps.sendChatViaWs).mockImplementation(() => {
      throw new Error('WS unavailable');
    });
    mocks.sendCommand.mockRejectedValueOnce(new Error('HTTP unavailable'));
    await c.sender.onSend();
    expect(c.input.value).toBe('send A');
    expect(c.files.value[0]?.id).toBe('file-A');
  });

  it('does not clear a newer follow-up draft when an older queue receipt arrives', async () => {
    const ack = deferred<{ accepted: boolean; queued: boolean }>();
    mocks.enqueue.mockReturnValueOnce(ack.promise);
    const c = setup('running');
    c.files.value = [];
    const pending = c.sender.onSend();
    await flushPromises();
    c.input.value = 'next follow-up';
    ack.resolve({ accepted: true, queued: true });
    await pending;
    expect(c.input.value).toBe('next follow-up');
    expect(mocks.enqueue).toHaveBeenCalledTimes(1);
  });

  it('does not turn a late run-ended response into a new send to another session', async () => {
    const ack = deferred<{ accepted: boolean; queued: boolean }>();
    mocks.enqueue.mockReturnValueOnce(ack.promise);
    const c = setup('running');
    c.files.value = [];
    const pending = c.sender.onSend();
    await flushPromises();
    c.session.selectedSession = { id: 'B', title: 'B' };
    c.input.value = 'draft B';
    ack.reject(new Error('CHAT_RUN_ENDED'));
    await pending;
    expect(c.deps.sendChatViaWs).not.toHaveBeenCalled();
    expect(mocks.sendCommand).not.toHaveBeenCalled();
    expect(c.input.value).toBe('draft B');
  });

  it('accepts only one submission while preflight is pending', async () => {
    const health = deferred<boolean>();
    mocks.alive.value = false;
    mocks.health.mockReturnValueOnce(health.promise);
    const c = setup();
    const first = c.sender.onSend();
    const second = c.sender.onSend();
    await flushPromises();
    health.resolve(true);
    await Promise.all([first, second]);
    expect(c.deps.sendChatViaWs).toHaveBeenCalledTimes(1);
    expect(c.sender.submitting.value).toBe(false);
  });

  it('allows B to submit while an HTTP receipt for A is still pending', async () => {
    const ack = deferred<object>();
    mocks.sendCommand.mockReturnValueOnce(ack.promise);
    const c = setup();
    vi.mocked(c.deps.sendChatViaWs).mockImplementationOnce(() => {
      throw new Error('WS unavailable');
    });
    const pendingA = c.sender.onSend();
    await flushPromises();
    c.session.selectedSession = { id: 'B', title: 'B' };
    c.files.value = [];
    c.input.value = 'send B';
    expect(c.sender.submitting.value).toBe(false);
    await c.sender.onSend();
    expect(c.deps.sendChatViaWs).toHaveBeenLastCalledWith(
      undefined,
      expect.objectContaining({
        payload: expect.objectContaining({ session_id: 'B', content: 'send B' }),
      }),
    );
    c.input.value = 'new B draft';
    ack.resolve({});
    await pendingA;
    expect(c.input.value).toBe('new B draft');
  });
});
