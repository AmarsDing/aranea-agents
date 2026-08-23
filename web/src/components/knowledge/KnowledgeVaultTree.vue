<template>
  <q-card flat class="app-pane-card knowledge-vault-tree">
    <!-- UX-004：降级横幅内嵌重试（此前只能用户自行发现中栏刷新按钮） -->
    <q-banner v-if="error" dense rounded class="app-banner-warning q-ma-sm">
      {{ t('knowledgePage.vaultTreeError') }}
      <template #action>
        <q-btn
          flat
          dense
          no-caps
          color="primary"
          :label="t('knowledgePage.retry')"
          :loading="loading"
          @click="$emit('retry')"
        />
      </template>
    </q-banner>

    <div class="app-pane-card__body knowledge-vault-tree__body">
      <q-tree
        v-if="nodes.length"
        :nodes="nodes"
        node-key="key"
        label-key="label"
        dense
        :selected="selectedKey"
        :expanded="expandedKeys"
        @update:selected="onSelect"
        @update:expanded="onExpanded"
        @lazy-load="onLazy"
      >
        <template #default-header="scope">
          <div
            class="knowledge-vault-tree__node"
            :class="{ 'knowledge-vault-tree__node--drop': dropHoverKey === scope.node.key }"
            @dragover="onNodeDragOver(scope.node, $event)"
            @dragleave="onNodeDragLeave(scope.node)"
            @drop="onNodeDrop(scope.node, $event)"
          >
            <q-icon
              :name="visualOf(scope.node).icon"
              size="18px"
              class="knowledge-vault-tree__icon"
              :class="[visualOf(scope.node).cls, { 'kv-icon--pulse': pulseOf(scope.node) }]"
            />
            <span class="knowledge-vault-tree__label ellipsis" :title="scope.node.label">{{ scope.node.label }}</span>
            <!-- 库节点文档计数徽标（hover 显示完整文案） -->
            <span
              v-if="scope.node.kind === 'vault' && scope.node.docCount != null"
              class="knowledge-vault-tree__count"
            >
              {{ scope.node.docCount }}
              <q-tooltip>{{ t('knowledgePage.treeDocCount', { count: scope.node.docCount }) }}</q-tooltip>
            </span>
            <!-- SP1-F/I-2：team 库「团队」徽标（local 库不显示，保持默认视觉安静） -->
            <span
              v-if="scope.node.kind === 'vault' && scope.node.vaultBackend === 'team'"
              class="knowledge-vault-tree__team-badge"
            >
              {{ t('knowledgePage.vaultTeamBadge') }}
            </span>
            <span
              v-if="scope.node.kind === 'vault' && scope.node.syncState"
              class="knowledge-vault-tree__sync-dot"
              :class="`knowledge-vault-tree__sync-dot--${syncTone(scope.node.syncState)}`"
            >
              <q-tooltip>{{ syncLabel(scope.node.syncState) }}</q-tooltip>
            </span>
            <q-btn
              flat
              dense
              round
              size="xs"
              icon="more_vert"
              class="knowledge-vault-tree__menu-btn"
              :aria-label="t('knowledgePage.treeNodeMenuAria')"
              @click.stop
            >
              <q-menu auto-close>
                <q-list dense class="knowledge-vault-tree__menu">
                  <q-item clickable @click="emitAction('new-dir', scope.node)">
                    <q-item-section avatar><q-icon name="create_new_folder" size="18px" /></q-item-section>
                    <q-item-section>{{ t('knowledgePage.newDirTitle') }}</q-item-section>
                  </q-item>
                  <q-item clickable @click="emitAction('new-doc', scope.node)">
                    <q-item-section avatar><q-icon name="note_add" size="18px" /></q-item-section>
                    <q-item-section>{{ t('knowledgePage.newDocTitle') }}</q-item-section>
                  </q-item>
                  <q-item clickable @click="emitAction('upload', scope.node)">
                    <q-item-section avatar><q-icon name="upload_file" size="18px" /></q-item-section>
                    <q-item-section>{{ t('knowledgePage.uploadHere') }}</q-item-section>
                  </q-item>
                  <template v-if="scope.node.kind === 'vault'">
                    <q-separator />
                    <!-- B2：仅词法库（无语义层）显示「启用语义检索」 -->
                    <q-item
                      v-if="lexicalVaultIds.includes(scope.node.vaultId)"
                      clickable
                      data-test="vault-enable-semantic"
                      @click="emitAction('enable-semantic', scope.node)"
                    >
                      <q-item-section avatar><q-icon name="travel_explore" size="18px" /></q-item-section>
                      <q-item-section>{{ t('knowledgePage.enableSemantic') }}</q-item-section>
                    </q-item>
                    <q-item clickable @click="emitAction('refresh', scope.node)">
                      <q-item-section avatar><q-icon name="refresh" size="18px" /></q-item-section>
                      <q-item-section>{{ t('knowledgePage.refreshVault') }}</q-item-section>
                    </q-item>
                    <q-item clickable class="text-negative" @click="emitAction('delete-vault', scope.node)">
                      <q-item-section avatar
                        ><q-icon name="delete_outline" size="18px" color="negative"
                      /></q-item-section>
                      <q-item-section>{{ t('knowledgePage.vaultDeleteAria') }}</q-item-section>
                    </q-item>
                  </template>
                </q-list>
              </q-menu>
            </q-btn>
          </div>
        </template>
      </q-tree>
      <div v-else-if="!loading && !error" class="knowledge-vault-tree__empty text-caption">
        {{ t('knowledgePage.treeEmpty') }}
      </div>
      <q-linear-progress v-if="loading" indeterminate color="primary" />
    </div>

    <div class="knowledge-vault-tree__footer">
      <q-btn
        flat
        dense
        no-caps
        icon="add"
        :label="t('knowledgePage.newVault')"
        class="knowledge-vault-tree__create"
        @click="$emit('create-vault')"
      />
    </div>
  </q-card>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  isValidDropTarget,
  vaultNodeVisual,
  vaultRootVisual,
  type DragFileRef,
  type VaultNodeVisual,
} from '../../features/knowledge/vaultTreeUi';
import type { VaultLazyLoadPayload, VaultQTreeNode } from '../../features/knowledge/useVaultExplorer';

