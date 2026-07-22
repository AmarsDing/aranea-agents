// web/src/features/chat/composables/__tests__/useChatMessageScroll.spec.ts
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { ref, nextTick } from 'vue';
import { useChatMessageScroll } from '../useChatMessageScroll';

function fakeEl(overrides: Record<string, unknown> = {}): HTMLElement {
  return {
    scrollTop: 0,
    scrollHeight: 1000,
    clientHeight: 400,
    contains: () => false,
    querySelector: () => null,
    ...overrides,
  } as unknown as HTMLElement;
}

function setup(elOverrides: Record<string, unknown> = {}) {
  const el = fakeEl(elOverrides);
  const messagesScrollEl = ref<HTMLElement | null>(el);
  const sessionKey = ref('s1');
  const messages = ref<any[]>([]);
  const contentSignature = ref('a');
  const lastTaskId = ref('t1');
  const api = useChatMessageScroll({ sessionKey, messages, messagesScrollEl, contentSignature, lastTaskId });
  return { el, sessionKey, messages, contentSignature, lastTaskId, ...api };
}

describe('useChatMessageScroll（状态制重构）', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => setTimeout(() => cb(0), 0));
    vi.stubGlobal('cancelAnimationFrame', (id: number) => clearTimeout(id));
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('不再返回 showScrollBtn；保留 turn 高亮/定位 API', () => {
    const api = setup();
    expect('showScrollBtn' in api).toBe(false);
    expect(typeof api.scrollToBottom).toBe('function');
    expect(typeof api.scrollToTurnId).toBe('function');
    expect(api.highlightedTurnId.value).toBeUndefined();
  });

  it('末端签名变化（跟随中）→ 节流滚底', async () => {
    const { el, contentSignature } = setup({ scrollTop: 600 });
    contentSignature.value = 'b';
    await nextTick();
    await vi.advanceTimersByTimeAsync(60);
    expect(el.scrollTop).toBe(600);
  });

  it('用户滚离后签名变化不滚动（无定时器强拽）', async () => {
    const { el, contentSignature, onMessagesScroll } = setup();
    el.scrollTop = 100;
    onMessagesScroll();
    contentSignature.value = 'b';
    await nextTick();
    await vi.advanceTimersByTimeAsync(10_000);
    expect(el.scrollTop).toBe(100); // 10s 后也不会被拽回
  });

  it('新 Task 出现 → scrollIntoView 锚定到 [data-task-id] 元素顶部', async () => {
    const anchor = { scrollIntoView: vi.fn() } as unknown as HTMLElement;
    const { lastTaskId } = setup({ querySelector: (sel: string) => (sel.includes('t2') ? anchor : null) });
    lastTaskId.value = 't2';
    await nextTick();
    await nextTick();
    expect(anchor.scrollIntoView).toHaveBeenCalledWith({ block: 'start' });
  });

  it('初始加载（lastTaskId 空→有值）不锚定——滚底生效（2026-07-22 会话切换 bug 修复）', async () => {
    const anchor = { scrollIntoView: vi.fn() } as unknown as HTMLElement;
    const el = fakeEl({ querySelector: (sel: string) => (sel.includes('t1') ? anchor : null) });
    const messagesScrollEl = ref<HTMLElement | null>(el);
    const sessionKey = ref('s1');
    const lastTaskId = ref(''); // 数据未加载
    useChatMessageScroll({ sessionKey, messages: ref([]), messagesScrollEl, contentSignature: ref('a'), lastTaskId });
    lastTaskId.value = 't1'; // 数据加载完成（初始加载，非用户发消息）
    await nextTick();
    await nextTick();
    expect(anchor.scrollIntoView).not.toHaveBeenCalled();
  });

  it('会话切换后首次 task 变化不锚定；同会话新增 Task 锚定', async () => {
    const anchorT2 = { scrollIntoView: vi.fn() } as unknown as HTMLElement;
    const anchorT3 = { scrollIntoView: vi.fn() } as unknown as HTMLElement;
    const el = fakeEl({
      querySelector: (sel: string) => {
        if (sel.includes('t2')) return anchorT2;
        if (sel.includes('t3')) return anchorT3;
        return null;
      },
    });
    const messagesScrollEl = ref<HTMLElement | null>(el);
    const sessionKey = ref('s1');
    const lastTaskId = ref('t1'); // 会话 s1 的最后 task
    useChatMessageScroll({ sessionKey, messages: ref([]), messagesScrollEl, contentSignature: ref('a'), lastTaskId });
    // 切换会话：sessionKey 先变，随后 lastTaskId 变为 s2 的最后 task
    sessionKey.value = 's2';
    await nextTick();
    lastTaskId.value = 't2';
    await nextTick();
    await nextTick();
    expect(anchorT2.scrollIntoView).not.toHaveBeenCalled(); // 会话切换不锚定（alignMessageScroll 滚底）
    // 同会话新增 Task（用户发消息）→ 锚定
    lastTaskId.value = 't3';
    await nextTick();
    await nextTick();
    expect(anchorT3.scrollIntoView).toHaveBeenCalledWith({ block: 'start' });
  });

  it('scrollToTurnId 定位并高亮 2s', async () => {
    const target = { scrollIntoView: vi.fn() } as unknown as HTMLElement;
    const { highlightedTurnId, scrollToTurnId } = setup({
      querySelector: () => target,
    });
    await scrollToTurnId('turn-9');
    expect(target.scrollIntoView).toHaveBeenCalled();
    expect(highlightedTurnId.value).toBe('turn-9');
    await vi.advanceTimersByTimeAsync(2100);
    expect(highlightedTurnId.value).toBeUndefined();
  });
});
