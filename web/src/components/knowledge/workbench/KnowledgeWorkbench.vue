<template>
  <div class="kb-workbench">
    <div class="kb-workbench__frame">
      <WorkbenchTopBar
        :collections="collections"
        :current-vault-id="currentVaultId"
        @switch-vault="$emit('switch-vault', $event)"
        @open-quick-switcher="openQuickSwitcher"
        @open-command-palette="openCommandPalette"
        @open-search="openSearch"
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
          :current-vault-id="currentVaultId"
          :current-prefix="currentPrefix"
          :collections="collections"
          @select-node="$emit('select-node', $event)"
          @update:expanded-keys="$emit('update:expanded-keys', $event)"
          @lazy-load="$emit('lazy-load', $event)"
          @node-action="(a, n) => $emit('node-action', a, n)"
          @create-vault="$emit('create-vault')"
          @drop-node="$emit('drop-node', $event)"
          @retry="$emit('retry')"
          @open-file="openFile"
          @new-note="createNote"
          @new-folder="createFolder"
          @file-action="(a, n) => $emit('file-action', a, n)"
          @file-drag-start="$emit('file-drag-start', $event)"
          @file-drag-end="$emit('file-drag-end')"
          @drop-current-dir="$emit('drop-current-dir')"
        >
          <!-- SP2-8：上传队列收纳位（页面注入 KnowledgeUploadQueue） -->
          <template #footer>
            <slot name="left-footer" />
          </template>
        </WorkbenchSidebar>

        <WorkbenchTabs
          ref="tabsRef"
          class="kb-workbench__center"
          :tabs="workbench.tabs.value"
          :active-tab-id="workbench.activeTabId.value"
          :candidates="candidates"
          :recent-docs="recentDocs"
          :get-headings="getHeadingsFor"
          :link-recency-rank="linkRecencyRank"
          @activate="workbench.activateTab"
          @close="workbench.closeTab"
          @reorder="workbench.reorderTabs"
          @save="saveDoc"
          @toggle-mode="workbench.toggleMode"
          @update-content="workbench.updateContent"
          @open-doc="openDocByName"
          @create-doc="createDocByName"
          @open-doc-id="openDocById"
          @pick-link="onPickLink"
          @create-note="createNote"
        />

        <!-- 右栏：五面板联动（SP2-5） -->
        <WorkbenchSidePanels
          class="kb-workbench__right"
          :active-tab="workbench.activeTab.value"
          :collection-id="currentVaultId"
          :refresh-nonce="panelsRefreshNonce"
          :summary="activeDocument?.summary"
          :tags="activeDocument?.tags"
          :doc-type="activeDocument?.doc_type"
          @open-doc-id="openDocById"
          @expand-graph="(docId: string) => $emit('open-graph', docId)"
          @jump-outline="jumpOutline"
          @apply-autolink="onApplyAutolinkFromPanel"
        />
      </div>
    </div>

    <!-- ⌘O 快速切换 / ⌘K 命令面板（SP2-6） -->
    <QuickSwitcher
      :open="quickSwitcherOpen"
      :documents="vaultDocs"
      :tabs="workbench.tabs.value"
      :vault-name="currentVaultName"
      :truncated="documentsTruncated"
      @update:open="quickSwitcherOpen = $event"
      @open="(d: KnowledgeDocument) => workbench.openDoc(d)"
    />
    <CommandPalette
      :open="commandPaletteOpen"
      :commands="commandItems"
      :collections="collections"
      :current-vault-id="currentVaultId"
      :mru="commandMru"
      @update:open="commandPaletteOpen = $event"
      @run="runCommand"
      @switch-vault="(id: string) => $emit('switch-vault', id)"
    />

    <!-- Ctrl+Shift+F 全库搜索（P1-3） -->
    <SearchPanel
      :open="searchOpen"
      :query="searchQuery"
      :items="searchItems"
      :loading="searchLoading"
      @update:open="searchOpen = $event"
      @update:query="searchQuery = $event"
      @pick="onSearchPick"
    />

    <WritebackReviewDialog
      v-model:open="pendingOpen"
      :items="pendingItems"
      :home-name="pendingHome.name"
      :home-is-current="!pendingHome.redirected"
      :loading="pendingApplying"
      @submit="onApplyPending"
      @switch-home="onSwitchPendingHome"
    />

    <GovernanceReviewDialog
      v-model:open="govOpen"
      :items="govItems"
      :home-name="govHome.name"
      :home-is-current="!govHome.redirected"
      :loading-id="govLoadingId"
      @resolve="onResolveGovernance"
      @switch-home="onSwitchGovHome"
    />

    <!-- 脏关闭确认（SP3：全站标准玻璃对话框） -->
    <q-dialog :model-value="!!confirmCloseTab" @update:model-value="onCancelClose">
      <q-card class="app-dialog-card kb-workbench__confirm">
        <q-card-section class="app-glass-dialog__head">
          <div class="app-glass-dialog__title">{{ t('knowledgePage.workbench.closeConfirmTitle') }}</div>
        </q-card-section>
        <q-card-section>
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
        </q-card-section>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
