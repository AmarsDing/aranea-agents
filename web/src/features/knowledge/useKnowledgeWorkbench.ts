// useKnowledgeWorkbench（SP2 §SP2-4）：工作台唯一状态机——tabs/激活/脏标记/CAS 保存。
// 数据流纪律：组件全部经该 composable 交互，不各自拉数（MetadataCache 单一真相源哲学的前端映射）。
import { computed, ref, type ComputedRef, type Ref } from 'vue';
import { getDocumentContent, updateDocumentContent, type KnowledgeDocumentContent } from './api';
import type { KnowledgeDocument } from './types';

export type WorkbenchTab = {
  docId: string;
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
};

export type WorkbenchDeps = {
  getDocumentContent: (id: string) => Promise<KnowledgeDocumentContent>;
  updateDocumentContent: (
    id: string,
    content: string,
    baseHash: string,
  ) => Promise<{ document: KnowledgeDocument; conflict: boolean }>;
};

const defaultDeps: WorkbenchDeps = { getDocumentContent, updateDocumentContent };

/** 可编辑判定：文本类 markdown 文档才可进编辑器（图片/音视频/PDF 等走预览/下载路径）。 */
function isEditable(d: KnowledgeDocument): boolean {
  if (d.mime_type === 'text/markdown') return true;
  const p = d.rel_path.toLowerCase();
  return /\.(md|markdown)$/.test(p) && !d.mime_type.startsWith('image/');
}

function baseName(relPath: string): string {
  const seg = relPath.split('/').filter(Boolean);
  return seg.length ? seg[seg.length - 1] : relPath;
}

export type KnowledgeWorkbench = ReturnType<typeof createKnowledgeWorkbench>;

export function createKnowledgeWorkbench(deps: WorkbenchDeps = defaultDeps) {
  const tabs: Ref<WorkbenchTab[]> = ref([]);
  const activeTabId: Ref<string> = ref('');
  /** 待确认的脏关闭（UI 层弹「保存/放弃/取消」） */
  const confirmCloseId: Ref<string> = ref('');

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
    const res = await deps.getDocumentContent(d.id);
    const editable = isEditable(d);
    tabs.value.push({
      docId: d.id,
      relPath: d.rel_path,
      title: baseName(d.rel_path) || d.source,
      mode: editable ? 'edit' : 'preview',
      editable,
      dirty: false,
      saving: false,
      baseHash: res.base_hash,
      content: res.content_text,
      conflict: false,
    });
    activeTabId.value = d.id;
  }

  function activateTab(docId: string): void {
    if (find(docId)) activeTabId.value = docId;
  }

  function updateContent(docId: string, content: string): void {
    const tab = find(docId);
    if (!tab) return;
    tab.content = content;
    tab.dirty = true;
    tab.conflict = false;
  }

  function toggleMode(docId: string): void {
    const tab = find(docId);
    if (!tab || !tab.editable) return;
    tab.mode = tab.mode === 'edit' ? 'preview' : 'edit';
  }

  async function saveTab(docId: string): Promise<boolean> {
    const tab = find(docId);
    if (!tab || tab.saving || !tab.dirty) return true;
    tab.saving = true;
    try {
      const res = await deps.updateDocumentContent(docId, tab.content, tab.baseHash);
      if (res.conflict) {
        // CAS 冲突：远端已写入留双份；重新拉取远端 hash 供下次保存，本地内容保留由用户决策
        const remote = await deps.getDocumentContent(docId);
        tab.baseHash = remote.base_hash;
        tab.conflict = true;
        return false;
      }
      tab.conflict = false;
      tab.dirty = false;
      // 保存成功后 baseHash 滚动：服务端内容即本地内容，重新取 hash 保证下次 CAS 不过期
      const fresh = await deps.getDocumentContent(docId);
      tab.baseHash = fresh.base_hash;
      return true;
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
  }

  return {
    tabs,
    activeTabId,
    activeTab,
    confirmCloseId,
    openDoc,
    activateTab,
    updateContent,
    toggleMode,
    saveTab,
    closeTab,
    dismissCloseConfirm,
    onDocRemoved,
    onDocRenamed,
  };
}
