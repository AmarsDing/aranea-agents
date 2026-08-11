// useKnowledgeWorkbench 状态机（SP2 §SP2-4）：tabs 开闭/去重激活/脏标记/CAS 冲突/删除联动。
import { describe, expect, it, vi } from 'vitest';
import { createKnowledgeWorkbench, type WorkbenchDeps } from '../useKnowledgeWorkbench';
import type { KnowledgeDocument } from '../types';

function doc(over: Partial<KnowledgeDocument>): KnowledgeDocument {
  return {
    id: 'd1',
    collection_id: 'c1',
    source: 'a.md',
    mime_type: 'text/markdown',
    size_bytes: 10,
    chunk_count: 1,
    status: 'indexed',
    error_message: '',
    created_at: '',
    updated_at: '',
    rel_path: 'a.md',
    summary: '',
    tags: [],
    doc_type: '',
    ...over,
  };
}

function makeDeps(over: Partial<WorkbenchDeps> = {}): WorkbenchDeps {
  return {
    getDocumentContent: vi.fn(async (id: string) => ({
      id,
      content_text: `# content of ${id}`,
      organized: false,
      raw_content: '',
      base_hash: `hash-${id}`,
    })),
    updateDocumentContent: vi.fn(async (id: string, content: string) => ({
      document: doc({ id }),
      conflict: false,
    })),
    ...over,
  };
}

