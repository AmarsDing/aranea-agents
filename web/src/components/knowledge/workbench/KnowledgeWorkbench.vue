<template>
  <div class="kb-workbench" :class="{ 'kb-workbench--perf': performanceMode }">
    <!-- M1：液态玻璃 SVG 滤镜单例（kb-liquid-refract 光纹 / kb-liquid-bg 真折射），全 Workbench 共享 -->
    <LiquidGlassDefs />
    <!-- 深空背景：粒子 + 极光光斑（不参与交互）；C7 性能模式下整体移除（停 rAF/动画/blur） -->
    <template v-if="!performanceMode">
      <ParticleField class="kb-workbench__particles" />
      <div class="kb-workbench__aurora kb-workbench__aurora--cyan" />
      <div class="kb-workbench__aurora kb-workbench__aurora--violet" />
      <div class="kb-workbench__aurora kb-workbench__aurora--teal" />
    </template>

    <div class="kb-workbench__frame">
      <WorkbenchTopBar
        :collections="collections"
        :current-vault-id="currentVaultId"
        :performance-mode="performanceMode"
        @switch-vault="$emit('switch-vault', $event)"
        @toggle-performance-mode="$emit('toggle-performance-mode')"
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
          @save="onSave"
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

    <!-- 脏关闭确认（SP2-8：kb-portal——q-dialog teleport 到 body 后重挂深空令牌） -->
    <q-dialog :model-value="!!confirmCloseTab" content-class="kb-portal" @update:model-value="onCancelClose">
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
// Container: approved because 工作台命令落盘（新建笔记/文件夹、索引重建）需就近访问 API，
// 数据流纪律：全部状态经 props 注入的 workbench 状态机与 explorer，子组件不各自拉数。
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import ParticleField from '../effects/ParticleField.vue';
import LiquidGlassDefs from '../effects/LiquidGlassDefs.vue';
import GlassPanel from '../effects/GlassPanel.vue';
import WorkbenchTopBar from './WorkbenchTopBar.vue';
import WorkbenchSidebar from './WorkbenchSidebar.vue';
import WorkbenchTabs from './WorkbenchTabs.vue';
import WorkbenchSidePanels from './WorkbenchSidePanels.vue';
import QuickSwitcher from './QuickSwitcher.vue';
import CommandPalette from './CommandPalette.vue';
import SearchPanel, { type SearchItem } from './SearchPanel.vue';
import WritebackReviewDialog from '../WritebackReviewDialog.vue';
import GovernanceReviewDialog from '../GovernanceReviewDialog.vue';
import {
  applyOutgoingAutolink,
  applyWriteBackPending,
  backfillAutolinkIndex,
  createVaultDir,
  createVaultDocument,
  getCollectionHealth,
  getWriteBackHome,
  listCollectionExperts,
  listGovernanceProposals,
  listRecentLinkUses,
  listWriteBackPending,
  previewOutgoingAutolink,
  recordLinkUse,
  rebuildKnowledgeIndex,
  resolveGovernanceProposal,
  searchKnowledge,
} from '../../../features/knowledge/api';
import { COMMAND_DEFS, pushMru, type CommandId, type CommandItem } from '../../../features/knowledge/commands';
import { normalizeTargetName } from '../../../features/knowledge/wikilink';
import { parseOutline } from '../../../features/knowledge/outline';
import type { KnowledgeWorkbench as Workbench } from '../../../features/knowledge/useKnowledgeWorkbench';
import type { DragFileRef } from '../../../features/knowledge/vaultTreeUi';
import type { VaultLazyLoadPayload, VaultQTreeNode } from '../../../features/knowledge/useVaultExplorer';
import type {
  GovernanceProposalItem,
  KnowledgeCollection,
  KnowledgeDocument,
  PendingWriteBackItem,
  VaultTreeNode,
} from '../../../features/knowledge/types';
import type { GovernanceDecision } from '../../../features/knowledge/governance';

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
  /** C7：性能模式（关粒子/极光，降 blur） */
  performanceMode?: boolean;
}>();

