import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { effectScope, reactive, ref } from 'vue';
import { createPinia, setActivePinia } from 'pinia';
import { useTaskWorkspaceView } from '../useTaskWorkspaceView';

const auth = reactive({ user: { id: 'user' } });
const artifact = reactive({
  previewTarget: null as { id: string; version?: number; surface: string; sessionId: string } | null,
  previewVersions: [],
  previewVersionsLoading: false,
  previewVersionsError: '',
  host: '',
  claimPreviewHost(sid: string) {
    artifact.host = sid;
    return () => {
      artifact.previewTarget = null;
      artifact.host = '';
    };
  },
  openArtifactPreview(id: string, version?: number) {
    artifact.previewTarget = { id, version, surface: 'panel', sessionId: artifact.host };
  },
  closeArtifactPreview() {
    artifact.previewTarget = null;
  },
  loadPreviewVersions: vi.fn(),
});
vi.mock('../../../../stores/auth', () => ({ useAuthStore: () => auth }));
vi.mock('../../../../stores/artifact', () => ({ useArtifactStore: () => artifact }));
vi.mock('../../../../config/runtime', () => ({ getBackendOrigin: () => 'backend' }));
const scopes: ReturnType<typeof effectScope>[] = [];
function setup(sid = ref<string | undefined>('S')) {
  const scope = effectScope();
  scopes.push(scope);
  const api = scope.run(() =>
    useTaskWorkspaceView(
      () => true,
      () => sid.value,
    ),
  )!;
  return { scope, api, sid };
}
beforeEach(() => {
  setActivePinia(createPinia());
  auth.user = { id: 'user' };
  artifact.previewTarget = null;
  artifact.host = '';
});
afterEach(() => scopes.splice(0).forEach((scope) => scope.stop()));
describe('workspace view lifecycle', () => {
  it('restores preview ID/version and read positions after a route unmount', () => {
    const first = setup();
    expect(first.api.contextOpen.value).toBe(false);
    artifact.openArtifactPreview('file', 0);
    first.api.readingExpanded.value = true;
    first.api.chatScrollTop.value = 0;
    first.api.saveContextScroll(180);
    first.scope.stop();
    expect(artifact.previewTarget).toBeNull();
    const returned = setup();
    expect(returned.api.panelTarget.value).toMatchObject({ id: 'file', version: 0 });
    expect(returned.api.contextScrollTop.value).toBe(180);
    expect(returned.api.chatScrollTop.value).toBe(0);
    expect(returned.api.readingExpanded.value).toBe(true);
  });
  it('keeps sessions and identities separate, preserving the previous session target', () => {
    const c = setup();
    artifact.openArtifactPreview('A', 1);
    c.sid.value = 'B';
    expect(c.api.panelTarget.value).toBeNull();
    artifact.openArtifactPreview('B', 2);
    c.sid.value = 'S';
    expect(c.api.panelTarget.value?.id).toBe('A');
    auth.user = { id: 'other' };
    expect(c.api.panelTarget.value).toBeNull();
    expect(c.api.contextOpen.value).toBe(false);
  });
  it('respects a manual close and does not resurrect the old preview on return', () => {
    const c = setup();
    artifact.openArtifactPreview('file', 1);
    c.api.closeArtifacts();
    c.scope.stop();
    const returned = setup();
    expect(returned.api.panelTarget.value).toBeNull();
    expect(returned.api.contextOpen.value).toBe(false);
  });
});
