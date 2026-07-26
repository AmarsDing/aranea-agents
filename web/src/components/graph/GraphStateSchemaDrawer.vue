<template>
  <Teleport to="body">
    <div v-if="open" :class="['state-schema-drawer', { 'is-dark': isDark }]">
      <div class="state-schema-drawer__mask" @click="emit('close')" />
      <aside class="state-schema-drawer__panel">
        <!-- R2-G.1 头部：标题 + 字段总数 + 新建字段 -->
        <div class="state-schema-drawer__header">
          <div class="state-schema-drawer__title">
            {{ t('graphs.schemaDrawerTitle') }}<span class="state-schema-drawer__count">{{ fields.length }}</span>
          </div>
          <q-btn
            flat
            dense
            no-caps
            color="primary"
            icon="add"
            data-action="add-field"
            :label="t('graphs.schemaNewField')"
            @click="addField"
          />
          <q-btn flat dense round icon="close" size="sm" data-action="close" @click="emit('close')" />
        </div>

        <!-- R2-G.7 未使用警告 -->
        <div v-if="unusedCount > 0" class="state-schema-drawer__unused-warning" data-testid="unused-warning">
          <q-icon name="warning" size="14px" />
          <span>{{ t('graphs.schemaUnusedWarning', { n: unusedCount }) }}</span>
        </div>

        <!-- R2-G.2 搜索 + R2-G.3 视图切换 -->
        <div class="state-schema-drawer__toolbar">
          <q-input
            v-model="search"
            dense
            outlined
            class="state-schema-drawer__search"
            :placeholder="t('graphs.schemaSearchPlaceholder')"
          />
          <div class="state-schema-drawer__views">
            <button
              v-for="v in VIEWS"
              :key="v"
              type="button"
              :data-view="v"
              :class="['state-schema-drawer__view-btn', { 'is-active': view === v }]"
              @click="view = v"
            >
              {{ t(VIEW_LABEL_KEYS[v]) }}
            </button>
          </div>
        </div>

        <!-- R2-G.4 类型过滤 chips（带计数，多选） -->
        <div class="state-schema-drawer__type-chips">
          <button
            v-for="tc in typeCounts"
            :key="tc.type"
            type="button"
            :data-type="tc.type"
            :class="['state-schema-drawer__type-chip', { 'is-active': selectedTypes.has(tc.type) }]"
            @click="toggleType(tc.type)"
          >
            {{ tc.type }} {{ tc.count }}
          </button>
        </div>

        <!-- R2-G.5/G.6 虚拟滚动列表（行高 36px） -->
        <div ref="containerRef" class="state-schema-drawer__vlist" @scroll="onScroll">
          <div class="state-schema-drawer__vlist-spacer" :style="{ height: `${totalHeight}px` }">
            <template v-for="vr in visibleRows" :key="vr.item.key">
              <!-- 分组头 -->
              <button
                v-if="vr.item.kind === 'group'"
                type="button"
                class="state-schema-drawer__group-row"
                :style="{ top: `${vr.top}px` }"
                :data-group="vr.item.groupKey"
                @click="toggleGroup(vr.item.groupKey)"
              >
                <q-icon :name="vr.item.collapsed ? 'chevron_right' : 'expand_more'" size="14px" />
                <span class="state-schema-drawer__group-label">{{ vr.item.label }}</span>
                <span class="state-schema-drawer__group-count">{{ vr.item.count }}</span>
              </button>
              <!-- 字段行 -->
              <div
                v-else
                :class="[
                  'state-schema-drawer__field-row',
                  { 'is-unused': vr.item.unused, 'is-editing': editingIndex === vr.item.index },
                ]"
                :style="{ top: `${vr.top}px` }"
                :data-field="vr.item.field.name"
                @click="toggleEdit(vr.item.index)"
              >
                <input
                  type="checkbox"
                  class="state-schema-drawer__checkbox"
                  :checked="selected.has(vr.item.index)"
                  :disabled="!vr.item.unused"
                  :data-select="vr.item.field.name"
                  @click.stop
                  @change="toggleSelect(vr.item.index)"
                />
                <span class="state-schema-drawer__field-name" :title="vr.item.field.name">{{ vr.item.field.name }}</span>
                <span class="state-schema-drawer__field-type">{{ fieldTypeLabel(vr.item.field.type) }}</span>
                <span class="state-schema-drawer__field-reducer">{{ reducerLabel(vr.item.field.reducer) }}</span>
                <span class="state-schema-drawer__field-default" :title="formatDefault(vr.item.field.defaultValue)">{{
                  formatDefault(vr.item.field.defaultValue)
                }}</span>
                <span class="state-schema-drawer__field-usage"
                  >↑{{ vr.item.usage.writes.length }} ↓{{ vr.item.usage.reads.length }}</span
                >
              </div>
            </template>
          </div>
          <div v-if="displayRows.length === 0" class="state-schema-drawer__empty">
            {{ t('graphs.schemaEmpty') }}
          </div>
        </div>

        <!-- R2-G.8 内联编辑（选中行底部编辑区，保持虚拟滚动固定行高） -->
        <div v-if="editingField" class="state-schema-drawer__editor" data-testid="field-editor">
          <div class="state-schema-drawer__editor-head">
            <span class="state-schema-drawer__editor-name">{{ editingField.name }}</span>
            <q-btn flat dense round icon="close" size="xs" data-action="close-editor" @click="editingIndex = null" />
          </div>
          <q-input
            :model-value="editingDefaultText"
            dense
            outlined
            :label="t('graphs.schemaEditDefault')"
            data-field-input="defaultValue"
            @update:model-value="(v: string | number | null) => updateDefault(String(v ?? ''))"
          />
          <q-select
            :model-value="editingField.reducer"
            dense
            outlined
            emit-value
            map-options
            :label="t('graphs.stateFieldColumnReducer')"
            :options="reducerOptions"
            data-field-input="reducer"
            @update:model-value="(v: ReducerType) => updateEditing('reducer', v)"
          />
          <q-toggle
            :model-value="editingField.required"
            :label="t('graphs.schemaEditRequired')"
            @update:model-value="(v: boolean) => updateEditing('required', v)"
          />
        </div>

        <!-- R2-G.9 底部批量操作条 -->
        <div v-if="selected.size > 0" class="state-schema-drawer__batch" data-testid="batch-bar">
          <span class="state-schema-drawer__batch-count">{{ t('graphs.schemaBatchSelected', { n: selected.size }) }}</span>
          <q-btn
            flat
            dense
            no-caps
            icon="download"
            data-action="export-selected"
            :label="t('graphs.schemaBatchExport')"
            @click="exportSelected"
          />
          <q-btn
            flat
            dense
            no-caps
            color="negative"
            icon="delete"
            data-action="delete-selected"
            :label="t('graphs.schemaBatchDelete')"
            @click="confirmDeleteSelected"
          />
        </div>
      </aside>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import type { GraphDefinition, ReducerType, StateFieldDef } from '../../features/graph/types';
