import { getCurrentScope, onScopeDispose, ref, type Ref } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import { randomUUID } from '../../utils/uuid';
import type { KnowledgeCollection, KnowledgeDocument, KnowledgeUploadTask } from './types';
import { useKnowledgeStore } from '../../stores/knowledge';
import { dirNodeKey, vaultNodeKey } from './vaultTreeUi';
import { createV2EventStream, type UseV2EventStreamOptions } from '../../realtime/useV2EventStream';
import { GLOBAL_WS_SESSION_ID } from '../../config/runtime';
import type { SystemNoticeEventPayload, V2WsEnvelope } from '../chat/v2Types';
import { inferUploadMime, isExtractSupported, readFileAsBase64, utf8ToBase64 } from './knowledgeUploadCodec';

/** G-21：解析阶段等待终态 WS 事件的超时上限（大 PDF 解析可至数分钟）。 */
export const UPLOAD_PARSE_TIMEOUT_MS = 240_000;

/** 二轮复核 F7：watcher 注册后的早对账延迟。小文档解析可在 POST 往返内完成，
 *  终态 WS 帧先于注册到达而丢失（代理缓冲/主线程卡顿）——没有它就得白等 240s。 */
export const UPLOAD_EARLY_RECONCILE_MS = 1_200;

/** G-21：可注入的 ingest 事件流工厂（测试可替换为假流）。 */
export type IngestStreamFactory = (opts: UseV2EventStreamOptions) => { connect: () => void; disconnect: () => void };

type IngestTerminal = 'indexed' | 'error';

/** docId → (taskId → 终态回调)：同 doc 多任务扇出簿记（内容哈希去重会让批量
 *  上传的多个任务落到同一 document_id，单值 Map 会互相砸掉 watcher）。 */
export type IngestWatchers = Map<
  string,
  Map<string, (status: IngestTerminal, errMsg: string, chunkCount: number) => void>
>;

/** 判定 knowledge_ingest 终态（indexing 中间态继续等待下一跳）。 */
function terminalStatus(raw: string): IngestTerminal | null {
  if (raw === 'indexed') return 'indexed';
  if (raw === 'error') return 'error';
  return null;
}

/** G-21：把 v2 envelope 路由给按 document_id 挂起的终态回调（纯函数，可测）。
 *  终态帧对同 doc 的全部注册任务扇出结算。 */
export function routeIngestEvent(envelope: V2WsEnvelope, watchers: IngestWatchers): void {
  if (envelope.kind !== 'system.notice') return;
  const payload = envelope.payload as SystemNoticeEventPayload;
  if (payload.NoticeType !== 'knowledge_ingest') return;
  const docId = String(payload.Meta?.document_id ?? '');
  const cbs = docId ? watchers.get(docId) : undefined;
  if (!cbs || !cbs.size) return;
  const term = terminalStatus(String(payload.Meta?.status ?? ''));
  if (!term) return; // indexing 中间态：保留 watcher 等下一跳
  watchers.delete(docId);
  const errMsg = String(payload.Meta?.error_message ?? '');
  const chunkCount = Number(payload.Meta?.chunk_count ?? 0) || 0;
  for (const cb of cbs.values()) cb(term, errMsg, chunkCount);
}

export type KnowledgeUploadDeps = {
  selectedId: Ref<string>;
  collections: { value: KnowledgeCollection[] };
  /** 按库重拉文档列表（F8：对账必须重拉任务目标库而非当前选中库）；省略 = 选中库。 */
  loadDocuments: (collectionId?: string) => Promise<void>;
  loadCollections: () => Promise<void>;
  friendlyError: (err: unknown) => string;
  selectTreeNode: (key: string) => Promise<void> | void;
  /** G-21：WS 事件流工厂（默认系统频道订阅 knowledge_ingest；测试注入假流）。 */
  ingestStreamFactory?: IngestStreamFactory;
};

