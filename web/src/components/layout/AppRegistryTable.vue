<template>
  <div v-if="shell" :class="shellClasses">
    <q-table v-bind="tableBind" :flat="flat" :dense="dense" :class="tableClasses">
      <template v-for="name in forwardedSlotNames" :key="name" #[name]="slotData">
        <slot :name="name" v-bind="slotData ?? {}" />
      </template>
      <template v-for="colName in resizableHeaderColumns" :key="colName" #[headerCellSlot(colName)]="slotData">
        <q-th :props="slotData" class="app-registry-th-resizable">
          {{ slotData.col.label }}
          <span
            class="app-registry-col-resize"
            role="separator"
            aria-orientation="vertical"
            :aria-label="`调整 ${slotData.col.label} 列宽`"
            @mousedown.stop.prevent="onResizeStart(String(colName), $event)"
          />
        </q-th>
      </template>
    </q-table>
  </div>
  <div v-else>
    <q-table v-bind="tableBind" :flat="flat" :dense="dense" :class="tableClasses">
      <template v-for="name in forwardedSlotNames" :key="name" #[name]="slotData">
        <slot :name="name" v-bind="slotData ?? {}" />
      </template>
      <template v-for="colName in resizableHeaderColumns" :key="colName" #[headerCellSlot(colName)]="slotData">
        <q-th :props="slotData" class="app-registry-th-resizable">
          {{ slotData.col.label }}
          <span
            class="app-registry-col-resize"
            role="separator"
            aria-orientation="vertical"
            :aria-label="`调整 ${slotData.col.label} 列宽`"
            @mousedown.stop.prevent="onResizeStart(String(colName), $event)"
          />
        </q-th>
      </template>
    </q-table>
  </div>
</template>

<script setup lang="ts">
import { computed, useAttrs, useSlots } from 'vue';
import type { QTableProps } from 'quasar';
import { useResizableRegistryColumns } from '../../features/ui/useResizableRegistryColumns';

defineOptions({ inheritAttrs: false });

const props = withDefaults(
  defineProps<{
    /** 玻璃外壳；设为 false 时由父级 panel / dialog 提供容器 */
    shell?: boolean;
    /** 使用 .app-data-table-shell（嵌套面板内表格） */
    dataShell?: boolean;
    /** q-table 附加类名，如 skill-table、plugins-table */
    tableClass?: string;
    /** 使用外部分页条时隐藏 q-table 底栏 */
    externalPagination?: boolean;
    dense?: boolean;
    flat?: boolean;
    /** 表头拖拽调整列宽（默认开启） */
    resizable?: boolean;
    /** localStorage 持久化 key；同页多表时请显式区分 */
    columnPersistKey?: string;
  }>(),
  {
    shell: true,
    dataShell: false,
    tableClass: '',
    externalPagination: true,
    dense: true,
    flat: true,
    resizable: true,
    columnPersistKey: '',
  },
);

const attrs = useAttrs();
const slots = useSlots();

const sourceColumns = computed(() => {
  const bind = attrs as Record<string, unknown>;
  const columns = bind.columns as QTableProps['columns'] | undefined;
  return Array.isArray(columns) ? columns : undefined;
});

const { displayColumns, onResizeStart } = useResizableRegistryColumns(() => sourceColumns.value, {
  enabled: () => props.resizable,
  persistKey: () => props.columnPersistKey || undefined,
});

/** 将 columns.style 同步到 headerStyle，并应用用户拖拽后的列宽 */
const tableBind = computed(() => {
  const bind = { ...(attrs as Record<string, unknown>) };
  if (displayColumns.value) {
    bind.columns = displayColumns.value;
  }
  return bind;
});

const resizableHeaderColumns = computed(() => {
  if (!props.resizable || !displayColumns.value) return [];
  return displayColumns.value
    .map((col) => (col && typeof col === 'object' ? String(col.name ?? '') : ''))
    .filter((name) => name && !slots[`header-cell-${name}`]);
});

const forwardedSlotNames = computed(() => Object.keys(slots));

function headerCellSlot(colName: string) {
  return `header-cell-${colName}`;
}

const shellClasses = computed(() => (props.dataShell ? 'app-data-table-shell' : 'app-registry-table-shell'));

const tableClasses = computed(() => {
  const classes = ['app-registry-table'];
  if (props.tableClass) classes.push(props.tableClass);
  if (props.externalPagination) classes.push('app-registry-table--external-pagination');
  if (props.resizable) classes.push('app-registry-table--resizable');
  return classes;
});
</script>
