<template>
  <div class="kb-sidebar">
    <!-- Vault 树（库 + 目录；复用 KnowledgeVaultTree 内核，玻璃换肤由 .kb-workbench 令牌注入） -->
    <GlassPanel flush class="kb-sidebar__tree">
      <KnowledgeVaultTree
        :nodes="nodes"
        :selected-key="selectedKey"
        :expanded-keys="expandedKeys"
        :loading="loading"
        :error="error"
        :drag-file="dragFile"
        :lexical-vault-ids="lexicalVaultIds"
        @select-node="$emit('select-node', $event)"
        @update:expanded-keys="$emit('update:expanded-keys', $event)"
        @lazy-load="$emit('lazy-load', $event)"
        @node-action="(a, n) => $emit('node-action', a, n)"
        @create-vault="$emit('create-vault')"
        @drop-node="$emit('drop-node', $event)"
        @retry="$emit('retry')"
      />
    </GlassPanel>

    <!-- 当前目录文件列表（点击打开为工作台 tab；SP2-8：行操作菜单 + 拖拽移动） -->
    <GlassPanel :title="t('knowledgePage.workbench.filesTitle')" icon="description" flush class="kb-sidebar__files">
      <template #header-actions>
        <!-- U0-5：仅看解析失败过滤（status=error 收敛排查视图） -->
        <q-btn
          flat
          dense
          round
          size="xs"
          icon="error_outline"
          class="kb-sidebar__new"
          :class="{ 'kb-sidebar__filter--on': onlyFailed }"
          :title="t('knowledgePage.workbench.onlyFailed')"
          data-test="filter-failed"
          @click="onlyFailed = !onlyFailed"
        />
        <q-btn
          flat
          dense
          round
          size="xs"
          icon="note_add"
          class="kb-sidebar__new"
          :title="t('knowledgePage.workbench.commands.new-note')"
          @click="$emit('new-note')"
        />
        <q-btn
          flat
          dense
          round
          size="xs"
          icon="create_new_folder"
          class="kb-sidebar__new"
          :title="t('knowledgePage.workbench.commands.new-folder')"
          @click="$emit('new-folder')"
        />
      </template>
      <div
        v-if="displayFiles.length"
        class="kb-sidebar__file-list"
        :class="{ 'kb-sidebar__file-list--drop': dropHover }"
        @dragover="onListDragOver"
        @dragleave="onListDragLeave"
        @drop="onListDrop"
      >
        <div
          v-for="f in displayFiles"
          :key="f.doc_id || f.path"
          class="kb-sidebar__file"
          :class="{
            'kb-sidebar__file--active': f.doc_id === activeDocId,
            'kb-sidebar__file--selected': selection.isSelected(idOf(f)),
          }"
          draggable="true"
          @click="onRowClick(f, $event)"
          @dragstart="onFileDragStart(f, $event)"
          @dragend="$emit('file-drag-end')"
        >
          <!-- U0-4：hover 浮现多选框（已选行常显）；ctrl/shift+click 语义见 onRowClick -->
          <q-btn
            flat
            dense
            round
            size="xs"
            :icon="selection.isSelected(idOf(f)) ? 'check_box' : 'check_box_outline_blank'"
            class="kb-sidebar__file-check"
            :class="{ 'kb-sidebar__file-check--on': selection.isSelected(idOf(f)) }"
            :aria-label="t('knowledgePage.workbench.bulk.selectRow')"
            data-test="file-check"
            @click.stop="selection.toggle(idOf(f))"
          />
          <q-icon :name="iconOf(f)" size="16px" class="kb-sidebar__file-icon" />
          <!-- U0-5：解析状态点（error 时 tooltip 带 error_message；其余状态纯色调提示） -->
          <span
            v-if="statusTone(f) !== 'none'"
            class="kb-sidebar__dot"
            :class="`kb-sidebar__dot--${statusTone(f)}`"
            :title="dotTitle(f)"
          />
          <span class="kb-sidebar__file-name ellipsis" :title="f.name">{{ f.name }}</span>
          <!-- U0-5：chunk_count 角标（缺字段/0 时隐藏） -->
          <span
            v-if="chunkLabel(f)"
            class="kb-sidebar__chunks"
            :title="t('knowledgePage.workbench.chunksTitle', { n: chunkLabel(f) })"
            >{{ chunkLabel(f) }}</span
          >
          <!-- U0-5：相对 updated_at（3 天前；非法/缺失隐藏） -->
          <span v-if="relTimeLabel(f)" class="kb-sidebar__mtime">{{ relTimeLabel(f) }}</span>
          <q-tooltip v-if="f.summary || f.doc_type || (f.tags && f.tags.length)" :delay="400">
            <DocumentSummaryCard :summary="f.summary" :tags="f.tags" :doc-type="f.doc_type" />
          </q-tooltip>
          <q-btn
            flat
            dense
            round
            size="xs"
            icon="more_vert"
            class="kb-sidebar__file-menu"
            :aria-label="t('knowledgePage.workbench.fileMenuAria')"
            @click.stop
          >
            <q-menu auto-close>
              <q-list dense class="kb-sidebar__menu">
                <q-item clickable @click="$emit('file-action', 'move', f)">
                  <q-item-section avatar><q-icon name="drive_file_move" size="18px" /></q-item-section>
                  <q-item-section>{{ t('knowledgePage.workbench.fileMove') }}</q-item-section>
                </q-item>
                <q-item clickable @click="$emit('file-action', 'download', f)">
                  <q-item-section avatar><q-icon name="download" size="18px" /></q-item-section>
                  <q-item-section>{{ t('knowledgePage.workbench.fileDownload') }}</q-item-section>
                </q-item>
                <!-- B1 入口①：文档重嵌入（词法库无语义层时置灰 + tooltip 说明） -->
                <q-item
                  clickable
                  data-test="file-reembed"
                  :disable="!currentHasSemantic"
                  @click="$emit('file-action', 'reembed', f)"
                >
                  <q-item-section avatar><q-icon name="psychology" size="18px" /></q-item-section>
                  <q-item-section>{{ t('knowledgePage.reembedDocument') }}</q-item-section>
                  <q-tooltip v-if="!currentHasSemantic">{{ t('knowledgePage.reembedNoSemantic') }}</q-tooltip>
                </q-item>
                <q-item clickable data-test="file-private" @click="$emit('file-action', 'private', f)">
                  <q-item-section avatar><q-icon name="lock" size="18px" /></q-item-section>
                  <q-item-section>{{ t('knowledgePage.workbench.filePrivate') }}</q-item-section>
                </q-item>
                <q-item clickable data-test="file-collection" @click="$emit('file-action', 'collection', f)">
                  <q-item-section avatar><q-icon name="folder_shared" size="18px" /></q-item-section>
                  <q-item-section>{{ t('knowledgePage.workbench.fileCollectionVisible') }}</q-item-section>
                </q-item>
                <q-separator />
                <q-item clickable class="kb-sidebar__menu-danger" @click="$emit('file-action', 'delete', f)">
                  <q-item-section avatar><q-icon name="delete_outline" size="18px" /></q-item-section>
                  <q-item-section>{{ t('knowledgePage.workbench.fileDelete') }}</q-item-section>
                </q-item>
              </q-list>
            </q-menu>
          </q-btn>
        </div>
      </div>
      <div v-else class="kb-sidebar__files-empty text-caption">
        {{ emptyHint }}
      </div>

      <!-- U0-4：批量操作条（选中 >0 浮现；动作经 useBulkDocActions 走既有 store/io 出口） -->
      <BulkActionsBar
        :count="selectionCount"
        :has-semantic="currentHasSemantic"
        :move-options="moveOptions"
        @move="onBulkMove"
        @delete="onBulkDelete"
        @reembed="onBulkReembed"
        @clear="selection.clear()"
      />
    </GlassPanel>

    <!-- SP2-8：左栏底部插槽（上传队列收纳位） -->
    <slot name="footer" />
  </div>
