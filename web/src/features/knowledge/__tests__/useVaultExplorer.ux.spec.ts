// UX 修复回归：详情加载错误态（ISSUE-003）+ 关联计数不重复文档口径（ISSUE-007）。
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { nextTick, ref } from 'vue';
import type { KnowledgeCollection, KnowledgeDocument, KnowledgeLink } from '../types';
import { getDocumentContent } from '../api';

const mockStore = {
  loadVaultTree: vi.fn(),
  invalidateTree: vi.fn(),
  loadDocumentLinks: vi.fn(),
  loadBlockBacklinks: vi.fn().mockResolvedValue([]),
  loadDanglingLinks: vi.fn().mockResolvedValue([]),
  search: vi.fn(),
  moveDocToDir: vi.fn(),
};

vi.mock('../../../stores/knowledge', () => ({
  useKnowledgeStore: () => mockStore,
}));

vi.mock('../api', () => ({
  fetchDocumentAsset: vi.fn(),
  getDocumentContent: vi.fn(),
  updateDocumentContent: vi.fn(),
}));

import { useVaultExplorer } from '../useVaultExplorer';

const mockGetContent = vi.mocked(getDocumentContent);

function makeDoc(id: string, relPath: string): KnowledgeDocument {
  const name = relPath.split('/').pop() ?? relPath;
  return {
    id,
    collection_id: 'c1',
    source: name,
    mime_type: 'text/markdown',
    size_bytes: 10,
    chunk_count: 1,
    status: 'indexed',
    error_message: '',
    created_at: '2026-07-30T00:00:00Z',
    updated_at: '2026-07-30T00:00:00Z',
    rel_path: relPath,
    summary: '',
    tags: [],
    doc_type: 'note',
  };
}

function makeLink(targetId: string, direction: string, linkType = 'explicit'): KnowledgeLink {
  return {
    target_doc_id: targetId,
    target_source: `${targetId}.md`,
    target_rel_path: `guides/${targetId}.md`,
    link_type: linkType,
    context: '',
    direction,
  };
}

const okContent = { content_text: '正文', organized: false, raw_content: '正文', base_hash: 'h1' };

function setup(docs: KnowledgeDocument[]) {
  const selectedId = ref('c1');
  const collections = ref<KnowledgeCollection[]>([
    {
      id: 'c1',
      name: 'Vault 1',
      description: '',
      embedding_model: 'm',
      dim: 1536,
      status: 'active',
      document_count: docs.length,
      chunk_count: docs.length,
      workspace: '',
      created_at: '2026-07-30T00:00:00Z',
      updated_at: '2026-07-30T00:00:00Z',
      root_path: '/vault',
      sync_state: 'active',
      last_sync_at: '',
    },
  ]);
  const documents = ref(docs);
  const notifyError = vi.fn();
  const ex = useVaultExplorer({
    selectedId,
    collections,
    documents,
    friendlyError: (e) => (e instanceof Error ? e.message : String(e)),
    notifyError,
    semanticErrorFallback: () => 'semantic failed',
  });
  return { ex, notifyError };
}

/** 选中文档并等详情加载 settle（watcher flush:pre + loadDetail 两段 await）。 */
async function selectAndSettle(ex: ReturnType<typeof useVaultExplorer>, docId: string) {
  ex.selectDocument(docId);
  await nextTick();
  await vi.waitFor(() => {
    expect(ex.previewLoading.value).toBe(false);
    expect(ex.linksLoading.value).toBe(false);
  });
}

describe('UX-003 详情加载错误态', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockStore.loadVaultTree.mockResolvedValue([makeDoc('d1', 'readme.md')].map((d) => ({
      name: d.source,
      path: d.rel_path,
      kind: 'file',
      doc_id: d.id,
      summary: '',
      tags: [],
      doc_type: 'note',
      status: 'indexed',
      size_bytes: 10,
      updated_at: d.updated_at,
      error_message: '',
    })));
  });

  it('内容/关联加载失败置错误态并通知；reloadDetail 重试成功后清除', async () => {
    const { ex, notifyError } = setup([makeDoc('d1', 'readme.md')]);
    mockGetContent.mockRejectedValueOnce(new Error('boom'));
    mockStore.loadDocumentLinks.mockRejectedValueOnce(new Error('boom'));

    await selectAndSettle(ex, 'd1');
    expect(ex.previewError.value).toBe(true);
    expect(ex.linksError.value).toBe(true);
    expect(ex.previewContent.value).toBe('');
    expect(notifyError).toHaveBeenCalledTimes(2);

    mockGetContent.mockResolvedValueOnce(okContent);
    mockStore.loadDocumentLinks.mockResolvedValueOnce([]);
    ex.reloadDetail();
    await vi.waitFor(() => expect(ex.previewError.value).toBe(false));
    await vi.waitFor(() => expect(ex.linksLoading.value).toBe(false));
    expect(ex.previewContent.value).toBe('正文');
    expect(ex.linksError.value).toBe(false);
  });

  it('重复点击已选中文档：错误态下重新拉取，正常时不重复拉取', async () => {
    const { ex } = setup([makeDoc('d1', 'readme.md')]);
    mockGetContent.mockResolvedValueOnce(okContent);
    mockStore.loadDocumentLinks.mockResolvedValueOnce([]);
    await selectAndSettle(ex, 'd1');
    expect(mockGetContent).toHaveBeenCalledTimes(1);

    // 正常态重复点击：不重新拉取。
    ex.selectDocument('d1');
    await nextTick();
    expect(mockGetContent).toHaveBeenCalledTimes(1);

    // 切走再切回失败一次，构造错误态。
    mockGetContent.mockResolvedValueOnce(okContent);
    mockStore.loadDocumentLinks.mockResolvedValueOnce([]);
    await selectAndSettle(ex, 'd2');
    mockGetContent.mockRejectedValueOnce(new Error('boom'));
    mockStore.loadDocumentLinks.mockRejectedValueOnce(new Error('boom'));
    await selectAndSettle(ex, 'd1');
    expect(ex.previewError.value).toBe(true);
    const callsAfterFail = mockGetContent.mock.calls.length;

    // 错误态重复点击同一文档：重新拉取。
    mockGetContent.mockResolvedValueOnce(okContent);
    mockStore.loadDocumentLinks.mockResolvedValueOnce([]);
    ex.selectDocument('d1');
    await vi.waitFor(() => expect(ex.previewError.value).toBe(false));
    expect(mockGetContent.mock.calls.length).toBe(callsAfterFail + 1);
  });
});

describe('UX-007 关联计数按不重复文档聚合', () => {
  it('同一目标文档双向/多记录只计一次', async () => {
    const { ex } = setup([makeDoc('d1', 'readme.md')]);
    mockGetContent.mockResolvedValueOnce(okContent);
    // readme 与 setup/advanced 互引（4 条记录、2 篇不重复文档）+ hello 单向。
    mockStore.loadDocumentLinks.mockResolvedValueOnce([
      makeLink('setup', 'out'),
      makeLink('setup', 'in'),
      makeLink('advanced', 'out'),
      makeLink('advanced', 'in'),
      makeLink('hello', 'in'),
      makeLink('setup', 'out', 'entity'),
    ]);

    await selectAndSettle(ex, 'd1');
    // explicit：setup/advanced/hello 共 3 篇（而非 5 条记录）；entity：setup 1 篇。
    expect(ex.linkCounts.value).toEqual({ explicit: 3, entity: 1, semantic: 0 });
  });
});
