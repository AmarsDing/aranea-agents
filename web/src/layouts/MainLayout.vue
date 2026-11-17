<template>
  <q-layout view="hHh LpR fFf" class="app-layout-root">
    <a href="#app-main-content" class="app-skip-link">{{ t('uiSystem.skipContent') }}</a>
    <q-header :elevated="false" :class="isDark ? 'dark-header' : 'cream-header'">
      <q-toolbar class="q-px-sm-md">
        <q-btn
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
          v-if="$q.screen.gt.sm"
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
          <q-btn flat round dense icon="search" :aria-label="t('shell.command.open')" @click="paletteOpen = true">
            <q-tooltip> {{ t('shell.command.open') }} · {{ t('shell.command.hint') }} </q-tooltip>
          </q-btn>
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
          <q-btn round flat dense class="cursor-pointer" :aria-label="t('uiSystem.accountMenu')">
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
        <q-btn flat round :icon="themeIcon" :aria-label="t('uiSystem.themeMenu')">
          <q-menu anchor="bottom right" self="top right">
            <q-list dense class="app-menu-min-160">
              <q-item
                v-close-popup
                clickable
                :active="themeMode === 'auto'"
                active-class="text-primary"
                @click="setTheme('auto')"
              >
                <q-item-section avatar>
                  <q-icon name="brightness_auto" size="xs" />
                </q-item-section>
                <q-item-section>{{ t('common.autoMode') }}</q-item-section>
              </q-item>
              <q-item
                v-close-popup
                clickable
                :active="themeMode === 'light'"
                active-class="text-primary"
                @click="setTheme('light')"
              >
                <q-item-section avatar>
                  <q-icon name="light_mode" size="xs" />
                </q-item-section>
                <q-item-section>{{ t('common.lightMode') }}</q-item-section>
              </q-item>
              <q-item
                v-close-popup
                clickable
                :active="themeMode === 'dark'"
                active-class="text-primary"
                @click="setTheme('dark')"
              >
                <q-item-section avatar>
                  <q-icon name="dark_mode" size="xs" />
                </q-item-section>
                <q-item-section>{{ t('common.darkMode') }}</q-item-section>
              </q-item>
              <q-separator />
              <q-item-label header class="text-caption">{{ t('shell.prefs.title') }}</q-item-label>
              <q-item
                clickable
                :active="a11yPrefs.density === 'compact'"
                active-class="text-primary"
                @click="toggleDensity"
              >
                <q-item-section avatar>
                  <q-icon name="compress" size="xs" />
                </q-item-section>
                <q-item-section>{{ t('shell.prefs.density') }}</q-item-section>
                <q-item-section v-if="a11yPrefs.density === 'compact'" side>
                  <q-icon name="check" size="xs" />
                </q-item-section>
              </q-item>
              <q-item
                clickable
                :active="a11yPrefs.colorvision === 'safe'"
                active-class="text-primary"
                @click="toggleColorvision"
              >
                <q-item-section avatar>
                  <q-icon name="visibility" size="xs" />
                </q-item-section>
                <q-item-section>{{ t('shell.prefs.colorvision') }}</q-item-section>
                <q-item-section v-if="a11yPrefs.colorvision === 'safe'" side>
                  <q-icon name="check" size="xs" />
                </q-item-section>
              </q-item>
              <q-item
                clickable
                :active="a11yPrefs.motion === 'reduced'"
                active-class="text-primary"
                @click="toggleMotion"
              >
                <q-item-section avatar>
                  <q-icon name="animation" size="xs" />
                </q-item-section>
                <q-item-section>{{ t('shell.prefs.motion') }}</q-item-section>
                <q-item-section v-if="a11yPrefs.motion === 'reduced'" side>
                  <q-icon name="check" size="xs" />
                </q-item-section>
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
          :aria-label="t('uiSystem.language')"
        />
      </q-toolbar>
    </q-header>

    <q-drawer
      v-model="drawerOpen"
      show-if-above
      :width="256"
      :mini="$q.screen.gt.sm && drawerMini"
      :mini-width="80"
      :breakpoint="1023"
      :class="['app-sidebar', { 'q-drawer--design-mini': drawerMini }]"
    >
      <nav id="app-navigation" class="app-sidebar-card" :aria-label="t('uiSystem.mainMenu')">
        <div v-show="!drawerMini" class="app-sidebar__filter">
          <q-input
            v-model="navFilter"
            class="app-sidebar-search"
            dense
            borderless
            clearable
            :placeholder="t('shell.nav.filterPlaceholder')"
            :aria-label="t('shell.nav.filterPlaceholder')"
          >
            <template #prepend>
              <q-icon name="search" size="16px" />
            </template>
          </q-input>
        </div>
        <div class="app-sidebar__scroll fit">
          <!-- 筛选态：扁平结果，附带所属分组，避免用户丢失上下文 -->
          <q-list v-if="!drawerMini && navFilter.trim()" class="app-sidebar-nav" dense>
            <q-item
              v-for="leaf in filterResults"
              :key="`filter-${leaf.to}`"
              v-ripple
              clickable
              class="app-sidebar-nav-item"
              :active="isNavItemActive(leaf)"
              active-class="app-sidebar-item--active"
              @click="navigateTo(leaf.to)"
            >
              <q-item-section avatar>
                <q-icon :name="leaf.icon" />
              </q-item-section>
              <q-item-section>
                <q-item-label>{{ t(leaf.labelKey) }}</q-item-label>
                <q-item-label caption class="text-caption">
                  {{ t(leaf.groupKey)
                  }}<template v-if="leaf.sectionLabelKey"> · {{ t(leaf.sectionLabelKey) }}</template>
                </q-item-label>
              </q-item-section>
            </q-item>
            <q-item v-if="filterResults.length === 0">
              <q-item-section class="text-caption text-grey">{{ t('shell.nav.noMatch') }}</q-item-section>
            </q-item>
          </q-list>
          <!--
            八个一级去处（docs/aranea-agents/design/2026-11-17-导航重组方案.md）。
            展开：手风琴，同时只展开一个领域；收起：图标栏，悬停或点击浮出本领域的页面。
          -->
          <q-list
            v-else
            class="app-sidebar-nav app-sidebar-domains"
            :class="{ 'app-sidebar-domains--rail': drawerMini }"
          >
            <template v-if="!drawerMini && favoriteLeaves.length">
              <q-item-label header class="app-sidebar-section-label">
                <span>{{ t('shell.nav.favorites') }}</span>
                <span class="app-sidebar-section-count">{{ favoriteLeaves.length }}</span>
              </q-item-label>
              <q-item
                v-for="item in favoriteLeaves"
                :key="`fav-${item.to}`"
                clickable
                class="app-sidebar-nav-item"
                :active="isNavItemActive(item)"
                active-class="app-sidebar-item--active"
                @click="navigateTo(item.to)"
              >
                <q-item-section avatar><q-icon :name="item.icon" /></q-item-section>
                <q-item-section>{{ t(item.labelKey) }}</q-item-section>
              </q-item>
              <q-separator class="app-sidebar-divider" />
            </template>

            <div
              v-for="domain in navDomains"
              :key="domain.key"
              class="app-sidebar-domain"
              :class="{
                'app-sidebar-domain--active': activeDomainKey === domain.key,
                'app-sidebar-domain--open': !drawerMini && openDomain === domain.key,
              }"
              @mouseenter="drawerMini && openFlyout(domain.key)"
              @mouseleave="drawerMini && scheduleFlyoutClose()"
            >
              <button
                type="button"
                class="app-sidebar-domain__head"
                :aria-expanded="drawerMini ? flyout === domain.key : openDomain === domain.key"
                :aria-current="activeDomainKey === domain.key ? 'true' : undefined"
                :aria-label="t(domain.labelKey)"
                @click="onDomainClick(domain.key)"
              >
                <span class="app-sidebar-domain__icon">
                  <q-icon :name="domain.icon" size="20px" aria-hidden="true" />
                  <span
                    v-if="domain.key === 'workbench' && inboxBadge > 0"
                    class="app-sidebar-badge app-sidebar-badge--corner"
                    aria-hidden="true"
                    >{{ inboxBadge > 99 ? '99+' : inboxBadge }}</span
                  >
                </span>
                <span class="app-sidebar-domain__label">{{
                  t(drawerMini ? domain.shortLabelKey : domain.labelKey)
                }}</span>
                <q-icon
                  v-if="!drawerMini"
                  class="app-sidebar-domain__chevron"
                  :name="openDomain === domain.key ? 'expand_less' : 'expand_more'"
                  size="18px"
                  aria-hidden="true"
                />
              </button>

              <q-menu
                v-if="drawerMini"
                :model-value="flyout === domain.key"
                no-parent-event
                anchor="top right"
                self="top left"
                :offset="[18, 0]"
                class="app-sidebar-flyout"
                @update:model-value="(open: boolean) => !open && flyout === domain.key && closeFlyout()"
                @mouseenter="cancelFlyoutClose"
                @mouseleave="scheduleFlyoutClose"
              >
                <div class="app-sidebar-flyout__title">{{ t(domain.labelKey) }}</div>
                <q-list dense>
                  <q-item
                    v-for="item in domain.primaries"
                    :key="item.to"
                    v-close-popup
                    clickable
                    class="app-sidebar-flyout__item"
                    :active="activePrimaryPath === item.to"
                    active-class="app-sidebar-item--active"
                    @click="navigateTo(item.to)"
                  >
                    <q-item-section avatar><q-icon :name="item.icon" size="18px" /></q-item-section>
                    <q-item-section>{{ t(item.tabLabelKey) }}</q-item-section>
                  </q-item>
                </q-list>
              </q-menu>

              <div v-if="!drawerMini" v-show="openDomain === domain.key" class="app-sidebar-domain__items">
                <q-item
                  v-for="item in domain.primaries"
                  :key="item.to"
                  clickable
                  class="app-sidebar-nav-item"
                  :active="activePrimaryPath === item.to"
                  active-class="app-sidebar-item--active"
                  @click="navigateTo(item.to)"
                >
                  <q-item-section avatar><q-icon :name="item.icon" /></q-item-section>
                  <q-item-section>
                    <span>
                      {{ t(item.tabLabelKey) }}
                      <span
                        v-if="item.to === '/workbench' && inboxBadge > 0"
                        class="app-sidebar-badge"
                        :aria-label="t('today.inbox.title') + ' ' + inboxBadge"
                        >{{ inboxBadge > 99 ? '99+' : inboxBadge }}</span
                      >
                    </span>
                  </q-item-section>
                  <q-item-section side>
                    <q-btn
                      flat
                      round
                      dense
                      class="app-sidebar-favorite"
                      :class="{ 'app-sidebar-favorite--active': isFavorite(item.to) }"
                      :icon="isFavorite(item.to) ? 'star' : 'star_border'"
                      :aria-label="t(isFavorite(item.to) ? 'shell.nav.removeFavorite' : 'shell.nav.addFavorite')"
                      @click.stop="toggleFavorite(item.to)"
                    />
                  </q-item-section>
                </q-item>
              </div>
            </div>
          </q-list>
        </div>
      </nav>
    </q-drawer>

    <q-page-container
      id="app-main-content"
      class="app-page-container"
      :data-layout="pageLayout"
      :data-visual="pageVisual"
      tabindex="-1"
    >
      <AppDomainTabs v-if="showDomainTabs" :trail="domainTrail" />
      <div v-else-if="showBreadcrumbs" class="app-breadcrumb-bar">
        <AppBreadcrumbs />
      </div>
      <router-view />
    </q-page-container>

    <GlobalCommandPalette v-model="paletteOpen" />
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
import { findNavTrail, flatNavLeaves, navLeafVisible } from '../config/navView';
import { buildNavDomains, findDomainTrail, type NavDomainKey } from '../config/navDomains';
import AppDomainTabs from '../components/layout/AppDomainTabs.vue';
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
// 侧栏收起（图标栏）/ 展开（手风琴）两种形态，按人记住
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
const navFilter = ref('');

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

