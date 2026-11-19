<script setup lang="ts">
// 平台健康灯条（整合 P2-8，自 ceo-dashboard TopBar.vue 的平台灯区移植；
// 品牌/登录/视图切换已由 MainLayout 与路由承担，不再重复）。
import { useI18n } from 'vue-i18n';
import type { PlatformView } from '../../features/ceoboard/types';
import { platformName } from '../../features/ceoboard/friendly';

defineProps<{ platforms: PlatformView[] }>();
const { t } = useI18n();

function fmt(at?: string): string {
  if (!at || new Date(at).getFullYear() < 2000) return t('ceoboard.common.neverOk');
  return new Date(at).toLocaleTimeString();
}
function tip(p: PlatformView): string {
  const lines = [t('ceoboard.chips.tip', { id: p.id, at: fmt(p.last_ok_at) })];
  if (p.last_err) lines.push(t('ceoboard.chips.tipErr', { msg: p.last_err }));
  return lines.join('\n');
}
</script>

<template>
  <div class="ceo-chips">
    <div v-for="p in platforms" :key="p.id" class="ceo-plat-chip" :class="{ down: p.health === 'red' }" :title="tip(p)">
      <span class="ceo-dot" :class="p.health"></span>
      <span>{{ platformName(p.id, p.name) }}</span>
    </div>
  </div>
</template>