// SP2 §SP2-1 工作台根：装配 TopBar + 三栏（树 / 标签页 / 联动面板）+ 浮层状态。
// 数据流纪律：命令编排（命令面板/搜索/新建/写回/治理/落链 recency）收口于 useWorkbenchCommands；
// 本组件只做布局、wikilink/大纲导航与脏关闭确认，状态经 props 注入的 workbench 状态机与 explorer。
import { computed, nextTick, onBeforeUnmount, onMounted, ref, toRef } from 'vue';
import { useI18n } from 'vue-i18n';
import WorkbenchTopBar from './WorkbenchTopBar.vue';
import WorkbenchSidebar from './WorkbenchSidebar.vue';
import WorkbenchTabs from './WorkbenchTabs.vue';
import WorkbenchSidePanels from './WorkbenchSidePanels.vue';
import QuickSwitcher from './QuickSwitcher.vue';
import CommandPalette from './CommandPalette.vue';
import SearchPanel from './SearchPanel.vue';
import WritebackReviewDialog from '../WritebackReviewDialog.vue';
import GovernanceReviewDialog from '../GovernanceReviewDialog.vue';
import { useWorkbenchCommands } from '../../../features/knowledge/useWorkbenchCommands';
import { normalizeTargetName } from '../../../features/knowledge/wikilink';
import { parseOutline } from '../../../features/knowledge/outline';
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
  /** 当前目录 prefix（wikilink 新建文档落点） */
  currentPrefix: string;
  /** SP2-8：图谱增量刷新信号（+1 → 右栏五面板重拉，缓存已被页面失效） */
  panelsRefreshNonce?: number;
  /** 文档列表被分页截断（⌘O / 图谱索引不完整）。 */
  documentsTruncated?: boolean;
}>();

const emit = defineEmits<{
  'switch-vault': [id: string];
  'select-node': [key: string];
  'update:expanded-keys': [keys: string[]];
  'lazy-load': [payload: VaultLazyLoadPayload];
  'node-action': [action: string, node: VaultQTreeNode];
  'create-vault': [];
  'drop-node': [node: VaultQTreeNode];
  retry: [];
  'refresh-tree': [];
  'open-graph': [focusDocId?: string];
  'open-settings': [];
  /** 命令面板「晋升到团队库」（SP2-8 由页面接入既有 PromoteDialog） */
  'promote-active': [docId: string];
  /** SP2-8：命令面板「粘贴文本入库」（页面打开既有 IngestDialog） */
  'ingest-text': [];
  /** SP2-8：文件行操作（move/download/delete）与拖拽，透传页面既有逻辑 */
  'file-action': [action: string, node: VaultTreeNode];
  'file-drag-start': [node: VaultTreeNode];
  'file-drag-end': [];
  'drop-current-dir': [];
}>();

const { t } = useI18n();

/** 右栏大纲跳转（SP2-5）：透传到活动 NoteEditor 滚动定位。 */
const tabsRef = ref<InstanceType<typeof WorkbenchTabs> | null>(null);

/** C6：外部保存入口——先 flush 编辑器防抖写回再 CAS 保存（冲突提示由 commands 层统一发）。 */
async function flushAndSave(docId: string): Promise<boolean> {
  tabsRef.value?.flushPendingContent();
  return props.workbench.saveTab(docId);
}

