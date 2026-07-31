<template>
  <q-card flat class="app-pane-card knowledge-doc-list">
    <div class="app-pane-card__header knowledge-doc-list__header">
      <!-- 面包屑：根目录 / 子目录…，点击任意段回跳；G3-F1 拖拽时兼作 drop 目标（hover 500ms 下钻） -->
      <div class="row items-center no-wrap ellipsis knowledge-doc-list__crumbs">
        <template v-for="(c, i) in crumbs" :key="c.prefix">
          <q-icon v-if="i > 0" name="chevron_right" size="14px" class="knowledge-doc-list__crumb-sep" />
          <span
            class="knowledge-doc-list__crumb ellipsis"
            :class="{
              'knowledge-doc-list__crumb--current': i === crumbs.length - 1,
              'knowledge-doc-list__crumb--drop': crumbDropPrefix === c.prefix,
            }"
            @click="$emit('navigate', c.prefix)"
            @dragover="onCrumbDragOver(c, $event)"
            @dragleave="onCrumbDragLeave(c)"
            @drop="onCrumbDrop(c, $event)"
          >
            <q-icon v-if="i === 0" name="home" size="14px" class="q-mr-xs" />{{ c.label }}
          </span>
        </template>
      </div>
      <div class="row items-center no-wrap">
        <q-btn
          flat
          dense
          round
          size="sm"
          icon="note_add"
          :aria-label="t('knowledgePage.pasteText')"
          @click="$emit('ingest')"
        >
          <q-tooltip>{{ t('knowledgePage.pasteText') }}</q-tooltip>
        </q-btn>
        <q-btn
          flat
          dense
          round
          size="sm"
          icon="refresh"
          :aria-label="t('knowledgePage.refreshAria')"
          @click="$emit('refresh')"
        >
          <q-tooltip>{{ t('knowledgePage.refreshAria') }}</q-tooltip>
        </q-btn>
      </div>
    </div>

    <div class="app-pane-card__body knowledge-doc-list__body">
      <table v-if="sortedEntries.length" class="knowledge-doc-list__table">
        <thead>
          <tr>
            <th
              v-for="col in columns"
              :key="col.key"
              class="knowledge-doc-list__th"
              :class="`knowledge-doc-list__th--${col.align}`"
              :style="{ width: col.width }"
              @click="toggleSort(col.key)"
            >
              <span class="row items-center no-wrap" :class="col.align === 'right' ? 'justify-end' : ''">
                {{ col.label }}
                <q-icon
                  v-if="sortBy === col.key"
                  :name="sortDesc ? 'arrow_downward' : 'arrow_upward'"
                  size="12px"
                  class="q-ml-xs"
                />
              </span>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="e in sortedEntries"
            :key="e.path || e.name"
            class="knowledge-doc-list__row"
            :class="{
              'knowledge-doc-list__row--selected': e.kind === 'file' && selectedDocId === e.doc_id,
              'knowledge-doc-list__row--dir': e.kind === 'dir',
            }"
            :draggable="e.kind === 'file'"
            @click="onRowClick(e)"
            @dragstart="onRowDragStart(e, $event)"
            @dragend="$emit('drag-end')"
          >
            <td class="knowledge-doc-list__cell knowledge-doc-list__cell--name">
              <div class="knowledge-doc-list__name-wrap">
                <q-icon
                  :name="visualOf(e).icon"
                  size="16px"
                  class="q-mr-xs knowledge-doc-list__icon"
                  :class="[visualOf(e).cls, { 'kv-icon--pulse': visualOf(e).pulse }]"
                />
                <span class="knowledge-doc-list__name-text" :title="e.name">{{ e.name }}</span>
              </div>
              <q-tooltip v-if="e.summary" max-width="320px" anchor="center left" self="center right">
                {{ e.summary }}
              </q-tooltip>
            </td>
            <td class="knowledge-doc-list__cell knowledge-doc-list__cell--time">
              {{ e.kind === 'file' ? formatKnowledgeTime(e.updated_at) : '' }}
            </td>
            <td class="knowledge-doc-list__cell">{{ typeLabel(e) }}</td>
            <td class="knowledge-doc-list__cell knowledge-doc-list__cell--size">
              {{ e.kind === 'file' ? formatKnowledgeDocSize(e.size_bytes) : '' }}
            </td>
            <td class="knowledge-doc-list__cell">
              <q-chip
                v-if="e.kind === 'file' && e.status"
                dense
                size="sm"
                :color="statusColor(e.status)"
                text-color="white"
              >
                {{ e.status }}
              </q-chip>
              <q-tooltip v-if="e.status === 'error' && e.error_message" max-width="320px">
                {{ e.error_message }}
              </q-tooltip>
            </td>
          </tr>
        </tbody>
      </table>
      <div v-else-if="!loading" class="app-registry-empty app-registry-empty--compact">
        <q-icon name="drafts" size="40px" color="grey-6" />
        <div class="text-body2">{{ t('knowledgePage.docListEmpty') }}</div>
      </div>
      <q-card-section v-else>
        <q-skeleton v-for="i in 3" :key="i" type="rect" height="40px" class="q-mb-sm" />
      </q-card-section>
    </div>
  </q-card>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import type { VaultTreeNode } from '../../features/knowledge/types';
