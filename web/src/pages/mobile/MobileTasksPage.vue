<template>
  <q-page class="mobile-tasks-page column no-wrap">
    <div v-if="!workspace.coreReady" class="col flex flex-center">
      <q-spinner-dots size="40px" color="accent" />
    </div>

    <template v-else>
      <!-- 作战总台 P0：简版签收卡（床头不诊断只签收；数据同源 useYouQueue） -->
      <section v-if="signoffVisible" class="mobile-signoff">
        <div v-if="signoffOffline" class="mobile-signoff__offline">{{ t('workbench.mobile.offline') }}</div>
        <div class="mobile-signoff__head row items-center q-px-md q-py-xs">
          <div class="text-subtitle2 text-weight-bold">{{ t('workbench.mobile.title') }}</div>
          <q-space />
          <span v-if="youQueue.total.value > 0" class="mobile-signoff__badge">{{ youQueue.total.value }}</span>
        </div>
        <div v-if="youQueue.total.value === 0" class="q-px-md q-pb-sm text-caption text-grey">
          {{ t('workbench.mobile.empty') }}
        </div>
        <div v-else class="mobile-signoff__list">
          <div v-for="item in signoffItems" :key="item.id" class="mobile-signoff__item">
            <div class="mobile-signoff__meta">
              <span class="text-weight-medium">{{ item.who }}</span>
              <span class="text-grey">{{ laneLabel(item.lane) }}</span>
            </div>
            <div class="mobile-signoff__summary">{{ item.summary }}</div>
            <div class="mobile-signoff__actions row q-gutter-sm">
              <q-btn
                v-if="item.action === 'approve'"
                unelevated
                no-caps
                class="mobile-signoff__btn mobile-signoff__btn--ok"
                :label="isArmed(item.id, 'approved') ? t('workbench.mobile.confirmHint') : t('workbench.you.approve')"
                :color="isArmed(item.id, 'approved') ? 'warning' : 'positive'"
                :disable="signoffOffline || busyMap.has(item.id)"
                @click="onSignoff(item, 'approved')"
              />
              <q-btn
                v-if="item.action === 'approve'"
                outline
                no-caps
                class="mobile-signoff__btn"
                color="negative"
                :label="isArmed(item.id, 'rejected') ? t('workbench.mobile.confirmHint') : t('workbench.you.reject')"
                :disable="signoffOffline || busyMap.has(item.id)"
                @click="onSignoff(item, 'rejected')"
              />
              <q-btn
                v-else-if="item.action === 'ack'"
                unelevated
                no-caps
                class="mobile-signoff__btn mobile-signoff__btn--ok"
                color="positive"
                :label="isArmed(item.id, 'acked') ? t('workbench.mobile.confirmHint') : t('workbench.you.ack')"
                :disable="signoffOffline || busyMap.has(item.id)"
                @click="onSignoff(item, 'acked')"
              />
            </div>
          </div>
        </div>
      </section>

      <div class="mobile-tasks-page__header column q-px-md q-py-sm">
        <div class="text-subtitle1 text-weight-bold">{{ t('mobile.tasksTitle') }}</div>
        <div v-if="session" class="text-caption text-grey ellipsis">{{ sessionTitle }}</div>
      </div>

      <!-- 未选会话：引导回会话 Tab -->
      <div v-if="viewState === 'no-session'" class="col column flex-center q-pa-xl text-grey">
        <q-icon name="task_alt" size="48px" class="q-mb-md" />
        <div class="text-body2 q-mb-md">{{ t('mobile.noSessionForTasks') }}</div>
        <q-btn unelevated no-caps color="primary" :label="t('mobile.goToSessions')" @click="goToSessions" />
      </div>

      <!-- 有会话无任务 -->
      <div v-else-if="viewState === 'empty'" class="col column flex-center q-pa-xl text-grey">
        <q-icon name="playlist_play" size="48px" class="q-mb-md" />
        <div class="text-body2">{{ t('mobile.tasksEmpty') }}</div>
      </div>

      <!-- 任务执行流：复用桌面 TaskList（纵向时间线，activityV2Store 全复用） -->
      <q-scroll-area v-else ref="scrollRef" class="col">
        <div class="q-pa-sm">
          <TaskList
            :session-id="session!.id"
            @regenerate="workspace.composer.regenerateV2Task"
            @add-to-eval="workspace.evalCase.openFromTask"
            @feedback="workspace.composer.onMessageFeedback"
            @resume-task="workspace.session.resumeTask"
            @pause-agent="handlers.onPauseAgent"
            @inject-agent="handlers.onInjectAgent"
            @expand="handlers.onExpandChildren"
            @confirm-step="workspace.session.onConfirmActivityGrant"
            @submit-clarification="workspace.session.onSubmitClarification"
          />
        </div>
      </q-scroll-area>
    </template>
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, provide, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import type { QScrollArea } from 'quasar';
import { useQuasar } from 'quasar';
import TaskList from '../../components/chat/v2/TaskList.vue';
import { injectChatWorkspace } from '../../features/chat/composables/chatWorkspaceInjection';
import { useChatMessagePanelBindings } from '../../features/chat/composables/useChatMessagePanelBindings';
import { useActivityQueries } from '../../features/chat/composables/useActivityQueries';
import { CHAT_SCROLL_EL_KEY } from '../../features/chat/composables/useLazyTaskHydration';
import { resolveMobileTasksView } from '../../features/chat/composables/mobileTasksView';
import { useYouQueue } from '../../features/workbench/useYouQueue';
import { globalWsHubConnected } from '../../realtime/globalWsHub';
import type { YouItem, YouLaneId } from '../../features/workbench/types';

