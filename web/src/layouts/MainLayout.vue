<template>
  <q-layout view="hHh LpR fFf" class="app-layout-root">
    <q-header :elevated="false" :class="isDark ? 'dark-header' : 'cream-header'">
      <q-toolbar class="q-px-sm-md">
        <q-btn flat dense round icon="menu" aria-label="Menu" @click="drawerOpen = !drawerOpen" />
        <q-btn
          flat
          dense
          round
          :icon="drawerMini ? 'chevron_right' : 'chevron_left'"
          :aria-label="t('common.expandSidebar')"
          @click="drawerMini = !drawerMini"
        />
        <q-toolbar-title class="q-pl-sm app-toolbar-title text-weight-bold">
          {{ t('common.appTitle') }}
        </q-toolbar-title>
        <q-space />
        <div class="row items-center q-gutter-sm q-mr-sm app-header-actions">
          <InboundNotificationBell
            :items="inboundStore.items"
            :unread-count="inboundStore.unreadCount"
            @open-session="onOpenInboundSession"
            @mark-read="inboundStore.markRead($event)"
            @mark-all-read="inboundStore.markAllRead()"
          />
          <q-btn round flat dense class="cursor-pointer">
            <q-avatar size="36px" class="app-header-avatar" font-size="14px">
              {{ auth.avatarLetter }}
            </q-avatar>
            <q-menu anchor="bottom right" self="top right">
              <q-list dense class="app-menu-min-180">
                <q-item>
                  <q-item-section class="text-caption text-grey">{{ auth.displayLabel }}</q-item-section>
                </q-item>
                <q-separator />
                <q-item v-close-popup clickable @click="pairingOpen = true">
                  <q-item-section avatar>
                    <q-icon name="qr_code_2" size="xs" />
                  </q-item-section>
                  <q-item-section>{{ t('mobile.pairingMenuItem') }}</q-item-section>
                </q-item>
                <q-item v-close-popup clickable @click="onLogout">
                  <q-item-section>{{ t('auth.logout') }}</q-item-section>
                </q-item>
              </q-list>
            </q-menu>
          </q-btn>
        </div>
        <q-btn flat round :icon="themeIcon" :aria-label="t('common.autoMode')">
          <q-menu anchor="bottom right" self="top right" auto-close>
            <q-list dense class="app-menu-min-160">
              <q-item clickable :active="themeMode === 'auto'" active-class="text-primary" @click="setTheme('auto')">
                <q-item-section avatar>
                  <q-icon name="brightness_auto" size="xs" />
                </q-item-section>
                <q-item-section>{{ t('common.autoMode') }}</q-item-section>
              </q-item>
              <q-item clickable :active="themeMode === 'light'" active-class="text-primary" @click="setTheme('light')">
                <q-item-section avatar>
                  <q-icon name="light_mode" size="xs" />
                </q-item-section>
                <q-item-section>{{ t('common.lightMode') }}</q-item-section>
              </q-item>
              <q-item clickable :active="themeMode === 'dark'" active-class="text-primary" @click="setTheme('dark')">
                <q-item-section avatar>
                  <q-icon name="dark_mode" size="xs" />
                </q-item-section>
                <q-item-section>{{ t('common.darkMode') }}</q-item-section>
              </q-item>
            </q-list>
          </q-menu>
        </q-btn>
        <q-select
          v-model="locale"
          :options="localeOptions"
          emit-value
          map-options
          dense
          :dark="isDark"
          borderless
          options-dense
          class="app-header-locale q-ml-sm app-menu-min-110"
        />
      </q-toolbar>
    </q-header>

    <q-drawer
      v-model="drawerOpen"
      show-if-above
      :width="256"
      :mini="drawerMini"
      :mini-width="90"
      :breakpoint="0"
      :class="['app-sidebar', { 'q-drawer--design-mini': drawerMini }]"
    >
      <div class="app-sidebar-card">
        <div class="app-sidebar__scroll fit">
          <q-list class="app-sidebar-nav" :dense="drawerMini">
            <template v-for="(group, gi) in sideNavGroups" :key="`g-${gi}`">
              <q-item-label v-show="!drawerMini" header class="app-sidebar-section-label">
                {{ t(group.labelKey) }}
              </q-item-label>
              <div class="app-sidebar-group__items">
                <q-item
                  v-for="(item, ii) in group.items"
                  :key="`g-${gi}-i-${ii}`"
                  v-ripple
                  clickable
                  class="app-sidebar-nav-item"
                  :active="isNavItemActive(item)"
                  active-class="app-sidebar-item--active"
                  @click="navigateTo(item.to)"
                >
                  <q-tooltip v-if="drawerMini" anchor="center right" self="center left" :offset="[8, 0]">
                    {{ t(item.labelKey) }}
                  </q-tooltip>
                  <q-item-section avatar>
                    <q-icon :name="item.icon" />
                  </q-item-section>
                  <q-item-section v-show="!drawerMini">{{ t(item.labelKey) }}</q-item-section>
                </q-item>
              </div>
              <q-separator v-if="gi < sideNavGroups.length - 1" v-show="!drawerMini" class="app-sidebar-divider" />
            </template>
          </q-list>
        </div>
      </div>
    </q-drawer>

    <q-page-container class="app-page-container">
      <router-view />
    </q-page-container>

    <PairingQrDialog v-model="pairingOpen" />
    <!-- 全局产物预览弹窗：目标由 artifactStore.previewTarget 驱动，
         会话页抽屉/工具产物卡片等任意深层组件均可触发（2026-09-01）。 -->
    <ArtifactPreviewDialog
      :model-value="artifactStore.previewTarget"
      @update:model-value="artifactStore.closeArtifactPreview()"
      @download="onArtifactDownload"
    />
  </q-layout>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { setQuasarLangFor } from '../i18n/quasar-lang';
