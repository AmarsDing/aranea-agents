import { nextTick, onScopeDispose, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import { useArtifactStore } from '../../../stores/artifact';
import {
  parseWorkspaceLocation,
  workspaceLocationQuery,
  workspaceRouteName,
  type WorkspacePanel,
} from '../taskWorkspaceNavigation';

interface LocationOptions {
  enabled: () => boolean;
  ready: () => boolean;
  sessionId: () => string | undefined;
  bindingLoading: () => boolean;
  bindingError: () => string;
  showPanel: (panel: WorkspacePanel) => void;
  locate: (taskId: string, stepId: string | undefined, current: () => boolean) => Promise<void>;
}

export function useTaskWorkspaceLocation(options: LocationOptions) {
  const route = useRoute();
  const router = useRouter();
  const { t } = useI18n();
  const q = useQuasar();
  const activity = useChatActivityStore();
  const artifacts = useArtifactStore();
  const error = ref('');
  const busy = ref(false);
  const retryNonce = ref(0);
  let generation = 0;
  let applied = '';
  watch(
    () =>
      [
        options.enabled(),
        options.ready(),
        options.sessionId(),
        options.bindingLoading(),
        options.bindingError(),
        route.fullPath,
        retryNonce.value,
      ] as const,
    async () => {
      const captured = ++generation;
      busy.value = false;
      error.value = '';
      if (!options.enabled()) return;
      const visibleSession = options.sessionId();
      const locationPath = route.fullPath;
      const current = () =>
        captured === generation &&
        options.enabled() &&
        options.ready() &&
        !options.bindingLoading() &&
        options.sessionId() === visibleSession &&
        route.fullPath === locationPath;
      let target;
      try {
        target = parseWorkspaceLocation(route.query);
      } catch {
        error.value = t('taskWorkspace.invalidLocation');
        return;
      }
      if (!target) return;
      if (options.bindingLoading() || !options.ready()) {
        busy.value = true;
        return;
      }
      if (options.bindingError()) {
        error.value = options.bindingError();
        return;
      }
      if (target.sessionId !== options.sessionId()) {
        error.value = t('taskWorkspace.locationUnavailable');
        return;
      }
      const signature = JSON.stringify([route.fullPath, target.sessionId, retryNonce.value]);
      if (signature === applied) return;
      busy.value = true;
      try {
        if (target.taskId) {
          const task = activity.getSessionTasks(target.sessionId).find((item) => item.ID === target.taskId);
          if (!task) throw new Error(t('taskWorkspace.locationUnavailable'));
          await activity.hydrateTask(task.ID);
          if (!current()) return;
          if (activity.taskHydration.get(task.ID) === 'error') throw new Error(t('taskWorkspace.locationLoadFailed'));
          if (target.stepId) {
            const step = activity.steps.get(target.stepId);
            if (!step || step.TaskID !== task.ID || step.SessionID !== target.sessionId)
              throw new Error(t('taskWorkspace.locationScopeMismatch'));
          }
        }
        if (!current()) return;
        if (target.artifactId) artifacts.openArtifactPreview(target.artifactId, target.version);
        if (target.panel) options.showPanel(target.panel);
        if (target.taskId) await options.locate(target.taskId, target.stepId, current);
        if (!current()) return;
        await nextTick();
        if (current()) applied = signature;
      } catch (cause) {
        if (current()) error.value = cause instanceof Error ? cause.message : t('taskWorkspace.locationUnavailable');
      } finally {
        if (current()) busy.value = false;
      }
    },
    { immediate: true, flush: 'post' },
  );
  onScopeDispose(() => {
    generation++;
  });

  async function copyTaskLink(taskId: string, stepId?: string) {
    const sid = options.sessionId();
    if (!sid || !activity.getSessionTasks(sid).some((task) => task.ID === taskId)) return;
    if (stepId) {
      const step = activity.steps.get(stepId);
      if (!step || step.TaskID !== taskId || step.SessionID !== sid) return;
    }
    const href = router.resolve({
      name: workspaceRouteName(route.path),
      query: workspaceLocationQuery({ sessionId: sid, taskId, stepId, panel: 'chat' }),
    }).href;
    try {
      await navigator.clipboard.writeText(new URL(href, window.location.href).href);
      q.notify({ type: 'positive', message: t('taskWorkspace.linkCopied') });
    } catch {
      q.notify({ type: 'warning', message: t('taskWorkspace.linkCopyFailed') });
    }
  }
  return {
    error,
    busy,
    retry: () => {
      retryNonce.value++;
    },
    copyTaskLink,
  };
}
