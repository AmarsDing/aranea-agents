<template>
  <footer class="app-registry-pagination app-registry-pagination--card q-mt-md">
    <div class="app-registry-pagination__summary">{{ total }} {{ label ?? t('common.pagination.unit') }}</div>
    <div class="app-registry-pagination__controls row items-center no-wrap">
      <q-select
        :model-value="pageSize"
        dense
        outlined
        emit-value
        map-options
        :label="t('common.pagination.perPage')"
        :options="pageSizeOptionsResolved"
        class="app-registry-pagination__page-size app-glass-control"
        @update:model-value="emit('update:pageSize', Number($event))"
      />
      <span class="app-registry-pagination__page-label">{{ t('common.pagination.pageOf', { page, max: pageMax }) }}</span>
      <q-btn
        round
        dense
        flat
        icon="chevron_left"
        :disable="page <= 1 || loading"
        @click="emit('update:page', page - 1)"
      />
      <q-btn
        round
        dense
        flat
        icon="chevron_right"
        :disable="page >= pageMax || loading"
        @click="emit('update:page', page + 1)"
      />
    </div>
  </footer>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';

const props = withDefaults(
  defineProps<{
    page: number;
    pageSize: number;
    pageMax: number;
    total: number;
    loading?: boolean;
    /** 汇总单位文案（如「条事件」）；缺省用 common.pagination.unit */
    label?: string;
    /** 默认 10 / 20 / 50，与 Skill 管理页一致 */
    pageSizeOptions?: number[];
  }>(),
  {
    loading: false,
    label: undefined,
    pageSizeOptions: () => [10, 20, 50],
  },
);

const emit = defineEmits<{
  'update:page': [value: number];
  'update:pageSize': [value: number];
}>();

const { t } = useI18n();

const pageSizeOptionsResolved = computed(() => props.pageSizeOptions.map((value) => ({ label: String(value), value })));
</script>
