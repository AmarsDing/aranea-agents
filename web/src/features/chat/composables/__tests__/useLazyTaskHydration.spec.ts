// web/src/features/chat/composables/__tests__/useLazyTaskHydration.spec.ts
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { ref } from 'vue';
import { useLazyTaskHydration, HYDRATE_DWELL_MS } from '../useLazyTaskHydration';

type IOCallback = (entries: Array<Partial<IntersectionObserverEntry>>) => void;

let ioCallback: IOCallback | null = null;
const observedEls = new Set<Element>();

class MockIntersectionObserver {
  constructor(cb: IOCallback, _opts?: IntersectionObserverInit) {
    ioCallback = cb;
  }
  observe(el: Element) {
    observedEls.add(el);
  }
  unobserve(el: Element) {
    observedEls.delete(el);
  }
  disconnect() {
    observedEls.clear();
  }
}

function makeScrollEl(): HTMLElement {
  const root = document.createElement('div');
  for (const id of ['t-1', 't-2', 't-3']) {
    const card = document.createElement('div');
    card.className = 'task-card';
    card.dataset.taskId = id;
    root.appendChild(card);
  }
  return root;
}

function entry(taskId: string, isIntersecting: boolean): Partial<IntersectionObserverEntry> {
  const target = document.querySelector(`.task-card[data-task-id="${taskId}"]`) as Element;
  return { target, isIntersecting };
}

describe('useLazyTaskHydration', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    observedEls.clear();
    ioCallback = null;
    vi.stubGlobal('IntersectionObserver', MockIntersectionObserver);
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    document.body.innerHTML = '';
  });

  it('syncCards observes only cards that need hydration', () => {
    const scrollEl = ref(makeScrollEl());
    document.body.appendChild(scrollEl.value!);
    const needsHydration = (id: string) => id !== 't-2'; // t-2 已水合
    const lazy = useLazyTaskHydration({ scrollEl, needsHydration, hydrate: vi.fn() });
    lazy.syncCards();
    expect([...observedEls].map((el) => (el as HTMLElement).dataset.taskId).sort()).toEqual(['t-1', 't-3']);
  });

  it('fires hydrate after 500ms dwell inside viewport', () => {
    const scrollEl = ref(makeScrollEl());
    document.body.appendChild(scrollEl.value!);
    const hydrate = vi.fn().mockResolvedValue(undefined);
    const lazy = useLazyTaskHydration({ scrollEl, needsHydration: () => true, hydrate });
    lazy.syncCards();

    ioCallback!([entry('t-1', true)]);
    expect(hydrate).not.toHaveBeenCalled();
    vi.advanceTimersByTime(HYDRATE_DWELL_MS - 1);
    expect(hydrate).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1);
    expect(hydrate).toHaveBeenCalledWith('t-1');
  });

  it('cancels dwell when the card leaves the viewport (fast scroll-by)', () => {
    const scrollEl = ref(makeScrollEl());
    document.body.appendChild(scrollEl.value!);
    const hydrate = vi.fn().mockResolvedValue(undefined);
    const lazy = useLazyTaskHydration({ scrollEl, needsHydration: () => true, hydrate });
    lazy.syncCards();

    ioCallback!([entry('t-1', true)]);
    vi.advanceTimersByTime(200);
    ioCallback!([entry('t-1', false)]);
    vi.advanceTimersByTime(HYDRATE_DWELL_MS);
    expect(hydrate).not.toHaveBeenCalled();
  });

  it('expandTask hydrates immediately without dwell', () => {
    const scrollEl = ref(makeScrollEl());
    document.body.appendChild(scrollEl.value!);
    const hydrate = vi.fn().mockResolvedValue(undefined);
    const lazy = useLazyTaskHydration({ scrollEl, needsHydration: () => true, hydrate });
    lazy.expandTask('t-3');
    expect(hydrate).toHaveBeenCalledWith('t-3');
  });

  it('compensates scrollTop when a card above the viewport expands', async () => {
    const scrollEl = ref(makeScrollEl());
    document.body.appendChild(scrollEl.value!);
    const el = scrollEl.value!;
    // 视口 top=0；t-1 卡片 top=-300（在视口上方）
    vi.spyOn(el, 'getBoundingClientRect').mockReturnValue({ top: 0 } as DOMRect);
    const card = el.querySelector<HTMLElement>('.task-card[data-task-id="t-1"]')!;
    vi.spyOn(card, 'getBoundingClientRect').mockReturnValue({ top: -300 } as DOMRect);
    let scrollHeight = 1000;
    Object.defineProperty(el, 'scrollHeight', { get: () => scrollHeight, configurable: true });
    el.scrollTop = 400;

    const hydrate = vi.fn().mockImplementation(async () => {
      scrollHeight = 1800; // 水合后 DOM 增高 800
    });
    const lazy = useLazyTaskHydration({ scrollEl, needsHydration: () => true, hydrate });
    lazy.expandTask('t-1');
    await vi.waitFor(() => expect(el.scrollTop).toBe(1200)); // 400 + 800
  });

  it('does not compensate scrollTop when the card is inside the viewport', async () => {
    const scrollEl = ref(makeScrollEl());
    document.body.appendChild(scrollEl.value!);
    const el = scrollEl.value!;
    vi.spyOn(el, 'getBoundingClientRect').mockReturnValue({ top: 0 } as DOMRect);
    const card = el.querySelector<HTMLElement>('.task-card[data-task-id="t-1"]')!;
    vi.spyOn(card, 'getBoundingClientRect').mockReturnValue({ top: 200 } as DOMRect);
    Object.defineProperty(el, 'scrollHeight', { get: () => 1000, configurable: true });
    el.scrollTop = 400;

    const hydrate = vi.fn().mockResolvedValue(undefined);
    const lazy = useLazyTaskHydration({ scrollEl, needsHydration: () => true, hydrate });
    lazy.expandTask('t-1');
    await Promise.resolve();
    expect(el.scrollTop).toBe(400);
  });

  it('toggleCollapse tracks manual collapse state', () => {
    const scrollEl = ref(makeScrollEl());
    const lazy = useLazyTaskHydration({ scrollEl, needsHydration: () => true, hydrate: vi.fn() });
    expect(lazy.isCollapsed('t-1')).toBe(false);
    lazy.toggleCollapse('t-1');
    expect(lazy.isCollapsed('t-1')).toBe(true);
    lazy.toggleCollapse('t-1');
    expect(lazy.isCollapsed('t-1')).toBe(false);
  });
});
