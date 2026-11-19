<script setup lang="ts">
// 老板视图 / 值班视图切换（整合 P2-8）：原 App.vue 的内存态 viewMode 在
// aranea 落地为两条路由（/ceo/overview、/ceo/oncall），同一口径两个入口。
// 无障碍（AGENTS.md §6.11）：tablist 语义 + 方向键/Home/End 切换 + 漫游 tabindex。
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRoute, useRouter } from 'vue-router';

const { t } = useI18n();
const route = useRoute();
const router = useRouter();

const tabs = [
  { name: 'ceo-overview', labelKey: 'ceoboard.page.viewBoss' },
  { name: 'ceo-oncall', labelKey: 'ceoboard.page.viewOncall' },
] as const;
type TabName = (typeof tabs)[number]['name'];

const tabRefs = ref<HTMLButtonElement[]>([]);

function go(name: TabName) {
  if (route.name !== name) void router.push({ name });
}

// ARIA tabs 键盘合同：←/→ 循环移动并激活，Home/End 跳首尾；激活后焦点跟随。
function onKeydown(e: KeyboardEvent) {
  const idx = tabs.findIndex((tab) => tab.name === route.name);
  let next: number;
  if (e.key === 'ArrowLeft') next = (idx - 1 + tabs.length) % tabs.length;
  else if (e.key === 'ArrowRight') next = (idx + 1) % tabs.length;
  else if (e.key === 'Home') next = 0;
  else if (e.key === 'End') next = tabs.length - 1;
  else return;
  e.preventDefault();
  if (next === idx) return;
  go(tabs[next].name);
  tabRefs.value[next]?.focus();
}
</script>

<template>
  <div class="ceo-viewswitch" role="tablist" @keydown="onKeydown">
    <button
      v-for="tab in tabs"
      :key="tab.name"
      ref="tabRefs"
      role="tab"
      :aria-selected="route.name === tab.name"
      :tabindex="route.name === tab.name ? 0 : -1"
      :class="{ on: route.name === tab.name }"
      @click="go(tab.name)"
    >
      {{ t(tab.labelKey) }}
    </button>
  </div>
</template>