import { sideNavGroups } from '../config/sideNav';
import { useAuthStore } from '../stores/auth';
import { useInboundNotificationStore } from '../stores/inboundNotifications';
import InboundNotificationBell from '../components/layout/InboundNotificationBell.vue';
import PairingQrDialog from '../components/mobile/PairingQrDialog.vue';
import ArtifactPreviewDialog from '../components/artifact/ArtifactPreviewDialog.vue';
import { useArtifactStore } from '../stores/artifact';
import type { ArtifactMeta } from '../features/artifact/types';
import { useGlobalInboundNotifications } from '../composables/useGlobalInboundNotifications';
import { useTheme } from '../composables/useTheme';

const { t, locale } = useI18n();
const $q = useQuasar();
const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const inboundStore = useInboundNotificationStore();
const artifactStore = useArtifactStore();
const drawerOpen = ref(true);
const drawerMini = ref(true);
const pairingOpen = ref(false);

/** 产物预览弹窗的下载动作：统一走 artifactStore.download，失败提示。 */
async function onArtifactDownload(meta: ArtifactMeta) {
  try {
    await artifactStore.download(meta);
  } catch {
    $q.notify({ type: 'negative', message: t('artifact.page.downloadLinkFailed') });
  }
}

useGlobalInboundNotifications();

// B7: Theme management via useTheme composable (auto/dark/light modes).
const { themeMode, isDark, setTheme } = useTheme();

// Icon reflects the current effective theme (not the mode selection).
// In auto mode, the icon shows the actual applied theme.
const themeIcon = computed(() => {
  if (themeMode.value === 'auto') return 'brightness_auto';
  return isDark.value ? 'dark_mode' : 'light_mode';
});

const localeOptions = [
  { label: t('common.localeZhCN'), value: 'zh-CN' as const },
  { label: t('common.localeEnUS'), value: 'en-US' as const },
];

watch(locale, (v) => {
  setQuasarLangFor(String(v));
  if (v === 'zh-CN' || v === 'en-US') {
    localStorage.setItem('locale', v);
  }
});

function isNavItemActive(item: { to: string; exact?: boolean }) {
  return item.exact === false ? route.path.startsWith(item.to) : route.path === item.to;
}

async function navigateTo(path: string) {
  if (route.path === path) return;
  await router.push(path);
}

function onOpenInboundSession(sessionId: string, agentId: string) {
  void router.push({ name: 'chat', query: { session: sessionId, agent: agentId } });
}

async function onLogout() {
  await auth.logout();
  await router.push('/login');
}
</script>
