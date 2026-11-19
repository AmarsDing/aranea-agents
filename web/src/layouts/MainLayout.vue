<template>
  <q-layout view="lHh LpR fFf" class="app-layout-root">
    <a href="#app-main-content" class="app-skip-link">{{ t('uiSystem.skipContent') }}</a>
    <!-- 顶栏只占一行：模块代号 · 模块名 › 页面，右边是日期和指令入口。工作台、看板的页签是它的第二行 -->
    <q-header :elevated="false" :class="isDark ? 'dark-header' : 'cream-header'">
      <q-toolbar class="app-topbar">
        <q-btn
          v-if="$q.screen.lt.md"
          flat
          dense
          round
          icon="menu"
          :aria-label="t('uiSystem.mainMenu')"
          :aria-expanded="drawerOpen"
          aria-controls="app-navigation"
          @click="drawerOpen = !drawerOpen"
        />
        <q-btn
          v-if="activeModule === 'harness' && !onShellPage && $q.screen.gt.sm"
          flat
          dense
          round
          :icon="drawerMini ? 'chevron_right' : 'chevron_left'"
          :aria-label="t('common.expandSidebar')"
          :aria-expanded="!drawerMini"
          @click="drawerMini = !drawerMini"
        />
        <div class="app-topbar__crumb">
          <span v-if="crumb.code" class="app-topbar__code" aria-hidden="true">{{ crumb.code }}</span>
          <span>{{ crumb.module }}</span>
          <template v-if="crumb.page">
            <span aria-hidden="true">›</span>
            <strong>{{ crumb.page }}</strong>
          </template>
        </div>
        <q-space />
        <span class="app-topbar__date">{{ todayLabel }}</span>
        <button type="button" class="app-topbar__cmd" :aria-label="t('shell.command.open')" @click="openPalette()">
          <q-icon name="search" size="16px" aria-hidden="true" />
          <span>{{ t('shell.command.placeholder') }}</span>
          <kbd>{{ t('shell.command.hint') }}</kbd>
        </button>
      </q-toolbar>
      <AppDomainTabs :trail="domainTrail" />
    </q-header>

    <q-drawer v-model="drawerOpen" show-if-above :width="showDrail ? 292 : 68" :breakpoint="1023" class="app-sidebar">
      <div class="app-shellnav">
        <!-- 模块栏：上面三个模块，下面是通知、应用、设置、我 -->
        <nav id="app-navigation" class="app-rail" :aria-label="t('uiSystem.mainMenu')">
          <router-link to="/" class="app-rail__brand" :aria-label="t('common.appTitle')">AR</router-link>
          <button
            v-for="mod in visibleModules"
            :key="mod.key"
            type="button"
            class="app-rail__btn"
            :class="{ 'app-rail__btn--active': !onShellPage && activeModule === mod.key }"
            :aria-current="!onShellPage && activeModule === mod.key ? 'true' : undefined"
            @click="onModuleClick(mod.key)"
          >
            <span class="app-rail__icon">
              <q-icon :name="mod.icon" size="22px" aria-hidden="true" />
              <span
                v-if="mod.key === 'workbench' && inboxBadge > 0"
                class="app-sidebar-badge app-sidebar-badge--corner"
                :aria-label="t('today.inbox.title') + ' ' + inboxBadge"
                >{{ inboxBadge > 99 ? '99+' : inboxBadge }}</span
              >
            </span>
            <span>{{ t(`shell.nav.module.${mod.key}`) }}</span>
          </button>
          <div class="app-rail__spacer" />
          <InboundNotificationBell
            :items="inboundStore.items"
            :unread-count="inboundStore.unreadCount"
            @open-session="onOpenInboundSession"
            @mark-read="inboundStore.markRead($event)"
            @mark-all-read="inboundStore.markAllRead()"
          />
          <OfficeNotificationBell
            :items="officeNotifyItems"
            :unread-count="officeNotifyUnread"
            @mark-read="officeNotifyDismiss($event)"
            @mark-all-read="officeNotifyMarkAll()"
          />
          <button
            v-for="item in shellPages"
            :key="item.to"
            type="button"
            class="app-rail__btn"
            :class="{ 'app-rail__btn--active': activePrimaryPath === item.to }"
            :aria-current="activePrimaryPath === item.to ? 'page' : undefined"
            @click="navigateTo(item.to)"
          >
            <q-icon :name="item.icon" size="20px" aria-hidden="true" />
            <span>{{ t(item.tabLabelKey) }}</span>
          </button>
          <q-btn round flat dense :aria-label="t('uiSystem.accountMenu')">
            <q-avatar size="34px" class="app-header-avatar" font-size="14px">
              {{ auth.avatarLetter }}
            </q-avatar>
            <q-menu anchor="top right" self="bottom left">
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
                <q-separator />
                <q-item-label header class="text-caption">{{ t('shell.nav.prefs') }}</q-item-label>
                <q-item
                  v-for="mode in themeModes"
                  :key="mode.value"
                  v-close-popup
                  clickable
                  :active="themeMode === mode.value"
                  active-class="text-primary"
                  @click="setTheme(mode.value)"
                >
                  <q-item-section avatar>
                    <q-icon :name="mode.icon" size="xs" />
                  </q-item-section>
                  <q-item-section>{{ t(mode.labelKey) }}</q-item-section>
                </q-item>
                <q-item
                  v-for="pref in prefToggles"
                  :key="pref.labelKey"
                  clickable
                  :active="pref.on"
                  active-class="text-primary"
                  @click="pref.toggle"
                >
                  <q-item-section avatar>
                    <q-icon :name="pref.icon" size="xs" />
                  </q-item-section>
                  <q-item-section>{{ t(pref.labelKey) }}</q-item-section>
                  <q-item-section v-if="pref.on" side>
                    <q-icon name="check" size="xs" />
                  </q-item-section>
                </q-item>
                <q-item
                  v-for="option in localeOptions"
                  :key="option.value"
                  v-close-popup
                  clickable
                  :active="locale === option.value"
                  active-class="text-primary"
                  @click="locale = option.value"
                >
                  <q-item-section avatar>
                    <q-icon name="translate" size="xs" />
                  </q-item-section>
                  <q-item-section>{{ option.label }}</q-item-section>
                </q-item>
                <q-separator />
                <q-item v-close-popup clickable @click="onLogout">
                  <q-item-section>{{ t('auth.logout') }}</q-item-section>
                </q-item>
              </q-list>
            </q-menu>
          </q-btn>
        </nav>

        <!-- Harness 的二级侧栏：领域 → 页面。手风琴，同时只展开一个领域 -->
        <nav v-if="showDrail" class="app-drail" :aria-label="t('shell.nav.module.harness')">
          <p class="app-drail__head">Harness · {{ t('shell.nav.domains') }}</p>
          <div class="app-sidebar-nav app-sidebar-domains">
            <div
              v-for="domain in harnessDomains"
              :key="domain.key"
              class="app-sidebar-domain"
              :class="{ 'app-sidebar-domain--active': activeDomainKey === domain.key }"
            >
              <button
                type="button"
                class="app-sidebar-domain__head"
                :aria-expanded="domain.primaries.length > 1 ? openDomain === domain.key : undefined"
                :aria-current="activeDomainKey === domain.key ? 'true' : undefined"
                @click="onDomainClick(domain)"
              >
                <span class="app-sidebar-domain__icon">
                  <q-icon :name="domain.icon" size="18px" aria-hidden="true" />
                </span>
                <span class="app-sidebar-domain__label">{{ t(domain.labelKey) }}</span>
                <span v-if="domain.primaries.length > 1" class="app-sidebar-domain__count">{{
                  domain.primaries.length
                }}</span>
              </button>
              <div
                v-if="domain.primaries.length > 1"
                v-show="openDomain === domain.key"
                class="app-sidebar-domain__items"
              >
                <q-item
                  v-for="item in domain.primaries"
                  :key="item.to"
                  clickable
                  dense
                  class="app-sidebar-nav-item"
                  :active="activePrimaryPath === item.to"
                  active-class="app-sidebar-item--active"
                  @click="navigateTo(item.to)"
                >
                  <q-item-section>{{ t(item.tabLabelKey) }}</q-item-section>
                </q-item>
              </div>
            </div>
          </div>
        </nav>
      </div>
    </q-drawer>

    <q-page-container
      id="app-main-content"
      class="app-page-container"
      :data-layout="pageLayout"
      :data-visual="pageVisual"
      tabindex="-1"
    >
      <div v-if="!domainTrail && showBreadcrumbs" class="app-breadcrumb-bar">
        <AppBreadcrumbs />
      </div>
      <router-view />
    </q-page-container>

    <GlobalCommandPalette v-model="paletteOpen" :seed="paletteSeed" />
    <AppCommandDock v-if="showDock" @submit="openPalette" />
    <!-- 全站唯一命令式确认框宿主：各页统一走 useAppConfirm 的 confirm()。 -->
    <AppConfirmHost />
    <!-- 全站唯一命令式输入框宿主：表单 prompt / 单选选择器统一走 useAppPrompt 的 prompt()。 -->
    <AppPromptHost />
    <PairingQrDialog v-model="pairingOpen" />
    <!-- 全局产物预览弹窗：目标由 artifactStore.previewTarget 驱动，
         会话页抽屉/工具产物卡片等任意深层组件均可触发（2026-09-01）。 -->
    <ArtifactPreviewDialog
      :model-value="artifactStore.dialogPreviewTarget"
      @update:model-value="artifactStore.closeArtifactPreview()"
      @download="onArtifactDownload"
    />
  </q-layout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { setQuasarLangFor } from '../i18n/quasar-lang';
