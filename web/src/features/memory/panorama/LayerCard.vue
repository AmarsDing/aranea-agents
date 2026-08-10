// Container: approved — 层级全景单层卡片；数据来自 MemoryPanoramaTab via props。
<template>
  <div class="layer-card" :style="cardStyle" @click="emit('drill', stat.layer)">
    <div class="layer-card__header">
      <q-avatar size="32px" :style="{ background: meta.color }" text-color="white" :icon="meta.icon" />
      <div class="col">
        <div class="text-subtitle2 text-weight-bold">
          {{ t(`memory.panorama.layers.${stat.layer}.name`) }}
          <span class="layer-card__key">{{ stat.layer }}</span>
        </div>
      </div>
      <q-chip
        dense
        square
        :color="stat.health === 'ok' ? 'positive' : 'warning'"
        text-color="white"
        class="layer-card__health"
      >
        {{ t(`memory.panorama.health.${stat.health === 'ok' ? 'ok' : 'warn'}`) }}
      </q-chip>
    </div>

    <div class="layer-card__caption text-caption text-grey-7">
      {{ t(`memory.panorama.layers.${stat.layer}.caption`) }}
    </div>

    <div class="layer-card__count">{{ stat.item_count }}</div>

    <div class="layer-card__stats text-caption">
      <span>
        <q-icon name="south" size="12px" />{{ t('memory.panorama.todayAdded') }}
        <b>{{ stat.today_added }}</b>
      </span>
      <span v-if="stat.layer === 'L3'">
        <q-icon name="north" size="12px" />{{ t('memory.panorama.recallHits') }}
        <b>{{ stat.recall_hits }}</b>
      </span>
    </div>

    <div v-if="headlineChips.length" class="layer-card__headline">
      <q-chip v-for="chip in headlineChips" :key="chip.label" dense outline size="sm" :color="chip.color">
        {{ chip.label }}
      </q-chip>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { MemoryLayerStat } from '../../types';
import { MEMORY_LAYER_META, type MemoryLayerKey } from './layerMeta';

const props = defineProps<{ stat: MemoryLayerStat }>();
const emit = defineEmits<{ (e: 'drill', layer: string): void }>();

const { t } = useI18n();

const meta = computed(() => MEMORY_LAYER_META[props.stat.layer as MemoryLayerKey] ?? MEMORY_LAYER_META.L0);
const cardStyle = computed(() => ({ borderTopColor: meta.value.color }));

const headline = computed<Record<string, unknown>>(() => {
  try {
    const parsed = JSON.parse(props.stat.headline_json || '{}');
    return parsed && typeof parsed === 'object' ? (parsed as Record<string, unknown>) : {};
  } catch {
    return {};
  }
});

const headlineChips = computed(() => {
  const h = headline.value;
  const chips: Array<{ label: string; color: string }> = [];
  if (typeof h.context_usage_pct === 'number') {
    const status = typeof h.compress_status === 'string' ? h.compress_status : 'normal';
    chips.push({
      label: `${t('memory.panorama.headline.contextUsage')} ${h.context_usage_pct}%`,
      color: status === 'normal' ? 'grey-7' : 'warning',
    });
  }
  if (typeof h.active_tasks === 'number') {
    chips.push({ label: `${t('memory.panorama.headline.activeTasks')} ${h.active_tasks}`, color: 'grey-7' });
  }
  if (typeof h.field_count === 'number' && h.field_count > 0) {
    chips.push({ label: `${t('memory.panorama.headline.fieldCount')} ${h.field_count}`, color: 'grey-7' });
  }
  if (typeof h.conflict_open === 'number' && h.conflict_open > 0) {
    chips.push({ label: `${t('memory.panorama.headline.conflictOpen')} ${h.conflict_open}`, color: 'negative' });
  }
  if (typeof h.relation_count === 'number') {
    chips.push({ label: `${t('memory.panorama.headline.relationCount')} ${h.relation_count}`, color: 'grey-7' });
  }
  return chips;
});
</script>

<style scoped>
.layer-card {
  background: var(--glass-surface);
  border: 1px solid var(--glass-border);
  border-top: 3px solid transparent;
  border-radius: 14px;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 180px;
  padding: 12px 14px;
  transition:
    box-shadow 0.2s ease,
    transform 0.2s ease;
}

.layer-card:hover {
  box-shadow: 0 6px 18px rgb(0 0 0 / 12%);
  transform: translateY(-2px);
}

.layer-card__header {
  align-items: center;
  display: flex;
  gap: 8px;
}

.layer-card__key {
  color: var(--color-text-secondary);
  font-size: 11px;
  font-weight: 400;
  margin-left: 4px;
}

.layer-card__health {
  margin-left: auto;
}

.layer-card__count {
  font-size: 28px;
  font-weight: 700;
  line-height: 1.1;
}

.layer-card__stats {
  color: var(--color-text-secondary);
  display: flex;
  gap: 12px;
}

.layer-card__stats b {
  margin-left: 2px;
}

.layer-card__headline {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
</style>