const { t } = useI18n();
const $q = useQuasar();
const router = useRouter();
const workspace = injectChatWorkspace();
const { handlers } = useChatMessagePanelBindings(workspace);
const queries = useActivityQueries();

// ——— 作战总台 P0：签收卡（与桌面 YOU 金门同源） ———
const youQueue = useYouQueue();
const signoffOffline = ref(!globalWsHubConnected());
const armedMap = ref(new Map<string, number>());
const busyMap = ref(new Set<string>());
const armTick = ref(0);
let offlineTimer: ReturnType<typeof setInterval> | null = null;
let armTimer: ReturnType<typeof setInterval> | null = null;

const signoffItems = computed(() => youQueue.lanes.value.flatMap((l) => l.items));
const signoffVisible = computed(() => true);

function laneLabel(lane: YouLaneId): string {
  return t(`workbench.lanes.${lane}`);
}

function isArmed(id: string, verb: string): boolean {
  void armTick.value;
  const at = armedMap.value.get(`${id}:${verb}`);
  return !!at && Date.now() - at < 4000;
}

/** 大触面 A/R + 二次点按确认（资金/高危类口径；滑动组件待 Quasar 原生件落地后升级）；离线灰条拒绝一切签批 */
async function onSignoff(item: YouItem, verb: 'approved' | 'rejected' | 'acked'): Promise<void> {
  if (signoffOffline.value || busyMap.value.has(item.id)) return;
  const key = `${item.id}:${verb}`;
  if (!isArmed(item.id, verb)) {
    const next = new Map(armedMap.value);
    next.set(key, Date.now());
    armedMap.value = next;
    return;
  }
  const next = new Map(armedMap.value);
  next.delete(key);
  armedMap.value = next;
  const busy = new Set(busyMap.value);
  busy.add(item.id);
  busyMap.value = busy;
  try {
    await youQueue.sign(item, verb, 'mobile');
  } catch (err) {
    // 异常合约（设计 §4.2）：驳回弹出 reason（不落戳、条目仍在等签，不欺骗）
    const reason = err instanceof Error ? err.message : String(err);
    $q.notify({ type: 'negative', message: t('workbench.sign.failed', { reason }) });
  } finally {
    const done = new Set(busyMap.value);
    done.delete(item.id);
    busyMap.value = done;
  }
}

const session = computed(() => workspace.session.selectedSessionForUi);
const sessionTitle = computed(() => session.value?.title || t('chat.untitledSession'));
const taskCount = computed(() => (session.value ? queries.getSessionTasks(session.value.id).length : 0));
const viewState = computed(() => resolveMobileTasksView(session.value?.id, taskCount.value));

// TaskList 的懒水合 IntersectionObserver 需要滚动容器：把 q-scroll-area 的
// 滚动目标 provide 出去（key 与桌面 ChatMessageList 相同）。
const scrollRef = ref<QScrollArea | null>(null);
const scrollEl = ref<HTMLElement | null>(null);
provide(CHAT_SCROLL_EL_KEY, scrollEl);

onMounted(() => {
  scrollEl.value = scrollRef.value?.getScrollTarget() ?? null;
  offlineTimer = setInterval(() => {
    signoffOffline.value = !globalWsHubConnected();
  }, 2000);
  // 二次点按 4s 武装窗口的视觉过期：tick 驱动 isArmed 重算并惰性清理过期键
  armTimer = setInterval(() => {
    armTick.value = Date.now();
    let dirty = false;
    const next = new Map(armedMap.value);
    for (const [k, at] of next) {
      if (Date.now() - at >= 4000) {
        next.delete(k);
        dirty = true;
      }
    }
    if (dirty) armedMap.value = next;
  }, 500);
});

onUnmounted(() => {
  if (offlineTimer) clearInterval(offlineTimer);
  if (armTimer) clearInterval(armTimer);
});

function goToSessions() {
  void router.push({ name: 'mobile-sessions' });
}
</script>

<style scoped>
.mobile-tasks-page__header {
  border-bottom: 1px solid rgb(0 0 0 / 8%);
}

.mobile-signoff {
  border-bottom: 1px solid color-mix(in srgb, var(--wb-await) 30%, transparent);
  background: color-mix(in srgb, var(--wb-await) 6%, transparent);
}

.mobile-signoff__offline {
  padding: 4px 16px;
  font-size: 12px;
  line-height: 1.4;
  color: var(--color-text-secondary, #9aa3b2);
  background: color-mix(in srgb, var(--color-text-primary, #fff) 8%, transparent);
}

.mobile-signoff__badge {
  min-width: 20px;
  padding: 1px 6px;
  border-radius: 10px;
  font-size: 11px;
  font-weight: 600;
  text-align: center;
  color: #14181f;
  background: var(--wb-await);
}

.mobile-signoff__list {
  max-height: 38vh;
  overflow-y: auto;
}

.mobile-signoff__item {
  padding: 8px 16px;
  border-top: 1px dashed color-mix(in srgb, var(--wb-await) 35%, transparent);
}

.mobile-signoff__item:first-child {
  border-top: 0;
}

.mobile-signoff__meta {
  display: flex;
  gap: 8px;
  font-size: 12px;
  margin-bottom: 2px;
}

.mobile-signoff__summary {
  font-size: 13px;
  line-height: 1.5;
  margin-bottom: 8px;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.mobile-signoff__btn {
  flex: 1;
  min-height: 44px;
  font-weight: 600;
}
</style>