import { findNavTrail } from '../config/navView';
import {
  buildNavDomains,
  findDomainTrail,
  moduleLanding,
  NAV_MODULES,
  type NavDomain,
  type NavDomainKey,
  type NavModuleKey,
} from '../config/navDomains';
import AppDomainTabs from '../components/layout/AppDomainTabs.vue';
import AppCommandDock from '../components/layout/AppCommandDock.vue';
import { useAuthStore } from '../stores/auth';
import { useNavStore } from '../stores/nav';
import { useInboundNotificationStore } from '../stores/inboundNotifications';
import InboundNotificationBell from '../components/layout/InboundNotificationBell.vue';
import OfficeNotificationBell from '../components/layout/OfficeNotificationBell.vue';
import AppBreadcrumbs from '../components/layout/AppBreadcrumbs.vue';
import AppConfirmHost from '../components/common/AppConfirmHost.vue';
import AppPromptHost from '../components/common/AppPromptHost.vue';
import GlobalCommandPalette from '../components/layout/GlobalCommandPalette.vue';
import { useOfficeNotifications } from '../features/office/useOfficeNotifications';
import { useInboxBadge } from '../features/today/useInboxBadge';
import PairingQrDialog from '../components/mobile/PairingQrDialog.vue';
import ArtifactPreviewDialog from '../components/artifact/ArtifactPreviewDialog.vue';
import { useArtifactStore } from '../stores/artifact';
import type { ArtifactMeta } from '../features/artifact/types';
import { useGlobalInboundNotifications } from '../composables/useGlobalInboundNotifications';
import { useTheme } from '../composables/useTheme';
import { useA11yPrefs } from '../composables/useA11yPrefs';
import { useNavPreferences } from '../composables/useNavPreferences';
import { resolveUiLayout, resolveUiVisual } from '../config/uiLayout';

