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
        <q-table
          flat
          dense
          :rows="segmentRows"
          :columns="columns"
          row-key="section"
          hide-pagination
          :pagination="{ rowsPerPage: 0 }"
        >
          <template #body-cell-token_estimate="props">
            <q-td :props="props">
              {{ props.row.token_estimate.toLocaleString() }}
            </q-td>
          </template>
          <template #body-cell-detail="props">
            <q-td :props="props">
              <span v-if="props.row.field_count != null" class="text-caption text-grey-7">
                {{ props.row.field_count }} fields
              </span>
              <span v-if="props.row.fact_count != null" class="text-caption text-grey-7">
                {{ props.row.fact_count }} facts
              </span>
              <span v-if="props.row.entity_count != null" class="text-caption text-grey-7">
                {{ props.row.entity_count }} entities
              </span>
              <span v-if="props.row.result_count != null" class="text-caption text-grey-7">
                {{ props.row.result_count }} results
              </span>
              <span v-if="props.row.turn_count != null" class="text-caption text-grey-7">
                {{ props.row.turn_count }} turns
              </span>
              <span v-if="props.row.from_turn != null && props.row.to_turn != null" class="text-caption text-grey-7">
                T{{ props.row.from_turn }}–T{{ props.row.to_turn }}
              </span>
            </q-td>
          </template>
          <template #no-data>
            <div class="full-width column items-center q-pa-md text-grey-7">
              <q-icon name="segment" size="32px" />
              <div class="q-mt-sm">暂无段落数据</div>
            </div>
          </template>
        </q-table>
        <div v-if="totalTokens > 0" class="q-mt-sm text-caption text-grey-7">
          合计: {{ totalTokens.toLocaleString() }} tokens
        </div>
      </div>
    </q-scroll-area>
  </q-drawer>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { QTableColumn } from 'quasar';
import type { L0AssemblySegmentStats, L0AssemblySegmentsMap, L0AssemblySnapshot } from './types';

const props = defineProps<{
  modelValue: boolean;
  snapshot: L0AssemblySnapshot | null;
}>();

defineEmits<{
  'update:modelValue': [value: boolean];
}>();

type SegmentRow = L0AssemblySegmentStats & { section: string };

const columns: QTableColumn<SegmentRow>[] = [
  { name: 'section', label: 'Section', field: 'section', align: 'left', sortable: true },
  { name: 'token_estimate', label: 'Token Estimate', field: 'token_estimate', align: 'right', sortable: true },
  { name: 'message_count', label: 'Message Count', field: 'message_count', align: 'right', sortable: true },
  { name: 'detail', label: 'Detail', field: () => '', align: 'left', sortable: false },
];

const segmentsMap = computed<L0AssemblySegmentsMap>(() =>
  props.snapshot ? parseJSON<L0AssemblySegmentsMap>(props.snapshot.segments_json, {}) : {},
);

const segmentRows = computed<SegmentRow[]>(() =>
  Object.entries(segmentsMap.value).map(([section, stats]) => ({ section, ...stats })),
);

const totalTokens = computed(() =>
  segmentRows.value.reduce((sum, r) => sum + (r.token_estimate || 0), 0),
);

function parseJSON<T>(raw: string, fallback: T): T {
  try {
    const parsed = JSON.parse(raw || '');
    return parsed ?? fallback;
  } catch {
    return fallback;
  }
}
</script>
