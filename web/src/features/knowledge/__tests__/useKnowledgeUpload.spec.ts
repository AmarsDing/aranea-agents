// G-21 上传闭环：读取→上传→解析(WS 终态)→入库 阶段流转 + 错误重传 + 清理无悬挂。
// 评审回归：去重短路/同 doc 扇出/读取错标/超时对账。
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { ref } from 'vue';
import type { V2WsEnvelope } from '../../chat/v2Types';
import type { KnowledgeDocument } from '../types';

const notifySpy = vi.fn();
vi.mock('quasar', () => ({
  useQuasar: () => ({ notify: notifySpy, dialog: vi.fn(() => ({ onOk: vi.fn() })) }),
}));
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (k: string, p?: Record<string, unknown>) => (p ? `${k}:${JSON.stringify(p)}` : k) }),
}));

const ingestSpy = vi.fn();
const documentsByCollectionRef = ref<Record<string, KnowledgeDocument[]>>({});
vi.mock('../../../stores/knowledge', () => ({
  // setup store 的 ref 在实例上自动解包——getter 模拟 pinia 的 UnwrapRef 行为。
  useKnowledgeStore: () => ({
    ingest: ingestSpy,
    get documentsByCollection() {
      return documentsByCollectionRef.value;
    },
  }),
}));
vi.mock('../../realtime/useV2EventStream', () => ({ createV2EventStream: vi.fn() }));
vi.mock('../../config/runtime', () => ({ GLOBAL_WS_SESSION_ID: 'global' }));
const readFileAsBase64Spy = vi.hoisted(() => {
  const fn = vi.fn();
  fn.mockResolvedValue('aGVsbG8=');
  return fn;
});
vi.mock('../knowledgeUploadCodec', async (importOriginal) => {
  const orig = await importOriginal<typeof import('../knowledgeUploadCodec')>();
  return {
    ...orig,
    readFileAsBase64: readFileAsBase64Spy,
    utf8ToBase64: vi.fn().mockReturnValue('aGVsbG8='),
  };
});

import {
  useKnowledgeUpload,
  routeIngestEvent,
  type IngestWatchers,
  type KnowledgeUploadDeps,
} from '../useKnowledgeUpload';
import type { SystemNoticeEventPayload } from '../../chat/v2Types';

let v2Handler: ((env: V2WsEnvelope) => void) | null = null;

function makeDeps(overrides?: Partial<KnowledgeUploadDeps>): KnowledgeUploadDeps {
  return {
    selectedId: ref('col-1'),
    collections: { value: [] },
    loadDocuments: vi.fn().mockResolvedValue(undefined),
    loadCollections: vi.fn().mockResolvedValue(undefined),
    friendlyError: (e: unknown) => (e instanceof Error ? e.message : String(e)),
    selectTreeNode: vi.fn(),
    ingestStreamFactory: (opts) => {
      v2Handler = opts.onV2Event;
      return { connect: vi.fn(), disconnect: vi.fn() };
    },
    ...overrides,
  };
}

function ingestEvent(docId: string, status: string, extra?: Partial<SystemNoticeEventPayload['Meta']>): V2WsEnvelope {
  return {
    kind: 'system.notice',
    payload: { NoticeType: 'knowledge_ingest', Meta: { document_id: docId, status, ...extra } },
  } as unknown as V2WsEnvelope;
}

const file = new File(['hello'], '笔记.txt', { type: 'text/plain' });

