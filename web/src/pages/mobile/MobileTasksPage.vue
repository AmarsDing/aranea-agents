<template>
  <q-page class="mobile-tasks-page column no-wrap">
    <div v-if="!workspace.coreReady" class="col flex flex-center">
      <q-spinner-dots size="40px" color="accent" />
    </div>

    <template v-else>
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
import { computed, onMounted, provide, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import type { QScrollArea } from 'quasar';
import TaskList from '../../components/chat/v2/TaskList.vue';
import { injectChatWorkspace } from '../../features/chat/composables/chatWorkspaceInjection';
import { useChatMessagePanelBindings } from '../../features/chat/composables/useChatMessagePanelBindings';
import { useActivityQueries } from '../../features/chat/composables/useActivityQueries';
import { CHAT_SCROLL_EL_KEY } from '../../features/chat/composables/useLazyTaskHydration';
import { resolveMobileTasksView } from '../../features/chat/composables/mobileTasksView';

const { t } = useI18n();
const router = useRouter();
const workspace = injectChatWorkspace();
const { handlers } = useChatMessagePanelBindings(workspace);
const queries = useActivityQueries();

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
});

function goToSessions() {
  void router.push({ name: 'mobile-sessions' });
}
</script>

<style scoped>
.mobile-tasks-page__header {
  border-bottom: 1px solid rgba(0, 0, 0, 0.08);
}
</style>
