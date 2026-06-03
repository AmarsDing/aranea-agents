<template>
  <q-markup-table flat dense :wrap-cells="wrapCells" :class="tableClasses">
    <thead>
      <tr>
        <th
          v-for="col in normalizedColumns"
          :key="String(col.name)"
          :class="cellAlignClass(col.align)"
          :style="col.headerStyle"
        >
          {{ col.label }}
        </th>
      </tr>
    </thead>
    <tbody>
      <tr v-for="row in rows" :key="String(resolveRowKey(row))">
        <td
          v-for="col in normalizedColumns"
          :key="String(col.name)"
          :class="cellAlignClass(col.align)"
          :style="col.style"
        >
          <slot :name="`cell-${col.name}`" :row="row" :col="col" :value="cellValue(row, col.field)">
            {{ cellValue(row, col.field) }}
          </slot>
        </td>
      </tr>
      <tr v-if="!rows.length">
        <td :colspan="normalizedColumns.length || 1" :class="emptyCellClass">
          <slot name="empty">暂无数据</slot>
        </td>
      </tr>
    </tbody>
  </q-markup-table>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import {
  normalizeRegistryColumns,
  registryFieldValue,
  type RegistryTableColumn,
} from '../../features/ui/registryTableColumns';

const props = withDefaults(
  defineProps<{
    rows: Record<string, unknown>[];
    columns: RegistryTableColumn<any>[];
    rowKey?: string;
    wrapCells?: boolean;
    tableClass?: string;
    emptyCellClass?: string;
  }>(),
  {
    rowKey: 'id',
    wrapCells: true,
    tableClass: '',
    emptyCellClass: 'text-center text-grey-7',
  },
);

const tableClasses = computed(() => {
  const classes = ['app-registry-markup-table'];
  if (props.tableClass) classes.push(props.tableClass);
  return classes;
});

const normalizedColumns = computed(() => normalizeRegistryColumns(props.columns) ?? []);

function cellAlignClass(align?: string) {
  if (align === 'right') return 'text-right';
  if (align === 'center') return 'text-center';
  return 'text-left';
}

function resolveRowKey(row: Record<string, unknown>) {
  return row[props.rowKey] ?? row.id ?? JSON.stringify(row);
}

function cellValue(row: Record<string, unknown>, field: RegistryTableColumn['field']) {
  const value = registryFieldValue(row, field);
  if (value == null || value === '') return '—';
  return value;
}
</script>