import {
  isValidDropTarget,
  vaultNodeVisual,
  type DragFileRef,
  type VaultNodeVisual,
} from '../../features/knowledge/vaultTreeUi';
import {
  formatKnowledgeDocSize,
  formatKnowledgeTime,
  knowledgeStatusColor,
} from '../../features/knowledge/knowledgeUi';

type SortKey = 'name' | 'updated_at' | 'type' | 'size' | 'status';

const props = defineProps<{
  prefix: string;
  /** 当前目录直接子节点（dir + file 混合，dir 可点击下钻）。 */
  entries: VaultTreeNode[];
  loading: boolean;
  selectedDocId: string;
  /** G3-F1：拖拽中的文件（非空时面包屑变为 drop 目标）；列表恒属当前 vault。 */
  dragFile: DragFileRef | null;
  vaultId: string;
}>();

const emit = defineEmits<{
  select: [node: VaultTreeNode];
  /** 面包屑回跳 / 目录下钻：目标 prefix（'' = 根）。 */
  navigate: [prefix: string];
  refresh: [];
  ingest: [];
  /** G3-F1：文件行拖拽开始/结束（拖拽源状态由 composable 持有）。 */
  'drag-start': [node: VaultTreeNode];
  'drag-end': [];
  /** G3-F1：drop 到面包屑段（目标目录 prefix）。 */
  'drop-prefix': [prefix: string];
}>();

const { t } = useI18n();
const statusColor = knowledgeStatusColor;

/** G1 科幻图标：dir=violet、md=teal、图片=magenta、音视频=orange、error=red 脉冲。 */
function visualOf(e: VaultTreeNode): VaultNodeVisual {
  return vaultNodeVisual({ kind: e.kind, name: e.name, status: e.status });
}

const columns = computed<{ key: SortKey; label: string; align: 'left' | 'right'; width: string }[]>(() => [
  { key: 'name', label: t('knowledgePage.colName'), align: 'left', width: '40%' },
  { key: 'updated_at', label: t('knowledgePage.colModified'), align: 'left', width: '20%' },
  { key: 'type', label: t('knowledgePage.colType'), align: 'left', width: '14%' },
  { key: 'size', label: t('knowledgePage.colSize'), align: 'right', width: '10%' },
  { key: 'status', label: t('knowledgePage.colStatus'), align: 'left', width: '16%' },
]);

// ---------- 面包屑 ----------

const crumbs = computed(() => {
  const parts = props.prefix.split('/').filter(Boolean);
  const out: { label: string; prefix: string }[] = [{ label: t('knowledgePage.vaultRoot'), prefix: '' }];
  let acc = '';
  for (const p of parts) {
    acc += `${p}/`;
    out.push({ label: p, prefix: acc });
  }
  return out;
});

// ---------- 类型列（资源管理器风格：扩展名映射） ----------

const EXT_TYPE_LABELS: Record<string, string> = {
  md: 'Markdown',
  markdown: 'Markdown',
  txt: 'Text',
  log: 'Text',
  pdf: 'PDF',
  json: 'JSON',
  csv: 'CSV',
  html: 'HTML',
  htm: 'HTML',
  xml: 'XML',
  yaml: 'YAML',
  yml: 'YAML',
  toml: 'TOML',
  doc: 'Word',
  docx: 'Word',
  xls: 'Excel',
  xlsx: 'Excel',
  ppt: 'PowerPoint',
  pptx: 'PowerPoint',
  png: 'Image',
  jpg: 'Image',
  jpeg: 'Image',
  webp: 'Image',
  gif: 'Image',
};

