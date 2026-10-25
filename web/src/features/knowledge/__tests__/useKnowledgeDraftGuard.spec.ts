import { afterEach, describe, expect, it, vi } from 'vitest';
import { createApp, ref, type App } from 'vue';
import type { KnowledgeWorkbench } from '../useKnowledgeWorkbench';

const control = vi.hoisted(() => ({
  guard: undefined as undefined | (() => Promise<boolean>),
  ok: undefined as undefined | ((choice: string) => Promise<void>),
  cancel: undefined as undefined | (() => void),
  notify: vi.fn(),
}));
vi.mock('vue-router', () => ({
  onBeforeRouteLeave: (guard: () => Promise<boolean>) => {
    control.guard = guard;
  },
}));
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }));
vi.mock('quasar', () => ({
  useQuasar: () => ({
    notify: control.notify,
    dialog: () => {
      const dialog = {
        onOk(fn: (choice: string) => Promise<void>) {
          control.ok = fn;
          return dialog;
        },
        onCancel(fn: () => void) {
          control.cancel = fn;
          return dialog;
        },
      };
      return dialog;
    },
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
    await control.ok!('save');
    expect(await pending).toBe(false);
  });
  it('allows explicitly keeping drafts but refuses to leave when draft storage fails', async () => {
    const { workbench } = setup();
    let pending = control.guard!();
    await control.ok!('draft');
    expect(await pending).toBe(true);
    workbench.draftError.value = true;
    pending = control.guard!();
    await control.ok!('draft');
    expect(await pending).toBe(false);
  });
});
