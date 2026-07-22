// web/src/features/chat/composables/__tests__/useFollowScroll.spec.ts
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { ref, nextTick } from 'vue';
import { useFollowScroll } from '../useFollowScroll';

/** 1000px 内容 / 400px 视口的假滚动容器。distanceFromBottom = 600 - scrollTop。 */
function fakeEl(overrides: Record<string, unknown> = {}): HTMLElement {
  return {
    scrollTop: 0,
    scrollHeight: 1000,
    clientHeight: 400,
    contains: () => false,
    ...overrides,
  } as unknown as HTMLElement;
}

function setup(elOverrides: Record<string, unknown> = {}) {
  const el = fakeEl(elOverrides);
  const scrollEl = ref<HTMLElement | null>(el);
  const contentSignature = ref('a');
  const enabled = ref(true);
  const api = useFollowScroll({ scrollEl, contentSignature, enabled });
  return { el, scrollEl, contentSignature, enabled, ...api };
}

describe('useFollowScroll', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => setTimeout(() => cb(0), 0));
    vi.stubGlobal('cancelAnimationFrame', (id: number) => clearTimeout(id));
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('初始为 FOLLOWING；签名变化时（节流后）滚底', async () => {
    const { el, contentSignature, following } = setup();
    expect(following.value).toBe(true);
    contentSignature.value = 'b';
    await nextTick();
    await vi.advanceTimersByTimeAsync(60);
    expect(el.scrollTop).toBe(600);
  });

  it('用户滚离底部 >80px → UNFOLLOWED，签名变化不再滚动', async () => {
    const { el, contentSignature, onScroll, following } = setup();
    el.scrollTop = 100; // dist = 500 > 80
    onScroll();
    expect(following.value).toBe(false);
    contentSignature.value = 'b';
    await nextTick();
    await vi.advanceTimersByTimeAsync(60);
    expect(el.scrollTop).toBe(100);
  });

  it('UNFOLLOWED 中滚回 ≤80px → 恢复 FOLLOWING，随后签名变化恢复滚底', async () => {
    const { el, contentSignature, onScroll, following } = setup();
    el.scrollTop = 100;
    onScroll();
    expect(following.value).toBe(false);
    el.scrollTop = 560; // dist = 40 ≤ 80
    onScroll();
    expect(following.value).toBe(true);
    contentSignature.value = 'b';
    await nextTick();
    await vi.advanceTimersByTimeAsync(60);
    expect(el.scrollTop).toBe(600);
  });

  it('程序滚底（scrollToBottom）产生的事件不切换状态', async () => {
    const { el, onScroll, scrollToBottom, following } = setup();
    el.scrollTop = 100;
    onScroll();
    expect(following.value).toBe(false);
    scrollToBottom();
    expect(el.scrollTop).toBe(600);
    onScroll(); // 消费 programmatic flag，不改 following
    expect(following.value).toBe(false); // 用户未主动滚回，仍 UNFOLLOWED
  });

  it('容器内非空选区 → 转 UNFOLLOWED 保护复制', async () => {
    const node = {} as Node;
    const { el, contentSignature, following } = setup({ contains: (n: Node) => n === node });
    vi.spyOn(window, 'getSelection').mockReturnValue({ isCollapsed: false, anchorNode: node } as Selection);
    contentSignature.value = 'b';
    await nextTick();
    expect(following.value).toBe(false);
    vi.restoreAllMocks();
  });

  it('容器外选区不影响跟随', async () => {
    const { el, contentSignature, following } = setup({ contains: () => false });
    vi.spyOn(window, 'getSelection').mockReturnValue({ isCollapsed: false, anchorNode: {} as Node } as Selection);
    contentSignature.value = 'b';
    await nextTick();
    await vi.advanceTimersByTimeAsync(60);
    expect(following.value).toBe(true);
    expect(el.scrollTop).toBe(600);
    vi.restoreAllMocks();
  });

  it('enabled false→true：滚底并进入 FOLLOWING；true→false：不动滚动条', async () => {
    const { el, enabled, contentSignature, onScroll, following } = setup();
    // 先进入 UNFOLLOWED 并关闭 enabled
    el.scrollTop = 100;
    onScroll();
    enabled.value = false;
    await nextTick();
    el.scrollTop = 50;
    contentSignature.value = 'b';
    await nextTick();
    await vi.advanceTimersByTimeAsync(60);
    expect(el.scrollTop).toBe(50); // disabled：不滚动
    // 重新启用 → 滚底 + FOLLOWING
    // 双 nextTick：watcher 回调体内的 nextTick(() => scrollToBottom()) 晚于首个 tick 挂入微任务队列
    // （与计划 Task 2「新 Task 出现」用例的双 tick 模式一致）
    enabled.value = true;
    await nextTick();
    await nextTick();
    expect(el.scrollTop).toBe(600);
    expect(following.value).toBe(true);
  });

  it('jumpToLatest() 滚底并恢复 FOLLOWING', async () => {
    const { el, onScroll, jumpToLatest, following } = setup();
    el.scrollTop = 100;
    onScroll();
    expect(following.value).toBe(false);
    jumpToLatest();
    expect(el.scrollTop).toBe(600);
    expect(following.value).toBe(true);
  });

  it('jumpToLatest(anchorEl) 调用 scrollIntoView 并恢复 FOLLOWING', async () => {
    const { onScroll, jumpToLatest, following } = setup();
    onScroll();
    // 仍处于 FOLLOWING（初始 true，dist=600? 不——初始 scrollTop=0 → dist=600>80）
    expect(following.value).toBe(false);
    const anchor = { scrollIntoView: vi.fn() } as unknown as HTMLElement;
    jumpToLatest(anchor);
    expect(anchor.scrollIntoView).toHaveBeenCalledWith({ block: 'start' });
    expect(following.value).toBe(true);
  });

  it('scrollEl 为 null 时全部 no-op', async () => {
    const scrollEl = ref<HTMLElement | null>(null);
    const contentSignature = ref('a');
    const { onScroll, jumpToLatest, scrollToBottom } = useFollowScroll({
      scrollEl,
      contentSignature,
      enabled: ref(true),
    });
    expect(() => {
      onScroll();
      jumpToLatest();
      scrollToBottom();
    }).not.toThrow();
    contentSignature.value = 'b';
    await nextTick();
    await vi.advanceTimersByTimeAsync(60);
  });
});
