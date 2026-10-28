import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { effectScope, reactive, ref } from 'vue';
import { flushPromises } from '@vue/test-utils';
import type { LocationQuery } from 'vue-router';
import { useTaskWorkspaceLocation } from '../useTaskWorkspaceLocation';

const fixture = vi.hoisted(() => ({
  tasks: [
    { ID: 'T', SessionID: 'S' },
    { ID: 'T2', SessionID: 'S' },
  ],
  steps: new Map<string, { ID: string; SessionID: string; TaskID: string }>(),
  taskHydration: new Map<string, string>(),
  hydrate: vi.fn(),
  openPreview: vi.fn(),
  notify: vi.fn(),
  resolve: vi.fn(),
}));
const route = reactive({ path: '/task-workspace', fullPath: '', query: {} as LocationQuery });
vi.mock('vue-router', () => ({ useRoute: () => route, useRouter: () => ({ resolve: fixture.resolve }) }));
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }));
vi.mock('quasar', () => ({ useQuasar: () => ({ notify: fixture.notify }) }));
vi.mock('../../../../stores/chat/activityV2Store', () => ({
  useChatActivityStore: () => ({
    getSessionTasks: (sid: string) => fixture.tasks.filter((t) => t.SessionID === sid),
    steps: fixture.steps,
    taskHydration: fixture.taskHydration,
    hydrateTask: fixture.hydrate,
  }),
}));
vi.mock('../../../../stores/artifact', () => ({
  useArtifactStore: () => ({ openArtifactPreview: fixture.openPreview }),
}));
const scopes: ReturnType<typeof effectScope>[] = [];
function navigate(query: LocationQuery) {
  route.query = query;
  route.fullPath = `/task-workspace?${JSON.stringify(query)}`;
}
function setup() {
  const sid = ref('S'),
    ready = ref(true),
    bindingLoading = ref(false),
    bindingError = ref('');
  const locate = vi.fn().mockResolvedValue(undefined),
    showPanel = vi.fn();
  const scope = effectScope();
  scopes.push(scope);
  const api = scope.run(() =>
    useTaskWorkspaceLocation({
      enabled: () => true,
      ready: () => ready.value,
      sessionId: () => sid.value,
      bindingLoading: () => bindingLoading.value,
      bindingError: () => bindingError.value,
      locate,
      showPanel,
    }),
  )!;
  return { api, sid, ready, bindingLoading, bindingError, locate, showPanel };
}
beforeEach(() => {
  vi.clearAllMocks();
  fixture.steps.clear();
  fixture.taskHydration.clear();
  fixture.steps.set('step', { ID: 'step', TaskID: 'T', SessionID: 'S' });
  fixture.steps.set('step2', { ID: 'step2', TaskID: 'T2', SessionID: 'S' });
  fixture.hydrate.mockResolvedValue(undefined);
  fixture.resolve.mockReturnValue({ href: '/#/task-workspace?session=S&task=T&step=step' });
  navigate({ session: 'S', task: 'T', step: 'step' });
});
afterEach(() => {
  scopes.splice(0).forEach((scope) => scope.stop());
  vi.unstubAllGlobals();
});
describe('read-only workspace deep links', () => {
  it('waits for authoritative session binding before locating a hydrated original step', async () => {
    const c = setup();
    c.bindingLoading.value = true;
    await flushPromises();
    expect(c.locate).not.toHaveBeenCalled();
    c.bindingLoading.value = false;
    await flushPromises();
    expect(fixture.hydrate).toHaveBeenCalledWith('T');
    expect(c.locate).toHaveBeenCalledWith('T', 'step', expect.any(Function));
    expect(c.api.error.value).toBe('');
  });
  it('rejects a foreign step without changing the panel or opening any artifact', async () => {
    fixture.steps.set('step', { ID: 'step', TaskID: 'T2', SessionID: 'S' });
    navigate({ session: 'S', task: 'T', step: 'step', artifact: 'file' });
    const c = setup();
    await flushPromises();
    expect(c.api.error.value).toContain('locationScopeMismatch');
    expect(c.locate).not.toHaveBeenCalled();
    expect(fixture.openPreview).not.toHaveBeenCalled();
  });
  it('discards an old hydration result after the requested task changes', async () => {
    let resolve!: () => void;
    fixture.hydrate.mockReturnValueOnce(
      new Promise<void>((r) => {
        resolve = r;
      }),
    );
    const c = setup();
    await flushPromises();
    navigate({ session: 'S', task: 'T2', step: 'step2' });
    await flushPromises();
    resolve();
    await flushPromises();
    expect(c.locate).toHaveBeenCalledTimes(1);
    expect(c.locate.mock.calls[0]?.slice(0, 2)).toEqual(['T2', 'step2']);
  });
  it('keeps failed hydration visible and retries only when explicitly requested', async () => {
    fixture.taskHydration.set('T', 'error');
    const c = setup();
    await flushPromises();
    expect(c.api.error.value).toContain('locationLoadFailed');
    expect(c.locate).not.toHaveBeenCalled();
    fixture.taskHydration.delete('T');
    c.api.retry();
    await flushPromises();
    expect(c.api.error.value).toBe('');
    expect(c.locate).toHaveBeenCalledTimes(1);
  });
  it('opens the requested version zero as a preview and never sends or approves', async () => {
    navigate({ session: 'S', panel: 'artifacts', artifact: 'file', version: '0' });
    const c = setup();
    await flushPromises();
    expect(fixture.openPreview).toHaveBeenCalledWith('file', 0);
    expect(c.showPanel).toHaveBeenCalledWith('artifacts');
    expect(c.locate).not.toHaveBeenCalled();
  });
  it('reports invalid parameters and copies only scope-checked location IDs', async () => {
    navigate({ session: 'S', step: 'step' });
    const c = setup();
    await flushPromises();
    expect(c.api.error.value).toContain('invalidLocation');
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal('navigator', { clipboard: { writeText } });
    await c.api.copyTaskLink('T', 'step');
    expect(fixture.resolve).toHaveBeenCalledWith({
      name: 'task-workspace',
      query: { session: 'S', task: 'T', step: 'step', panel: 'chat' },
    });
    expect(writeText).toHaveBeenCalledTimes(1);
    await c.api.copyTaskLink('foreign');
    expect(writeText).toHaveBeenCalledTimes(1);
  });
});