import { REDUCER_OPTIONS, STATE_FIELD_TYPE_OPTIONS, normalizeStateFieldType } from '../../features/graph/types';
import { buildFieldUsageMap, type FieldUsage } from '../../features/graph/portTypes';
import { useVirtualRows } from '../../features/graph/useVirtualRows';

const ROW_HEIGHT = 36;
const BUFFER = 5;

const VIEWS = ['flat', 'prefix', 'usage'] as const;
type ViewMode = (typeof VIEWS)[number];
const VIEW_LABEL_KEYS: Record<ViewMode, string> = {
  flat: 'graphs.schemaViewFlat',
  prefix: 'graphs.schemaViewPrefix',
  usage: 'graphs.schemaViewUsage',
};

type GroupRow = { kind: 'group'; key: string; groupKey: string; label: string; count: number; collapsed: boolean };
type FieldRow = {
  kind: 'field';
  key: string;
  index: number;
  field: StateFieldDef;
  usage: FieldUsage;
  unused: boolean;
};
type DrawerRow = GroupRow | FieldRow;

const props = defineProps<{
  open: boolean;
  isDark: boolean;
}>();

const graphDef = defineModel<GraphDefinition | null>('graphDef', { required: true });

const emit = defineEmits<{
  close: [];
  change: [];
}>();

const { t } = useI18n();
const $q = useQuasar();

const reducerOptions = REDUCER_OPTIONS.map((o) => ({ label: t(o.labelKey), value: o.value }));

// 行内 reducer 展示走 i18n（与详情面板一致，禁止裸渲染英文枚举）
function reducerLabel(reducer: string): string {
  const labelKey = REDUCER_OPTIONS.find((o) => o.value === reducer)?.labelKey;
  return labelKey ? t(labelKey) : reducer;
}

