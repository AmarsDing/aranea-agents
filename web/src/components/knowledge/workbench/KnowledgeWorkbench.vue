<template>
  <div class="kb-workbench">
    <!-- 深空背景：粒子 + 极光光斑（不参与交互） -->
    <ParticleField class="kb-workbench__particles" />
    <div class="kb-workbench__aurora kb-workbench__aurora--cyan" />
    <div class="kb-workbench__aurora kb-workbench__aurora--violet" />

    <div class="kb-workbench__frame">
      <WorkbenchTopBar
        :collections="collections"
        :current-vault-id="currentVaultId"
        @switch-vault="$emit('switch-vault', $event)"
        @open-quick-switcher="openQuickSwitcher"
        @open-command-palette="openCommandPalette"
        @open-graph="$emit('open-graph')"
        @open-settings="$emit('open-settings')"
      />

      <div class="kb-workbench__grid">
        <WorkbenchSidebar
          class="kb-workbench__left"
          :nodes="nodes"
          :selected-key="selectedKey"
          :expanded-keys="expandedKeys"
          :loading="treeLoading"
          :error="treeError"
          :drag-file="dragFile"
          :files="files"
          :active-doc-id="workbench.activeTabId.value"
          @select-node="$emit('select-node', $event)"
          @update:expanded-keys="$emit('update:expanded-keys', $event)"
          @lazy-load="$emit('lazy-load', $event)"
          @node-action="(a, n) => $emit('node-action', a, n)"
          @create-vault="$emit('create-vault')"
          @drop-node="$emit('drop-node', $event)"
          @retry="$emit('retry')"
          @open-file="openFile"
        />

        <WorkbenchTabs
          class="kb-workbench__center"
          :tabs="workbench.tabs.value"
          :active-tab-id="workbench.activeTabId.value"
          :candidates="candidates"
          @activate="workbench.activateTab"
          @close="workbench.closeTab"
          @save="onSave"
          @toggle-mode="workbench.toggleMode"
          @update-content="workbench.updateContent"
          @open-doc="openDocByName"
          @create-doc="createDocByName"
        />

        <!-- 右栏：五面板占位（SP2-5 接入） -->
        <GlassPanel
          class="kb-workbench__right"
          :title="t('knowledgePage.workbench.panelsPlaceholder')"
          icon="dashboard_customize"
        >
          <div class="kb-workbench__panels-hint">
            {{ t('knowledgePage.workbench.panelsPlaceholderHint') }}
          </div>
        </GlassPanel>
      </div>
    </div>

    <!-- 脏关闭确认 -->
    <q-dialog :model-value="!!confirmCloseTab" @update:model-value="onCancelClose">
      <GlassPanel strong :title="t('knowledgePage.workbench.closeConfirmTitle')" class="kb-workbench__confirm">
        <div class="kb-workbench__confirm-hint">
          {{ t('knowledgePage.workbench.closeConfirmHint', { title: confirmCloseTab?.title ?? '' }) }}
        </div>
        <div class="kb-workbench__confirm-actions">
          <q-btn flat no-caps :label="t('common.cancel')" @click="onCancelClose" />
          <q-btn
            flat
            no-caps
            color="negative"
            :label="t('knowledgePage.workbench.closeConfirmDiscard')"
            @click="onDiscardClose"
          />
          <q-btn
            unelevated
            no-caps
            color="primary"
            :label="t('knowledgePage.workbench.closeConfirmSave')"
            :loading="confirmSaving"
            @click="onSaveAndClose"
          />
        </div>
      </GlassPanel>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
// SP2 §SP2-1 工作台根：装配 TopBar + 三栏（树 / 标签页 / 联动面板）+ 浮层状态。
// 数据流纪律：全部状态经 props 注入的 workbench 状态机与 explorer，组件不各自拉数。
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import ParticleField from '../effects/ParticleField.vue';
import GlassPanel from '../effects/GlassPanel.vue';
import WorkbenchTopBar from './WorkbenchTopBar.vue';
import WorkbenchSidebar from './WorkbenchSidebar.vue';
import WorkbenchTabs from './WorkbenchTabs.vue';
import type { KnowledgeWorkbench as Workbench } from '../../../features/knowledge/useKnowledgeWorkbench';
import type { DragFileRef } from '../../../features/knowledge/vaultTreeUi';
import type { VaultLazyLoadPayload, VaultQTreeNode } from '../../../features/knowledge/useVaultExplorer';
import type { KnowledgeCollection, KnowledgeDocument, VaultTreeNode } from '../../../features/knowledge/types';

