// U0-4 批量操作：移动/删除/重嵌入 的纯逻辑 + Quasar 胶水 composable。
// 纯函数（moveTargetsFor/runSequential）供组件与测试直接复用；composable 负责
// 确认框/全局 toast/知识库 store 刷新，全部走既有 store 出口（本地缓存与树监听联动）。
import { computed, type ComputedRef } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import { useKnowledgeStore } from '../../stores/knowledge';
import { parseKratosApiError } from '../../utils/kratosError';
import type { KnowledgeCollection, VaultTreeNode } from './types';

export interface MoveTargetOption {
  label: string;
  value: string;
  disable?: boolean;
  dim?: number;
}

/**
 * moveTargetsFor —— 批量移动目标库选项（与单文档移动同口径）：
 * 排除当前库；dim 与当前库不一致的禁用（后端拒绝跨检索类型移动）。
 */
export function moveTargetsFor(
  collections: readonly KnowledgeCollection[],
  currentVaultId: string,
): MoveTargetOption[] {
  const current = collections.find((c) => c.id === currentVaultId);
  return collections
    .filter((c) => c.id !== currentVaultId)
    .map((c) => ({
      label: c.name || c.id,
      value: c.id,
      disable: !!current && (c.dim !== current.dim || current.vault_backend === 'local' || c.vault_backend === 'local'),
      dim: c.dim,
    }));
}

export interface SequentialResult<T> {
  /** 全部成功项（保序） */
  ok: T[];
  /** 失败项（保序）+ 原始错误，调用方决定汇总口径 */
  failed: Array<{ item: T; error: unknown }>;
}

/**
 * runSequential —— 顺序执行批量项，单失败不中断后续（Obsidian 批量语义：最大化完成度）。
 * 返回成功/失败分明细，供汇总 toast 与“保留失败项在选择集”。
 */
export async function runSequential<T>(
  items: readonly T[],
  fn: (item: T) => Promise<unknown>,
): Promise<SequentialResult<T>> {
  const ok: T[] = [];
  const failed: Array<{ item: T; error: unknown }> = [];
  for (const item of items) {
    try {
      await fn(item);
      ok.push(item);
    } catch (error) {
      failed.push({ item, error });
    }
  }
  return { ok, failed };
}

/** 批量动作作用于可定位文档：缺 doc_id 的行节点静默过滤（防御）。 */
function docIds(nodes: readonly VaultTreeNode[]): string[] {
  return nodes.map((n) => n.doc_id).filter((id): id is string => !!id);
}

export interface BulkActionOutcome {
  okCount: number;
  /** 失败后仍保留在选择集中的 doc_id（供选择集“留下失败项、重试”）。 */
  failedIds: string[];
}

