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
    updateDocumentContent: vi.fn(async (id: string, _content: string) => ({
      document: doc({ id }),
      conflict: false,
      base_hash: 'saved-hash',
    })),
    ...over,
  };
}

describe('useKnowledgeWorkbench', () => {
  it('reloads a conflicting draft only when explicitly requested', async () => {
    const deps = makeDeps();
    const wb = createKnowledgeWorkbench(deps);
    await wb.openDoc(doc({ id: 'reload' }));
    wb.updateContent('reload', '# local draft');
    wb.activeTab.value!.conflict = true;
    expect(await wb.reloadTab('reload')).toBe(true);
    expect(wb.activeTab.value).toMatchObject({
      content: '# content of reload',
      baseHash: 'hash-reload',
      dirty: false,
      conflict: false,
    });
  });

  it('preserves the draft when reloading is denied', async () => {
    const deps = makeDeps();
    const wb = createKnowledgeWorkbench(deps);
    await wb.openDoc(doc({ id: 'denied-reload' }));
    wb.updateContent('denied-reload', '# keep this draft');
    vi.mocked(deps.getDocumentContent).mockRejectedValueOnce(new Error('not found'));
    await expect(wb.reloadTab('denied-reload')).rejects.toThrow('not found');
    expect(wb.activeTab.value).toMatchObject({ content: '# keep this draft', dirty: true, saving: false });
  });

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

describe('knowledge repair regressions', () => {
  it('keeps edits made while saving dirty and rolls forward only the submitted hash', async () => {
    let finish!: (value: { document: KnowledgeDocument; conflict: boolean; base_hash: string }) => void;
    const update = vi.fn(
      () =>
        new Promise<{ document: KnowledgeDocument; conflict: boolean; base_hash: string }>((resolve) => {
          finish = resolve;
        }),
    );
    const wb = createKnowledgeWorkbench(makeDeps({ updateDocumentContent: update }));
    await wb.openDoc(doc({ id: 'race' }));
    wb.updateContent('race', '# submitted');
    const pending = wb.saveTab('race');
    wb.updateContent('race', '# typed during save');
    expect(await wb.saveTab('race')).toBe(false);
    finish({ document: doc({ id: 'race' }), conflict: false, base_hash: 'submitted-hash' });
    expect(await pending).toBe(false);
    expect(wb.activeTab.value).toMatchObject({
      content: '# typed during save',
      dirty: true,
      baseHash: 'submitted-hash',
    });
    expect(update).toHaveBeenCalledWith('race', '# submitted', 'hash-race');
  });

  it('deduplicates two concurrent opens instead of creating duplicate tabs', async () => {
    const deps = makeDeps();
    const wb = createKnowledgeWorkbench(deps);
    await Promise.all([wb.openDoc(doc({ id: 'same' })), wb.openDoc(doc({ id: 'same' }))]);
    expect(wb.tabs.value).toHaveLength(1);
    expect(deps.getDocumentContent).toHaveBeenCalledTimes(1);
  });

  it('keeps legacy binary originals read-only even when their stored MIME says markdown', async () => {
    const deps = makeDeps();
    const wb = createKnowledgeWorkbench(deps);
    await wb.openDoc(doc({ id: 'pdf', rel_path: 'original.pdf', mime_type: 'text/markdown' }));
    expect(wb.activeTab.value).toMatchObject({ editable: false, mode: 'preview', collectionId: 'c1' });
    wb.updateContent('pdf', '# do not overwrite');
    expect(await wb.saveTab('pdf')).toBe(false);
    expect(deps.updateDocumentContent).not.toHaveBeenCalled();
  });

  it('restores drafts only after checking current document access and original hash', async () => {
    const rows = new Map<string, string>();
    const storage = {
      getItem: (key: string) => rows.get(key) ?? null,
      setItem: (key: string, value: string) => {
        rows.set(key, value);
      },
      removeItem: (key: string) => {
        rows.delete(key);
      },
    };
    const drafts = { key: 'user-1:workspace', storage };
    const wb = createKnowledgeWorkbench(makeDeps(), drafts);
    await wb.openDoc(doc({ id: 'draft' }));
    wb.updateContent('draft', '# keep me');
    const getDocument = vi.fn(async (id: string) => doc({ id, collection_id: 'actual-vault' }));
    const restored = createKnowledgeWorkbench(makeDeps({ getDocument }), drafts);
    expect(await restored.restoreDrafts()).toBe(1);
    expect(getDocument).toHaveBeenCalledWith('draft');
    expect(restored.activeTab.value).toMatchObject({ content: '# keep me', dirty: true, collectionId: 'actual-vault' });
    const denied = createKnowledgeWorkbench(
      makeDeps({ getDocument: vi.fn().mockRejectedValue(new Error('not found')) }),
      drafts,
    );
    expect(await denied.restoreDrafts()).toBe(0);
    expect(denied.tabs.value).toHaveLength(0);
  });

  it('retains the old CAS hash when a legacy response is followed by somebody else’s write', async () => {
    const deps = makeDeps({
      updateDocumentContent: vi.fn(async (id: string) => ({ document: doc({ id }), conflict: false })),
    });
    const wb = createKnowledgeWorkbench(deps);
    await wb.openDoc(doc({ id: 'changed' }));
    wb.updateContent('changed', '# ours');
    expect(await wb.saveTab('changed')).toBe(false);
    expect(wb.activeTab.value).toMatchObject({ dirty: true, conflict: true, baseHash: 'hash-changed' });
  });
});