function typeLabel(e: VaultTreeNode): string {
  if (e.kind === 'dir') return t('knowledgePage.typeFolder');
  const i = e.name.lastIndexOf('.');
  if (i <= 0 || i === e.name.length - 1) return t('knowledgePage.typeFile');
  const ext = e.name.slice(i + 1).toLowerCase();
  return EXT_TYPE_LABELS[ext] ?? ext.toUpperCase();
}

// ---------- 排序（目录恒在前，组内按列排序） ----------

const sortBy = ref<SortKey>('name');
const sortDesc = ref(false);

function toggleSort(col: SortKey) {
  if (sortBy.value === col) {
    sortDesc.value = !sortDesc.value;
  } else {
    sortBy.value = col;
    sortDesc.value = false;
  }
}

function compareBy(a: VaultTreeNode, b: VaultTreeNode, key: SortKey): number {
  switch (key) {
    case 'name':
      return a.name.localeCompare(b.name, undefined, { sensitivity: 'base' });
    case 'updated_at':
      return a.updated_at.localeCompare(b.updated_at);
    case 'type':
      return typeLabel(a).localeCompare(typeLabel(b));
    case 'size':
      return a.size_bytes - b.size_bytes;
    case 'status':
      return a.status.localeCompare(b.status);
  }
}

const sortedEntries = computed(() => {
  const mul = sortDesc.value ? -1 : 1;
  const cmp = (a: VaultTreeNode, b: VaultTreeNode) => mul * compareBy(a, b, sortBy.value);
  const dirs = props.entries.filter((e) => e.kind === 'dir').sort(cmp);
  const files = props.entries.filter((e) => e.kind === 'file').sort(cmp);
  return [...dirs, ...files];
});

// ---------- 行点击：目录下钻 / 文件选中 ----------

function onRowClick(e: VaultTreeNode) {
  if (e.kind === 'dir') {
    emit('navigate', e.path);
    return;
  }
  emit('select', e);
}

// ---------- G3-F1 拖拽移动（V12.5：幽灵卡 + 面包屑 drop 目标） ----------

/** 文件行 dragstart：dataTransfer + 自定义幽灵卡（离屏渲染，setDragImage 捕获后即移除）。 */
function onRowDragStart(e: VaultTreeNode, ev: DragEvent) {
  if (e.kind !== 'file' || !ev.dataTransfer) return;
  ev.dataTransfer.setData('text/plain', e.name);
  ev.dataTransfer.effectAllowed = 'move';
  const ghost = document.createElement('div');
  ghost.className = 'knowledge-drag-ghost';
  ghost.textContent = e.name;
  document.body.appendChild(ghost);
  ev.dataTransfer.setDragImage(ghost, -8, 14);
  requestAnimationFrame(() => ghost.remove());
  emit('drag-start', e);
}

/** 面包屑 drop 高亮（当前 dragover 的合法目标段）。 */
const crumbDropPrefix = ref<string | null>(null);
/** hover 500ms 下钻定时器（拖拽悬停面包屑段 → 中栏切到该目录，可继续深入）。 */
let crumbHoverTimer: ReturnType<typeof setTimeout> | null = null;

function clearCrumbHover() {
  if (crumbHoverTimer) {
    clearTimeout(crumbHoverTimer);
    crumbHoverTimer = null;
  }
}

function crumbTargetValid(prefix: string): boolean {
  return isValidDropTarget(props.dragFile, { vaultId: props.vaultId, prefix });
}

function onCrumbDragOver(c: { prefix: string }, ev: DragEvent) {
  if (!props.dragFile) return;
  if (crumbTargetValid(c.prefix)) {
    ev.preventDefault();
    ev.dataTransfer!.dropEffect = 'move';
    if (crumbDropPrefix.value !== c.prefix) {
      crumbDropPrefix.value = c.prefix;
      clearCrumbHover();
      // 500ms 悬停下钻（当前段已是末段则无需跳转）。
      if (c.prefix !== props.prefix) {
        crumbHoverTimer = setTimeout(() => emit('navigate', c.prefix), 500);
      }
    }
  } else {
    ev.dataTransfer!.dropEffect = 'none';
  }
}

