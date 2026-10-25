// useKnowledgeWorkbench（SP2 §SP2-4）：工作台唯一状态机——tabs/激活/脏标记/CAS 保存。
// 数据流纪律：组件全部经该 composable 交互，不各自拉数（MetadataCache 单一真相源哲学的前端映射）。
import { computed, ref, watch, type ComputedRef, type Ref } from 'vue';
import { useKnowledgeStore } from '../../stores/knowledge';
import type { KnowledgeDocument, KnowledgeDocumentContent } from './types';

export type WorkbenchTab = {
  docId: string;
  collectionId?: string;
  relPath: string;
  title: string;
  /** edit = CM6 编辑器；preview = 只读预览。非 markdown 文档恒 preview 且不可切换 */
  mode: 'edit' | 'preview';
  editable: boolean;
  dirty: boolean;
  saving: boolean;
  /** CAS：UpdateDocumentContent 的 expectedHash */
  baseHash: string;
  content: string;
  /** 上次保存发生 CAS 冲突（远端已变更，本地留双份）；刷新 baseHash 后置位等待用户决策 */
  conflict: boolean;
  /** 保存期间继续输入时比较此修订号，不把新内容误标为已保存。 */
  revision?: number;
  sourceMime?: string;
};

export type WorkbenchDeps = {
  getDocumentContent: (id: string) => Promise<KnowledgeDocumentContent>;
  updateDocumentContent: (
    id: string,
    content: string,
    baseHash: string,
  ) => Promise<{ document: KnowledgeDocument; conflict: boolean; base_hash?: string }>;
  getDocument?: (id: string) => Promise<KnowledgeDocument>;
};

function storeWorkbenchDeps(): WorkbenchDeps {
  const store = useKnowledgeStore();
  return {
    getDocumentContent: (id) => store.loadDocumentContent(id),
    updateDocumentContent: (id, content, baseHash) => store.saveDocumentContent(id, content, baseHash),
    getDocument: (id) => store.loadDocument(id),
  };
}

/** 可编辑判定：文本类 markdown 文档才可进编辑器（图片/音视频/PDF 等走预览/下载路径）。 */
function isEditable(d: KnowledgeDocument): boolean {
  // 历史导入行可能误标 text/markdown，原件扩展优先拦截二进制文件。
  if (
    /\.(pdf|docx?|xlsx?|pptx?|png|jpe?g|gif|webp|bmp|svg|tiff?|mp[34]|wav|ogg|webm|zip)$/i.test(d.rel_path || d.source)
  ) {
    return false;
  }
  if (d.mime_type === 'text/markdown') return true;
  const p = d.rel_path.toLowerCase();
  return /\.(md|markdown)$/.test(p) && !d.mime_type.startsWith('image/');
}

function baseName(relPath: string): string {
  const seg = relPath.split('/').filter(Boolean);
  return seg.length ? seg[seg.length - 1] : relPath;
}

export type KnowledgeWorkbench = ReturnType<typeof createKnowledgeWorkbench>;

export type WorkbenchDraftOptions = { storage: Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>; key: string };

