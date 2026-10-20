// U0-4 useBulkDocActions：纯函数（moveTargetsFor/runSequential）+ composable 胶水（mock store/quasar/i18n）。
import { describe, expect, it, vi, beforeEach } from 'vitest';
import type { KnowledgeCollection, VaultTreeNode } from '../types';

const notifyMock = vi.fn();
let dialogOutcome: boolean | null = true;
const dialogMock = vi.fn(() => ({
  onOk(cb: () => void) {
    if (dialogOutcome === true) cb();
    return {
      onCancel(cb2: () => void) {
        if (dialogOutcome === false) cb2();
        return {
          onDismiss(cb3: () => void) {
            if (dialogOutcome === null) cb3();
          },
        };
      },
    };
  },
}));

vi.mock('quasar', () => ({
  useQuasar: () => ({ notify: notifyMock, dialog: dialogMock }),
}));

// 仅覆盖 useI18n；createI18n 等保持真实（src/i18n/index.ts 在 axiosHandler 链上会真实初始化）。
vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>();
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, named?: Record<string, unknown>) => (named ? `${key}${JSON.stringify(named)}` : key),
    }),
  };
});

const mockStore = {
  removeDocument: vi.fn(),
  moveDoc: vi.fn(),
  reembedDocuments: vi.fn(),
  loadDocuments: vi.fn(),
  loadCollections: vi.fn(),
};

vi.mock('../../../stores/knowledge', () => ({
  useKnowledgeStore: () => mockStore,
}));

import { moveTargetsFor, runSequential, useBulkDocActions } from '../useBulkDocActions';

function col(id: string, dim: number, name = ''): KnowledgeCollection {
  return { id, name, dim } as KnowledgeCollection;
}

function node(docId: string): VaultTreeNode {
  return { name: `${docId}.md`, path: `${docId}.md`, kind: 'file', doc_id: docId } as VaultTreeNode;
}

describe('moveTargetsFor（批量移动目标选项）', () => {
  it('排除当前库；dim 不一致禁用；名称缺失回退 id', () => {
    const opts = moveTargetsFor(
      [col('v1', 1536, 'A'), col('v2', 768, 'B'), col('v3', 1536), col('v4', 1536, 'D')],
      'v1',
    );
    expect(opts.map((o) => o.value)).toEqual(['v2', 'v3', 'v4']);
    expect(opts.find((o) => o.value === 'v2')?.disable).toBe(true);
    expect(opts.find((o) => o.value === 'v3')?.disable).toBe(false);
    expect(opts.find((o) => o.value === 'v3')?.label).toBe('v3');
    expect(opts.find((o) => o.value === 'v4')?.label).toBe('D');
  });

  it('当前库不在列表（防御）时全部启用', () => {
    const opts = moveTargetsFor([col('v2', 768)], 'gone');
    expect(opts).toEqual([{ label: 'v2', value: 'v2', disable: false, dim: 768 }]);
  });
});

describe('runSequential（顺序批量、失败不中断）', () => {
  it('全部成功：保序返回，failed 为空', async () => {
    const calls: number[] = [];
    const res = await runSequential([1, 2, 3], async (n) => {
      calls.push(n);
    });
    expect(calls).toEqual([1, 2, 3]);
    expect(res.ok).toEqual([1, 2, 3]);
    expect(res.failed).toEqual([]);
  });

  it('单项失败不中断后续，失败项携带原错误', async () => {
    const boom = new Error('boom');
    const res = await runSequential([1, 2, 3], async (n) => {
      if (n === 2) throw boom;
    });
    expect(res.ok).toEqual([1, 3]);
    expect(res.failed).toEqual([{ item: 2, error: boom }]);
  });

  it('空输入直接返回空结果', async () => {
    const res = await runSequential([], async () => undefined);
    expect(res).toEqual({ ok: [], failed: [] });
  });
});

