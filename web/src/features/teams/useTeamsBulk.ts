import { computed, ref, type Ref } from 'vue';
import type { BulkAction } from '../../components/common/bulkAction';
import { isValidStatusTransition } from '../../components/teams/teamConstants';
import { i18n } from '../../i18n';
import { runBulkSequential } from '../../shared/bulkRun';
import { deleteTeam, retryTeam, archiveTeam } from './api';
import type { Team } from './types';

/**
 * Team 列表的「选择 + 批量执行」。
 * 只接 teams/api.ts 里真实存在的接口：重试（RetryTeam）、归档（ArchiveTeam 状态机）、删除（DeleteTeam）；
 * 后端没有 enable/disable 语义（Team 状态是运行状态机，不是启用开关），故不造该动作。
 * 文案走全局 i18n 实例（而非 useI18n），以便该组合式可在页面 setup 之外被直接调用/测试。
 */
export function useTeamsBulk(rows: Ref<Team[]>, reload: () => Promise<void>) {
  const t = i18n.global.t;
  const selectedIds = ref<Set<string>>(new Set());
  const busy = ref('');
  const error = ref('');

  const count = computed(() => selectedIds.value.size);
  const selectedRows = computed(() => rows.value.filter((row) => selectedIds.value.has(row.id)));
  /** 「本页全选」语义：只看当前页 rows，不跨页累积。 */
  const pageAllSelected = computed(
    () => rows.value.length > 0 && rows.value.every((row) => selectedIds.value.has(row.id)),
  );

  const retryable = computed(() =>
    selectedRows.value.filter((row) => !row.readonly && ['failed', 'cancelled'].includes(row.status)),
  );
  const archivable = computed(() =>
    selectedRows.value.filter((row) => !row.readonly && isValidStatusTransition(row.status, 'archived')),
  );
  const removable = computed(() => selectedRows.value.filter((row) => !row.readonly));

  /** 批量条动作合同：删除为破坏性操作，需二次确认。 */
  const actions = computed<BulkAction[]>(() => [
    {
      key: 'retry',
      label: t('shell.common.retry'),
      icon: 'replay',
      tone: 'warn',
      disabled: retryable.value.length === 0,
      hint: retryable.value.length === 0 ? t('shell.bulkBar.teamsRetryHint') : undefined,
    },
    {
      key: 'archive',
      label: t('shell.bulkBar.archive'),
      icon: 'archive',
      tone: 'neutral',
      disabled: archivable.value.length === 0,
      hint: archivable.value.length === 0 ? t('shell.bulkBar.teamsArchiveHint') : undefined,
    },
    {
      key: 'delete',
      label: t('shell.bulkBar.delete'),
      icon: 'delete',
      tone: 'danger',
      requiresConfirm: true,
      confirmLabel: t('shell.bulkBar.teamsDeleteConfirm', { n: count.value }),
      disabled: removable.value.length === 0,
      hint: removable.value.length === 0 ? t('shell.bulkBar.teamsDeleteHint') : undefined,
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
    retry: (id) => retryTeam(id),
    archive: (id) => archiveTeam(id),
    delete: (id) => deleteTeam(id),
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
