<template>
  <div class="event-filter-bar row q-col-gutter-sm items-center">
    <q-select
      :model-value="filters.typeFilter"
      dense
      outlined
      emit-value
      map-options
      class="col-12 col-sm-2"
      label="类型"
      :options="typeOptions"
      @update:model-value="update('typeFilter', $event)"
    />
    <q-input
      :model-value="filters.filterKey"
      dense
      outlined
      clearable
      class="col-12 col-sm-2"
      label="FilterKey"
      @update:model-value="update('filterKey', String($event ?? ''))"
    />
    <q-input
      :model-value="filters.branchPrefix"
      dense
      outlined
      clearable
      class="col-12 col-sm-2"
      label="分支前缀"
      @update:model-value="update('branchPrefix', String($event ?? ''))"
    />
    <q-input
      :model-value="filters.tag"
      dense
      outlined
      clearable
      class="col-12 col-sm-2"
      label="标签"
      @update:model-value="update('tag', String($event ?? ''))"
    />
    <q-input
      :model-value="filters.keyword"
      dense
      outlined
      clearable
      class="col-12 col-sm-4"
      label="搜索"
      @update:model-value="update('keyword', String($event ?? ''))"
    />
  </div>
</template>

<script setup lang="ts">
import type { EventFilterState } from '../../features/chat/eventFilter';

const props = defineProps<{
  filters: EventFilterState;
}>();

const emit = defineEmits<{
  'update:filters': [value: EventFilterState];
}>();

const typeOptions = [
  { label: '全部', value: 'all' },
  ...(
    [
      'tool_call',
      'tool_result',
      'state_delta',
      'transfer',
      'text_delta',
      'runner_completion',
      'error',
      'team_run_started',
      'team_run_finished',
    ] as string[]
  ).map((t) => ({ label: t, value: t })),
];

function update<K extends keyof EventFilterState>(key: K, value: EventFilterState[K]): void {
  emit('update:filters', { ...props.filters, [key]: value });
}
</script>
