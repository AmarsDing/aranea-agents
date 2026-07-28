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
import { useBlockingStepNotifications } from '../features/chat/composables/useBlockingStepNotifications';
import { initLocalNotifications } from '../services/localNotification';

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const { isDark } = useTheme();

// P1: one shared ChatWorkspace for all mobile tabs (sessions list / chat
// detail). Created once here so the WS stream manager and bootstrap run a
// single time across tab switches; pages inject it via CHAT_WORKSPACE_KEY.
provide(CHAT_WORKSPACE_KEY, useChatWorkspace());

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