</template>

<script setup lang="ts">
// SP2 §SP2-1 左栏：Vault 树 + 当前目录文件列表（Obsidian 文件资源管理器语义）。
// SP2-8：文件行操作菜单（移动/下载/删除）+ 行拖拽 + 列表 drop 到当前目录。
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import GlassPanel from '../effects/GlassPanel.vue';
import KnowledgeVaultTree from '../KnowledgeVaultTree.vue';
import DocumentSummaryCard from '../DocumentSummaryCard.vue';
import BulkActionsBar from './BulkActionsBar.vue';
import { isValidDropTarget, type DragFileRef } from '../../../features/knowledge/vaultTreeUi';
import { useBulkDocActions } from '../../../features/knowledge/useBulkDocActions';
import {
  docStatusTone,
  formatChunkCount,
  relativeTimeParts,
  useSidebarSelection,
  type DocStatusTone,
} from '../../../features/knowledge/useSidebarSelection';
import type { VaultLazyLoadPayload, VaultQTreeNode } from '../../../features/knowledge/useVaultExplorer';
import type { KnowledgeCollection, VaultTreeNode } from '../../../features/knowledge/types';

const props = withDefaults(
  defineProps<{
    nodes: VaultQTreeNode[];
    selectedKey: string | null;
    expandedKeys: string[];
    loading: boolean;
    error: string;
    dragFile: DragFileRef | null;
    /** 当前目录文件（explorer.currentFiles） */
    files: VaultTreeNode[];
    /** 工作台当前活动文档（高亮） */
    activeDocId: string;
    /** SP2-8：当前库 id + 目录 prefix（文件列表 drop 合法性判定） */
    currentVaultId: string;
    currentPrefix: string;
    /** B1：集合列表（按 currentVaultId 查语义层标记，驱动重嵌入菜单置灰） */
    collections?: KnowledgeCollection[];
  }>(),
  { collections: () => [] },
);

