<script setup lang="ts">
// 待拍板面板（整合 P2-8，自 ceo-dashboard ApprovalsPanel.vue 移植）——值班视图保留。
import { useI18n } from 'vue-i18n';
import type { ApprovalsView } from '../../features/ceoboard/types';

defineProps<{ approvals: ApprovalsView }>();
const { t } = useI18n();

function fmt(at?: string): string {
  if (!at) return t('ceoboard.common.neverOk');
  return new Date(at).toLocaleTimeString();
}
</script>

<template>
  <div class="ceo-approvals">
    <h2>
      {{ t('ceoboard.approvalsPanel.title') }}
      <span class="ceo-tag ceo-mono">{{ approvals.count }}</span>
      <span class="ceo-dim ceo-approvals-asof">· {{ t('ceoboard.approvalsPanel.asOf', { at: fmt(approvals.last_ok_at) }) }}</span>
    </h2>
    <div v-if="approvals.note" class="ceo-reason" :class="approvals.health">
      ⚠ {{ t('ceoboard.approvalsPanel.noteLine', { note: approvals.note, at: fmt(approvals.last_ok_at) }) }}
    </div>
    <div v-if="!approvals.items.length && !approvals.note" class="ceo-empty">{{ t('ceoboard.approvalsPanel.empty') }}</div>
    <div v-for="(it, i) in approvals.items" :key="it.lane + it.id + i" class="ceo-approval-item">
      <span class="ceo-tag">{{ it.lane === 'publish' ? t('ceoboard.decide.lanePublish') : t('ceoboard.decide.laneListing') }}</span>
      <span>{{ it.title || it.id }}</span>
      <span class="ceo-dim ceo-mono">{{ it.extra }}</span>
    </div>
  </div>
</template>