export function useBulkDocActions(input: {
  /** 当前库 id（getter 以保证响应式取值） */
  vaultId: () => string;
  /** 当前可用知识库列表（getter；moveTargetsFor 数据源） */
  collections: () => KnowledgeCollection[];
}) {
  const $q = useQuasar();
  const { t } = useI18n();
  // Pinia store 惰性解析：composable setup 期不触碰 active pinia（既有侧栏 spec 无 pinia 环境）。
  // 动作由组件事件触发时，运行环境必有 app pinia。
  const store = () => useKnowledgeStore();

  /** KnowledgeMoveDialog 的 options（仅库列表；批量移动 = 逐文档提交同一目标）。 */
  const moveOptions: ComputedRef<MoveTargetOption[]> = computed(() =>
    moveTargetsFor(input.collections(), input.vaultId()),
  );

  function friendlyError(e: unknown): string {
    return parseKratosApiError(e).message || t('knowledgePage.workbench.bulk.opFailed');
  }

  /** 统一确认框 → Promise<boolean>（QDialog 回调式转 promise，取消/关闭同窗）。 */
  function confirmDialog(title: string, message: string): Promise<boolean> {
    return new Promise((resolve) => {
      $q.dialog({ title, message, cancel: true, persistent: true })
        .onOk(() => resolve(true))
        .onCancel(() => resolve(false))
        .onDismiss(() => resolve(false));
    });
  }

  /** 汇总 toast：全成功 positive；部分失败 negative 列计数。 */
  function notifyOutcome(
    res: { ok: unknown[]; failed: unknown[] },
    okKey: string,
    partialKey: string,
    extra?: Record<string, unknown>,
  ) {
    if (!res.failed.length) {
      $q.notify({ type: 'positive', message: t(okKey, { ok: res.ok.length, ...extra }) });
    } else if (res.ok.length) {
      $q.notify({
        type: 'warning',
        message: t(partialKey, { ok: res.ok.length, failed: res.failed.length, ...extra }),
      });
    } else {
      $q.notify({ type: 'negative', message: t(partialKey, { ok: 0, failed: res.failed.length, ...extra }) });
    }
  }

  /**
   * 批量删除：确认框列计数 → 逐个删除（失败不中断）→ 汇总 toast；
   * store.removeDocument 已同步本地文档缓存与树失效，页面列表由既有 watch 联动刷新。
   */
  async function deleteSelected(nodes: readonly VaultTreeNode[]): Promise<BulkActionOutcome | null> {
    const ids = docIds(nodes);
    if (!ids.length) return null;
    const yes = await confirmDialog(
      t('knowledgePage.workbench.bulk.deleteTitle'),
      t('knowledgePage.workbench.bulk.deleteMessage', { n: ids.length }),
    );
    if (!yes) return null;
    try {
      const res = await runSequential(ids, (id) => store().removeDocument(id, input.vaultId()));
      notifyOutcome(res, 'knowledgePage.workbench.bulk.deleteDone', 'knowledgePage.workbench.bulk.deletePartial');
      if (res.ok.length) void store().loadCollections({});
      if (res.failed.length) console.warn('[bulk] delete failed docs:', res.failed.map((f) => f.item).join(','));
      return { okCount: res.ok.length, failedIds: res.failed.map((f) => f.item) };
    } catch (e) {
      // 防御：循环外异常（如网络层抛错于 confirm 后）
      $q.notify({ type: 'negative', message: friendlyError(e) });
      return { okCount: 0, failedIds: ids };
    }
  }

  /**
   * 批量移动：KnowledgeMoveDialog 仅支持单文档 ⇒ 逐文档循环提交同一目标库并汇总；
   * 成功后按页面单文档移动同口径刷新文档列表与库计数。
   */
  async function moveSelected(nodes: readonly VaultTreeNode[], targetId: string): Promise<BulkActionOutcome | null> {
    const ids = docIds(nodes);
    if (!ids.length || !targetId) return null;
    try {
      const res = await runSequential(ids, (id) => store().moveDoc(id, targetId));
      const target = input.collections().find((c) => c.id === targetId);
      const name = target?.name ?? targetId;
      if (res.failed.length) console.warn('[bulk] move failed docs:', res.failed.map((f) => f.item).join(','));
      notifyOutcome(res, 'knowledgePage.workbench.bulk.moveDone', 'knowledgePage.workbench.bulk.movePartial', { name });
      if (res.ok.length) {
        await store().loadDocuments(input.vaultId(), {});
        void store().loadCollections({});
      }
      return { okCount: res.ok.length, failedIds: res.failed.map((f) => f.item) };
    } catch (e) {
      $q.notify({ type: 'negative', message: friendlyError(e) });
      return { okCount: 0, failedIds: ids };
    }
  }

  /**
   * 批量重嵌入：复用既有批量受理接口（docIds 数组）+ 既有确认框/toast 文案；
   * 受理后文档状态由摄取 WS 实时刷新（与单文档入口同口径）。
   */
  function reembedSelected(nodes: readonly VaultTreeNode[]): void {
    const ids = docIds(nodes);
    const vaultId = input.vaultId();
    if (!ids.length || !vaultId) return;
    void confirmDialog(
      t('knowledgePage.reembedConfirmTitle'),
      t('knowledgePage.reembedConfirmBody', { n: ids.length }),
    ).then((yes) => {
      if (!yes) return;
      store()
        .reembedDocuments(vaultId, ids)
        .then((r) => {
          $q.notify({ type: 'positive', message: t('knowledgePage.reembedAccepted', { n: r.accepted_count }) });
        })
        .catch((e) => $q.notify({ type: 'negative', message: friendlyError(e) }));
    });
  }

  return { moveOptions, deleteSelected, moveSelected, reembedSelected };
}