const emit = defineEmits<{
  'select-node': [key: string];
  'update:expanded-keys': [keys: string[]];
  'lazy-load': [payload: VaultLazyLoadPayload];
  'node-action': [action: string, node: VaultQTreeNode];
  'create-vault': [];
  'drop-node': [node: VaultQTreeNode];
  retry: [];
  'open-file': [node: VaultTreeNode];
  /** SP2-7：当前目录新建笔记/文件夹（KnowledgeWorkbench 弹命名框并落盘） */
  'new-note': [];
  'new-folder': [];
  /** SP2-8：文件行操作（move/download/delete，页面侧弹窗/执行） */
  'file-action': [action: string, node: VaultTreeNode];
  /** SP2-8：文件行拖拽（dragstart 记录 / dragend 清空） */
  'file-drag-start': [node: VaultTreeNode];
  'file-drag-end': [];
  /** SP2-8：drop 到文件列表 = 移到当前目录 */
  'drop-current-dir': [];
}>();

const { t, te } = useI18n();

/** B1：当前集合有语义层（embedding_model 非空）才可重新向量化；词法库置灰。 */
const currentHasSemantic = computed(() => {
  const c = props.collections.find((x) => x.id === props.currentVaultId);
  return Boolean(c?.embedding_model);
});

/** B2：词法库（embedding_model 空）id 集 → 树根菜单「启用语义检索」可见性。 */
const lexicalVaultIds = computed(() => props.collections.filter((c) => !c.embedding_model).map((c) => c.id));

// ---------- U0-4 多选（ctrl/shift+click）+ 批量操作 ----------

/** 选择域 = 库 + 当前目录；切换目录自动清空。 */
const selection = useSidebarSelection(() => `${props.currentVaultId}|${props.currentPrefix}`);
const selectionCount = selection.count;

/** 行选择 id：doc_id 优先，无 doc_id（防御）退回 path。 */
function idOf(f: VaultTreeNode): string {
  return f.doc_id || f.path;
}

/** 行点击：shift → 范围选；ctrl/cmd → 切换单选；否则维持原「打开文件」语义。 */
function onRowClick(f: VaultTreeNode, ev: MouseEvent) {
  const id = idOf(f);
  if (ev.shiftKey) {
    ev.preventDefault();
    selection.rangeTo(id, displayFiles.value.map(idOf));
    return;
  }
  if (ev.ctrlKey || ev.metaKey) {
    ev.preventDefault();
    selection.toggle(id);
    return;
  }
  emit('open-file', f);
}