describe('useKnowledgeWorkbench', () => {
  it('openDoc fetches content and activates a new tab', async () => {
    const wb = createKnowledgeWorkbench(makeDeps());
    await wb.openDoc(doc({ id: 'd1' }));
    expect(wb.tabs.value).toHaveLength(1);
    const tab = wb.tabs.value[0];
    expect(tab.docId).toBe('d1');
    expect(tab.title).toBe('a.md');
    expect(tab.content).toBe('# content of d1');
    expect(tab.baseHash).toBe('hash-d1');
    expect(tab.mode).toBe('edit');
    expect(tab.dirty).toBe(false);
    expect(wb.activeTabId.value).toBe('d1');
  });

  it('openDoc dedupes: re-selecting an open doc activates instead of reopening', async () => {
    const deps = makeDeps();
    const wb = createKnowledgeWorkbench(deps);
    await wb.openDoc(doc({ id: 'd1' }));
    await wb.openDoc(doc({ id: 'd2', rel_path: 'b.md', source: 'b.md' }));
    await wb.openDoc(doc({ id: 'd1' }));
    expect(wb.tabs.value).toHaveLength(2);
    expect(wb.activeTabId.value).toBe('d1');
    expect(deps.getDocumentContent).toHaveBeenCalledTimes(2); // d1 未重复拉取
  });

  it('non-markdown docs are locked to preview mode', async () => {
    const wb = createKnowledgeWorkbench(makeDeps());
    await wb.openDoc(doc({ id: 'img1', rel_path: 'p.png', source: 'p.png', mime_type: 'image/png' }));
    const tab = wb.tabs.value[0];
    expect(tab.mode).toBe('preview');
    expect(tab.editable).toBe(false);
  });

  it('markDirty / toggleMode update the active tab', async () => {
    const wb = createKnowledgeWorkbench(makeDeps());
    await wb.openDoc(doc({ id: 'd1' }));
    wb.updateContent('d1', '# edited');
    expect(wb.tabs.value[0].dirty).toBe(true);
    expect(wb.tabs.value[0].content).toBe('# edited');
    wb.toggleMode('d1');
    expect(wb.tabs.value[0].mode).toBe('preview');
    wb.toggleMode('d1');
    expect(wb.tabs.value[0].mode).toBe('edit');
  });

  it('toggleMode is a no-op for non-editable tabs', async () => {
    const wb = createKnowledgeWorkbench(makeDeps());
    await wb.openDoc(doc({ id: 'img1', mime_type: 'image/png', rel_path: 'p.png' }));
    wb.toggleMode('img1');
    expect(wb.tabs.value[0].mode).toBe('preview');
  });

  it('saveTab persists via CAS and clears dirty on success', async () => {
    const deps = makeDeps();
    const wb = createKnowledgeWorkbench(deps);
    await wb.openDoc(doc({ id: 'd1' }));
    wb.updateContent('d1', '# v2');
    const ok = await wb.saveTab('d1');
    expect(ok).toBe(true);
    expect(deps.updateDocumentContent).toHaveBeenCalledWith('d1', '# v2', 'hash-d1');
    expect(wb.tabs.value[0].dirty).toBe(false);
    expect(wb.tabs.value[0].conflict).toBe(false);
  });

  it('saveTab on conflict refreshes baseHash, keeps dirty, flags conflict', async () => {
    const deps = makeDeps({
      updateDocumentContent: vi.fn(async (id: string) => ({
        document: doc({ id }),
        conflict: true,
      })),
      // 冲突后重新拉取拿到远端 hash
      getDocumentContent: vi.fn(async (id: string) => ({
        id,
        content_text: '# remote',
        organized: false,
        raw_content: '',
        base_hash: 'hash-remote',
      })),
    });
    const wb = createKnowledgeWorkbench(deps);
    await wb.openDoc(doc({ id: 'd1' }));
    wb.updateContent('d1', '# mine');
    const ok = await wb.saveTab('d1');
    expect(ok).toBe(false);
    const tab = wb.tabs.value[0];
    expect(tab.conflict).toBe(true);
    expect(tab.dirty).toBe(true); // 本地内容未丢
    expect(tab.content).toBe('# mine');
    expect(tab.baseHash).toBe('hash-remote'); // 远端 hash 已刷新，下次保存可过 CAS
  });

  it('closeTab removes the tab and activates a neighbor', async () => {
    const wb = createKnowledgeWorkbench(makeDeps());
    await wb.openDoc(doc({ id: 'd1' }));
    await wb.openDoc(doc({ id: 'd2', rel_path: 'b.md' }));
    await wb.openDoc(doc({ id: 'd3', rel_path: 'c.md' }));
    expect(wb.activeTabId.value).toBe('d3');
    wb.closeTab('d3');
    expect(wb.tabs.value.map((t) => t.docId)).toEqual(['d1', 'd2']);
    expect(wb.activeTabId.value).toBe('d2'); // 相邻左侧
  });

  it('closeTab on dirty tab sets confirmClose instead of closing', async () => {
    const wb = createKnowledgeWorkbench(makeDeps());
    await wb.openDoc(doc({ id: 'd1' }));
    wb.updateContent('d1', '# dirty');
    wb.closeTab('d1');
    expect(wb.tabs.value).toHaveLength(1);
    expect(wb.confirmCloseId.value).toBe('d1');
    // 确认放弃后真正关闭
    wb.closeTab('d1', { discard: true });
    expect(wb.tabs.value).toHaveLength(0);
    expect(wb.confirmCloseId.value).toBe('');
    expect(wb.activeTabId.value).toBe('');
  });

  it('onDocRemoved closes the tab without confirm', async () => {
    const wb = createKnowledgeWorkbench(makeDeps());
    await wb.openDoc(doc({ id: 'd1' }));
    wb.updateContent('d1', '# dirty');
    wb.onDocRemoved('d1');
    expect(wb.tabs.value).toHaveLength(0);
  });

  it('onDocRenamed syncs relPath/title of the open tab', async () => {
    const wb = createKnowledgeWorkbench(makeDeps());
    await wb.openDoc(doc({ id: 'd1', rel_path: 'old.md' }));
    wb.onDocRenamed(doc({ id: 'd1', rel_path: 'new/name.md', source: 'name.md' }));
    expect(wb.tabs.value[0].relPath).toBe('new/name.md');
    expect(wb.tabs.value[0].title).toBe('name.md');
  });

  it('activateTab switches activeTabId only when tab exists', async () => {
    const wb = createKnowledgeWorkbench(makeDeps());
    await wb.openDoc(doc({ id: 'd1' }));
    wb.activateTab('nope');
    expect(wb.activeTabId.value).toBe('d1');
    wb.activateTab('d1');
    expect(wb.activeTabId.value).toBe('d1');
  });

  it('reorderTabs moves a tab to the target index without touching activeTabId', async () => {
    const wb = createKnowledgeWorkbench(makeDeps());
    await wb.openDoc(doc({ id: 'd1' }));
    await wb.openDoc(doc({ id: 'd2', rel_path: 'b.md' }));
    await wb.openDoc(doc({ id: 'd3', rel_path: 'c.md' }));
    wb.activateTab('d1');
    wb.reorderTabs(0, 2);
    expect(wb.tabs.value.map((t) => t.docId)).toEqual(['d2', 'd3', 'd1']);
    expect(wb.activeTabId.value).toBe('d1'); // 重排不改变激活态
    wb.reorderTabs(2, 0);
    expect(wb.tabs.value.map((t) => t.docId)).toEqual(['d1', 'd2', 'd3']);
  });

  it('reorderTabs ignores out-of-range and no-op indexes', async () => {
    const wb = createKnowledgeWorkbench(makeDeps());
    await wb.openDoc(doc({ id: 'd1' }));
    await wb.openDoc(doc({ id: 'd2', rel_path: 'b.md' }));
    wb.reorderTabs(0, 0); // no-op
    wb.reorderTabs(-1, 1);
    wb.reorderTabs(0, 5);
    expect(wb.tabs.value.map((t) => t.docId)).toEqual(['d1', 'd2']);
  });
});