const emit = defineEmits<{
  'switch-vault': [id: string];
  'toggle-performance-mode': [];
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
const $q = useQuasar();

/** wikilink 候选：当前库全部文档 relPath（补全 + 存在性判定同口径）。 */
const candidates = computed(() =>
  props.documents.filter((d) => d.collection_id === props.currentVaultId).map((d) => d.rel_path || d.source),
);

// ---------- wikilink 落链 recency（B4 #8） ----------
// 归一化名 → 名次（0=最近）；仅空查询补全消费。库切换时整表拉取一次（≤32 条），
// 落链时乐观更新 + best-effort 上报（失败静默，recency 非正确性依赖）。
const linkRecencyRank = ref<ReadonlyMap<string, number>>(new Map());

async function loadLinkRecency(vaultId: string) {
  if (!vaultId) {
    linkRecencyRank.value = new Map();
    return;
  }
  try {
    const items = await listRecentLinkUses(vaultId);
    const byId = new Map(props.documents.filter((d) => d.collection_id === vaultId).map((d) => [d.id, d]));
    const rank = new Map<string, number>();
    for (const it of items) {
      const doc = byId.get(it.doc_id);
      if (!doc) continue; // 已删除/移动文档的孤儿行不映射候选
      const name = normalizeTargetName(doc.rel_path || doc.source);
      if (name && !rank.has(name)) rank.set(name, rank.size);
    }
    linkRecencyRank.value = rank;
  } catch {
    linkRecencyRank.value = new Map(); // 拉取失败降级为无 recency 排序
  }
}

watch(
  () => props.currentVaultId,
  (id) => void loadLinkRecency(id),
  { immediate: true },
);

function onPickLink(target: string) {
  const doc = props.documents.find(
    (d) => d.collection_id === props.currentVaultId && (d.rel_path || d.source) === target,
  );
  if (!doc) return;
  // 乐观置顶：目标 rank 0，其余按原名次顺延。
  const next = new Map<string, number>();
  const name = normalizeTargetName(target);
  if (name) next.set(name, 0);
  for (const [k] of [...linkRecencyRank.value.entries()].sort((a, b) => a[1] - b[1])) {
    if (k !== name) next.set(k, next.size);
  }
  linkRecencyRank.value = next;
  recordLinkUse(props.currentVaultId, doc.id).catch(() => undefined);
}

// ⌘O / ⌘K 浮层状态（SP2-6）
const quickSwitcherOpen = ref(false);
const commandPaletteOpen = ref(false);

function openQuickSwitcher() {
  quickSwitcherOpen.value = true;
}

function openCommandPalette() {
  commandPaletteOpen.value = true;
}

// ---------- 全库搜索（Ctrl+Shift+F，P1-3） ----------
// 容器内检索（数据流纪律）：SearchPanel 纯受控；防抖 300ms + seq 竞态守卫（慢响应不覆盖新查询）。
const searchOpen = ref(false);
const searchQuery = ref('');
const searchItems = ref<SearchItem[]>([]);
const searchLoading = ref(false);
let searchSeq = 0;
let searchTimer: ReturnType<typeof setTimeout> | undefined;

/** 命中文本窗口：以首个匹配词为中心截取片段（Obsidian 语义）。 */
function buildSnippet(content: string, query: string): string {
  const flat = content.replace(/\s+/g, ' ').trim();
  const idx = flat.toLowerCase().indexOf(query.trim().toLowerCase());
  if (idx < 0) return flat.slice(0, 160);
  const start = Math.max(0, idx - 48);
  const prefix = start > 0 ? '…' : '';
  const tail = start + 160 < flat.length ? '…' : '';
  return `${prefix}${flat.slice(start, start + 160)}${tail}`;
}

async function runSearch(q: string) {
  const seq = ++searchSeq;
  if (!q.trim() || !props.currentVaultId) {
    searchItems.value = [];
    searchLoading.value = false;
    return;
  }
  searchLoading.value = true;
  try {
    const chunks = await searchKnowledge({ collection_id: props.currentVaultId, query: q.trim(), top_k: 12 });
    if (seq !== searchSeq) return; // 已有更新的查询
    searchItems.value = chunks.map((chunk) => {
      const doc = props.documents.find((d) => d.id === chunk.doc_id);
      const rel = doc?.rel_path || doc?.source || chunk.doc_id;
      const name = rel.split('/').filter(Boolean).pop() || rel;
      return {
        chunk,
        docId: chunk.doc_id,
        name,
        path: rel,
        snippet: buildSnippet(chunk.content, q),
        score: chunk.score,
      };
    });
  } catch {
    if (seq === searchSeq) searchItems.value = [];
  } finally {
    if (seq === searchSeq) searchLoading.value = false;
  }
}

watch(searchQuery, (q) => {
  if (searchTimer) clearTimeout(searchTimer);
  searchTimer = setTimeout(() => void runSearch(q), 300);
});

watch(searchOpen, (on) => {
  if (on) return;
  // 关闭时清理：取消防抖与在途结果，下次打开从零开始
  if (searchTimer) clearTimeout(searchTimer);
  searchSeq += 1;
  searchQuery.value = '';
  searchItems.value = [];
  searchLoading.value = false;
});

function openSearch() {
  quickSwitcherOpen.value = false;
  commandPaletteOpen.value = false;
  searchOpen.value = true;
}

function onSearchPick(it: SearchItem) {
  const doc = props.documents.find((d) => d.id === it.docId);
  if (doc) void props.workbench.openDoc(doc);
}

/** 当前库文档（快速切换数据源）。 */
const vaultDocs = computed(() => props.documents.filter((d) => d.collection_id === props.currentVaultId));

/** 空态 RingCarousel：近期更新文档前 8 条（SP2-7）。 */
const recentDocs = computed(() =>
  [...vaultDocs.value].sort((a, b) => (b.updated_at || '').localeCompare(a.updated_at || '')).slice(0, 8),
);

const currentVaultName = computed(
  () => props.collections.find((c) => c.id === props.currentVaultId)?.name ?? props.currentVaultId,
);

// ---------- 命令面板（SP2-6） ----------

// MRU（P2-6）：最近执行命令置顶；localStorage 持久化，隐私模式等写失败时静默降级为会话内 MRU。
const MRU_STORAGE_KEY = 'kb.command.mru';
function loadCommandMru(): CommandId[] {
  try {
    const raw = localStorage.getItem(MRU_STORAGE_KEY);
    const arr: unknown = raw ? JSON.parse(raw) : [];
    return Array.isArray(arr) ? arr.filter((x): x is CommandId => typeof x === 'string') : [];
  } catch {
    return [];
  }
}
const commandMru = ref<CommandId[]>(loadCommandMru());
function recordCommandMru(id: CommandId) {
  commandMru.value = pushMru(commandMru.value, id);
  try {
    localStorage.setItem(MRU_STORAGE_KEY, JSON.stringify(commandMru.value));
  } catch {
    /* 写失败不影响功能 */
  }
}

const commandItems = computed<CommandItem[]>(() => {
  const active = props.workbench.activeTab.value;
  return COMMAND_DEFS.map((def) => ({
    def,
    title: t(`knowledgePage.workbench.commands.${def.id}`),
    disabled: !active
      ? (['save', 'toggle-mode', 'close-tab', 'promote', 'apply-autolink'] as CommandId[]).includes(def.id)
      : (def.id === 'save' || def.id === 'toggle-mode') && !active.editable,
  }));
});

/** 新建笔记（命令面板/后续侧栏共用，SP2-7 复用）：当前目录落点 + 打开 + 刷新树。 */
function createNote() {
  if (!props.currentVaultId) return;
  $q.dialog({
    title: t('knowledgePage.workbench.commands.new-note'),
    prompt: { model: '', type: 'text', label: t('knowledgePage.workbench.noteNamePrompt') },
    cancel: true,
    class: 'kb-portal',
  }).onOk(async (name: string) => {
    const base = normalizeTargetName(name);
    if (!base) return;
    try {
      const doc = await createVaultDocument(props.currentVaultId, `${props.currentPrefix}${base}.md`);
      emit('refresh-tree');
      await props.workbench.openDoc(doc);
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    }
  });
}

function createFolder() {
  if (!props.currentVaultId) return;
  $q.dialog({
    title: t('knowledgePage.workbench.commands.new-folder'),
    prompt: { model: '', type: 'text', label: t('knowledgePage.workbench.folderNamePrompt') },
    cancel: true,
    class: 'kb-portal',
  }).onOk(async (name: string) => {
    const base = normalizeTargetName(name);
    if (!base) return;
    try {
      await createVaultDir(props.currentVaultId, `${props.currentPrefix}${base}`);
      emit('refresh-tree');
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    }
  });
}

async function onApplyAutolinkFromPanel() {
  const active = props.workbench.activeTab.value;
  if (!active) return;
  try {
    const prev = await previewOutgoingAutolink(active.docId);
    if (prev.unchanged || prev.replacements <= 0) {
      $q.notify({ type: 'info', message: t('knowledgePage.workbench.autolinkNone') });
      return;
    }
    $q.dialog({
      title: t('knowledgePage.workbench.commands.apply-autolink'),
      message: t('knowledgePage.workbench.autolinkConfirm', { n: prev.replacements }),
      cancel: true,
      class: 'kb-portal',
    }).onOk(async () => {
      try {
        const res = await applyOutgoingAutolink(active.docId);
        $q.notify({
          type: 'positive',
          message: t('knowledgePage.workbench.autolinkDone', { n: res.replacements }),
        });
        emit('refresh-tree');
      } catch (e) {
        $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
      }
    });
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  }
}

type WriteBackTarget = { id: string; name: string; redirected: boolean };

async function resolveWriteBackTarget(): Promise<WriteBackTarget | null> {
  try {
    const home = await getWriteBackHome();
    if (!home.found || !home.collection_id) {
      $q.notify({ type: 'info', message: t('knowledgePage.workbench.writebackHomeMissing') });
      return null;
    }
    return {
      id: home.collection_id,
      name: home.name,
      redirected: home.collection_id !== props.currentVaultId,
    };
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    return null;
  }
}

function offerSwitchHome(home: WriteBackTarget) {
  if (!home.redirected) return;
  $q.notify({
    type: 'info',
    timeout: 8000,
    message: t('knowledgePage.workbench.writebackHomeHint', { name: home.name }),
    actions: [{ label: t('knowledgePage.workbench.writebackHomeSwitch'), handler: () => emit('switch-vault', home.id) }],
  });
}

async function showKnowledgeHealth() {
  if (!props.currentVaultId) return;
  try {
    const h = await getCollectionHealth(props.currentVaultId);
    let message = t('knowledgePage.workbench.healthSummary', {
      docs: h.document_count,
      edges: h.edge_count,
      explicit: h.explicit_edges,
      orphan: Math.round(h.orphan_rate * 100),
      dangling: h.dangling_count,
    });
    const home = await getWriteBackHome().catch(() => null);
    if (home?.found && home.collection_id && home.collection_id !== props.currentVaultId) {
      message += ` · ${t('knowledgePage.workbench.healthWritebackElsewhere', { name: home.name })}`;
    }
    $q.notify({ type: 'info', timeout: 8000, message });
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  }
}

async function showKnowledgeExperts() {
  const home = await resolveWriteBackTarget();
  if (!home) return;
  try {
    const items = await listCollectionExperts(home.id);
    if (!items.length) {
      $q.notify({ type: 'info', message: t('knowledgePage.workbench.expertsEmpty') });
      offerSwitchHome(home);
      return;
    }
    const lines = items
      .slice(0, 8)
      .map((e) => `${e.agent_id || e.user_id} (${e.fact_count})`)
      .join('\n');
    const message = home.redirected
      ? `${t('knowledgePage.workbench.expertsFromHome', { name: home.name })}\n${lines}`
      : lines;
    $q.dialog({
      title: t('knowledgePage.workbench.commands.list-experts'),
      message,
      class: 'kb-portal',
    });
    offerSwitchHome(home);
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  }
}

const pendingOpen = ref(false);
const pendingItems = ref<PendingWriteBackItem[]>([]);
const pendingHome = ref<WriteBackTarget>({ id: '', name: '', redirected: false });
const pendingApplying = ref(false);

async function reviewWriteBackPending() {
  const home = await resolveWriteBackTarget();
  if (!home) return;
  try {
    const items = await listWriteBackPending(home.id);
    if (!items.length) {
      $q.notify({ type: 'info', message: t('knowledgePage.workbench.pendingEmpty') });
      offerSwitchHome(home);
      return;
    }
    pendingHome.value = home;
    pendingItems.value = items;
    pendingOpen.value = true;
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  }
}

function onSwitchPendingHome() {
  if (pendingHome.value.id) emit('switch-vault', pendingHome.value.id);
}

async function onApplyPending(factIds: string[]) {
  if (!pendingHome.value.id || factIds.length === 0) return;
  pendingApplying.value = true;
  try {
    const res = await applyWriteBackPending(pendingHome.value.id, factIds);
    pendingOpen.value = false;
    $q.notify({
      type: 'positive',
      message: t('knowledgePage.workbench.pendingDone', { n: res.appended }),
    });
    emit('refresh-tree');
    offerSwitchHome(pendingHome.value);
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  } finally {
    pendingApplying.value = false;
  }
}

const govOpen = ref(false);
const govItems = ref<GovernanceProposalItem[]>([]);
const govHome = ref<WriteBackTarget>({ id: '', name: '', redirected: false });
const govLoadingId = ref<number | undefined>(undefined);

async function loadGovernanceItems(collectionId: string): Promise<GovernanceProposalItem[]> {
  return listGovernanceProposals(collectionId, 'pending');
}

async function reviewGovernance() {
  if (!props.currentVaultId) return;
  try {
    let items = await loadGovernanceItems(props.currentVaultId);
    let home: WriteBackTarget = { id: props.currentVaultId, name: '', redirected: false };
    if (!items.length) {
      const inbox = await getWriteBackHome().catch(() => null);
      if (inbox?.found && inbox.collection_id && inbox.collection_id !== props.currentVaultId) {
        items = await loadGovernanceItems(inbox.collection_id);
        if (items.length) {
          home = { id: inbox.collection_id, name: inbox.name, redirected: true };
        }
      }
    }
    if (!items.length) {
      $q.notify({ type: 'info', message: t('knowledgePage.workbench.govEmpty') });
      return;
    }
    govHome.value = home;
    govItems.value = items;
    govOpen.value = true;
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  }
}

function onSwitchGovHome() {
  if (govHome.value.id) emit('switch-vault', govHome.value.id);
}

async function onResolveGovernance(payload: { id: number; decision: GovernanceDecision }) {
  govLoadingId.value = payload.id;
  try {
    await resolveGovernanceProposal(payload.id, payload.decision);
    govItems.value = govItems.value.filter((it) => it.id !== payload.id);
    $q.notify({ type: 'positive', message: t('knowledgePage.workbench.govDone') });
    if (!govItems.value.length) govOpen.value = false;
    emit('refresh-tree');
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  } finally {
    govLoadingId.value = undefined;
  }
}

function confirmBackfillAutolink() {
  if (!props.currentVaultId) return;
  $q.dialog({
    title: t('knowledgePage.workbench.commands.backfill-autolink'),
    message: t('knowledgePage.workbench.backfillAutolinkConfirm'),
    cancel: true,
    class: 'kb-portal',
  }).onOk(async () => {
    try {
      await backfillAutolinkIndex(props.currentVaultId);
      $q.notify({ type: 'info', message: t('knowledgePage.workbench.backfillStarted') });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    }
  });
}

async function runCommand(id: CommandId) {
  recordCommandMru(id);
  const active = props.workbench.activeTab.value;
  switch (id) {
    case 'new-note':
      createNote();
      break;
    case 'new-folder':
      createFolder();
      break;
    case 'save':
      if (active) await onSave(active.docId);
      break;
    case 'toggle-mode':
      if (active) props.workbench.toggleMode(active.docId);
      break;
    case 'open-graph':
      emit('open-graph');
      break;
    case 'rebuild-index':
      if (!props.currentVaultId) break;
      try {
        await rebuildKnowledgeIndex(props.currentVaultId);
        $q.notify({ type: 'info', message: t('knowledgePage.workbench.rebuildStarted') });
      } catch (e) {
        $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
      }
      break;
    case 'backfill-autolink':
      confirmBackfillAutolink();
      break;
    case 'ingest-text':
      emit('ingest-text');
      break;
    case 'promote':
      if (active) emit('promote-active', active.docId);
      break;
    case 'apply-autolink':
      void onApplyAutolinkFromPanel();
      break;
    case 'knowledge-health':
      void showKnowledgeHealth();
      break;
    case 'list-experts':
      void showKnowledgeExperts();
      break;
    case 'review-writeback':
      void reviewWriteBackPending();
      break;
    case 'review-governance':
      void reviewGovernance();
      break;
    case 'close-tab':
      if (active) props.workbench.closeTab(active.docId);
      break;
    case 'switch-vault':
      break; // 浮层内二级选择，不经 runCommand
  }
}

// ---------- 全局快捷键（capture：输入框聚焦时 ⌘O/⌘K 仍可唤起） ----------

function onGlobalKeydown(e: KeyboardEvent) {
  // Ctrl+Shift+F：全库搜索（P1-3；shift 组合先行判定，下方守卫会排除其余 shift 组合）
  if ((e.ctrlKey || e.metaKey) && e.shiftKey && !e.altKey && e.key.toLowerCase() === 'f') {
    e.preventDefault();
    openSearch();
    return;
  }
  if (!(e.ctrlKey || e.metaKey) || e.altKey || e.shiftKey) return;
  const key = e.key.toLowerCase();
  if (key === 'o') {
    e.preventDefault();
    commandPaletteOpen.value = false;
    searchOpen.value = false;
    openQuickSwitcher();
  } else if (key === 'k') {
    e.preventDefault();
    quickSwitcherOpen.value = false;
    searchOpen.value = false;
    openCommandPalette();
  } else if (key === 'e') {
    const active = props.workbench.activeTab.value;
    if (active?.editable) {
      e.preventDefault();
      props.workbench.toggleMode(active.docId);
    }
  } else if (key === 'g') {
    e.preventDefault();
    emit('open-graph');
  }
}

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

/** 右栏大纲跳转（SP2-5）：透传到活动 NoteEditor 滚动定位。 */
const tabsRef = ref<InstanceType<typeof WorkbenchTabs> | null>(null);

function jumpOutline(offset: number) {
  tabsRef.value?.scrollToOffset(offset);
}

/** dangling 链接点击：当前目录新建 `target.md` 并打开 + 刷新树（Obsidian 语义）。 */
async function createDocByName(target: string) {
  if (!props.currentVaultId) return;
  const relPath = `${props.currentPrefix}${normalizeTargetName(target) || target}.md`;
  try {
    const doc = await createVaultDocument(props.currentVaultId, relPath);
    emit('refresh-tree');
    await props.workbench.openDoc(doc);
  } catch (e) {
    $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
  }
}

async function onSave(docId: string) {
  // C6：命令面板/快捷键等外部保存入口，先 flush 编辑器防抖写回再 CAS 保存。
  tabsRef.value?.flushPendingContent();
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
    tabsRef.value?.flushPendingContent();
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

  // U2 光晕增强：视口相对超大色团（研究 A6：直径 60–120vw、四角分布），慢速漂移
  &__aurora
    position: absolute
    border-radius: 50%
    filter: blur(120px)
    pointer-events: none

    &--cyan
      width: 46vw
      height: 46vw
      top: -18vw
      right: -12vw
      opacity: 0.14
      background: radial-gradient(circle, var(--kb-accent-cyan), transparent 65%)
      animation: kb-aurora-drift 26s ease-in-out infinite alternate

    &--violet
      width: 40vw
      height: 40vw
      bottom: -16vw
      left: -10vw
      opacity: 0.12
      background: radial-gradient(circle, var(--kb-accent-violet), transparent 65%)
      animation: kb-aurora-drift 32s ease-in-out infinite alternate-reverse

    // 第三团：底部中央微青光，补深空纵深
    &--teal
      width: 34vw
      height: 34vw
      bottom: -20vw
      right: 18vw
      opacity: 0.08
      background: radial-gradient(circle, var(--kb-accent-cyan), transparent 68%)
      animation: kb-aurora-drift 38s ease-in-out infinite alternate

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

// U2 光晕漂移动画（仅 transform/opacity，GPU 合成层；reduced-motion 由 deep-space.sass 统一降级）
@keyframes kb-aurora-drift
  0%
    transform: translate3d(0, 0, 0) scale(1)
  100%
    transform: translate3d(3vw, 2vw, 0) scale(1.08)
</style>