const bulk = useBulkDocActions({
  vaultId: () => props.currentVaultId,
  collections: () => props.collections,
});
const moveOptions = bulk.moveOptions;

/** 选中集 → 当前目录下的实际行节点（批量动作输入）。 */
function selectedNodes(): VaultTreeNode[] {
  const ids = new Set(selection.selectedIds.value);
  return props.files.filter((f) => ids.has(idOf(f)));
}

async function onBulkMove(targetId: string) {
  const out = await bulk.moveSelected(selectedNodes(), targetId);
  if (out) selection.retain(out.failedIds);
}

async function onBulkDelete() {
  const out = await bulk.deleteSelected(selectedNodes());
  if (out) selection.retain(out.failedIds);
}

function onBulkReembed() {
  bulk.reembedSelected(selectedNodes());
}

// ---------- U0-5 行信息密度（状态点/相对时间/角标/仅看失败） ----------

/** 「仅看解析失败」过滤 toggle。 */
const onlyFailed = ref(false);

const displayFiles = computed(() => (onlyFailed.value ? props.files.filter((f) => f.status === 'error') : props.files));

const emptyHint = computed(() =>
  onlyFailed.value ? t('knowledgePage.workbench.filesEmptyFiltered') : t('knowledgePage.workbench.filesEmpty'),
);

function statusTone(f: VaultTreeNode): DocStatusTone {
  return docStatusTone(f.status);
}

/** 状态点 tooltip：error 时带 error_message；其余为状态文案（i18n 键缺失时回退原始 status）。 */
function dotTitle(f: VaultTreeNode): string {
  const key = `knowledgePage.workbench.status.${f.status}`;
  const base = te(key) ? t(key) : f.status;
  return f.status === 'error' && f.error_message ? `${base}: ${f.error_message}` : base;
}

/** chunk_count 角标（字段缺失/0 → 隐藏）。VaultTreeNode 契约暂无该字段，防御性读取。 */
function chunkLabel(f: VaultTreeNode): string {
  return formatChunkCount((f as { chunk_count?: number }).chunk_count);
}

/** 相对 updated_at（justNow/minutes/hours/days/date 分段）；非法输入隐藏。 */
function relTimeLabel(f: VaultTreeNode): string {
  const p = relativeTimeParts(f.updated_at);
  if (!p) return '';
  if (p.kind === 'date') return p.date;
  if (p.kind === 'justNow') return t('knowledgePage.workbench.relTime.justNow');
  return t(`knowledgePage.workbench.relTime.${p.kind}`, { n: p.n });
}

function iconOf(f: VaultTreeNode): string {
  const p = f.path.toLowerCase();
  if (/\.(md|markdown)$/.test(p)) return 'description';
  if (/\.(png|jpe?g|gif|webp|svg)$/.test(p)) return 'image';
  if (/\.(mp3|wav|ogg|m4a)$/.test(p)) return 'audiotrack';
  if (/\.(mp4|webm|mov)$/.test(p)) return 'movie';
  if (/\.pdf$/.test(p)) return 'picture_as_pdf';
  return 'insert_drive_file';
}

// ---------- SP2-8 文件行拖拽 + 列表 drop（移到当前目录） ----------

/** 列表 drop 高亮（仅合法目标）。 */
const dropHover = ref(false);

function currentDirValid(): boolean {
  return isValidDropTarget(props.dragFile, { vaultId: props.currentVaultId, prefix: props.currentPrefix });
}

function onFileDragStart(f: VaultTreeNode, ev: DragEvent) {
  if (ev.dataTransfer) {
    ev.dataTransfer.effectAllowed = 'move';
    // Firefox 要求 setData 才触发拖拽。
    ev.dataTransfer.setData('text/plain', f.name);
  }
  emit('file-drag-start', f);
}

function onListDragOver(ev: DragEvent) {
  if (!props.dragFile || !ev.dataTransfer) return;
  if (currentDirValid()) {
    ev.preventDefault();
    ev.dataTransfer.dropEffect = 'move';
    dropHover.value = true;
  } else {
    ev.dataTransfer.dropEffect = 'none';
    dropHover.value = false;
  }
}