const { t, locale } = useI18n();
const $q = useQuasar();
const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const inboundStore = useInboundNotificationStore();
const artifactStore = useArtifactStore();
const drawerOpen = ref($q.screen.gt.sm);
// Harness 的二级侧栏收起 / 展开，按人记住
const MINI_KEY = 'nav:mini';
function readMini(): boolean {
  try {
    return localStorage.getItem(MINI_KEY) === '1';
  } catch {
    return false;
  }
}
const drawerMini = ref(readMini());
watch(drawerMini, (mini) => {
  try {
    localStorage.setItem(MINI_KEY, mini ? '1' : '0');
  } catch {
    // 存不了不影响本次会话
  }
});
const pairingOpen = ref(false);
const paletteOpen = ref(false);
// 指令面板打开时带上的一句话（从右下角指令窗发出时才有）
const paletteSeed = ref('');
function openPalette(seed = '') {
  paletteSeed.value = seed;
  paletteOpen.value = true;
}

// E 阶段 W1c：office 通知中心铃铛（30s 轮询 + 聚焦即刷；已读管理走
// /v1/office/notifications*，全认证用户可用）。
const {
  items: officeNotifyItems,
  unreadCount: officeNotifyUnread,
  dismiss: officeNotifyDismiss,
  markAll: officeNotifyMarkAll,
} = useOfficeNotifications();

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