const {
  linkRecencyRank,
  onPickLink,
  quickSwitcherOpen,
  commandPaletteOpen,
  openQuickSwitcher,
  openCommandPalette,
  searchOpen,
  searchQuery,
  searchItems,
  searchLoading,
  openSearch,
  onSearchPick,
  commandMru,
  commandItems,
  runCommand,
  createNote,
  createFolder,
  createDocByName,
  saveDoc,
  onApplyAutolinkFromPanel,
  pendingOpen,
  pendingItems,
  pendingHome,
  pendingApplying,
  onApplyPending,
  onSwitchPendingHome,
  govOpen,
  govItems,
  govHome,
  govLoadingId,
  onResolveGovernance,
  onSwitchGovHome,
  onGlobalKeydown,
} = useWorkbenchCommands({
  workbench: props.workbench,
  currentVaultId: toRef(props, 'currentVaultId'),
  currentPrefix: toRef(props, 'currentPrefix'),
  documents: toRef(props, 'documents'),
  collections: toRef(props, 'collections'),
  events: {
    refreshTree: () => emit('refresh-tree'),
    switchVault: (id) => emit('switch-vault', id),
    openGraph: (focusDocId) => emit('open-graph', focusDocId),
    promoteActive: (docId) => emit('promote-active', docId),
    ingestText: () => emit('ingest-text'),
    saveDoc: flushAndSave,
  },
});

/** wikilink 候选：当前库全部文档 relPath（补全 + 存在性判定同口径）。 */
const candidates = computed(() =>
  props.documents.filter((d) => d.collection_id === props.currentVaultId).map((d) => d.rel_path || d.source),
);

/** 当前库文档（快速切换数据源）。 */
const vaultDocs = computed(() => props.documents.filter((d) => d.collection_id === props.currentVaultId));

/** 空态近期文档列表：近期更新文档前 8 条（SP3）。 */
const recentDocs = computed(() =>
  [...vaultDocs.value].sort((a, b) => (b.updated_at || '').localeCompare(a.updated_at || '')).slice(0, 8),
);

const currentVaultName = computed(
  () => props.collections.find((c) => c.id === props.currentVaultId)?.name ?? props.currentVaultId,
);

const activeDocument = computed(
  () => props.documents.find((d) => d.id === props.workbench.activeTabId.value) ?? null,
);

// ---------- 全局快捷键注册（处理器在 commands 层） ----------

onMounted(() => window.addEventListener('keydown', onGlobalKeydown, { capture: true }));
onBeforeUnmount(() => window.removeEventListener('keydown', onGlobalKeydown, { capture: true }));

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

/** wikilink 跳链：按名归一化匹配当前库文档后 openDoc；带 #heading 时打开后滚动定位（P2-5）。 */
async function openDocByName(target: string, heading?: string) {
  const want = normalizeTargetName(target);
  const doc = props.documents.find(
    (d) => d.collection_id === props.currentVaultId && normalizeTargetName(d.rel_path || d.source) === want,
  );
  if (!doc) return;
  await props.workbench.openDoc(doc);
  if (!heading) return;
  await nextTick(); // 等 NoteEditor 按新 activeTab 挂载
  const content = props.workbench.activeTab.value?.content ?? '';
  const wantHeading = heading.trim().toLowerCase();
  const hit = parseOutline(content).find((h) => h.text.trim().toLowerCase() === wantHeading);
  if (hit) tabsRef.value?.scrollToOffset(hit.offset);
}

/** `[[target#` 标题补全数据源（P2-5）：已打开 tab 的大纲标题（未打开文档不拉取，保持补全零网络）。 */
function getHeadingsFor(target: string): string[] {
  const want = normalizeTargetName(target);
  const tab = props.workbench.tabs.value.find((tb) => normalizeTargetName(tb.relPath) === want);
  return tab ? parseOutline(tab.content).map((h) => h.text) : [];
}

/** 右栏面板跳链（SP2-5）：按 docId 直接打开；不在当前文档列表时提示（跨库反链目标）。 */
function openDocById(docId: string) {
  const doc = props.documents.find((d) => d.id === docId);
  if (doc) void props.workbench.openDoc(doc);
}

function jumpOutline(offset: number) {
  tabsRef.value?.scrollToOffset(offset);
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
    const ok = await flushAndSave(id);
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
  background: var(--gradient-page)
  color: var(--kb-text-primary)

  &__frame
    position: relative
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

  // 脏关闭确认对话框（teleport 到 body，用全站令牌；!important 压过 app-dialog-card 全局默认宽度）
  &__confirm
    width: 420px !important
    max-width: 90vw

  &__confirm-hint
    font-size: 13px
    color: var(--color-text-primary)
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
