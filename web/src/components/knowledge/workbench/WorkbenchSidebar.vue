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
        @select-node="$emit('select-node', $event)"
        @update:expanded-keys="$emit('update:expanded-keys', $event)"
        @lazy-load="$emit('lazy-load', $event)"
        @node-action="(a, n) => $emit('node-action', a, n)"
        @create-vault="$emit('create-vault')"
        @drop-node="$emit('drop-node', $event)"
        @retry="$emit('retry')"
      />
    </GlassPanel>

    <!-- 当前目录文件列表（点击打开为工作台 tab） -->
    <GlassPanel :title="t('knowledgePage.workbench.filesTitle')" icon="description" flush class="kb-sidebar__files">
      <div v-if="files.length" class="kb-sidebar__file-list">
        <button
          v-for="f in files"
          :key="f.doc_id || f.path"
          type="button"
          class="kb-sidebar__file"
          :class="{ 'kb-sidebar__file--active': f.doc_id === activeDocId }"
          @click="$emit('open-file', f)"
        >
          <q-icon :name="iconOf(f)" size="16px" class="kb-sidebar__file-icon" />
          <span class="kb-sidebar__file-name ellipsis" :title="f.name">{{ f.name }}</span>
          <q-icon
            v-if="f.status === 'error'"
            name="error_outline"
            size="14px"
            class="kb-sidebar__file-err"
            :title="f.error_message"
          />
        </button>
      </div>
      <div v-else class="kb-sidebar__files-empty text-caption">
        {{ t('knowledgePage.workbench.filesEmpty') }}
      </div>
    </GlassPanel>
  </div>
</template>

<script setup lang="ts">
// SP2 §SP2-1 左栏：Vault 树 + 当前目录文件列表（Obsidian 文件资源管理器语义）。
import { useI18n } from 'vue-i18n';
import GlassPanel from '../effects/GlassPanel.vue';
import KnowledgeVaultTree from '../KnowledgeVaultTree.vue';
import type { DragFileRef } from '../../../features/knowledge/vaultTreeUi';
import type { VaultLazyLoadPayload, VaultQTreeNode } from '../../../features/knowledge/useVaultExplorer';
import type { VaultTreeNode } from '../../../features/knowledge/types';

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
}>();

defineEmits<{
  'select-node': [key: string];
  'update:expanded-keys': [keys: string[]];
  'lazy-load': [payload: VaultLazyLoadPayload];
  'node-action': [action: string, node: VaultQTreeNode];
  'create-vault': [];
  'drop-node': [node: VaultQTreeNode];
  retry: [];
  'open-file': [node: VaultTreeNode];
}>();

const { t } = useI18n();

function iconOf(f: VaultTreeNode): string {
  const p = f.path.toLowerCase();
  if (/\.(md|markdown)$/.test(p)) return 'description';
  if (/\.(png|jpe?g|gif|webp|svg)$/.test(p)) return 'image';
  if (/\.(mp3|wav|ogg|m4a)$/.test(p)) return 'audiotrack';
  if (/\.(mp4|webm|mov)$/.test(p)) return 'movie';
  if (/\.pdf$/.test(p)) return 'picture_as_pdf';
  return 'insert_drive_file';
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
    color: #ff6b81

  &__files-empty
    padding: 14px
    color: var(--kb-text-dim)
</style>