// 显示偏好：密度 / 色觉安全 / 动效，写入 <html data-*>，由 _nt-tokens.css 消费。
const { prefs: a11yPrefs, setDensity, setColorvision, setMotion } = useA11yPrefs();

function toggleDensity() {
  setDensity(a11yPrefs.density === 'compact' ? 'comfortable' : 'compact');
}

function toggleColorvision() {
  setColorvision(a11yPrefs.colorvision === 'safe' ? 'default' : 'safe');
}

function toggleMotion() {
  setMotion(a11yPrefs.motion === 'reduced' ? 'full' : 'reduced');
}

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

// P1-7：侧栏按后端下发的可见菜单过滤（仅展示；后端对每个 API 硬判）。
// 下发失败时 nav.isPathVisible 放行全部——可用性优先，安全由后端兜底。
const nav = useNavStore();
// 侧栏「工作台」角标：与工作台列表同一个服务端口径（/v1/inbox/summary）。
const { count: inboxBadge, refresh: refreshInboxBadge } = useInboxBadge();
// 换页时顺手对一次账：在工作台处理完事项后，角标不用等下一轮轮询。
watch(
  () => route.path,
  () => void refreshInboxBadge(),
);

const navDomains = computed(() => buildNavDomains(nav.isPathVisible));
const domainTrail = computed(() => findDomainTrail(route.path, navDomains.value));
const activeDomainKey = computed(() => domainTrail.value?.domain.key ?? '');
const activePrimaryPath = computed(() => domainTrail.value?.primary.to ?? '');

// 当前模块跟着页面走；在壳层页面（设置、应用）或导航之外的页面时，保持上一个模块
const activeModule = ref<NavModuleKey>('workbench');
watch(
  () => domainTrail.value?.domain.module,
  (mod) => {
    if (mod && mod !== 'shell') activeModule.value = mod;
  },
  { immediate: true },
);
const visibleModules = computed(() =>
  NAV_MODULES.filter((mod) => navDomains.value.some((domain) => domain.module === mod.key)),
);
const harnessDomains = computed(() => navDomains.value.filter((domain) => domain.module === 'harness'));
// 模块栏底部的壳层入口：应用、设置
const shellPages = computed(() => navDomains.value.find((domain) => domain.module === 'shell')?.primaries ?? []);
// 二级侧栏只在 Harness 下出现；宽屏可以收起，窄屏的抽屉里始终带上
// 在应用、设置这类壳层页面上，模块不高亮，二级侧栏也不出现
const onShellPage = computed(() => domainTrail.value?.domain.module === 'shell');
const showDrail = computed(
  () => activeModule.value === 'harness' && !onShellPage.value && !($q.screen.gt.sm && drawerMini.value),
);
function onModuleClick(key: NavModuleKey) {
  activeModule.value = key;
  const to = moduleLanding(key, navDomains.value);
  if (to) void navigateTo(to);
}

