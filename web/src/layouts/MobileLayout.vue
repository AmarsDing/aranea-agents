<template>
  <q-layout view="hHh lpR fFf" class="app-layout-root">
    <q-header :elevated="false" :class="isDark ? 'dark-header' : 'cream-header'">
      <q-toolbar>
        <q-toolbar-title class="app-toolbar-title text-weight-bold">
          {{ t('common.appTitle') }}
        </q-toolbar-title>
        <q-btn round flat dense class="cursor-pointer">
          <q-avatar size="32px" class="app-header-avatar" font-size="13px">
            {{ auth.avatarLetter }}
          </q-avatar>
          <q-menu anchor="bottom right" self="top right">
            <q-list dense style="min-width: 160px">
              <q-item>
                <q-item-section class="text-caption text-grey">{{ auth.displayLabel }}</q-item-section>
              </q-item>
              <q-separator />
              <q-item v-close-popup clickable @click="onLogout">
                <q-item-section>{{ t('auth.logout') }}</q-item-section>
              </q-item>
            </q-list>
          </q-menu>
        </q-btn>
      </q-toolbar>
      <!-- P3.2c: global offline indicator; the sessions tab serves cached data in this state. -->
      <div v-if="!online" class="mobile-offline-banner" role="status">
        <q-icon name="cloud_off" size="16px" />
        <span>{{ t('mobile.offlineCached') }}</span>
      </div>
    </q-header>

    <q-footer :elevated="false" :class="isDark ? 'dark-header' : 'cream-header'">
      <q-tabs
        :model-value="activeTab"
        dense
        narrow-indicator
        align="justify"
        active-color="primary"
        @update:model-value="onTabChange"
      >
        <q-tab name="sessions" icon="forum" :label="t('mobile.tabSessions')" />
        <q-tab name="tasks" icon="task_alt" :label="t('mobile.tabTasks')" />
        <q-tab name="me" icon="person" :label="t('mobile.tabMe')" />
      </q-tabs>
    </q-footer>

    <q-page-container class="app-page-container">
      <router-view />
    </q-page-container>

    <!-- 对话转评估用例对话框（P1-1）：workspace 在布局层共享，对话框需在布局层渲染，
         否则移动任务页 @add-to-eval 只改状态无 UI。绑定与桌面 ChatPage 一致。 -->
    <AddEvalCaseDialog
      v-model:open="workspace.evalCase.open"
      :mode="workspace.evalCase.mode"
      :dataset-id="workspace.evalCase.datasetId"
      :dataset-options="workspace.evalCase.datasetOptions"
      :datasets-loading="workspace.evalCase.datasetsLoading"
      :new-dataset-name="workspace.evalCase.newDatasetName"
      :input="workspace.evalCase.input"
      :expected-output="workspace.evalCase.expectedOutput"
      :rubric="workspace.evalCase.rubric"
      :submitting="workspace.evalCase.submitting"
      @update:mode="workspace.evalCase.mode = $event"
      @update:dataset-id="workspace.evalCase.datasetId = $event"
      @update:new-dataset-name="workspace.evalCase.newDatasetName = $event"
      @update:input="workspace.evalCase.input = $event"
      @update:expected-output="workspace.evalCase.expectedOutput = $event"
      @update:rubric="workspace.evalCase.rubric = $event"
      @submit="workspace.evalCase.submit()"
    />
  </q-layout>
</template>

<script setup lang="ts">
import { computed, onMounted, provide } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useAuthStore } from '../stores/auth';
import { useTheme } from '../composables/useTheme';
import { useChatWorkspace } from '../features/chat/composables/useChatWorkspace';
import { CHAT_WORKSPACE_KEY } from '../features/chat/composables/chatWorkspaceInjection';
import AddEvalCaseDialog from '../components/evaluation/AddEvalCaseDialog.vue';
import { useBlockingStepNotifications } from '../features/chat/composables/useBlockingStepNotifications';
import { initLocalNotifications } from '../services/localNotification';
import { useNetworkStatus } from '../features/mobile/useNetworkStatus';

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const { isDark } = useTheme();

// P3.2c: drives the offline banner in the header.
const { online } = useNetworkStatus();

// P1: one shared ChatWorkspace for all mobile tabs (sessions list / chat
// detail). Created once here so the WS stream manager and bootstrap run a
// single time across tab switches; pages inject it via CHAT_WORKSPACE_KEY.
const workspace = useChatWorkspace();
provide(CHAT_WORKSPACE_KEY, workspace);

// P2: local notifications for steps blocked on user input (confirm/clarify).
// The watcher is no-op outside the Tauri shell; the click handler deep-links
// into the mobile chat route carried in the notification payload.
useBlockingStepNotifications();
onMounted(() => {
  void initLocalNotifications((target) => {
    void router.push(target);
  });
});

// Bottom tab navigation (sessions / tasks / me). The chat detail page maps
// to the sessions tab.
const activeTab = computed(() => {
  if (route.path.startsWith('/mobile/tasks')) return 'tasks';
  if (route.path.startsWith('/mobile/me')) return 'me';
  return 'sessions';
});

async function onTabChange(name: string | number) {
  const target = `/mobile/${name}`;
  if (route.path === target) return;
  await router.push(target);
}

async function onLogout() {
  await auth.logout();
  await router.push('/login');
}
</script>

<style scoped>
.mobile-offline-banner {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 6px 16px;
  font-size: 12px;
  line-height: 16px;
  color: var(--color-warning);
  background: color-mix(in srgb, var(--color-warning) 14%, transparent);
}
</style>
