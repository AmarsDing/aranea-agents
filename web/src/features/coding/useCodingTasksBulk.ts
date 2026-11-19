import { computed, ref, type Ref } from 'vue';
import type { BulkAction } from '../../components/common/bulkAction';
import { i18n } from '../../i18n';
import { runBulkSequential } from '../../shared/bulkRun';
import { cancelCodingTask, isActiveStatus, isTerminalStatus, reviewCodingTask } from './api';
import type { CodingTask } from './api';

/**
 * 编码任务列表的「选择 + 批量执行」。
 * 只接 features/coding 里真实存在的接口：中止（CancelTask）、通过验收 / 归档（ReviewTask）；
 * 后端没有按 id 删除任务的 RPC，因此不提供「批量删除」。
 * 文案走全局 i18n 实例（而非 useI18n），以便该组合式可在页面 setup 之外被直接调用/测试。
 */
export function useCodingTasksBulk(rows: Ref<CodingTask[]>, reload: () => Promise<void>) {
  const t = i18n.global.t;
  const selectedIds = ref<Set<string>>(new Set());
  const busy = ref('');
  const error = ref('');

  const count = computed(() => selectedIds.value.size);
  const selectedRows = computed(() =>
    rows.value.filter((row) => Boolean(row.id) && selectedIds.value.has(row.id as string)),
  );
  /** 「本页全选」语义：只看当前列表 rows，不跨筛选累积。 */
  const pageAllSelected = computed(
    () =>
      rows.value.length > 0 &&
      rows.value.every((row) => Boolean(row.id) && selectedIds.value.has(row.id as string)),
  );

  const activeTasks = computed(() => selectedRows.value.filter((row) => isActiveStatus(row.status)));
  const terminalTasks = computed(() => selectedRows.value.filter((row) => isTerminalStatus(row.status)));

  /** 批量条动作合同：中止会杀掉工具进程，属破坏性动作，需二次确认。 */
  const actions = computed<BulkAction[]>(() => [
    {
      key: 'cancel',
      label: t('shell.bulkBar.cancel'),
      icon: 'stop_circle',
      tone: 'danger',
      requiresConfirm: true,
      confirmLabel: t('shell.bulkBar.codingCancelConfirm', { n: count.value }),
      disabled: activeTasks.value.length === 0,
      hint: activeTasks.value.length === 0 ? t('shell.bulkBar.codingCancelHint') : undefined,
    },
    {
      key: 'approve',
      label: t('shell.bulkBar.approve'),
      icon: 'check_circle',
      tone: 'primary',
      disabled: terminalTasks.value.length === 0,
      hint: terminalTasks.value.length === 0 ? t('shell.bulkBar.codingReviewHint') : undefined,
    },
    {
      key: 'archive',
      label: t('shell.bulkBar.archive'),
      icon: 'archive',
      tone: 'neutral',
      disabled: terminalTasks.value.length === 0,
      hint: terminalTasks.value.length === 0 ? t('shell.bulkBar.codingArchiveHint') : undefined,
    },
  ]);

  function isSelected(id: string) {
    return selectedIds.value.has(id);
  }

  function toggleRow(id: string, checked: boolean) {
    if (!id) return;
    const next = new Set(selectedIds.value);
    if (checked) next.add(id);
    else next.delete(id);
    selectedIds.value = next;
  }

  function toggleAll(checked: boolean) {
    const next = new Set(selectedIds.value);
    for (const row of rows.value) {
      if (!row.id) continue;
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
    cancel: (id) => cancelCodingTask(id),
    approve: (id) => reviewCodingTask(id, 'approve'),
    archive: (id) => reviewCodingTask(id, 'archive'),
  };

  async function run(key: string) {
    const invoke = runners[key];
    if (!invoke || busy.value) return;
    const ids = [...selectedIds.value].filter(Boolean);
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
