import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { mount, type VueWrapper } from '@vue/test-utils';
import { defineComponent, nextTick, ref } from 'vue';
import { useChatMessageScroll } from '../useChatMessageScroll';
const wrappers: VueWrapper[] = [];
beforeEach(() => {
  vi.useFakeTimers();
  vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => setTimeout(() => cb(0), 0));
  vi.stubGlobal('cancelAnimationFrame', (id: number) => clearTimeout(id));
});
afterEach(() => {
  wrappers.splice(0).forEach((w) => w.unmount());
  vi.useRealTimers();
  vi.unstubAllGlobals();
});
function setup(top: number | null) {
  const el = {
    scrollTop: 600,
    scrollHeight: 1000,
    clientHeight: 400,
    contains: () => false,
    querySelector: () => null,
  } as unknown as HTMLElement;
  const session = ref('A'),
    signature = ref('initial'),
    position = ref(top);
  let api!: ReturnType<typeof useChatMessageScroll>;
  wrappers.push(
    mount(
      defineComponent({
        setup() {
          api = useChatMessageScroll({
            sessionKey: session,
            messages: ref([]),
            messagesScrollEl: ref(el),
            contentSignature: signature,
            lastTaskId: ref('task'),
            initialScrollTop: position,
          });
          return () => null;
        },
      }),
    ),
  );
  return {
    el,
    session,
    signature,
    position,
    get api() {
      return api;
    },
  };
}
describe('workspace reading position restoration', () => {
  it('restores zero as a real reading position and resumes following only on explicit return', async () => {
    const c = setup(0);
    await vi.advanceTimersByTimeAsync(15);
    expect(c.el.scrollTop).toBe(0);
    expect(c.api.following.value).toBe(false);
    c.signature.value = 'new tokens';
    await nextTick();
    await vi.advanceTimersByTimeAsync(100);
    expect(c.el.scrollTop).toBe(0);
    c.api.jumpToLatest();
    expect(c.el.scrollTop).toBe(600);
    expect(c.api.following.value).toBe(true);
  });
  it('keeps the newest session alignment when a prior alignment is awaiting a frame', async () => {
    const c = setup(100);
    c.session.value = 'B';
    c.position.value = 120;
    await nextTick();
    c.session.value = 'C';
    c.position.value = 340;
    await nextTick();
    await vi.advanceTimersByTimeAsync(15);
    expect(c.el.scrollTop).toBe(340);
    expect(c.api.following.value).toBe(false);
  });
  it('restores immediately in a viewport with smooth scrolling, without persisting an intermediate position', async () => {
    const c = setup(120);
    const style = { scrollBehavior: 'smooth' };
    let actualTop = 600;
    Object.defineProperty(c.el, 'style', { value: style, configurable: true });
    Object.defineProperty(c.el, 'scrollTop', {
      get: () => actualTop,
      set: (value: number) => {
        // A smooth scroll starts asynchronously; its intermediate offset is not
        // the restored reading position that the page may save on scroll.
        if (style.scrollBehavior !== 'smooth') actualTop = value;
      },
    });
    await vi.advanceTimersByTimeAsync(15);
    expect(c.el.scrollTop).toBe(120);
    expect(style.scrollBehavior).toBe('smooth');
    expect(c.api.following.value).toBe(false);
  });
});