function fieldTypeLabel(type: string): string {
  const normalized = normalizeStateFieldType(type);
  const labelKey = STATE_FIELD_TYPE_OPTIONS.find((o) => o.value === normalized)?.labelKey;
  return labelKey ? t(labelKey) : type;
}

// ---- 数据派生 ----
const fields = computed<StateFieldDef[]>(() => graphDef.value?.stateFields ?? []);
const usageMap = computed(() => buildFieldUsageMap(graphDef.value?.nodes ?? []));

const EMPTY_USAGE: FieldUsage = { reads: [], writes: [] };
function usageOf(name: string): FieldUsage {
  return usageMap.value.get(name) ?? EMPTY_USAGE;
}

const unusedCount = computed(
  () => fields.value.filter((f) => usageOf(f.name).reads.length + usageOf(f.name).writes.length === 0).length,
);

// ---- R2-G.2 搜索 + R2-G.4 类型过滤 ----
const search = ref('');
const selectedTypes = ref<Set<string>>(new Set());

const typeCounts = computed(() => {
  const counts = new Map<string, number>();
  for (const f of fields.value) {
    counts.set(f.type, (counts.get(f.type) ?? 0) + 1);
  }
  return [...counts.entries()].map(([type, count]) => ({ type, count }));
});

function toggleType(type: string) {
  const next = new Set(selectedTypes.value);
  if (next.has(type)) {
    next.delete(type);
  } else {
    next.add(type);
  }
  selectedTypes.value = next;
}

type IndexedField = { index: number; field: StateFieldDef };

const filteredFields = computed<IndexedField[]>(() => {
  const q = search.value.trim().toLowerCase();
  const types = selectedTypes.value;
  return fields.value
    .map((field, index) => ({ index, field }))
    .filter(({ field }) => {
      if (types.size > 0 && !types.has(field.type)) return false;
      if (!q) return true;
      return field.name.toLowerCase().includes(q) || formatDefault(field.defaultValue).toLowerCase().includes(q);
    });
});

// ---- R2-G.3 三视图行构建 ----
const view = ref<ViewMode>('flat');
const collapsedGroups = ref<Set<string>>(new Set());

function toggleGroup(key: string) {
  const next = new Set(collapsedGroups.value);
  if (next.has(key)) {
    next.delete(key);
  } else {
    next.add(key);
  }
  collapsedGroups.value = next;
}

function toFieldRow(item: IndexedField): FieldRow {
  const usage = usageOf(item.field.name);
  return {
    kind: 'field',
    key: `f:${item.index}`,
    index: item.index,
    field: item.field,
    usage,
    unused: usage.reads.length + usage.writes.length === 0,
  };
}

function buildGroupedRows(groups: Array<{ groupKey: string; label: string; items: IndexedField[] }>): DrawerRow[] {
  const rows: DrawerRow[] = [];
  for (const g of groups) {
    const collapsed = collapsedGroups.value.has(g.groupKey);
    rows.push({ kind: 'group', key: `g:${view.value}:${g.groupKey}`, groupKey: g.groupKey, label: g.label, count: g.items.length, collapsed });
    if (!collapsed) {
      for (const item of g.items) rows.push(toFieldRow(item));
    }
  }
  return rows;
}

const displayRows = computed<DrawerRow[]>(() => {
  const items = filteredFields.value;
  if (view.value === 'flat') {
    return items.map(toFieldRow);
  }
  if (view.value === 'prefix') {
    // 前缀分组：name.split('_')[0]；单字段前缀归入「其他」
    const buckets = new Map<string, IndexedField[]>();
    for (const item of items) {
      const prefix = item.field.name.includes('_') ? item.field.name.split('_')[0] : '';
      const list = buckets.get(prefix) ?? [];
      list.push(item);
      buckets.set(prefix, list);
    }
    const multi: Array<{ groupKey: string; label: string; items: IndexedField[] }> = [];
    const other: IndexedField[] = [];
    for (const [prefix, list] of [...buckets.entries()].sort(([a], [b]) => a.localeCompare(b))) {
      if (prefix && list.length > 1) {
        multi.push({ groupKey: prefix, label: `${prefix}_*`, items: list });
      } else {
        other.push(...list);
      }
    }
    if (other.length > 0) {
      multi.push({ groupKey: '__other__', label: t('graphs.schemaGroupOther'), items: other });
    }
    return buildGroupedRows(multi);
  }
  // usage 视图：有写入 / 仅被读取 / 未使用
  const written: IndexedField[] = [];
  const readOnly: IndexedField[] = [];
  const unused: IndexedField[] = [];
  for (const item of items) {
    const u = usageOf(item.field.name);
    if (u.writes.length > 0) {
      written.push(item);
    } else if (u.reads.length > 0) {
      readOnly.push(item);
    } else {
      unused.push(item);
    }
  }
  return buildGroupedRows([
    { groupKey: 'written', label: t('graphs.schemaGroupWritten'), items: written },
    { groupKey: 'readonly', label: t('graphs.schemaGroupReadOnly'), items: readOnly },
    { groupKey: 'unused', label: t('graphs.schemaGroupUnused'), items: unused },
  ]);
});