function onListDragLeave() {
  dropHover.value = false;
}

function onListDrop(ev: DragEvent) {
  ev.preventDefault();
  dropHover.value = false;
  if (currentDirValid()) emit('drop-current-dir');
}
</script>

<style lang="sass" scoped>
.kb-sidebar
  display: flex
  flex-direction: column
  gap: 10px
  min-height: 0

  &__tree
    flex: 1 1 55%
    min-height: 0

  &__files
    flex: 1 1 45%
    min-height: 120px

  &__file-list
    display: flex
    flex-direction: column
    padding: 4px
    border-radius: 0 0 var(--kb-radius-glass) var(--kb-radius-glass)
    transition: box-shadow 0.15s ease

    // SP2-8：drop 到列表 = 移到当前目录（合法目标发光，与树节点同语言）
    &--drop
      box-shadow: inset 0 0 0 1px var(--kb-accent-cyan), inset 0 0 18px rgba(var(--hud-sky-rgb), 0.12)

  &__file
    display: flex
    align-items: center
    gap: 8px
    width: 100%
    padding: 6px 10px
    border: 0
    border-radius: 8px
    background: transparent
    color: var(--kb-text-primary)
    font-size: 13px
    text-align: left
    cursor: pointer
    transition: background 0.15s ease, color 0.15s ease

    &:hover
      background: var(--kb-glass-highlight)

    &--active
      background: rgba(var(--hud-sky-rgb), 0.12)
      color: var(--kb-accent-cyan)

    // U0-4：多选高亮（与 active 同语言、更淡）
    &--selected
      background: rgba(var(--hud-sky-rgb), 0.08)

  &__file-icon
    flex: none
    color: var(--kb-text-dim)

  &__file--active &__file-icon
    color: var(--kb-accent-cyan)

  // U0-4：hover 浮现多选框（已选行常显），与行操作菜单同一显现机制
  &__file-check
    flex: none
    opacity: 0
    transition: opacity 0.15s ease
    color: var(--kb-text-dim)

    &--on
      opacity: 1
      color: var(--kb-accent-cyan)

  &__file:hover &__file-check,
  &__file-check:focus-visible
    opacity: 1

  // U0-5：解析状态点
  &__dot
    flex: none
    width: 7px
    height: 7px
    border-radius: 50%
    background: var(--kb-text-dim)

    &--ok
      background: var(--kb-ok, #3ddc97)

    &--busy
      background: var(--kb-accent-cyan)
      animation: kb-sidebar-dot-pulse 1.2s ease-in-out infinite

    &--wait
      background: var(--kb-text-dim)

    &--err
      background: var(--kb-danger)

  // U0-5：chunk_count 角标 / 相对时间（次级信息，弱化层级；缺字段时模板侧隐藏）
  &__chunks
    flex: none
    padding: 0 5px
    border-radius: 6px
    background: var(--kb-glass-highlight)
    color: var(--kb-text-dim)
    font-size: 10px
    line-height: 16px

  &__mtime
    flex: none
    color: var(--kb-text-dim)
    font-size: 11px
    white-space: nowrap

  &__file-name
    min-width: 0
    flex: 1

  // SP2-8：行操作菜单按钮（hover 显现，与树节点菜单同语言）
  &__file-menu
    flex: none
    opacity: 0
    transition: opacity 0.15s ease
    color: var(--kb-text-dim)

  &__file:hover &__file-menu,
  &__file-menu:focus-visible
    opacity: 1

  &__menu
    min-width: 160px

    :deep(.q-item__section--avatar)
      min-width: 36px

  &__menu-danger
    color: var(--kb-danger)

  &__files-empty
    padding: 14px
    color: var(--kb-text-dim)

  &__new
    color: var(--kb-text-dim)

    &:hover
      color: var(--kb-accent-cyan)

  // U0-5：「仅看解析失败」激活态
  &__filter--on
    color: var(--kb-danger)

@keyframes kb-sidebar-dot-pulse
  0%, 100%
    opacity: 0.35
  50%
    opacity: 1
</style>
