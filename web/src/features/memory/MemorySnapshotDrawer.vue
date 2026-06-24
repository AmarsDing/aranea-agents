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
            <div class="text-h6">Prompt 段落统计</div>
            <div class="text-caption text-grey-7">{{ snapshot?.id }}</div>
          </div>
          <q-btn flat round icon="close" aria-label="关闭快照详情" @click="$emit('update:modelValue', false)" />
        </div>
        <AppRegistryTable
          :shell="false"
          :resizable="false"
          :rows="segmentRows"
          :columns="MEMORY_SNAPSHOT_SEGMENT_COLUMNS"
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
                {{ slotProps.row.field_count }} fields
              </span>
              <span v-if="slotProps.row.fact_count != null" class="text-caption text-grey-7">
                {{ slotProps.row.fact_count }} facts
              </span>
              <span v-if="slotProps.row.entity_count != null" class="text-caption text-grey-7">
                {{ slotProps.row.entity_count }} entities
              </span>
              <span v-if="slotProps.row.result_count != null" class="text-caption text-grey-7">
                {{ slotProps.row.result_count }} results
              </span>
              <span v-if="slotProps.row.turn_count != null" class="text-caption text-grey-7">
                {{ slotProps.row.turn_count }} turns
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
              <div class="q-mt-sm">暂无段落数据</div>
            </div>
          </template>
        </AppRegistryTable>
        <div v-if="totalTokens > 0" class="q-mt-sm text-caption text-grey-7">
          合计: {{ totalTokens.toLocaleString() }} tokens
        </div>
      </div>
    </q-scroll-area>
  </q-drawer>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import AppRegistryTable from '../../components/layout/AppRegistryTable.vue';
import { MEMORY_SNAPSHOT_SEGMENT_COLUMNS, type MemorySnapshotSegmentRow } from './memoryTableUi';
import type { L0AssemblySegmentsMap, L0AssemblySnapshot } from './types';

const props = defineProps<{
  modelValue: boolean;
  snapshot: L0AssemblySnapshot | null;
}>();

defineEmits<{
  'update:modelValue': [value: boolean];
}>();

const segmentsMap = computed<L0AssemblySegmentsMap>(() =>
  props.snapshot ? parseJSON<L0AssemblySegmentsMap>(props.snapshot.segments_json, {}) : {},
);

const segmentRows = computed<MemorySnapshotSegmentRow[]>(() =>
  Object.entries(segmentsMap.value).map(([section, stats]) => ({ section, ...stats })),
);

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