function onCrumbDragLeave(c: { prefix: string }) {
  if (crumbDropPrefix.value === c.prefix) {
    crumbDropPrefix.value = null;
    clearCrumbHover();
  }
}

function onCrumbDrop(c: { prefix: string }, ev: DragEvent) {
  ev.preventDefault();
  crumbDropPrefix.value = null;
  clearCrumbHover();
  if (crumbTargetValid(c.prefix)) emit('drop-prefix', c.prefix);
}

onBeforeUnmount(clearCrumbHover);
</script>

<style lang="scss" scoped>
.knowledge-doc-list {
  display: flex;
  flex-direction: column;
  min-height: 0;

  &__header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }

  &__crumbs {
    min-width: 0;
    font-size: 13px;
  }

  &__crumb {
    cursor: pointer;
    border-radius: 4px;
    padding: 1px 4px;
    color: var(--color-text-secondary);

    &:hover {
      background: var(--color-warm-muted-surface);
      color: var(--color-text-primary);
    }

    &--current {
      color: var(--color-text-primary);
      font-weight: 600;
      cursor: default;

      &:hover {
        background: transparent;
        color: inherit;
      }
    }

    // G3-F1：合法 drop 目标发光高亮。
    &--drop {
      background: var(--interaction-surface-hover);
      color: var(--color-accent);
      box-shadow:
        0 0 0 1px var(--color-accent),
        0 0 10px color-mix(in srgb, var(--color-accent) 40%, transparent);
    }
  }

  &__crumb-sep {
    color: var(--color-text-tertiary);
    margin: 0 1px;
  }

  &__body {
    overflow-y: auto;
    min-height: 200px;
    max-height: 520px;
  }

  &__table {
    width: 100%;
    table-layout: fixed;
    border-collapse: collapse;
    font-size: 13px;
    color: var(--color-text-primary);
  }

  &__th {
    position: sticky;
    top: 0;
    z-index: 1;
    background: var(--color-surface-soft);
    text-align: left;
    font-size: 12px;
    font-weight: 600;
    color: var(--color-text-secondary);
    padding: 6px 8px;
    border-bottom: 1px solid var(--color-border-soft);
    cursor: pointer;
    user-select: none;
    white-space: nowrap;

    &:hover {
      color: var(--color-text-primary);
    }

    &--right {
      text-align: right;
    }
  }

  &__row {
    cursor: pointer;
    border-bottom: 1px solid var(--color-border-soft);

    &:hover {
      background: var(--color-warm-muted-surface);
    }

    &--selected {
      background: color-mix(in srgb, var(--q-primary) 12%, transparent);
    }
  }

  &__cell {
    padding: 6px 8px;
    vertical-align: middle;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;

    &--time {
      color: var(--color-text-secondary);
      font-size: 12px;
    }

    &--size {
      text-align: right;
      font-variant-numeric: tabular-nums;
      color: var(--color-text-secondary);
    }
  }

  &__name-wrap {
    display: flex;
    align-items: center;
    min-width: 0;
  }

  &__name-text {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  &__icon {
    flex: none;
    // 颜色由全局 .kv-icon--* 色板决定（scoped 覆盖会压过全局类，故此处不设 color）
  }
}
</style>

<!-- G3-F1 拖拽幽灵卡：appendChild 到 body（无 scoped 属性），须非作用域样式。 -->
<style lang="scss">
.knowledge-drag-ghost {
  position: fixed;
  top: -1000px;
  left: -1000px;
  z-index: 9999;
  padding: 6px 14px;
  border-radius: 10px;
  font-size: 13px;
  color: var(--color-text-primary);
  background: var(--glass-elevated);
  backdrop-filter: blur(var(--glass-blur-elevated));
  -webkit-backdrop-filter: blur(var(--glass-blur-elevated));
  border: 1px solid var(--color-accent);
  box-shadow: 0 0 16px color-mix(in srgb, var(--color-accent) 35%, transparent);
  pointer-events: none;
  white-space: nowrap;
}
</style>
