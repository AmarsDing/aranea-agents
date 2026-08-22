// Container: approved — feature-local panel/dialog; data from Page composable via props.
<template>
  <q-drawer
    :model-value="modelValue"
    side="right"
    overlay
    bordered
    :width="600"
    class="memory-drawer"
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <q-scroll-area class="fit">
      <div class="q-pa-md">
        <div class="row items-center justify-between q-mb-md">
          <div>
            <div class="text-h6">{{ t('memory.snapshotDrawer.title') }}</div>
            <div class="text-caption text-grey-7">{{ snapshot?.id }}</div>
          </div>
          <q-btn
            flat
            round
            icon="close"
            :aria-label="t('memory.snapshotDrawer.closeAria')"
            @click="$emit('update:modelValue', false)"
          />
        </div>
        <memory-l0-waterfall :bars="waterfallBars" class="q-mb-md" />
        <AppRegistryTable
          :shell="false"
          :resizable="false"
          :rows="segmentRows"
          :columns="segmentColumns"
          row-key="section"
          hide-pagination
          :pagination="{ rowsPerPage: 0 }"
          column-persist-key="memory-snapshot-segments"
        >
          <template #body-cell-token_estimate="slotProps">
            <q-td :props="slotProps">
              {{ slotProps.row.token_estimate.toLocaleString() }}
            </q-td>
          </template>
          <template #body-cell-detail="slotProps">
            <q-td :props="slotProps">
              <span v-if="slotProps.row.field_count != null" class="text-caption text-grey-7">
                {{ t('memory.snapshotDrawer.fields', { count: slotProps.row.field_count }) }}
              </span>
              <span v-if="slotProps.row.fact_count != null" class="text-caption text-grey-7">
                {{ t('memory.snapshotDrawer.facts', { count: slotProps.row.fact_count }) }}
              </span>
              <span v-if="slotProps.row.entity_count != null" class="text-caption text-grey-7">
                {{ t('memory.snapshotDrawer.entities', { count: slotProps.row.entity_count }) }}
              </span>
              <span v-if="slotProps.row.result_count != null" class="text-caption text-grey-7">
                {{ t('memory.snapshotDrawer.results', { count: slotProps.row.result_count }) }}
              </span>
              <span v-if="slotProps.row.turn_count != null" class="text-caption text-grey-7">
                {{ t('memory.snapshotDrawer.turns', { count: slotProps.row.turn_count }) }}
              </span>
              <span
                v-if="slotProps.row.from_turn != null && slotProps.row.to_turn != null"
                class="text-caption text-grey-7"
              >
                T{{ slotProps.row.from_turn }}–T{{ slotProps.row.to_turn }}
              </span>
            </q-td>
          </template>
          <template #no-data>
            <div class="full-width column items-center q-pa-md text-grey-7">
              <q-icon name="segment" size="32px" />
              <div class="q-mt-sm">{{ t('memory.snapshotDrawer.empty') }}</div>
            </div>
          </template>
        </AppRegistryTable>
        <div v-if="totalTokens > 0" class="q-mt-sm text-caption text-grey-7">
          {{ t('memory.snapshotDrawer.totalTokens', { count: totalTokens.toLocaleString() }) }}
        </div>
      </div>
    </q-scroll-area>
  </q-drawer>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppRegistryTable from '../../components/layout/AppRegistryTable.vue';
import MemoryL0Waterfall from '../../components/memory/MemoryL0Waterfall.vue';
import { buildL0Waterfall } from './l0Waterfall';
import { buildMemorySnapshotSegmentColumns, type MemorySnapshotSegmentRow } from './memoryTableUi';
import type { L0AssemblySegmentsMap, L0AssemblySnapshot } from './types';

const { t } = useI18n();

const props = defineProps<{
  modelValue: boolean;
  snapshot: L0AssemblySnapshot | null;
}>();

defineEmits<{
  'update:modelValue': [value: boolean];
}>();

const segmentColumns = computed(() => buildMemorySnapshotSegmentColumns(t));

const segmentsMap = computed<L0AssemblySegmentsMap>(() =>
  props.snapshot ? parseJSON<L0AssemblySegmentsMap>(props.snapshot.segments_json, {}) : {},
);

const segmentRows = computed<MemorySnapshotSegmentRow[]>(() =>
  Object.entries(segmentsMap.value).map(([section, stats]) => ({ section, ...stats })),
);

const waterfallBars = computed(() => buildL0Waterfall(segmentsMap.value));
const totalTokens = computed(() => segmentRows.value.reduce((sum, r) => sum + (r.token_estimate || 0), 0));

function parseJSON<T>(raw: string, fallback: T): T {
  try {
    const parsed = JSON.parse(raw || '');
    return parsed ?? fallback;
  } catch {
    return fallback;
  }
}
</script>
