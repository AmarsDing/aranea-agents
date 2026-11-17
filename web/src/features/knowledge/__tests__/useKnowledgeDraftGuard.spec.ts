import { afterEach, describe, expect, it, vi } from 'vitest';
import { createApp, ref, type App } from 'vue';
import type { KnowledgeWorkbench } from '../useKnowledgeWorkbench';

const control = vi.hoisted(() => ({
  guard: undefined as undefined | (() => Promise<boolean>),
  choice: undefined as undefined | { resolve: (value: string | null) => void },
  notify: vi.fn(),
}));
vi.mock('vue-router', () => ({
  onBeforeRouteLeave: (guard: () => Promise<boolean>) => {
    control.guard = guard;
  },
}));
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }));
vi.mock('quasar', () => ({
  useQuasar: () => ({ notify: control.notify }),
}));
// 离开前分支选择已从 $q.dialog 迁到 useAppPrompt：这里只捕获「用户选了什么」
vi.mock('../../../composables/useAppPrompt', () => ({
  prompt: () =>
    new Promise<string | null>((resolve) => {
      control.choice = { resolve };
    }),
}));

import { useKnowledgeDraftGuard } from '../useKnowledgeDraftGuard';
let app: App | undefined;
afterEach(() => {
  app?.unmount();
  app = undefined;
  vi.clearAllMocks();
});
function setup() {
  const workbench = {
    hasUnsaved: ref(true),
    draftError: ref(false),
    draftsAvailable: true,
    tabs: ref([{ docId: 'd', dirty: true, saving: false }]),
    persistDrafts: vi.fn(),
    saveTab: vi.fn().mockResolvedValue(false),
  } as unknown as KnowledgeWorkbench;
  const flush = vi.fn();
  app = createApp({
    setup() {
      useKnowledgeDraftGuard(workbench, flush);
      return () => null;
    },
  });
  app.mount(document.createElement('div'));
  return { workbench, flush };
}

describe('knowledge unsaved changes guard', () => {
  it('flushes pending editor text and blocks browser close while unsaved', () => {
    const { workbench, flush } = setup();
    const event = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(event);
    expect(flush).toHaveBeenCalled();
    expect(workbench.persistDrafts).toHaveBeenCalled();
    expect(event.defaultPrevented).toBe(true);
  });
  it('keeps the route when saving conflicts or has newer edits', async () => {
    setup();
    const pending = control.guard!();
    control.choice!.resolve('save');
    expect(await pending).toBe(false);
  });
  it('allows explicitly keeping drafts but refuses to leave when draft storage fails', async () => {
    const { workbench } = setup();
    let pending = control.guard!();
    control.choice!.resolve('draft');
    expect(await pending).toBe(true);
    workbench.draftError.value = true;
    pending = control.guard!();
    control.choice!.resolve('draft');
    expect(await pending).toBe(false);
  });
  it('blocks navigation when the choice dialog is dismissed', async () => {
    setup();
    const pending = control.guard!();
    control.choice!.resolve(null);
    expect(await pending).toBe(false);
  });
});