const props = withDefaults(
  defineProps<{
    /** 一级节点=库（库融树），目录经 lazy-load。 */
    nodes: VaultQTreeNode[];
    selectedKey: string | null;
    expandedKeys: string[];
    loading: boolean;
    error: string;
    /** G3-F1：拖拽中的文件（非空时目录/库节点变为 drop 目标，合法者发光）。 */
    dragFile: DragFileRef | null;
    /** B2：词法库（embedding_model 空）vault id 集——根菜单「启用语义检索」可见性（单一事实源在 collections）。 */
    lexicalVaultIds?: string[];
  }>(),
  { lexicalVaultIds: () => [] },
);

const emit = defineEmits<{
  'select-node': [key: string];
  'update:expanded-keys': [keys: string[]];
  'lazy-load': [payload: VaultLazyLoadPayload];
  'node-action': [action: string, node: VaultQTreeNode];
  'create-vault': [];
  /** G3-F1：drop 到目录/库节点（目标 = 节点 prefix；库节点 = 库根）。 */
  'drop-node': [node: VaultQTreeNode];
  /** UX-004：降级横幅「重试」。 */
  retry: [];
}>();

const { t } = useI18n();

function visualOf(node: VaultQTreeNode): VaultNodeVisual {
  return node.kind === 'vault' ? vaultRootVisual() : vaultNodeVisual({ kind: node.kind });
}

/** vault 同步异常 → 红色脉冲（与文件 error 脉冲同语言）。 */
function pulseOf(node: VaultQTreeNode): boolean {
  return node.kind === 'vault' && node.syncState === 'error';
}

function onSelect(key: string | null) {
  if (key) emit('select-node', key);
}

function onExpanded(keys: string[]) {
  emit('update:expanded-keys', keys);
}

function onLazy(payload: VaultLazyLoadPayload) {
  emit('lazy-load', payload);
}

function emitAction(action: string, node: VaultQTreeNode) {
  emit('node-action', action, node);
}

// ---------- G3-F1 拖拽 drop 目标（V12.5：目录/库节点发光高亮，非法禁用） ----------

/** 当前 dragover 的合法目标节点 key（发光高亮）。 */
const dropHoverKey = ref('');

function nodeTargetValid(node: VaultQTreeNode): boolean {
  return isValidDropTarget(props.dragFile, { vaultId: node.vaultId, prefix: node.prefix });
}

function onNodeDragOver(node: VaultQTreeNode, ev: DragEvent) {
  if (!props.dragFile || !ev.dataTransfer) return;
  if (nodeTargetValid(node)) {
    ev.preventDefault();
    ev.dataTransfer.dropEffect = 'move';
    dropHoverKey.value = node.key;
  } else {
    // 非法目标（跨库/原地）：禁用落点。
    ev.dataTransfer.dropEffect = 'none';
    if (dropHoverKey.value === node.key) dropHoverKey.value = '';
  }
}