// 手风琴：同时只展开一个领域；换页时跟着展开当前领域
const openDomain = ref<NavDomainKey | ''>('');
watch(
  activeDomainKey,
  (key) => {
    if (key) openDomain.value = key;
  },
  { immediate: true },
);
function onDomainClick(domain: NavDomain) {
  // 只有一个页面的领域（基座总览）直接进页面
  if (domain.primaries.length === 1) {
    void navigateTo(domain.primaries[0].to);
    return;
  }
  openDomain.value = openDomain.value === domain.key ? '' : domain.key;
}

// 顶栏左边：模块代号 · 模块名 › 页面（Harness 多带一层领域名）
const crumb = computed(() => {
  const trail = domainTrail.value;
  if (!trail) return { code: '', module: t('common.appTitle'), page: '' };
  const leaf = t((trail.child ?? trail.primary).tabLabelKey);
  const mod = NAV_MODULES.find((m) => m.key === trail.domain.module);
  if (!mod) return { code: '', module: leaf, page: '' };
  const parts = [leaf];
  if (trail.child) parts.unshift(t(trail.primary.tabLabelKey));
  if (trail.domain.module === 'harness' && trail.domain.primaries.length > 1) parts.unshift(t(trail.domain.labelKey));
  return { code: mod.code, module: t(trail.domain.moduleLabelKey), page: parts.join(' › ') };
});
const todayLabel = computed(() =>
  new Intl.DateTimeFormat(String(locale.value), { month: 'long', day: 'numeric', weekday: 'short' }).format(new Date()),
);

// 「我」菜单里的外观项
const themeModes = [
  { value: 'auto', icon: 'brightness_auto', labelKey: 'common.autoMode' },
  { value: 'light', icon: 'light_mode', labelKey: 'common.lightMode' },
  { value: 'dark', icon: 'dark_mode', labelKey: 'common.darkMode' },
] as const;
const prefToggles = computed(() => [
  { icon: 'compress', labelKey: 'shell.prefs.density', on: a11yPrefs.density === 'compact', toggle: toggleDensity },
  {
    icon: 'visibility',
    labelKey: 'shell.prefs.colorvision',
    on: a11yPrefs.colorvision === 'safe',
    toggle: toggleColorvision,
  },
  { icon: 'animation', labelKey: 'shell.prefs.motion', on: a11yPrefs.motion === 'reduced', toggle: toggleMotion },
]);

// 当前路由在导航树中的位置：用于高亮、自动展开与面包屑归属。
const navTrail = computed(() => findNavTrail(route.path));
const activeNavPath = computed(() => navTrail.value?.item.to ?? '');

const showBreadcrumbs = computed(() => route.meta.breadcrumb !== false);
// 指令窗：全屏类页面（对话、实时运转等）底部有自己的输入区，不去挡它
const showDock = computed(() => pageLayout.value !== 'immersive' && pageLayout.value !== 'canvas');
const pageLayout = computed(() => resolveUiLayout(route.meta.uiLayout));
// 视觉档位：指挥舱（deck）保留光效与网格底，其余为安静的工作区
const pageVisual = computed(() => resolveUiVisual(route.meta.uiVisual));
const { recordVisit } = useNavPreferences();

watch(
  activeNavPath,
  (path) => {
    if (path) recordVisit(path);
  },
  { immediate: true },
);

onMounted(() => {
  window.addEventListener('keydown', onGlobalKeydown);
});

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onGlobalKeydown);
});

function onGlobalKeydown(event: KeyboardEvent) {
  if (!(event.ctrlKey || event.metaKey) || event.key.toLowerCase() !== 'k') return;
  event.preventDefault();
  openPalette();
}

watch(
  () => route.path,
  () => {
    if ($q.screen.lt.md) drawerOpen.value = false;
  },
);

watch(
  () => auth.user?.id ?? null,
  async (id) => {
    if (id !== null) await nav.ensureMenus();
  },
  { immediate: true },
);

async function navigateTo(path: string) {
  if ($q.screen.lt.md) drawerOpen.value = false;
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
