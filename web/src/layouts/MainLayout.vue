<template>
  <q-layout view="hHh LpR fFf" class="app-layout-root">
    <q-header
      :elevated="false"
      :class="isDark ? 'dark-header' : 'cream-header'"
    >
      <q-toolbar class="q-px-sm-md">
        <q-btn
          flat
          dense
          round
          icon="menu"
          aria-label="Menu"
          @click="drawerOpen = !drawerOpen"
        />
        <q-btn
          v-if="isDesktop"
          flat
          dense
          round
          :icon="drawerMini ? 'chevron_right' : 'chevron_left'"
          :aria-label="t('common.expandSidebar')"
          @click="drawerMini = !drawerMini"
        />
        <q-toolbar-title class="q-pl-sm app-toolbar-title text-weight-bold">
          {{ t("common.appTitle") }}
        </q-toolbar-title>
        <q-space />
        <div v-if="isDesktop" class="row items-center q-gutter-sm q-mr-sm app-header-actions">
          <q-btn round flat class="app-header-icon-btn" icon="notifications_none" size="md" />
          <q-btn round flat dense class="cursor-pointer">
            <q-avatar size="36px" class="app-header-avatar" font-size="14px">
              {{ auth.avatarLetter }}
            </q-avatar>
            <q-menu anchor="bottom right" self="top right">
              <q-list dense style="min-width: 180px">
                <q-item>
                  <q-item-section class="text-caption text-grey">{{ auth.displayLabel }}</q-item-section>
                </q-item>
                <q-separator />
                <q-item v-close-popup clickable @click="onLogout">
                  <q-item-section>{{ t("auth.logout") }}</q-item-section>
                </q-item>
              </q-list>
            </q-menu>
          </q-btn>
        </div>
        <q-btn
          flat
          round
          :icon="isDark ? 'light_mode' : 'dark_mode'"
          :aria-label="isDark ? t('common.lightMode') : t('common.darkMode')"
          @click="toggleTheme"
        />
        <q-select
          v-model="locale"
          :options="localeOptions"
          emit-value
          map-options
          dense
          :dark="isDark"
          borderless
          options-dense
          class="app-header-locale q-ml-sm"
          style="min-width: 110px"
        />
      </q-toolbar>
    </q-header>

    <q-drawer
      v-model="drawerOpen"
      show-if-above
      :width="256"
      :mini="drawerMini"
      :mini-width="90"
      :breakpoint="1024"
      :class="['app-sidebar', { 'q-drawer--design-mini': drawerMini }]"
    >
      <div class="app-sidebar-card">
        <div class="app-sidebar__scroll fit">
        <q-list class="app-sidebar-nav" :dense="drawerMini">
          <template v-for="(group, gi) in sideNavGroups" :key="`g-${gi}`">
            <q-item-label
              v-show="!drawerMini"
              header
              class="app-sidebar-section-label"
            >
              {{ t(group.labelKey) }}
            </q-item-label>
            <div class="app-sidebar-group__items">
              <q-item
                v-for="(item, ii) in group.items"
                v-ripple
                :key="`g-${gi}-i-${ii}`"
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
            <q-separator
              v-if="gi < sideNavGroups.length - 1"
              v-show="!drawerMini"
              class="app-sidebar-divider"
            />
          </template>
        </q-list>
        </div>
      </div>
    </q-drawer>

    <q-page-container class="app-page-container">
      <router-view />
    </q-page-container>
  </q-layout>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { useQuasar } from "quasar";
import { setQuasarLangFor } from "../i18n/quasar-lang";
import { sideNavGroups } from "../config/sideNav";
import { useAuthStore } from "../stores/auth";

const { t, locale } = useI18n();
const $q = useQuasar();
const route = useRoute();
const router = useRouter();
const auth = useAuthStore();
const drawerOpen = ref(true);
const drawerMini = ref(true);

const isDesktop = computed(() => $q.screen.gt.xs);
const isDark = computed(() => $q.dark.isActive);

const localeOptions = [
  { label: "中文", value: "zh-CN" as const },
  { label: "English", value: "en-US" as const }
];

watch(
  () => $q.dark.isActive,
  (on) => {
    if (typeof localStorage === "undefined") return;
    localStorage.setItem("theme", on ? "dark" : "light");
  }
);

watch(locale, (v) => {
  setQuasarLangFor(String(v));
  if (v === "zh-CN" || v === "en-US") {
    localStorage.setItem("locale", v);
  }
});

function toggleTheme() {
  $q.dark.toggle();
}

function isNavItemActive(item: { to: string; exact?: boolean }) {
  return item.exact === false ? route.path.startsWith(item.to) : route.path === item.to;
}

async function navigateTo(path: string) {
  if (route.path === path) return;
  await router.push(path);
}

async function onLogout() {
  await auth.logout();
  await router.push("/login");
}
</script>
