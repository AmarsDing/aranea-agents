import { computed, ref, watch, type ComputedRef, type Ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRouter } from 'vue-router';
import { useArtifactStore } from '../../../stores/artifact';
import { useChatRuntimeStore } from '../../../stores/chat/runtimeStore';
import { useSessionStore } from '../../../stores/session/index';
import type { ArtifactMeta } from '../../artifact/types';
import type { SessionTimeline } from '../../session/types';
import type { ChatEntityKind } from '../../../components/chat/types';

export function useChatTraceDialog(
  selectedEntityKind: Ref<ChatEntityKind>,
  displaySessions: ComputedRef<Array<{ id: string; title?: string }>>,
) {
  const { t } = useI18n();
  const sessionStore = useSessionStore();
  const runtimeStore = useChatRuntimeStore();
  const traceOpen = ref(false);
  const traceSessionId = ref<string | null>(null);
  const traceSessionTitle = ref('');
  const traceInitialTab = ref<'trace' | 'events'>('trace');
  const traceSessionOwnerKind = ref<'agent' | 'team'>('agent');

  // ── Timeline data loading (moved from SessionTimelineDialog) ──
  const timeline = ref<SessionTimeline | null>(null);
  const timelineLoading = ref(false);
  const timelineError = ref('');

  async function loadTimeline() {
    const sessionId = String(traceSessionId.value ?? '').trim();
    // 预加载：dialog 打开时即加载 trace 数据，不论 initialTab。
    // 用户从 events Tab 进入后切到 trace Tab 时可立即看到数据，
    // 切换时通过 SessionTimelineDialog 的 refresh-trace 事件触发刷新。
    if (!traceOpen.value || !sessionId) return;
    timelineLoading.value = true;
    timelineError.value = '';
    try {
      timeline.value = await sessionStore.fetchTimeline(sessionId);
    } catch (err) {
      timelineError.value = err instanceof Error ? err.message : 'Failed to load session trace.';
      timeline.value = null;
    } finally {
      timelineLoading.value = false;
    }
  }

  watch(
    () => [traceOpen.value, traceSessionId.value, traceInitialTab.value] as const,
    () => void loadTimeline(),
    { immediate: true },
  );

  function openSessionTrace(sessionId: string, tab: 'trace' | 'events' = 'trace') {
    const session = displaySessions.value.find((item) => item.id === sessionId);
    traceSessionId.value = sessionId;
    traceSessionTitle.value = session?.title ?? t('chat.untitledSession');
    traceInitialTab.value = tab;
    traceSessionOwnerKind.value = selectedEntityKind.value === 'team' ? 'team' : 'agent';
    traceOpen.value = true;
  }

  const traceStreamDeps = computed(() => ({
    ownerKind: traceSessionOwnerKind.value,
    /**
     * Phase 5 Blocker A: register a callback fired when the WS transport
     * reconnects for the inspected session. Watches
     * runtimeStore.wsConnectedBySession for false → true transitions
     * (after the initial registration) and invokes the handler so the
     * inspector can re-fetch Activities via ListActivities RPC.
     */
    onReconnect: (handler: () => void): (() => void) => {
      const sid = traceSessionId.value;
      if (!sid) return () => {};
      // Initialize prevConnected to true so the initial WS connection
      // (if not yet established) does NOT trigger the handler — only
      // actual reconnects (false → true after a disconnect) fire it.
      let prevConnected = true;
      const stop = watch(
        () => runtimeStore.wsConnectedBySession[sid],
        (next) => {
          if (!prevConnected && next) {
            handler();
          }
          prevConnected = !!next;
        },
      );
      return stop;
    },
  }));

  function openSessionEvents(selectedSessionId: string | undefined) {
    if (!selectedSessionId) return;
    openSessionTrace(selectedSessionId, 'events');
  }

  return {
    traceOpen,
    traceSessionId,
    traceSessionTitle,
    traceInitialTab,
    traceStreamDeps,
    openSessionTrace,
    openSessionEvents,
    timeline,
    timelineLoading,
    timelineError,
    reloadTimeline: loadTimeline,
  };
}

export function useChatSessionArtifacts(sessionId: ComputedRef<string | undefined>) {
  const router = useRouter();
  const artifactStore = useArtifactStore();
  const sessionArtifacts = ref<ArtifactMeta[]>([]);
  const sessionArtifactsLoading = ref(false);
  // P0 会话产物抽屉（2026-09-01）：header 产物按钮的会话内查看入口，
  // 点击条目经 artifactStore.openArtifactPreview 打开全局预览弹窗。
  const artifactsDrawerOpen = ref(false);

  async function loadSessionArtifacts(sid: string) {
    if (!sid) {
      sessionArtifacts.value = [];
      return;
    }
    sessionArtifactsLoading.value = true;
    try {
      const res = await artifactStore.loadArtifacts({ session_id: sid, limit: 20 });
      sessionArtifacts.value = res.items;
    } finally {
      sessionArtifactsLoading.value = false;
    }
  }

  watch(
    () => sessionId.value ?? '',
    (sid) => void loadSessionArtifacts(sid),
    { immediate: true },
  );

  /** 打开会话内产物抽屉（header 产物按钮的默认行为，替代直接跳页）。 */
  function openArtifactsDrawer() {
    artifactsDrawerOpen.value = true;
  }

  /** 抽屉「管理全部产物」：跳转产物管理页（保留会话筛选）。 */
  function goArtifactsPage() {
    const sid = String(sessionId.value ?? '').trim();
    artifactsDrawerOpen.value = false;
    void router.push({ path: '/artifacts', query: sid ? { session: sid } : {} });
  }

  /**
   * 产物点击统一入口（消息内产物卡片 / 历史绑定）：打开全局预览弹窗。
   * 预览失败或 binary 类型由弹窗内下载按钮降级（2026-09-01，原为
   * window.open 签名 URL 强制下载，点击无法「查看」）。
   */
  function openSessionArtifact(id: string) {
    artifactStore.openArtifactPreview(id);
  }

  function onArtifactDeleted(id: string) {
    sessionArtifacts.value = sessionArtifacts.value.filter((a) => a.id !== id);
  }

  return {
    sessionArtifacts,
    sessionArtifactsLoading,
    artifactsDrawerOpen,
    openArtifactsDrawer,
    goArtifactsPage,
    openSessionArtifact,
    onArtifactDeleted,
  };
}