// 手风琴：同时只展开一个领域；换页时跟着展开当前领域
const openDomain = ref<NavDomainKey | ''>('');
watch(
  activeDomainKey,
  (key) => {
    if (key) openDomain.value = key;
  },
  { immediate: true },
);

// 图标栏：悬停或点击浮出本领域页面；移出后稍等再收，避免从图标移到浮层时闪断
const flyout = ref<NavDomainKey | ''>('');
let flyoutTimer: ReturnType<typeof setTimeout> | null = null;
function cancelFlyoutClose() {
  if (flyoutTimer) clearTimeout(flyoutTimer);
  flyoutTimer = null;
}
function openFlyout(key: NavDomainKey) {
  cancelFlyoutClose();
  flyout.value = key;
}
function closeFlyout() {
  cancelFlyoutClose();
  flyout.value = '';
}
function scheduleFlyoutClose() {
  cancelFlyoutClose();
  flyoutTimer = setTimeout(closeFlyout, 180);
}
function onDomainClick(key: NavDomainKey) {
  if (drawerMini.value) {
    if (flyout.value === key) closeFlyout();
    else openFlyout(key);
    return;
  }
  openDomain.value = openDomain.value === key ? '' : key;
}

const filterResults = computed(() => {
  const keyword = navFilter.value.trim().toLowerCase();
  if (!keyword) return [];
  return flatNavLeaves()
    .filter((leaf) => navLeafVisible(leaf, nav.isPathVisible))
    .filter((leaf) =>
      [t(leaf.labelKey), t(leaf.groupKey), leaf.sectionLabelKey ? t(leaf.sectionLabelKey) : '', leaf.to].some((field) =>
        field.toLowerCase().includes(keyword),
      ),
    );
});

