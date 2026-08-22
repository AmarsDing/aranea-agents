// 工作台命令编排（SP2 §SP2-6/7/8）：⌘O/⌘K/Ctrl+Shift+F 浮层、命令面板调度、新建/落链/自动链接/
// 写回/治理/健康度/专家/索引命令、全局快捷键。KnowledgeWorkbench 仅做 UI 装配，业务编排全部收口于此。
// TECH-DEBT(FL5): 一次性编辑器命令直调 features/knowledge/api（writeback/governance/health/experts/
// backfill/rebuild/recency/autolink）——无共享状态，照 useTeamCompilePreview 先例；有缓存语义的
// （新建文档/目录、全库检索）走 knowledge store，保证树/文档缓存一致性。
import { computed, ref, watch, type Ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { useQuasar } from 'quasar';
import { useKnowledgeStore } from '../../stores/knowledge';
import {
  applyOutgoingAutolink,
  applyWriteBackPending,
  backfillAutolinkIndex,
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
} from './api';
import { COMMAND_DEFS, pushMru, type CommandId, type CommandItem } from './commands';
import { normalizeTargetName } from './wikilink';
import type { GovernanceDecision } from './governance';
import type { KnowledgeWorkbench } from './useKnowledgeWorkbench';
import type {
  GovernanceProposalItem,
  KnowledgeChunk,
  KnowledgeCollection,
  KnowledgeDocument,
  PendingWriteBackItem,
} from './types';

/** 全库搜索命中项（与 SearchPanel 的 SearchItem 结构一致，避免 feature 反向依赖组件）。 */
export interface WorkbenchSearchItem {
  chunk: KnowledgeChunk;
  docId: string;
  name: string;
  path: string;
  snippet: string;
  score: number;
}

/** 写回目标库（home 重定向时携带名称提示）。 */
type WriteBackTarget = { id: string; name: string; redirected: boolean };

export interface WorkbenchCommandsDeps {
  workbench: KnowledgeWorkbench;
  currentVaultId: Ref<string>;
  currentPrefix: Ref<string>;
  documents: Ref<KnowledgeDocument[]>;
  collections: Ref<KnowledgeCollection[]>;
  events: {
    /** 文档结构变化（新建/落链/写回/治理）后通知页面刷新树。 */
    refreshTree: () => void;
    switchVault: (id: string) => void;
    openGraph: (focusDocId?: string) => void;
    promoteActive: (docId: string) => void;
    ingestText: () => void;
    /** C6：外部保存入口——先 flush 编辑器防抖写回再 CAS 保存；返回是否成功（冲突=false）。 */
    saveDoc: (docId: string) => Promise<boolean>;
  };
}

export function useWorkbenchCommands(deps: WorkbenchCommandsDeps) {
  const { t } = useI18n();
  const $q = useQuasar();
  const knowledgeStore = useKnowledgeStore();

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
      const byId = new Map(deps.documents.value.filter((d) => d.collection_id === vaultId).map((d) => [d.id, d]));
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

  watch(deps.currentVaultId, (id) => void loadLinkRecency(id), { immediate: true });

  function onPickLink(target: string) {
    const doc = deps.documents.value.find(
      (d) => d.collection_id === deps.currentVaultId.value && (d.rel_path || d.source) === target,
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
    recordLinkUse(deps.currentVaultId.value, doc.id).catch(() => undefined);
  }

  // ---------- ⌘O / ⌘K 浮层状态（SP2-6） ----------
  const quickSwitcherOpen = ref(false);
  const commandPaletteOpen = ref(false);

  function openQuickSwitcher() {
    quickSwitcherOpen.value = true;
  }

  function openCommandPalette() {
    commandPaletteOpen.value = true;
  }

  // ---------- 全库搜索（Ctrl+Shift+F，P1-3） ----------
  // 防抖 300ms + seq 竞态守卫（慢响应不覆盖新查询）；检索走 store（统一 HTTP 门面）。
  const searchOpen = ref(false);
  const searchQuery = ref('');
  const searchItems = ref<WorkbenchSearchItem[]>([]);
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
    if (!q.trim()) {
      searchItems.value = [];
      searchLoading.value = false;
      return;
    }
    searchLoading.value = true;
    try {
      // US-14：工作台搜索默认全部可见知识库（collection_id 留空走联邦路由）。
      const chunks = await knowledgeStore.search({
        query: q.trim(),
        top_k: 12,
      });
      if (seq !== searchSeq) return; // 已有更新的查询
      searchItems.value = chunks.map((chunk) => {
        const doc = deps.documents.value.find((d) => d.id === chunk.doc_id);
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

  function onSearchPick(it: WorkbenchSearchItem) {
    const doc = deps.documents.value.find((d) => d.id === it.docId);
    if (doc) void deps.workbench.openDoc(doc);
  }

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
    const active = deps.workbench.activeTab.value;
    return COMMAND_DEFS.map((def) => ({
      def,
      title: t(`knowledgePage.workbench.commands.${def.id}`),
      disabled: !active
        ? (['save', 'toggle-mode', 'close-tab', 'promote', 'apply-autolink'] as CommandId[]).includes(def.id)
        : (def.id === 'save' || def.id === 'toggle-mode') && !active.editable,
    }));
  });

  /** 新建笔记（命令面板/侧栏共用）：当前目录落点 + 打开 + 刷新树；走 store 保证树缓存失效。 */
  function createNote() {
    if (!deps.currentVaultId.value) return;
    $q.dialog({
      title: t('knowledgePage.workbench.commands.new-note'),
      prompt: { model: '', type: 'text', label: t('knowledgePage.workbench.noteNamePrompt') },
      cancel: true,
      class: 'kb-portal',
    }).onOk(async (name: string) => {
      const base = normalizeTargetName(name);
      if (!base) return;
      try {
        const doc = await knowledgeStore.addVaultDocument(
          deps.currentVaultId.value,
          `${deps.currentPrefix.value}${base}.md`,
        );
        deps.events.refreshTree();
        await deps.workbench.openDoc(doc);
      } catch (e) {
        $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
      }
    });
  }

  function createFolder() {
    if (!deps.currentVaultId.value) return;
    $q.dialog({
      title: t('knowledgePage.workbench.commands.new-folder'),
      prompt: { model: '', type: 'text', label: t('knowledgePage.workbench.folderNamePrompt') },
      cancel: true,
      class: 'kb-portal',
    }).onOk(async (name: string) => {
      const base = normalizeTargetName(name);
      if (!base) return;
      try {
        await knowledgeStore.addVaultDir(deps.currentVaultId.value, `${deps.currentPrefix.value}${base}`);
        deps.events.refreshTree();
      } catch (e) {
        $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
      }
    });
  }

  /** dangling 链接点击：当前目录新建 `target.md` 并打开 + 刷新树（Obsidian 语义）。 */
  async function createDocByName(target: string) {
    if (!deps.currentVaultId.value) return;
    const relPath = `${deps.currentPrefix.value}${normalizeTargetName(target) || target}.md`;
    try {
      const doc = await knowledgeStore.addVaultDocument(deps.currentVaultId.value, relPath);
      deps.events.refreshTree();
      await deps.workbench.openDoc(doc);
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    }
  }

  /** 外部保存入口（命令面板/快捷键/标签页保存按钮）：flush + CAS 保存，冲突时提示保留 tab。 */
  async function saveDoc(docId: string) {
    const ok = await deps.events.saveDoc(docId);
    if (!ok) {
      $q.notify({ type: 'warning', message: t('knowledgePage.workbench.conflictHint'), timeout: 6000 });
    }
  }

  async function onApplyAutolinkFromPanel() {
    const active = deps.workbench.activeTab.value;
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
          deps.events.refreshTree();
        } catch (e) {
          $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
        }
      });
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    }
  }

  // ---------- 写回（US-46） ----------

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
        redirected: home.collection_id !== deps.currentVaultId.value,
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
      actions: [
        { label: t('knowledgePage.workbench.writebackHomeSwitch'), handler: () => deps.events.switchVault(home.id) },
      ],
    });
  }

  async function showKnowledgeHealth() {
    if (!deps.currentVaultId.value) return;
    try {
      const h = await getCollectionHealth(deps.currentVaultId.value);
      let message = t('knowledgePage.workbench.healthSummary', {
        docs: h.document_count,
        edges: h.edge_count,
        explicit: h.explicit_edges,
        orphan: Math.round(h.orphan_rate * 100),
        dangling: h.dangling_count,
      });
      const home = await getWriteBackHome().catch(() => null);
      if (home?.found && home.collection_id && home.collection_id !== deps.currentVaultId.value) {
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
    if (pendingHome.value.id) deps.events.switchVault(pendingHome.value.id);
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
      deps.events.refreshTree();
      offerSwitchHome(pendingHome.value);
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    } finally {
      pendingApplying.value = false;
    }
  }

  // ---------- 治理提案 ----------

  const govOpen = ref(false);
  const govItems = ref<GovernanceProposalItem[]>([]);
  const govHome = ref<WriteBackTarget>({ id: '', name: '', redirected: false });
  const govLoadingId = ref<number | undefined>(undefined);

  async function loadGovernanceItems(collectionId: string): Promise<GovernanceProposalItem[]> {
    return listGovernanceProposals(collectionId, 'pending');
  }

  async function reviewGovernance() {
    if (!deps.currentVaultId.value) return;
    try {
      let items = await loadGovernanceItems(deps.currentVaultId.value);
      let home: WriteBackTarget = { id: deps.currentVaultId.value, name: '', redirected: false };
      if (!items.length) {
        const inbox = await getWriteBackHome().catch(() => null);
        if (inbox?.found && inbox.collection_id && inbox.collection_id !== deps.currentVaultId.value) {
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
    if (govHome.value.id) deps.events.switchVault(govHome.value.id);
  }

  async function onResolveGovernance(payload: { id: number; decision: GovernanceDecision }) {
    govLoadingId.value = payload.id;
    try {
      await resolveGovernanceProposal(payload.id, payload.decision);
      govItems.value = govItems.value.filter((it) => it.id !== payload.id);
      $q.notify({ type: 'positive', message: t('knowledgePage.workbench.govDone') });
      if (!govItems.value.length) govOpen.value = false;
      deps.events.refreshTree();
    } catch (e) {
      $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
    } finally {
      govLoadingId.value = undefined;
    }
  }

  function confirmBackfillAutolink() {
    if (!deps.currentVaultId.value) return;
    $q.dialog({
      title: t('knowledgePage.workbench.commands.backfill-autolink'),
      message: t('knowledgePage.workbench.backfillAutolinkConfirm'),
      cancel: true,
      class: 'kb-portal',
    }).onOk(async () => {
      try {
        await backfillAutolinkIndex(deps.currentVaultId.value);
        $q.notify({ type: 'info', message: t('knowledgePage.workbench.backfillStarted') });
      } catch (e) {
        $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
      }
    });
  }

  async function runCommand(id: CommandId) {
    recordCommandMru(id);
    const active = deps.workbench.activeTab.value;
    switch (id) {
      case 'new-note':
        createNote();
        break;
      case 'new-folder':
        createFolder();
        break;
      case 'save':
        if (active) await saveDoc(active.docId);
        break;
      case 'toggle-mode':
        if (active) deps.workbench.toggleMode(active.docId);
        break;
      case 'open-graph':
        deps.events.openGraph();
        break;
      case 'rebuild-index':
        if (!deps.currentVaultId.value) break;
        try {
          await rebuildKnowledgeIndex(deps.currentVaultId.value);
          $q.notify({ type: 'info', message: t('knowledgePage.workbench.rebuildStarted') });
        } catch (e) {
          $q.notify({ type: 'negative', message: e instanceof Error ? e.message : String(e) });
        }
        break;
      case 'backfill-autolink':
        confirmBackfillAutolink();
        break;
      case 'ingest-text':
        deps.events.ingestText();
        break;
      case 'promote':
        if (active) deps.events.promoteActive(active.docId);
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
        if (active) deps.workbench.closeTab(active.docId);
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
      const active = deps.workbench.activeTab.value;
      if (active?.editable) {
        e.preventDefault();
        deps.workbench.toggleMode(active.docId);
      }
    } else if (key === 'g') {
      e.preventDefault();
      deps.events.openGraph();
    }
  }

  return {
    // 落链 recency
    linkRecencyRank,
    onPickLink,
    // ⌘O / ⌘K / 搜索浮层
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
    // 命令面板
    commandMru,
    commandItems,
    runCommand,
    // 文档命令
    createNote,
    createFolder,
    createDocByName,
    saveDoc,
    onApplyAutolinkFromPanel,
    // 写回 / 治理浮层
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
    // 快捷键
    onGlobalKeydown,
  };
}
