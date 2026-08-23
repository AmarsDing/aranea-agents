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
        v-if="files.length"
        class="kb-sidebar__file-list"
        :class="{ 'kb-sidebar__file-list--drop': dropHover }"
        @dragover="onListDragOver"
        @dragleave="onListDragLeave"
        @drop="onListDrop"
      >
        <div
          v-for="f in files"
          :key="f.doc_id || f.path"
          class="kb-sidebar__file"
          :class="{ 'kb-sidebar__file--active': f.doc_id === activeDocId }"
          draggable="true"
          @click="$emit('open-file', f)"
          @dragstart="onFileDragStart(f, $event)"
          @dragend="$emit('file-drag-end')"
        >
          <q-icon :name="iconOf(f)" size="16px" class="kb-sidebar__file-icon" />
          <span class="kb-sidebar__file-name ellipsis" :title="f.name">{{ f.name }}</span>
          <q-tooltip v-if="f.summary || f.doc_type || (f.tags && f.tags.length)" :delay="400">
            <DocumentSummaryCard :summary="f.summary" :tags="f.tags" :doc-type="f.doc_type" />
          </q-tooltip>
          <q-icon
            v-if="f.status === 'error'"
            name="error_outline"
            size="14px"
            class="kb-sidebar__file-err"
            :title="f.error_message"
          />
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
                <q-item
                  clickable
                  data-test="file-private"
                  @click="$emit('file-action', 'private', f)"
                >
                  <q-item-section avatar><q-icon name="lock" size="18px" /></q-item-section>
                  <q-item-section>{{ t('knowledgePage.workbench.filePrivate') }}</q-item-section>
                </q-item>
                <q-item
                  clickable
                  data-test="file-collection"
                  @click="$emit('file-action', 'collection', f)"
                >
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
        {{ t('knowledgePage.workbench.filesEmpty') }}
      </div>
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
import { isValidDropTarget, type DragFileRef } from '../../../features/knowledge/vaultTreeUi';
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

const { t } = useI18n();

/** B1：当前集合有语义层（embedding_model 非空）才可重新向量化；词法库置灰。 */
const currentHasSemantic = computed(() => {
  const c = props.collections.find((x) => x.id === props.currentVaultId);
  return Boolean(c?.embedding_model);
});

/** B2：词法库（embedding_model 空）id 集 → 树根菜单「启用语义检索」可见性。 */
const lexicalVaultIds = computed(() => props.collections.filter((c) => !c.embedding_model).map((c) => c.id));

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
      box-shadow: inset 0 0 0 1px var(--kb-accent-cyan), inset 0 0 18px rgba(79, 216, 255, 0.12)

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
      background: rgba(79, 216, 255, 0.12)
      color: var(--kb-accent-cyan)

  &__file-icon
    flex: none
    color: var(--kb-text-dim)

  &__file--active &__file-icon
    color: var(--kb-accent-cyan)

  &__file-name
    min-width: 0
    flex: 1

  &__file-err
    flex: none
    color: var(--kb-danger)

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
</style>