describe('useBulkDocActions（composable 胶水）', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    dialogOutcome = true;
    mockStore.removeDocument.mockResolvedValue(undefined);
    mockStore.moveDoc.mockResolvedValue({});
    mockStore.reembedDocuments.mockResolvedValue({ accepted_count: 2, skipped_count: 0 });
    mockStore.loadDocuments.mockResolvedValue({});
    mockStore.loadCollections.mockResolvedValue({});
  });

  function setup(vaultId = 'v1') {
    return useBulkDocActions({ vaultId: () => vaultId, collections: () => [col('v1', 1536), col('v2', 1536, 'B')] });
  }

  it('deleteSelected：确认框列计数 → 逐个删除 → positive 汇总 → 刷新库计数', async () => {
    const b = setup();
    const out = await b.deleteSelected([node('d1'), node('d2')]);
    expect(dialogMock).toHaveBeenCalledTimes(1);
    expect(mockStore.removeDocument.mock.calls.map((c) => c[0])).toEqual(['d1', 'd2']);
    expect(out).toEqual({ okCount: 2, failedIds: [] });
    expect(notifyMock.mock.calls[0][0].type).toBe('positive');
    expect(String(notifyMock.mock.calls[0][0].message)).toContain('deleteDone');
    expect(mockStore.loadCollections).toHaveBeenCalled();
  });

  it('deleteSelected：用户取消 → 不调用任何接口、不发通知', async () => {
    dialogOutcome = false;
    const b = setup();
    const out = await b.deleteSelected([node('d1')]);
    expect(out).toBeNull();
    expect(mockStore.removeDocument).not.toHaveBeenCalled();
    expect(notifyMock).not.toHaveBeenCalled();
  });

  it('deleteSelected：部分失败 → warning 汇总并返回失败 id（供选择集保留）', async () => {
    mockStore.removeDocument.mockRejectedValueOnce(new Error('nope')).mockResolvedValueOnce(undefined);
    const b = setup();
    const out = await b.deleteSelected([node('d1'), node('d2')]);
    expect(out).toEqual({ okCount: 1, failedIds: ['d1'] });
    expect(notifyMock.mock.calls[0][0].type).toBe('warning');
    expect(String(notifyMock.mock.calls[0][0].message)).toContain('deletePartial');
  });

  it('moveSelected：逐文档提交同一目标库，成功后刷新文档列表与库计数', async () => {
    const b = setup();
    const out = await b.moveSelected([node('d1'), node('d2')], 'v2');
    expect(mockStore.moveDoc.mock.calls).toEqual([
      ['d1', 'v2'],
      ['d2', 'v2'],
    ]);
    expect(out).toEqual({ okCount: 2, failedIds: [] });
    expect(mockStore.loadDocuments).toHaveBeenCalledWith('v1', {});
    expect(mockStore.loadCollections).toHaveBeenCalled();
  });

  it('moveSelected：目标为空或文档为空 → no-op', async () => {
    const b = setup();
    expect(await b.moveSelected([node('d1')], '')).toBeNull();
    expect(await b.moveSelected([], 'v2')).toBeNull();
    expect(mockStore.moveDoc).not.toHaveBeenCalled();
  });

  it('moveSelected：全部失败 → negative 汇总且不刷新', async () => {
    mockStore.moveDoc.mockRejectedValue(new Error('x'));
    const b = setup();
    const out = await b.moveSelected([node('d1')], 'v2');
    expect(out).toEqual({ okCount: 0, failedIds: ['d1'] });
    expect(notifyMock.mock.calls[0][0].type).toBe('negative');
    expect(mockStore.loadDocuments).not.toHaveBeenCalled();
  });

  it('reembedSelected：批量受理接口（docIds 数组）+ 既有受理文案', async () => {
    const b = setup();
    b.reembedSelected([node('d1'), node('d2')]);
    // confirmDialog.then 是异步链，等待两个微任务让 then 落地。
    await Promise.resolve();
    await Promise.resolve();
    expect(mockStore.reembedDocuments).toHaveBeenCalledWith('v1', ['d1', 'd2']);
    await vi.waitFor(() => expect(notifyMock).toHaveBeenCalled());
    expect(String(notifyMock.mock.calls[0][0].message)).toContain('reembedAccepted');
  });

  it('reembedSelected：确认框取消 → 不提交', async () => {
    dialogOutcome = false;
    const b = setup();
    b.reembedSelected([node('d1')]);
    await Promise.resolve();
    await Promise.resolve();
    expect(mockStore.reembedDocuments).not.toHaveBeenCalled();
  });

  it('moveOptions 透传纯函数结果', () => {
    const b = setup();
    expect(b.moveOptions.value).toEqual([{ label: 'B', value: 'v2', disable: false, dim: 1536 }]);
  });
});