function onNodeDragLeave(node: VaultQTreeNode) {
  if (dropHoverKey.value === node.key) dropHoverKey.value = '';
}

function onNodeDrop(node: VaultQTreeNode, ev: DragEvent) {
  ev.preventDefault();
  dropHoverKey.value = '';
  if (nodeTargetValid(node)) emit('drop-node', node);
}

// 拖拽结束（无论落点）清空高亮。
watch(
  () => props.dragFile,
  (f) => {
    if (!f) dropHoverKey.value = '';
  },
);

function syncTone(state: string): string {
  if (state === 'active') return 'ok';
  if (state === 'error') return 'err';
  if (state === 'migrating') return 'warn';
  return 'idle';
}

function syncLabel(state: string): string {
  if (state === 'active') return t('knowledgePage.vaultSyncActive');
  if (state === 'paused') return t('knowledgePage.vaultSyncPaused');
  if (state === 'error') return t('knowledgePage.vaultSyncError');
  if (state === 'migrating') return t('knowledgePage.vaultSyncMigrating');
  return state || t('knowledgePage.vaultSyncActive');
}
</script>

<style lang="scss" scoped>
// G1 科幻节点色板为全局 token（theme/_css-vars-*.sass + app-global.sass .kv-icon--*）；
// 本组件只保留布局样式。
.knowledge-vault-tree {
  --kv-ok: var(--color-success);
  --kv-warn: var(--color-warning);
  --kv-idle: var(--color-icon-muted);

  display: flex;
  flex-direction: column;
  min-height: 0;

  &__body {
    overflow-y: auto;
    min-height: 120px;
    flex: 1;
  }

  &__node {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
    flex: 1;
    padding-right: 2px;
    border-radius: 6px;

    // G3-F1：合法 drop 目标发光高亮。
    &--drop {
      background: var(--interaction-surface-hover);
      box-shadow:
        0 0 0 1px var(--color-accent),
        0 0 12px color-mix(in srgb, var(--color-accent) 45%, transparent);
    }
  }

  &__icon {
    flex: none;
  }

  &__label {
    min-width: 0;
    flex: 1;
    font-size: 13px;
  }

  // 库文档计数徽标：低调灰底数字，不抢同步状态点语义。
  &__count {
    flex: none;
    font-size: 10px;
    line-height: 16px;
    padding: 0 6px;
    border-radius: 8px;
    color: var(--color-text-secondary);
    background: var(--color-warm-muted-surface);
    font-variant-numeric: tabular-nums;
  }

  // SP1-F/I-2：team 库徽标——主色描边小胶囊，与计数徽标（灰底）/同步点（状态色）区分语义。
  &__team-badge {
    flex: none;
    font-size: 10px;
    line-height: 16px;
    padding: 0 6px;
    border-radius: 8px;
    color: var(--color-primary, var(--q-primary));
    border: 1px solid color-mix(in srgb, var(--color-primary, var(--q-primary)) 55%, transparent);
  }

  &__sync-dot {
    flex: none;
    width: 7px;
    height: 7px;
    border-radius: 50%;

    &--ok {
      background: var(--kv-ok);
    }
    &--err {
      background: var(--kv-red);
      box-shadow: 0 0 6px var(--kv-red);
    }
    &--warn {
      background: var(--kv-warn);
    }
    &--idle {
      background: var(--kv-idle);
    }
  }

  &__menu-btn {
    flex: none;
    opacity: 0;
    transition: opacity 0.15s ease;
  }

  &__node:hover &__menu-btn,
  &__menu-btn:focus-visible {
    opacity: 1;
  }

  &__menu {
    min-width: 180px;

    :deep(.q-item__section--avatar) {
      min-width: 36px;
    }
  }

  &__empty {
    padding: 12px;
    color: var(--color-text-secondary);
  }

  &__footer {
    flex: none;
    border-top: 1px solid var(--color-border-soft);
    padding: 4px 8px;
  }

  &__create {
    width: 100%;
    justify-content: flex-start;
    color: var(--color-text-secondary);

    &:hover {
      color: var(--color-accent);
    }
  }
}

// q-tree 选中态：与其他侧栏一致的高亮。
.knowledge-vault-tree :deep(.q-tree__node--selected) {
  background: var(--interaction-surface-hover);
}
</style>