describe('useKnowledgeUpload G-21 上传闭环', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    v2Handler = null;
    documentsByCollectionRef.value = {};
    readFileAsBase64Spy.mockResolvedValue('aGVsbG8=');
    ingestSpy.mockResolvedValue({ id: 'doc-1', collection_id: 'col-1' });
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('上传成功后进入 parsing，webSocket indexed 终态转 success 并带 chunk 数', async () => {
    const up = useKnowledgeUpload(makeDeps());
    await up.enqueueUploadFiles([file]);
    expect(ingestSpy).toHaveBeenCalledTimes(1);
    let task = up.uploadTasks.value[0];
    expect(task.status).toBe('parsing');
    expect(v2Handler).not.toBeNull();

    // 中间态 indexing：保持 parsing，不被完成
    v2Handler!(ingestEvent('doc-1', 'indexing'));
    task = up.uploadTasks.value[0];
    expect(task.status).toBe('parsing');

    v2Handler!(ingestEvent('doc-1', 'indexed', { chunk_count: '7' }));
    task = up.uploadTasks.value[0];
    expect(task.status).toBe('success');
    expect(task.chunk_count).toBe(7);
  });

  it('解析期 WS error 终态 → error + fail_stage=parse + 可重传', async () => {
    const up = useKnowledgeUpload(makeDeps());
    await up.enqueueUploadFiles([file]);
    v2Handler!(ingestEvent('doc-1', 'error', { error_message: 'boom' }));
    let task = up.uploadTasks.value[0];
    expect(task.status).toBe('error');
    expect(task.fail_stage).toBe('parse');

    // 重传：重新走完整流程（第二次 ingest），重新挂起终态等待
    await up.retryUploadTask(task.id);
    expect(ingestSpy).toHaveBeenCalledTimes(2);
    task = up.uploadTasks.value[0];
    expect(task.status).toBe('parsing');
    expect(task.fail_stage).toBeUndefined();

    v2Handler!(ingestEvent('doc-1', 'indexed', { chunk_count: '2' }));
    task = up.uploadTasks.value[0];
    expect(task.status).toBe('success');
  });

  it('上传阶段失败 → fail_stage=upload；重传后仍可成功', async () => {
    ingestSpy.mockRejectedValueOnce(new Error('network down'));
    const up = useKnowledgeUpload(makeDeps());
    await up.enqueueUploadFiles([file]);
    let task = up.uploadTasks.value[0];
    expect(task.status).toBe('error');
    expect(task.fail_stage).toBe('upload');

    await up.retryUploadTask(task.id);
    expect(ingestSpy).toHaveBeenCalledTimes(2);
    task = up.uploadTasks.value[0];
    expect(task.status).toBe('parsing');
  });

  it('移除任务后迟到的终态事件不再产生回调（无悬挂）', async () => {
    const up = useKnowledgeUpload(makeDeps());
    await up.enqueueUploadFiles([file]);
    expect(up.uploadTasks.value[0].status).toBe('parsing');
    up.removeUploadTask(up.uploadTasks.value[0].id);
    expect(up.uploadTasks.value.length).toBe(0);
    v2Handler!(ingestEvent('doc-1', 'indexed', { chunk_count: '1' }));
    expect(up.uploadTasks.value.length).toBe(0);
  });

  it('routeIngestEvent 只路由 knowledge_ingest 终态', () => {
    const cbs = { hit: 0, status: '' };
    const watchers: IngestWatchers = new Map([
      [
        'doc-9',
        new Map([
          [
            'task-1',
            (s: 'indexed' | 'error') => {
              cbs.hit++;
              cbs.status = s;
            },
          ],
        ]),
      ],
    ]);
    routeIngestEvent({ kind: 'chat.delta', payload: {} } as unknown as V2WsEnvelope, watchers);
    routeIngestEvent(ingestEvent('doc-9', 'indexing'), watchers);
    expect(cbs.hit).toBe(0);
    expect(watchers.has('doc-9')).toBe(true);
    routeIngestEvent(ingestEvent('doc-9', 'error'), watchers);
    expect(cbs.hit).toBe(1);
    expect(cbs.status).toBe('error');
    expect(watchers.has('doc-9')).toBe(false);
  });

  it('内容哈希去重返回已 indexed 文档：短路直成，不挂 watcher 等事件', async () => {
    // 对抗回归：重传同文件命中去重时服务端不发新 knowledge_ingest 帧——
    // 旧实现守 240s 后假超时；现在按返回的 status=indexed 立即结算。
    ingestSpy.mockResolvedValue({ id: 'doc-dup', collection_id: 'col-1', status: 'indexed', chunk_count: 5 });
    const up = useKnowledgeUpload(makeDeps());
    await up.enqueueUploadFiles([file]);
    const task = up.uploadTasks.value[0];
    expect(task.status).toBe('success');
    expect(task.chunk_count).toBe(5);
    expect(v2Handler).toBeNull(); // 未建事件流
  });

  it('同 doc 多任务扇出：一帧终态结算共享 document_id 的全部任务', async () => {
    // 对抗回归：批量同内容文件被去重到同一 doc，单值 watcher 互相砸掉曾致
    // 先到任务假超时。
    ingestSpy.mockResolvedValue({ id: 'doc-share', collection_id: 'col-1', status: 'indexing' });
    const otherFile = new File(['hello'], '副本.txt', { type: 'text/plain' });
    const up = useKnowledgeUpload(makeDeps());
    await up.enqueueUploadFiles([file, otherFile]);
    expect(up.uploadTasks.value.map((t) => t.status)).toEqual(['parsing', 'parsing']);
    v2Handler!(ingestEvent('doc-share', 'indexed', { chunk_count: '3' }));
    expect(up.uploadTasks.value.map((t) => t.status)).toEqual(['success', 'success']);
  });

  it('读取失败标 fail_stage=read 而非 upload', async () => {
    // 对抗回归：FileReader 失败曾被统一错标 upload，阶段条高亮错误段。
    readFileAsBase64Spy.mockRejectedValueOnce(new Error('disk gone'));
    const up = useKnowledgeUpload(makeDeps());
    await up.enqueueUploadFiles([file]);
    const task = up.uploadTasks.value[0];
    expect(task.status).toBe('error');
    expect(task.fail_stage).toBe('read');
  });

  it('解析超时先对账：服务端已 indexed 时按成功结算而非假超时', async () => {
    // 对抗回归：终态 WS 帧可被 DropNewest 丢弃——超时重拉文档状态再定夺。
    vi.useFakeTimers();
    ingestSpy.mockResolvedValue({ id: 'doc-slow', collection_id: 'col-1', status: 'indexing' });
    const up = useKnowledgeUpload(makeDeps());
    await up.enqueueUploadFiles([file]);
    expect(up.uploadTasks.value[0].status).toBe('parsing');
    documentsByCollectionRef.value = {
      'col-1': [{ id: 'doc-slow', status: 'indexed', chunk_count: 9 } as KnowledgeDocument],
    };
    await vi.advanceTimersByTimeAsync(240_000 + 10);
    const task = up.uploadTasks.value[0];
    expect(task.status).toBe('success');
    expect(task.chunk_count).toBe(9);
  });

  it('解析超时且文档仍未就绪 → 才转可重试错误', async () => {
    vi.useFakeTimers();
    const up = useKnowledgeUpload(makeDeps());
    await up.enqueueUploadFiles([file]);
    expect(up.uploadTasks.value[0].status).toBe('parsing');
    await vi.advanceTimersByTimeAsync(240_000 + 10);
    expect(up.uploadTasks.value[0].status).toBe('error');
    expect(up.uploadTasks.value[0].fail_stage).toBe('parse');
  });
});