// ---- R2-G.6 虚拟滚动 ----
const { containerRef, visibleRows, totalHeight, onScroll } = useVirtualRows({
  rows: displayRows,
  rowHeight: ROW_HEIGHT,
  buffer: BUFFER,
});

// ---- R2-G.8 内联编辑（写回 graphDef.stateFields → 置 dirty） ----
const editingIndex = ref<number | null>(null);
const editingField = computed(() =>
  editingIndex.value !== null ? (fields.value[editingIndex.value] ?? null) : null,
);
const editingDefaultText = ref('');

function formatDefault(v: unknown): string {
  if (v === undefined || v === null || v === '') return '';
  return typeof v === 'string' ? v : JSON.stringify(v);
}

function toggleEdit(index: number) {
  if (editingIndex.value === index) {
    editingIndex.value = null;
    return;
  }
  editingIndex.value = index;
  editingDefaultText.value = formatDefault(fields.value[index]?.defaultValue);
}

function updateEditing(key: 'reducer' | 'required', value: ReducerType | boolean) {
  const idx = editingIndex.value;
  const list = graphDef.value?.stateFields;
  if (idx === null || !list?.[idx]) return;
  if (key === 'reducer') {
    list[idx].reducer = value as ReducerType;
  } else {
    list[idx].required = Boolean(value);
  }
  emit('change');
}

function updateDefault(text: string) {
  editingDefaultText.value = text;
  const idx = editingIndex.value;
  const list = graphDef.value?.stateFields;
  if (idx === null || !list?.[idx]) return;
  const trimmed = text.trim();
  if (!trimmed) {
    list[idx].defaultValue = undefined;
  } else {
    try {
      list[idx].defaultValue = JSON.parse(trimmed);
    } catch {
      list[idx].defaultValue = text;
    }
  }
  emit('change');
}

// ---- R2-G.1 新建字段 ----
function addField() {
  const list = graphDef.value?.stateFields;
  if (!list) return;
  let n = list.length + 1;
  let name = `field_${n}`;
  const names = new Set(list.map((f) => f.name));
  while (names.has(name)) {
    n += 1;
    name = `field_${n}`;
  }
  list.push({ name, type: 'string', reducer: 'append', required: false, disableDeepCopy: false });
  emit('change');
}

// ---- R2-G.9 批量选择 / 导出 / 删除 ----
const selected = ref<Set<number>>(new Set());

function toggleSelect(index: number) {
  const next = new Set(selected.value);
  if (next.has(index)) {
    next.delete(index);
  } else {
    next.add(index);
  }
  selected.value = next;
}

function exportSelected() {
  const list = fields.value;
  const items = [...selected.value].sort((a, b) => a - b).map((i) => list[i]);
  const json = JSON.stringify(items, null, 2);
  const blob = new Blob([json], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = 'state-fields.json';
  anchor.click();
  URL.revokeObjectURL(url);
}

function confirmDeleteSelected() {
  // 被引用字段复选框已禁用，此处双保险再过滤一次
  const removable = [...selected.value].filter((i) => {
    const f = fields.value[i];
    if (!f) return false;
    const u = usageOf(f.name);
    return u.reads.length + u.writes.length === 0;
  });
  if (removable.length === 0) return;
  $q.dialog({
    title: t('graphs.schemaBatchDelete'),
    message: t('graphs.schemaDeleteConfirm', { n: removable.length }),
    cancel: true,
    persistent: true,
  }).onOk(() => {
    const list = graphDef.value?.stateFields;
    if (!list) return;
    for (const i of removable.sort((a, b) => b - a)) {
      list.splice(i, 1);
    }
    selected.value = new Set();
    if (editingIndex.value !== null && !list[editingIndex.value]) {
      editingIndex.value = null;
    }
    emit('change');
  });
}
</script>