const props = defineProps<{
  workbench: Workbench;
  collections: KnowledgeCollection[];
  /** 完整文档列表（openFile 时解析 mime_type 判定可编辑性） */
  documents: KnowledgeDocument[];
  currentVaultId: string;
  nodes: VaultQTreeNode[];
  selectedKey: string | null;
  expandedKeys: string[];
  treeLoading: boolean;
  treeError: string;
  dragFile: DragFileRef | null;
  /** 当前目录文件（explorer.currentFiles） */
  files: VaultTreeNode[];
}>();

defineEmits<{
  'switch-vault': [id: string];
  'select-node': [key: string];
  'update:expanded-keys': [keys: string[]];
  'lazy-load': [payload: VaultLazyLoadPayload];
  'node-action': [action: string, node: VaultQTreeNode];
  'create-vault': [];
  'drop-node': [node: VaultQTreeNode];
  retry: [];
  'open-graph': [];
  'open-settings': [];
}>();

const { t } = useI18n();
const $q = useQuasar();

// ⌘O / ⌘K 浮层状态（SP2-6 接入浮层组件）
const quickSwitcherOpen = ref(false);
const commandPaletteOpen = ref(false);

function openQuickSwitcher() {
  quickSwitcherOpen.value = true;
}

function openCommandPalette() {
  commandPaletteOpen.value = true;
}

/** 文件节点 → 完整文档（mime_type 供 isEditable 判定）；列表未覆盖时按节点合成。 */
function resolveDocument(node: VaultTreeNode): KnowledgeDocument {
  const found = props.documents.find((d) => d.id === node.doc_id);
  if (found) return found;
  return {
    id: node.doc_id,
    source: node.name,
    rel_path: node.path,
    collection_id: props.currentVaultId,
    mime_type: '',
  } as KnowledgeDocument;
}

function openFile(node: VaultTreeNode) {
  if (!node.doc_id) return;
  void props.workbench.openDoc(resolveDocument(node));
}

async function onSave(docId: string) {
  const ok = await props.workbench.saveTab(docId);
  if (!ok) {
    $q.notify({ type: 'warning', message: t('knowledgePage.workbench.conflictHint'), timeout: 6000 });
  }
}

// ---------- 脏关闭确认 ----------
const confirmSaving = ref(false);
const confirmCloseTab = computed(() => {
  const id = props.workbench.confirmCloseId.value;
  return props.workbench.tabs.value.find((x) => x.docId === id) ?? null;
});

function onCancelClose() {
  props.workbench.dismissCloseConfirm();
}

function onDiscardClose() {
  const id = props.workbench.confirmCloseId.value;
  if (id) props.workbench.closeTab(id, { discard: true });
}

async function onSaveAndClose() {
  const id = props.workbench.confirmCloseId.value;
  if (!id) return;
  confirmSaving.value = true;
  try {
    const ok = await props.workbench.saveTab(id);
    if (ok) {
      props.workbench.closeTab(id, { discard: true });
    } else {
      // CAS 冲突：保留 tab 让用户决策（内容/冲突标记已由状态机刷新）
      props.workbench.dismissCloseConfirm();
    }
  } finally {
    confirmSaving.value = false;
  }
}
</script>

<style lang="sass" scoped>
.kb-workbench
  position: relative
  height: 100%
  min-height: 0
  overflow: hidden
  background: var(--kb-bg-deep)
  color: var(--kb-text-primary)

  &__particles
    position: absolute
    inset: 0
    pointer-events: none

  &__aurora
    position: absolute
    width: 480px
    height: 480px
    border-radius: 50%
    filter: blur(120px)
    opacity: 0.16
    pointer-events: none

    &--cyan
      top: -160px
      right: -80px
      background: radial-gradient(circle, var(--kb-accent-cyan), transparent 70%)

    &--violet
      bottom: -200px
      left: -120px
      background: radial-gradient(circle, var(--kb-accent-violet), transparent 70%)

  &__frame
    position: relative
    z-index: 1
    display: flex
    flex-direction: column
    gap: 10px
    height: 100%
    min-height: 0
    padding: 10px

  &__grid
    flex: 1
    min-height: 0
    display: grid
    grid-template-columns: 280px minmax(0, 1fr) 320px
    gap: 10px

  &__left,
  &__center,
  &__right
    min-height: 0
    min-width: 0

  &__panels-hint
    font-size: 12px
    color: var(--kb-text-dim)
    line-height: 1.8

  &__confirm
    width: 420px
    max-width: 90vw

  &__confirm-hint
    font-size: 13px
    color: var(--kb-text-primary)
    margin-bottom: 16px

  &__confirm-actions
    display: flex
    justify-content: flex-end
    gap: 8px

@media (max-width: 1100px)
  .kb-workbench__grid
    grid-template-columns: 240px minmax(0, 1fr)

  .kb-workbench__right
    display: none
</style>