// 当前路由在导航树中的位置：用于高亮、自动展开与面包屑归属。
const navTrail = computed(() => findNavTrail(route.path));
const activeNavPath = computed(() => navTrail.value?.item.to ?? '');

function isNavItemActive(item: { to: string }) {
  return activeNavPath.value === item.to;
}

const showBreadcrumbs = computed(() => route.meta.breadcrumb !== false);
// 全屏类页面（聊天、实时运转、经营看板等）自己占满视口，不插页签条，靠侧栏切换
const showDomainTabs = computed(
  () => Boolean(domainTrail.value) && pageLayout.value !== 'immersive' && pageLayout.value !== 'canvas',
);
const pageLayout = computed(() => resolveUiLayout(route.meta.uiLayout));
// 视觉档位：指挥舱（deck）保留光效与网格底，其余为安静的工作区
const pageVisual = computed(() => resolveUiVisual(route.meta.uiVisual));
const { favorites, toggleFavorite, recordVisit, isFavorite } = useNavPreferences();

// 收藏留在展开形态的侧栏顶部；「最近访问」改由 Ctrl+K 指令面板提供
const favoriteLeaves = computed(() =>
  favorites.value
    .map((path) => flatNavLeaves().find((leaf) => leaf.to === path && navLeafVisible(leaf, nav.isPathVisible)))
    .filter((leaf): leaf is NonNullable<typeof leaf> => Boolean(leaf)),
);

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
  cancelFlyoutClose();
});

function onGlobalKeydown(event: KeyboardEvent) {
  if (!(event.ctrlKey || event.metaKey) || event.key.toLowerCase() !== 'k') return;
  event.preventDefault();
  paletteOpen.value = true;
}

watch(
  () => route.path,
  () => {
    closeFlyout();
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