export function createKnowledgeWorkbench(deps?: WorkbenchDeps, drafts?: WorkbenchDraftOptions) {
  const resolved = deps ?? storeWorkbenchDeps();
  const tabs: Ref<WorkbenchTab[]> = ref([]);
  const activeTabId: Ref<string> = ref('');
  /** 待确认的脏关闭（UI 层弹「保存/放弃/取消」） */
  const confirmCloseId: Ref<string> = ref('');
  const draftError = ref(false);
  const hasUnsaved = computed(() => tabs.value.some((t) => t.dirty || t.saving));
  const opening = new Map<string, Promise<void>>();
  let restoring = false;
  const deferredDrafts = new Map<string, { docId: string; content: string; baseHash: string }>();

  function persistDrafts(): void {
    if (!drafts || restoring) return;
    try {
      const dirty = [
        ...new Map([
          ...deferredDrafts.entries(),
          ...tabs.value.filter((tab) => tab.dirty).map((tab) => [tab.docId, tab] as const),
        ]).values(),
      ];
      if (dirty.length)
        drafts.storage.setItem(
          drafts.key,
          JSON.stringify(dirty.map((tab) => ({ docId: tab.docId, content: tab.content, baseHash: tab.baseHash }))),
        );
      else drafts.storage.removeItem(drafts.key);
      draftError.value = false;
    } catch {
      draftError.value = true;
    }
  }

  async function restoreDrafts(): Promise<number> {
    if (!drafts || !resolved.getDocument) return 0;
    let saved: unknown;
    try {
      saved = JSON.parse(drafts.storage.getItem(drafts.key) || '[]');
    } catch {
      return 0;
    }
    if (!Array.isArray(saved)) return 0;
    restoring = true;
    let restored = 0;
    for (const item of saved) {
      if (
        item &&
        typeof item.docId === 'string' &&
        typeof item.content === 'string' &&
        typeof item.baseHash === 'string'
      )
        deferredDrafts.set(item.docId, item);
    }
    for (const item of saved) {
      if (
        !item ||
        typeof item.docId !== 'string' ||
        typeof item.content !== 'string' ||
        typeof item.baseHash !== 'string'
      )
        continue;
      try {
        // 恢复前重新验证当前权限与真实库归属，不能直接展示旧账户的缓存正文。
        const doc = await resolved.getDocument(item.docId);
        if (!isEditable(doc)) continue;
        if (find(doc.id)?.dirty) continue;
        await openDoc(doc);
        const tab = find(doc.id);
        if (!tab) continue;
        tab.conflict = tab.baseHash !== item.baseHash;
        tab.baseHash = item.baseHash;
        tab.content = item.content;
        tab.dirty = true;
        tab.mode = 'edit';
        deferredDrafts.delete(doc.id);
        restored++;
      } catch {
        // 失去权限或资料暂不可用时不恢复；保留原草稿供下次恢复。
      }
    }
    restoring = false;
    return restored;
  }

  if (drafts) watch(tabs, persistDrafts, { deep: true, flush: 'sync' });

  const activeTab: ComputedRef<WorkbenchTab | null> = computed(
    () => tabs.value.find((t) => t.docId === activeTabId.value) ?? null,
  );

  function find(docId: string): WorkbenchTab | undefined {
    return tabs.value.find((t) => t.docId === docId);
  }

  async function openDoc(d: KnowledgeDocument): Promise<void> {
    const existing = find(d.id);
    if (existing) {
      activeTabId.value = existing.docId;
      return;
    }
    const inFlight = opening.get(d.id);
    if (inFlight) return inFlight;
    const pending = loadDoc(d);
    opening.set(d.id, pending);
    try {
      await pending;
    } finally {
      opening.delete(d.id);
    }
  }

  async function loadDoc(d: KnowledgeDocument): Promise<void> {
    const res = await resolved.getDocumentContent(d.id);
    const editable = isEditable(d);
    tabs.value.push({
      docId: d.id,
      collectionId: d.collection_id,
      sourceMime: d.mime_type,
      relPath: d.rel_path,
      title: baseName(d.rel_path) || d.source,
      mode: editable ? 'edit' : 'preview',
      editable,
      dirty: false,
      saving: false,
      baseHash: res.base_hash,
      content: res.content_text,
      conflict: false,
      revision: 0,
    });
    activeTabId.value = d.id;
  }

  function activateTab(docId: string): void {
    if (find(docId)) activeTabId.value = docId;
  }

  /** Explicit conflict resolution: discard the local draft only after UI confirmation. */
  async function reloadTab(docId: string): Promise<boolean> {
    const tab = find(docId);
    if (!tab || tab.saving) return false;
    const revision = tab.revision ?? 0;
    tab.saving = true;
    try {
      const [metadata, content] = await Promise.all([
        resolved.getDocument ? resolved.getDocument(docId) : Promise.resolve(null),
        resolved.getDocumentContent(docId),
      ]);
      if (!find(docId) || (tab.revision ?? 0) !== revision) return false;
      if (metadata) {
        tab.collectionId = metadata.collection_id;
        tab.relPath = metadata.rel_path;
        tab.title = baseName(metadata.rel_path) || metadata.source;
        tab.editable = isEditable(metadata);
        tab.sourceMime = metadata.mime_type;
      }
      tab.content = content.content_text;
      tab.baseHash = content.base_hash;
      tab.dirty = false;
      tab.conflict = false;
      if (!tab.editable) tab.mode = 'preview';
      return true;
    } finally {
      tab.saving = false;
    }
  }

  /** 拖拽重排（P1-4）：把 from 索引的 tab 移到 to 索引；激活态不变。 */
  function reorderTabs(from: number, to: number): void {
    const len = tabs.value.length;
    if (from === to || from < 0 || to < 0 || from >= len || to >= len) return;
    const copy = [...tabs.value];
    const [moved] = copy.splice(from, 1);
    copy.splice(to, 0, moved);
    tabs.value = copy;
  }

  function updateContent(docId: string, content: string): void {
    const tab = find(docId);
    if (!tab || !tab.editable || tab.content === content) return;
    tab.content = content;
    tab.dirty = true;
    tab.conflict = false;
    tab.revision = (tab.revision ?? 0) + 1;
  }

  function toggleMode(docId: string): void {
    const tab = find(docId);
    if (!tab || !tab.editable) return;
    tab.mode = tab.mode === 'edit' ? 'preview' : 'edit';
  }

  async function saveTab(docId: string): Promise<boolean> {
    const tab = find(docId);
    if (!tab) return true;
    if (tab.saving) return false;
    if (!tab.editable) return false;
    if (!tab.dirty) return true;
    const submitted = tab.content;
    const revision = tab.revision ?? 0;
    tab.saving = true;
    try {
      const res = await resolved.updateDocumentContent(docId, submitted, tab.baseHash);
      if (res.conflict) {
        // CAS 冲突：远端已写入留双份；重新拉取远端 hash 供下次保存，本地内容保留由用户决策
        if (res.base_hash) tab.baseHash = res.base_hash;
        tab.conflict = true;
        return false;
      }
      tab.conflict = false;
      if (res.base_hash) {
        tab.baseHash = res.base_hash;
      } else {
        // 旧服务端兼容：只在回读正文仍等于提交稿时采纳 hash。
        const fresh = await resolved.getDocumentContent(docId);
        if ((fresh.raw_content || fresh.content_text) !== submitted) {
          tab.conflict = true;
          return false;
        }
        tab.baseHash = fresh.base_hash;
      }
      tab.dirty = (tab.revision ?? 0) !== revision;
      return !tab.dirty;
    } finally {
      tab.saving = false;
    }
  }

  function closeTab(docId: string, opts: { discard?: boolean } = {}): void {
    const tab = find(docId);
    if (!tab) return;
    if (tab.dirty && !opts.discard) {
      confirmCloseId.value = docId;
      return;
    }
    const idx = tabs.value.findIndex((t) => t.docId === docId);
    tabs.value.splice(idx, 1);
    if (confirmCloseId.value === docId) confirmCloseId.value = '';
    if (activeTabId.value === docId) {
      const neighbor = tabs.value[Math.min(idx, tabs.value.length - 1)];
      activeTabId.value = neighbor ? neighbor.docId : '';
    }
  }

  /** 取消脏关闭确认（UI 弹窗「取消」路径）。 */
  function dismissCloseConfirm(): void {
    confirmCloseId.value = '';
  }

  function onDocRemoved(docId: string): void {
    const idx = tabs.value.findIndex((t) => t.docId === docId);
    if (idx < 0) return;
    tabs.value.splice(idx, 1);
    if (confirmCloseId.value === docId) confirmCloseId.value = '';
    if (activeTabId.value === docId) {
      const neighbor = tabs.value[Math.min(idx, tabs.value.length - 1)];
      activeTabId.value = neighbor ? neighbor.docId : '';
    }
  }

  function onDocRenamed(d: KnowledgeDocument): void {
    const tab = find(d.id);
    if (!tab) return;
    tab.relPath = d.rel_path;
    tab.title = baseName(d.rel_path) || d.source;
    tab.collectionId = d.collection_id;
  }

  return {
    tabs,
    activeTabId,
    activeTab,
    confirmCloseId,
    hasUnsaved,
    draftError,
    draftsAvailable: !!drafts,
    persistDrafts,
    restoreDrafts,
    openDoc,
    activateTab,
    reloadTab,
    reorderTabs,
    updateContent,
    toggleMode,
    saveTab,
    closeTab,
    dismissCloseConfirm,
    onDocRemoved,
    onDocRenamed,
  };
}
