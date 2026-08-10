// Container: approved — L2 情景时间线；useMemoryEpisodes 拉取数据，卡片纯展示 + 点击展开摘要。
<template>
  <q-card flat class="memory-card">
    <q-card-section class="row items-center no-wrap">
      <div>
        <div class="text-h6">{{ t('memory.episodes.title') }}</div>
        <div class="text-caption text-grey-7">{{ t('memory.episodes.caption') }}</div>
      </div>
      <q-space />
      <q-chip dense square size="sm" class="episode-count-chip" text-color="white">
        {{ t('memory.episodes.showing', { shown: items.length, total }) }}
      </q-chip>
    </q-card-section>

    <q-banner v-if="error" rounded class="bg-negative text-white q-mx-md q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat color="white" :label="t('memory.error.retry')" @click="reload" />
      </template>
    </q-banner>

    <div v-if="loading && !items.length" class="text-center q-pa-xl">
      <q-spinner-dots size="40px" color="primary" />
    </div>

    <q-card-section v-else-if="!items.length" class="text-grey-7 text-center q-pa-xl">
      <q-icon name="timeline" size="40px" class="q-mb-sm" />
      <div>{{ t('memory.episodes.empty') }}</div>
    </q-card-section>

    <q-list v-else separator class="episode-timeline">
      <q-item v-for="ep in items" :key="ep.id" clickable class="episode-item" @click="toggleExpand(ep.id)">
        <q-item-section avatar top>
          <q-avatar size="30px" :style="{ background: layerColor }" text-color="white" icon="timeline" />
        </q-item-section>
        <q-item-section>
          <q-item-label class="row items-center no-wrap q-gutter-x-sm">
            <span class="text-weight-medium ellipsis">{{ ep.title || t('memory.episodes.untitled') }}</span>
            <q-chip
              v-if="ep.consolidation_status === 'consolidated'"
              dense
              square
              size="sm"
              color="positive"
              text-color="white"
              icon="check_circle"
            >
              {{ t('memory.episodes.status.consolidated', { count: ep.consolidated_l3_count }) }}
            </q-chip>
            <q-chip v-else dense square size="sm" class="episode-pending-chip" text-color="white" icon="hourglass_top">
              {{ t('memory.episodes.status.pending') }}
            </q-chip>
          </q-item-label>
          <q-item-label caption :lines="isExpanded(ep.id) ? undefined : 2" class="episode-summary">
            {{ ep.outcome_summary || t('memory.episodes.noSummary') }}
          </q-item-label>
          <q-item-label caption class="row items-center q-gutter-x-sm q-mt-xs">
            <q-rating :model-value="importanceStars(ep.importance)" size="14px" color="amber" icon="star" readonly />
            <span class="text-grey-6">{{ ep.episode_kind }}</span>
          </q-item-label>
        </q-item-section>
        <q-item-section side top>
          <span class="text-caption text-grey-6">{{ relativeTime(ep.created_at) }}</span>
          <q-icon :name="isExpanded(ep.id) ? 'expand_less' : 'expand_more'" size="18px" color="grey-6" />
        </q-item-section>
      </q-item>
    </q-list>

    <q-card-actions v-if="hasMore" align="center" class="q-pb-md">
      <q-btn flat color="primary" :label="t('memory.episodes.loadMore')" :loading="loadingMore" @click="loadMore" />
    </q-card-actions>
  </q-card>
</template>

<script setup lang="ts">
import { ref, toRef } from 'vue';
import { useI18n } from 'vue-i18n';
import { relativeTime } from '../../graph/utils';
import { memoryLayerColor } from '../panorama/layerMeta';
import { useMemoryEpisodes } from './composables/useMemoryEpisodes';

const props = defineProps<{ agentId: string | null; sessionId: string | null }>();

const { t } = useI18n();
const { items, total, loading, loadingMore, error, hasMore, reload, loadMore } = useMemoryEpisodes(
  toRef(props, 'agentId'),
  toRef(props, 'sessionId'),
);

const layerColor = memoryLayerColor('L2');

const expandedIds = ref<Set<string>>(new Set());

function toggleExpand(id: string) {
  const next = new Set(expandedIds.value);
  if (next.has(id)) {
    next.delete(id);
  } else {
    next.add(id);
  }
  expandedIds.value = next;
}

function isExpanded(id: string): boolean {
  return expandedIds.value.has(id);
}

/** importance ∈ [0,1] → 0~5 星。 */
function importanceStars(value: number): number {
  const clamped = Math.max(0, Math.min(1, Number(value) || 0));
  return Math.round(clamped * 5 * 2) / 2;
}
</script>

<style scoped>
.episode-timeline {
  max-height: 560px;
  overflow-y: auto;
}

.episode-count-chip {
  background: #4db6ac;
}

.episode-pending-chip {
  background: #f2a541;
  animation: episode-pulse 1.6s ease-in-out infinite;
}

.episode-summary {
  white-space: pre-wrap;
}

@keyframes episode-pulse {
  0%,
  100% {
    opacity: 100%;
  }

  50% {
    opacity: 55%;
  }
}
</style>