export function useKnowledgeUpload(deps: KnowledgeUploadDeps) {
  const $q = useQuasar();
  const { t } = useI18n();
  const knowledgeStore = useKnowledgeStore();

  const ingestOpen = ref(false);
  const ingestLoading = ref(false);
  const ingestForm = ref({
    source: '',
    mime_type: 'text/plain',
    text: '',
    chunk_strategy: '',
    chunk_size: 0,
    chunk_overlap: 0,
  });
  const uploadTasks = ref<KnowledgeUploadTask[]>([]);
  const pendingUploadTarget = ref<{ collectionId: string; prefix: string } | null>(null);

  // ---------- G-21：解析阶段 WS 终态等待 ----------
  // 页生命周期内懒建单条系统频道流；docId → taskId → 终态回调（多任务扇出）。
  const ingestWatchers: IngestWatchers = new Map();
  const parseTimers = new Map<string, Array<ReturnType<typeof setTimeout>>>();
  let ingestStream: { connect: () => void; disconnect: () => void } | null = null;

  function ensureIngestStream() {
    if (ingestStream) return;
    const factory = deps.ingestStreamFactory ?? ((opts) => createV2EventStream(opts));
    ingestStream = factory({
      sessionId: GLOBAL_WS_SESSION_ID,
      channels: ['system'],
      autoConnect: false,
      onV2Event: (env) => routeIngestEvent(env, ingestWatchers),
    });
    ingestStream.connect();
  }

  // 页面无 keep-alive，卸载必须释放全局 hub 消费者与全部悬挂定时器——否则
  // 每次带上传的访问都泄漏一个订阅 + 最长 240s 的回调继续修补已死组件状态。
  if (getCurrentScope()) {
    onScopeDispose(() => {
      for (const timers of parseTimers.values()) for (const timer of timers) clearTimeout(timer);
      parseTimers.clear();
      ingestWatchers.clear();
      ingestStream?.disconnect();
      ingestStream = null;
    });
  }

  /** POST 返回后进入「解析/索引」段：等待 document 级终态事件，超时先对账再转错误。 */
  function watchDocParse(taskId: string, docId: string, collectionId?: string) {
    ensureIngestStream();
    settleWatch(taskId, docId); // 同任务重试时先清旧悬挂（不触碰其他任务的同 doc 注册）
    let cbs = ingestWatchers.get(docId);
    if (!cbs) {
      cbs = new Map();
      ingestWatchers.set(docId, cbs);
    }
    cbs.set(taskId, (status, errMsg, chunkCount) => {
      finishWatch(taskId, docId);
      if (status === 'error') {
        patchUploadTask(taskId, {
          status: 'error',
          fail_stage: 'parse',
          message: errMsg || t('knowledgePage.uploadParseFailed'),
        });
      } else {
        patchUploadTask(taskId, {
          status: 'success',
          chunk_count: chunkCount,
          message: t('knowledgePage.uploadIndexed', { n: chunkCount }),
        });
      }
    });
    parseTimers.set(taskId, [
      // F7 早对账：终态帧可先于 watcher 注册到达而永久丢失（小文档 + WAN/卡顿），
      // 注册后短延迟先对账一次——只认已 indexed 的成功，未就绪则保留 watcher。
      setTimeout(() => {
        void reconcileDocStatus(taskId, docId, collectionId, false);
      }, UPLOAD_EARLY_RECONCILE_MS),
      // 超时兜底：DropNewest 通道丢帧或服务端真的超时时，按库对账后才转错误。
      setTimeout(() => {
        void reconcileDocStatus(taskId, docId, collectionId, true);
      }, UPLOAD_PARSE_TIMEOUT_MS),
    ]);
  }

  /** 对账：WS 终态帧走 DropNewest 通道可被丢弃（服务端大文档也可能真的超过
   *  240s）——重拉任务目标库（F8：不是当前选中库），已 indexed 按成功结算；
   *  超时兜底未命中才转可重试错误。早对账未命中保持静默（watcher 继续等）。 */
  async function reconcileDocStatus(
    taskId: string,
    docId: string,
    collectionId: string | undefined,
    isTimeout: boolean,
  ) {
    if (isTimeout) finishWatch(taskId, docId);
    await deps.loadDocuments(collectionId).catch(() => undefined);
    const doc = findStoreDocument(docId);
    if (doc && doc.status === 'indexed') {
      finishWatch(taskId, docId);
      const n = doc.chunk_count ?? 0;
      patchUploadTask(taskId, {
        status: 'success',
        chunk_count: n,
        message: t('knowledgePage.uploadIndexed', { n }),
      });
      return;
    }
    if (!isTimeout) return; // 早对账未就绪：不动刀，240s 兜底继续
    patchUploadTask(taskId, {
      status: 'error',
      fail_stage: 'parse',
      message: t('knowledgePage.uploadParseTimeout'),
    });
  }

  function findStoreDocument(docId: string): KnowledgeDocument | null {
    // setup store 的 ref 在 store 实例上自动解包：这里直接拿到 Record，无 .value。
    const byCol = knowledgeStore.documentsByCollection;
    if (!byCol) return null;
    for (const list of Object.values(byCol)) {
      const hit = list.find((d) => d.id === docId);
      if (hit) return hit;
    }
    return null;
  }

  /** 终态/超时后的共同清理（摘本任务 watcher、清全部定时器）。 */
  function finishWatch(taskId: string, docId: string) {
    const cbs = ingestWatchers.get(docId);
    if (cbs) {
      cbs.delete(taskId);
      if (!cbs.size) ingestWatchers.delete(docId);
    }
    const timers = parseTimers.get(taskId);
    if (timers) {
      for (const timer of timers) clearTimeout(timer);
      parseTimers.delete(taskId);
    }
  }

  /** 清除 task 上可能悬挂的旧 watch（重试或移除任务时）。 */
  function settleWatch(taskId: string, docIdHint?: string) {
    if (docIdHint) finishWatch(taskId, docIdHint);
    const timers = parseTimers.get(taskId);
    if (timers) {
      for (const timer of timers) clearTimeout(timer);
      parseTimers.delete(taskId);
    }
    for (const docId of [...ingestWatchers.keys()]) finishWatch(taskId, docId);
  }

  function validateUploadMime(mime: string): string | null {
    if (!isExtractSupported(mime)) {
      return t('knowledgePage.uploadUnsupportedFormat', { mime });
    }
    return null;
  }

  function patchUploadTask(id: string, patch: Partial<KnowledgeUploadTask>) {
    const i = uploadTasks.value.findIndex((task) => task.id === id);
    if (i >= 0) {
      uploadTasks.value[i] = { ...uploadTasks.value[i], ...patch };
    }
  }

  async function submitIngest() {
    const text = ingestForm.value.text.trim();
    if (!text) {
      $q.notify({ type: 'warning', message: t('knowledgePage.ingestTextRequired', 'Paste the text content first') });
      return;
    }
    ingestLoading.value = true;
    try {
      const doc = await knowledgeStore.ingest({
        collection_id: deps.selectedId.value || undefined,
        source: ingestForm.value.source || 'paste',
        mime_type: ingestForm.value.mime_type || 'text/plain',
        content_base64: utf8ToBase64(text),
        chunk_strategy: ingestForm.value.chunk_strategy || undefined,
        chunk_size: ingestForm.value.chunk_size || undefined,
        chunk_overlap: ingestForm.value.chunk_overlap || undefined,
        organize_to_markdown: true,
      });
      ingestOpen.value = false;
      ingestForm.value = {
        source: '',
        mime_type: 'text/plain',
        text: '',
        chunk_strategy: '',
        chunk_size: 0,
        chunk_overlap: 0,
      };
      if (!deps.selectedId.value && doc.collection_id) {
        deps.selectedId.value = doc.collection_id;
      }
      await deps.loadDocuments(doc.collection_id || undefined);
      await deps.loadCollections();
      $q.notify({
        type: 'positive',
        message: t('knowledgePage.ingestSubmitted', 'Document submitted; parsing in the background...'),
      });
    } catch (e) {
      $q.notify({ type: 'negative', message: deps.friendlyError(e) || t('knowledgePage.uploadFailed') });
    } finally {
      ingestLoading.value = false;
    }
  }

  /** G-21 单文件执行体（入队/重试共用）：读取 → 上传 → 解析/索引（WS 终态）。 */
  async function processUploadFile(task: KnowledgeUploadTask, mime: string, targetId: string): Promise<string> {
    const invalid = validateUploadMime(mime);
    if (invalid) {
      patchUploadTask(task.id, { status: 'error', fail_stage: 'read', message: invalid });
      return '';
    }
    // fail_stage 随阶段推进：读取失败错标 upload 曾让阶段条高亮错误段。
    let failStage: 'read' | 'upload' = 'read';
    try {
      patchUploadTask(task.id, { status: 'reading' });
      if (!task.file) throw new Error(t('knowledgePage.uploadReadFailed'));
      const b64 = await readFileAsBase64(task.file).catch(() => {
        throw new Error(t('knowledgePage.uploadReadFailed'));
      });
      failStage = 'upload';
      patchUploadTask(task.id, { status: 'uploading' });
      const doc = await knowledgeStore.ingest({
        collection_id: targetId,
        source: task.name,
        mime_type: mime,
        content_base64: b64,
        organize_to_markdown: true,
        target_dir: task.target_dir || undefined,
      });
      if (!doc.id) throw new Error('ingest returned empty document id');
      // 短路：内容哈希去重命中已 indexed 文档时服务端不发新事件——立刻按终态
      // 成功结算；error 文档由服务端重跑管线并以 indexing 返回（走监听路径）。
      if (doc.status === 'indexed') {
        const n = doc.chunk_count ?? 0;
        patchUploadTask(task.id, {
          status: 'success',
          chunk_count: n,
          message: t('knowledgePage.uploadIndexed', { n }),
        });
        return doc.collection_id;
      }
      patchUploadTask(task.id, { status: 'parsing', message: '' });
      watchDocParse(task.id, doc.id, doc.collection_id || targetId || undefined);
      return doc.collection_id;
    } catch (e) {
      patchUploadTask(task.id, {
        status: 'error',
        fail_stage: failStage,
        message: deps.friendlyError(e) || t('knowledgePage.uploadFailed'),
      });
      return '';
    }
  }

  async function enqueueUploadFiles(files: File[], target?: { collectionId: string; prefix: string }) {
    if (!files.length) return;
    let targetId = deps.selectedId.value;
    let targetDir = '';
    let targetLabel: string | undefined;
    if (target) {
      const col = deps.collections.value.find((c) => c.id === target.collectionId);
      if (col) {
        targetId = col.id;
        targetDir = target.prefix || '/';
        targetLabel = target.prefix ? `${col.name} / ${target.prefix}` : col.name;
        await deps.selectTreeNode(target.prefix ? dirNodeKey(col.id, target.prefix) : vaultNodeKey(col.id));
      }
    }
    let firstIngestedCollectionId = '';
    for (const file of files) {
      const mime = inferUploadMime(file);
      const task: KnowledgeUploadTask = {
        id: randomUUID(),
        name: file.name,
        size: file.size,
        mime_type: mime,
        status: 'reading',
        collection_label: targetLabel ?? (targetId ? undefined : t('knowledgePage.defaultCollectionBadge')),
        file,
        target_collection_id: targetId,
        target_dir: targetDir,
      };
      uploadTasks.value.push(task);
      const collectionId = await processUploadFile(task, mime, targetId);
      if (!firstIngestedCollectionId && collectionId) firstIngestedCollectionId = collectionId;
    }
    await deps.loadCollections();
    if (targetId) {
      await deps.loadDocuments(targetId);
    } else if (firstIngestedCollectionId) {
      deps.selectedId.value = firstIngestedCollectionId;
    }
  }

  /** G-21 错误重传：先清悬挂 watch，再按原载荷（file + 目标库/目录）重跑全流程。 */
  async function retryUploadTask(id: string) {
    const task = uploadTasks.value.find((t) => t.id === id);
    if (!task || task.status !== 'error' || !task.file) return;
    settleWatch(id);
    patchUploadTask(id, { status: 'reading', message: '', fail_stage: undefined, chunk_count: undefined });
    await processUploadFile(task, task.mime_type, task.target_collection_id ?? '');
    await deps.loadCollections().catch(() => undefined);
    if (task.target_collection_id) await deps.loadDocuments(task.target_collection_id).catch(() => undefined);
  }

  async function onUploadFilesPicked(files: File[]) {
    const target = pendingUploadTarget.value;
    pendingUploadTarget.value = null;
    await enqueueUploadFiles(files, target ?? undefined);
  }

  function removeUploadTask(id: string) {
    settleWatch(id);
    uploadTasks.value = uploadTasks.value.filter((task) => task.id !== id);
  }

  function clearFinishedUploadTasks() {
    for (const task of uploadTasks.value) {
      if (task.status === 'success' || task.status === 'error') settleWatch(task.id);
    }
    uploadTasks.value = uploadTasks.value.filter(
      (task) => task.status === 'reading' || task.status === 'uploading' || task.status === 'parsing',
    );
  }

  return {
    ingestOpen,
    ingestLoading,
    ingestForm,
    submitIngest,
    uploadTasks,
    pendingUploadTarget,
    enqueueUploadFiles,
    onUploadFilesPicked,
    removeUploadTask,
    clearFinishedUploadTasks,
    retryUploadTask,
  };
}
