import { computed, nextTick, ref, watch } from 'vue';
import { useArtifactStore } from '../../../stores/artifact';
import { useAuthStore } from '../../../stores/auth';
import {
  useChatWorkspaceViewStore,
  newWorkspaceView,
  type ChatWorkspaceView,
} from '../../../stores/chat/workspaceViewStore';
import { getBackendOrigin } from '../../../config/runtime';

/** Page-only view state. Task/Turn/Step facts continue to live in the v2 store. */
export function useTaskWorkspaceView(enabled: () => boolean, sessionId: () => string | undefined) {
  const artifactStore = useArtifactStore();
  const auth = useAuthStore();
  const cache = useChatWorkspaceViewStore();
  const current = ref<ChatWorkspaceView>(newWorkspaceView());
  let restoring = false;
  let releasing = false;
  const field = <K extends keyof ChatWorkspaceView>(key: K) =>
    computed({
      get: () => current.value[key],
      set: (value: ChatWorkspaceView[K]) => {
        current.value[key] = value;
      },
    });
  const pane = field('pane');
  const navigation = field('navigation');
  const contextOpen = field('contextOpen');
  const contextTab = field('contextTab');
  const readingExpanded = field('readingExpanded');
  const chatScrollTop = field('chatScrollTop');
  const processTaskId = field('processTaskId');
  const processTurnId = field('processTurnId');
  let previewTrigger: HTMLElement | null = null;
  const panelTarget = computed(() => {
    const target = artifactStore.previewTarget;
    return enabled() && target?.surface === 'panel' && target.sessionId === sessionId() ? target : null;
  });
  const contextScrollTop = computed(() =>
    panelTarget.value ? current.value.artifactScrollTop : current.value.listScrollTop,
  );
  watch(
    () => [enabled(), getBackendOrigin(), auth.user?.id, sessionId()] as const,
    ([active, origin, userId, sid], _old, onCleanup) => {
      current.value = active && sid ? cache.ensure(JSON.stringify([origin, userId ?? null, sid])) : newWorkspaceView();
      if (!active || !sid) return;
      restoring = true;
      const release = artifactStore.claimPreviewHost(sid);
      const target = current.value.preview;
      if (target) artifactStore.openArtifactPreview(target.id, target.version);
      restoring = false;
      onCleanup(() => {
        releasing = true;
        release();
        releasing = false;
      });
    },
    { immediate: true, flush: 'sync' },
  );
  watch(
    panelTarget,
    (target, previous) => {
      if (restoring || releasing) return;
      if (target) {
        if (previous?.id !== target.id || previous?.version !== target.version) current.value.artifactScrollTop = 0;
        current.value.preview = { id: target.id, version: target.version };
        if (!previous && document.activeElement instanceof HTMLElement) previewTrigger = document.activeElement;
        contextTab.value = 'artifacts';
        openArtifacts();
      } else current.value.preview = null;
    },
    { flush: 'sync' },
  );

  function openArtifacts() {
    contextOpen.value = true;
    pane.value = 'artifacts';
  }
  function closeArtifacts() {
    artifactStore.closeArtifactPreview();
    current.value.preview = null;
    contextOpen.value = false;
    pane.value = 'chat';
    const action = contextTab.value === 'activity' ? 'activity' : 'artifacts';
    void nextTick(() => {
      const trigger =
        previewTrigger?.isConnected && previewTrigger.getClientRects().length
          ? previewTrigger
          : document.querySelector<HTMLElement>(`[data-task-action="${action}"]`);
      trigger?.focus();
      previewTrigger = null;
    });
  }
  function showNavigation(tab: 'sessions' | 'members' | 'tree') {
    navigation.value = tab;
    pane.value = 'navigation';
  }
  function saveContextScroll(top: number) {
    const value = Number.isFinite(top) ? Math.max(0, top) : 0;
    if (panelTarget.value) current.value.artifactScrollTop = value;
    else current.value.listScrollTop = value;
  }
  return {
    pane,
    navigation,
    contextOpen,
    contextTab,
    readingExpanded,
    chatScrollTop,
    processTaskId,
    processTurnId,
    contextScrollTop,
    saveContextScroll,
    panelTarget,
    openArtifacts,
    closeArtifacts,
    showNavigation,
    previewVersions: computed(() => artifactStore.previewVersions),
    versionsLoading: computed(() => artifactStore.previewVersionsLoading),
    versionsError: computed(() => artifactStore.previewVersionsError),
    reloadVersions: artifactStore.loadPreviewVersions,
  };
}
