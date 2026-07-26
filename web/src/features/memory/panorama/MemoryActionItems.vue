// Container: approved — 需要关注待办列表；数据来自 MemoryPanoramaTab via props。
<template>
  <q-card flat class="memory-card">
    <q-card-section>
      <div class="text-h6">{{ t('memory.panorama.actionTitle') }}</div>
      <div class="text-caption text-grey-7">{{ t('memory.panorama.actionCaption') }}</div>
    </q-card-section>
    <q-list v-if="items.length" separator>
      <q-item v-for="item in items" :key="item.kind" clickable @click="emit('navigate', item)">
        <q-item-section avatar>
          <q-avatar :color="kindMeta[item.kind]?.color ?? 'grey'" text-color="white" :icon="kindMeta[item.kind]?.icon ?? 'info'" />
        </q-item-section>
        <q-item-section>
          <q-item-label>{{ t(`memory.panorama.actions.${item.kind}.title`) }}</q-item-label>
          <q-item-label caption>{{ t(`memory.panorama.actions.${item.kind}.caption`) }}</q-item-label>
        </q-item-section>
        <q-item-section side>
          <q-chip dense :color="kindMeta[item.kind]?.color ?? 'grey'" text-color="white">{{ item.count }}</q-chip>
        </q-item-section>
      </q-item>
    </q-list>
    <q-card-section v-else class="text-grey-7 text-center q-pa-lg">
      <q-icon name="check_circle" size="28px" color="positive" class="q-mb-sm" />
      <div>{{ t('memory.panorama.actionEmpty') }}</div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { MemoryActionItem } from '../../types';

defineProps<{ items: MemoryActionItem[] }>();
const emit = defineEmits<{ (e: 'navigate', item: MemoryActionItem): void }>();

const { t } = useI18n();

const kindMeta: Record<string, { icon: string; color: string }> = {
  fact_conflict: { icon: 'rule', color: 'negative' },
  evolution_pending: { icon: 'auto_awesome', color: 'info' },
  context_risk: { icon: 'report', color: 'warning' },
};
</script>
