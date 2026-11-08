<script setup lang="ts">
// 原始告警条（整合 P2-8，自 ceo-dashboard AlertList.vue 移植）——值班视图保留。
import { useI18n } from 'vue-i18n';
import type { AlertView } from '../../features/ceoboard/types';

defineProps<{ alerts: AlertView[] }>();
const { t } = useI18n();
</script>

<template>
  <div v-if="alerts.length" class="ceo-alerts">
    <!-- 红色恒顶:sticky 定位,不随刷新沉底(§7.3/§15.1) -->
    <div v-for="a in alerts" :key="a.id" class="ceo-alert" :class="a.level" :role="a.level === 'red' ? 'alert' : undefined">
      <span class="lvl">{{ a.level === 'red' ? t('ceoboard.alertList.lvlRed') : t('ceoboard.alertList.lvlYellow') }}</span>
      <span class="ceo-mono ceo-dim">{{ a.target }}</span>
      <span>{{ a.text }}</span>
    </div>
  </div>
</template>
