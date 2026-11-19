import { computed, ref, type Ref } from 'vue';
import type { BulkAction } from '../../components/common/bulkAction';
import { i18n } from '../../i18n';
import { runBulkSequential } from '../../shared/bulkRun';
import { archiveSession, deleteSession, restoreSession } from './api';
import type { Session } from './types';

/**
 * 会话列表的「选择 + 批量执行」。
 * 归档/取消归档/删除均逐项调用 api.ts 里已有的单条接口——后端没有按 id 列表的批量 RPC，
 * 这里不伪造批量接口，只在前端聚合结果。
 * 文案走全局 i18n 实例（而非 useI18n），以便该组合式可在页面 setup 之外被直接调用/测试。
 */
export function useSessionsBulk(rows: Ref<Session[]>, reload: () => Promise<void>) {
  const t = i18n.global.t;
  const selectedIds = ref<Set<string>>(new Set());
  /** 正在执行的批量动作键，供批量条做按钮级 loading。 */
  const busy = ref('');
  /** 批量动作失败信息，就地展示在批量条内（不退化为 toast）。 */
  const error = ref('');

  const count = computed(() => selectedIds.value.size);
  /** 「本页全选」语义：只看当前页 rows，不跨页累积。 */
  const pageAllSelected = computed(
    () => rows.value.length > 0 && rows.value.every((row) => selectedIds.value.has(row.id)),
  );

  /** 批量条动作合同：删除为破坏性操作，需二次确认。 */
  const actions = computed<BulkAction[]>(() => [
    { key: 'archive', label: t('shell.bulkBar.archive'), icon: 'archive', tone: 'primary' },
    { key: 'restore', label: t('shell.bulkBar.restore'), icon: 'unarchive', tone: 'warn' },
    {
      key: 'delete',
      label: t('shell.bulkBar.delete'),
      icon: 'delete',
      tone: 'danger',
      requiresConfirm: true,
      confirmLabel: t('shell.bulkBar.sessionsDeleteConfirm', { n: count.value }),
    },
  ]);

  function isSelected(id: string) {
    return selectedIds.value.has(id);
  }

  function toggleRow(id: string, checked: boolean) {
    const next = new Set(selectedIds.value);
    if (checked) next.add(id);
    else next.delete(id);
    selectedIds.value = next;
  }

  function toggleAll(checked: boolean) {
    const next = new Set(selectedIds.value);
    for (const row of rows.value) {
      if (checked) next.add(row.id);
      else next.delete(row.id);
    }
    selectedIds.value = next;
  }

  function clear() {
    error.value = '';
    selectedIds.value = new Set();
  }

  const runners: Record<string, (id: string) => Promise<unknown>> = {
    archive: (id) => archiveSession(id),
    restore: (id) => restoreSession(id),
    delete: (id) => deleteSession(id),
  };

  async function run(key: string) {
    const invoke = runners[key];
    if (!invoke || busy.value) return;
    const ids = [...selectedIds.value];
    if (ids.length === 0) return;
    busy.value = key;
    error.value = '';
    try {
      const report = await runBulkSequential(ids, invoke);
      if (report.failures.length > 0) {
        // 部分失败：保留选择让用户重试，失败原因就地展示。
        error.value = t('shell.bulkBar.failure', {
          failed: report.failures.length,
          total: ids.length,
          reason: report.failures[0]?.message ?? '',
        });
      } else {
        selectedIds.value = new Set();
      }
      await reload();
    } catch (err) {
      error.value = err instanceof Error ? err.message : t('shell.bulkBar.failed');
    } finally {
      busy.value = '';
    }
  }

  return {
    selectedIds,
    count,
    pageAllSelected,
    busy,
    error,
    actions,
    isSelected,
    toggleRow,
    toggleAll,
    clear,
    run,
  };
}
