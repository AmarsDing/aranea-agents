<template>
  <q-page class="app-standard-page app-registry-page approvals-page">
    <AppPageHero
      :kicker="t('approvalsPage.kicker')"
      :title="t('approvalsPage.title')"
      :subtitle="t('approvalsPage.subtitle')"
    >
      <template #actions>
        <q-btn
          flat
          rounded
          no-caps
          icon="refresh"
          :label="t('approvalsPage.btnRefresh')"
          :loading="loading"
          @click="reloadAll"
        />
      </template>
    </AppPageHero>

    <q-tabs
      v-model="tab"
      dense
      no-caps
      active-color="primary"
      indicator-color="primary"
      align="left"
      class="approvals-page__tabs"
    >
      <q-tab name="queue" :label="t('approvalsPage.tabQueue')" />
      <q-tab v-if="canPolicy" name="matrix" :label="t('approvalsPage.tabMatrix')" />
    </q-tabs>

    <q-tab-panels v-model="tab" animated class="approvals-page__panels">
      <!-- ============ 待办队列（列表 + 证据轨 + 常驻裁决坞） ============ -->
      <q-tab-panel name="queue" class="q-pa-none">
        <AppPageToolbar>
          <q-select
            v-model="kindFilter"
            class="app-page-toolbar__search approvals-kind-filter"
            dense
            outlined
            emit-value
            map-options
            :options="kindOptions"
            :label="t('approvalsPage.kindFilterLabel')"
          />
        </AppPageToolbar>

        <AppStatePanel
          v-if="pageState === 'error'"
          class="q-mb-md"
          variant="error"
          :description="error"
          :action-label="t('approvalsPage.retry')"
          action-icon="refresh"
          @action="loadList"
        />
        <AppStatePanel v-else-if="pageState === 'forbidden'" class="q-mb-md" variant="forbidden" />
        <AppStatePanel v-else-if="pageState === 'loading'" variant="loading" :rows="4" />
        <AppStatePanel v-else-if="pageState === 'empty'" variant="empty" :title="t('approvalsPage.emptyQueue')" />

        <template v-else>
          <AppStatePanel
            v-if="pageState === 'partial'"
            class="q-mb-md"
            variant="partial"
            dense
            :description="error"
            :action-label="t('approvalsPage.retry')"
            action-icon="refresh"
            @action="loadList"
          />

        <div
          class="approvals-workbench"
          tabindex="0"
          :aria-keyshortcuts="t('shell.registry.approvalShortcuts')"
          @keydown="handleQueueKeydown"
        >
          <div class="approvals-workbench__hint">{{ t('shell.registry.approvalShortcuts') }}</div>

          <section class="approvals-queue__list" :aria-label="t('approvalsPage.tabQueue')">
            <div class="approvals-list__head">
              <q-checkbox
                :model-value="allVisibleChecked"
                :indeterminate="someVisibleChecked"
                :disable="checkableCount === 0"
                dense
                @update:model-value="onSelectAllToggle"
              >
                <span class="approvals-list__head-label">
                  {{
                    selectedKeys.length
                      ? t('shell.approvalDock.selectedCount', { n: selectedKeys.length })
                      : t('approvalsPage.tabQueue')
                  }}
                </span>
              </q-checkbox>
              <span class="approvals-list__head-total mono">{{ total }}</span>
            </div>

            <q-list separator class="approvals-list">
              <q-item
                v-for="it in items"
                :key="approvalItemKey(it)"
                v-ripple
                clickable
                class="approvals-list__item"
                :class="{ 'approvals-list__item--checked': isChecked(it) }"
                :active="isSelected(it)"
                active-class="approvals-list__item--active"
                @click="select(it)"
              >
                <q-item-section side top class="approvals-list__check">
                  <q-checkbox
                    :model-value="isChecked(it)"
                    :disable="!canDecideApproval(it.kind)"
                    :aria-label="t('shell.approvalDock.selectedCount', { n: 1 })"
                    dense
                    @update:model-value="toggleSelect(it)"
                    @click.stop
                  />
                </q-item-section>
                <q-item-section>
                  <q-item-label class="approvals-list__title">
                    <q-badge
                      v-if="it.risk_level"
                      outline
                      :color="riskColor(it.risk_level)"
                      :label="riskLabel(it.risk_level)"
                    />
                    <span class="approvals-list__title-text">{{ it.title || it.id }}</span>
                  </q-item-label>
                  <q-item-label v-if="it.summary" caption lines="1">{{ it.summary }}</q-item-label>
                  <q-item-label caption class="mono">
                    {{ t(`approvalsPage.kind_${it.kind}`) }} · {{ it.requester || '—' }} · {{ fmtTime(it.created_at) }}
                  </q-item-label>
                  <q-item-label
                    v-if="pendingOutcome(it)"
                    caption
                    class="approvals-list__outcome mono"
                    :data-status="pendingOutcome(it)?.status"
                  >
                    {{ outcomeLabel(pendingOutcome(it)!) }}
                  </q-item-label>
                </q-item-section>
                <q-item-section side>
                  <q-icon :name="isChecked(it) ? 'task_alt' : 'chevron_right'" :color="isChecked(it) ? 'primary' : undefined" />
                </q-item-section>
              </q-item>
            </q-list>
            <AppRegistryPagination
              v-model:page="page"
              v-model:page-size="pageSize"
              :page-max="pageMax"
              :total="total"
              :loading="loading"
              :label="t('approvalsPage.paginationLabel')"
            />
          </section>

          <section class="approvals-queue__detail" :aria-label="t('shell.approvalEvidence.detailTitle')">
            <EvidenceRail :item="selected" :backlog="selectedBacklog" />
          </section>

          <DecisionDock
            ref="dockRef"
            class="approvals-workbench__dock"
            :item="selected"
            :note="note"
            :deciding="deciding"
            :selected-count="selectedKeys.length"
            :batch-running="batchRunning"
            :batch-done="batchDone"
            :batch-total="batchTotal"
            :outcomes="outcomes"
            @update:note="note = $event"
            @decide="onDockDecide"
            @batch="batchDecide"
            @clear-selection="clearSelection"
            @next="selectNext"
            @retry-failed="retryFailed"
            @dismiss-results="dismissOutcomes"
          />
        </div>
        </template>
      </q-tab-panel>

      <!-- ============ 风险矩阵（A-4；approval.policy 门） ============ -->
      <q-tab-panel v-if="canPolicy" name="matrix" class="q-pa-none">
        <p class="approvals-matrix__hint">{{ t('approvalsPage.matrixHint') }}</p>

        <AppStatePanel
          v-if="matrixError"
          class="q-mb-md"
          variant="error"
          :description="matrixError"
          :action-label="t('approvalsPage.retry')"
          action-icon="refresh"
          @action="loadMatrix"
        />
        <AppStatePanel v-else-if="matrixLoading && matrixRows.length === 0" variant="loading" :rows="3" />
        <AppStatePanel
          v-else-if="matrixRows.length === 0"
          variant="empty"
          :title="t('approvalsPage.matrixEmpty')"
        />

        <div v-else class="app-registry-table-shell">
          <q-table
            :rows="matrixRows"
            :columns="matrixColumns"
            row-key="matcher_key"
            flat
            :loading="matrixLoading"
            :pagination="{ rowsPerPage: 0 }"
            hide-pagination
          >
            <template #body-cell-matcher_key="props">
              <q-td :props="props">
                <q-input
                  v-model="props.row.matcher_key"
                  dense
                  borderless
                  class="mono"
                  :placeholder="t('approvalsPage.colMatcher')"
                  @update:model-value="markMatrixDirty"
                />
              </q-td>
            </template>
            <template #body-cell-risk="props">
              <q-td :props="props">
                <q-select
                  v-model="props.row.risk"
                  dense
                  borderless
                  emit-value
                  map-options
                  :options="riskOptions"
                  @update:model-value="markMatrixDirty"
                />
              </q-td>
            </template>
            <template #body-cell-default_mode="props">
              <q-td :props="props">
                <q-select
                  v-model="props.row.default_mode"
                  dense
                  borderless
                  emit-value
                  map-options
                  :options="modeOptions"
                  @update:model-value="markMatrixDirty"
                />
              </q-td>
            </template>
            <template #body-cell-auto_approve_times="props">
              <q-td :props="props">
                <q-input
                  v-model.number="props.row.auto_approve_times"
                  dense
                  borderless
                  type="number"
                  min="0"
                  class="mono"
                  @update:model-value="markMatrixDirty"
                />
              </q-td>
            </template>
            <template #body-cell-updated_by="props">
              <q-td :props="props" class="mono">{{ props.row.updated_by || '—' }}</q-td>
            </template>
            <template #body-cell-updated_at="props">
              <q-td :props="props" class="mono">{{ fmtTime(props.row.updated_at) }}</q-td>
            </template>
            <template #body-cell-actions="props">
              <q-td :props="props" class="text-right">
                <q-btn
                  flat
                  dense
                  round
                  icon="delete"
                  color="negative"
                  :aria-label="t('approvalsPage.btnDeleteRow')"
                  @click="removeMatrixRow(props.rowIndex)"
                />
              </q-td>
            </template>
          </q-table>
        </div>

        <div class="approvals-matrix__actions">
          <q-btn flat rounded no-caps icon="add" :label="t('approvalsPage.btnAddRow')" @click="addMatrixRow" />
          <q-btn
            unelevated
            rounded
            color="primary"
            no-caps
            icon="save"
            :label="t('approvalsPage.btnSaveMatrix')"
            :disable="!matrixDirty"
            :loading="matrixSaving"
            @click="saveMatrix"
          />
        </div>
      </q-tab-panel>
    </q-tab-panels>
  </q-page>
</template>

<script setup lang="ts">
/**
 * /approvals 统一审批中心（E 阶段 W1b，需求文档 §5.3）。
 * 列表 + EvidenceRail（证据与影响面）+ DecisionDock（常驻裁决坞）三区：
 * 单条连裁、批量勾选裁决、409 冲突与失败逐条呈现；决议出口走各领域 RPC
 * （见 features/approval/api.ts decideApproval）。风险矩阵 tab 仅
 * approval.policy 持有者可见（展示层过滤；后端 PutRiskMatrix 硬判同码）。
 */
import { computed, nextTick, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import { resolvePageState } from './pageState';
import DecisionDock from '../components/approval/DecisionDock.vue';
import EvidenceRail from '../components/approval/EvidenceRail.vue';
import {
  resolveApprovalShortcut,
  useApprovalsPage,
  type DecisionOutcome,
} from '../features/approval/useApprovalsPage';
import {
  APPROVAL_KINDS,
  MATRIX_MODES,
  RISK_LEVELS,
  approvalItemKey,
  canDecideApproval,
  type ApprovalItemRow,
  type ApprovalKindKey,
  type DecideAction,
  type RiskLevelKey,
} from '../features/approval/types';

const { t } = useI18n();
const {
  tab,
  canPolicy,
  kindFilter,
  items,
  total,
  page,
  pageSize,
  pageMax,
  loading,
  error,
  forbidden,
  summaryCounts,
  summaryTotal,
  selected,
  note,
  deciding,
  selectedKeys,
  outcomes,
  batchRunning,
  batchDone,
  batchTotal,
  allVisibleChecked,
  someVisibleChecked,
  isChecked,
  toggleSelect,
  selectAllVisible,
  clearSelection,
  dismissOutcomes,
  batchDecide,
  retryFailed,
  loadList,
  select,
  selectNext,
  selectPrevious,
  decide,
  matrixRows,
  matrixLoading,
  matrixSaving,
  matrixError,
  matrixDirty,
  loadMatrix,
  addMatrixRow,
  removeMatrixRow,
  markMatrixDirty,
  saveMatrix,
} = useApprovalsPage();

function reloadAll() {
  void loadList();
}

const dockRef = ref<{ focusNote: () => void } | null>(null);

/** 五态判定：错误优先于空态，有数据时失败降级为 partial 横幅。 */
const pageState = computed(() =>
  resolvePageState({
    loading: loading.value,
    error: error.value,
    count: items.value.length,
    forbidden: forbidden.value,
  }),
);

const checkableCount = computed(() => items.value.filter((it) => canDecideApproval(it.kind)).length);
const selectedBacklog = computed(() => (selected.value ? (summaryCounts.value[selected.value.kind] ?? 0) : 0));

function onSelectAllToggle(value: boolean | null) {
  if (value) selectAllVisible();
  else clearSelection();
}

function onDockDecide(action: DecideAction) {
  if (action === 'reject' && !note.value.trim()) void nextTick(() => dockRef.value?.focusNote());
  void decide(action);
}

function pendingOutcome(item: ApprovalItemRow): DecisionOutcome | undefined {
  return outcomes.value.find(
    (entry) => entry.key === approvalItemKey(item) && entry.status !== 'done' && entry.status !== 'pending',
  );
}

function outcomeLabel(entry: DecisionOutcome): string {
  if (entry.status === 'processing') return t('shell.approvalDock.submitting');
  if (entry.status === 'conflict') return t('approvalsPage.notifyConflict');
  if (entry.status === 'failed') return entry.message || t('shell.approvalDock.failedTitle');
  return '';
}

function handleQueueKeydown(event: KeyboardEvent) {
  const action = resolveApprovalShortcut(event);
  if (!action) return;
  if (action === 'selectAll') {
    event.preventDefault();
    selectAllVisible();
    return;
  }
  if (action === 'toggleSelect') {
    if (!selected.value) return;
    event.preventDefault();
    toggleSelect(selected.value);
    return;
  }
  if (action === 'clearSelection') {
    if (!selectedKeys.value.length) return;
    event.preventDefault();
    clearSelection();
    return;
  }
  if (!selected.value) return;
  event.preventDefault();
  if (action === 'next') selectNext();
  else if (action === 'previous') selectPrevious();
  else {
    if (action === 'reject' && !note.value.trim()) void nextTick(() => dockRef.value?.focusNote());
    void decide(action);
  }
}

const kindOptions = computed(() => [
  { value: '' as const, label: `${t('approvalsPage.kindAll')} · ${summaryTotal.value}` },
  ...APPROVAL_KINDS.map((k) => ({
    value: k as ApprovalKindKey | '',
    label: `${t(`approvalsPage.kind_${k}`)} · ${summaryCounts.value[k] ?? 0}`,
  })),
]);

const riskOptions = computed(() => RISK_LEVELS.map((r) => ({ value: r, label: t(`approvalsPage.risk_${r}`) })));
const modeOptions = computed(() => MATRIX_MODES.map((m) => ({ value: m, label: t(`approvalsPage.mode_${m}`) })));

const matrixColumns = computed(() => [
  { name: 'matcher_key', label: t('approvalsPage.colMatcher'), field: 'matcher_key', align: 'left' as const },
  { name: 'risk', label: t('approvalsPage.colRisk'), field: 'risk', align: 'left' as const },
  { name: 'default_mode', label: t('approvalsPage.colMode'), field: 'default_mode', align: 'left' as const },
  {
    name: 'auto_approve_times',
    label: t('approvalsPage.colAutoTimes'),
    field: 'auto_approve_times',
    align: 'right' as const,
  },
  { name: 'updated_by', label: t('approvalsPage.colUpdatedBy'), field: 'updated_by', align: 'left' as const },
  { name: 'updated_at', label: t('approvalsPage.colUpdatedAt'), field: 'updated_at', align: 'left' as const },
  { name: 'actions', label: t('approvalsPage.colActions'), field: 'actions', align: 'right' as const },
]);

function isSelected(it: ApprovalItemRow): boolean {
  return !!selected.value && selected.value.kind === it.kind && selected.value.id === it.id;
}

function riskColor(risk: RiskLevelKey | ''): string {
  switch (risk) {
    case 'high':
      return 'negative';
    case 'medium':
      return 'warning';
    case 'low':
      return 'positive';
    default:
      return 'grey';
  }
}

function riskLabel(risk: RiskLevelKey | ''): string {
  return risk ? t(`approvalsPage.risk_${risk}`) : '—';
}

function fmtTime(iso: string): string {
  if (!iso) return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}
</script>

<style scoped lang="sass">
// 令牌只取 --nt-*（AGENTS.md §6）；数字/ID/时间一律等宽 + tabular-nums。
.mono
  font-family: var(--nt-font-mono)
  font-variant-numeric: tabular-nums
:deep(.mono)
  font-family: var(--nt-font-mono)
  font-variant-numeric: tabular-nums

.approvals-page__tabs
  margin-bottom: var(--nt-space-3)

.approvals-kind-filter
  min-width: 240px

.approvals-workbench
  display: grid
  grid-template-columns: minmax(320px, 420px) minmax(0, 1fr)
  gap: var(--nt-space-4)
  align-items: start
  padding-bottom: var(--nt-space-3)

.approvals-workbench__hint
  grid-column: 1 / -1
  color: var(--nt-text-2)
  font-family: var(--nt-font-mono)
  font-size: var(--nt-fs-caption)

.approvals-workbench__dock
  grid-column: 1 / -1
  position: sticky
  bottom: 8px
  z-index: 5

.approvals-queue__list
  display: flex
  flex-direction: column
  gap: var(--nt-space-2)
  min-width: 0
  padding: var(--nt-space-3)
  border: 1px solid var(--nt-border-1)
  border-radius: var(--nt-radius-l)
  background: var(--nt-bg-1)

.approvals-queue__detail
  min-width: 0

.approvals-list__head
  display: flex
  align-items: center
  justify-content: space-between
  gap: var(--nt-space-2)
  padding: 0 var(--nt-space-1) var(--nt-space-2)
  border-bottom: 1px solid var(--nt-border-1)

.approvals-list__head-label
  font-size: var(--nt-fs-panel)
  color: var(--nt-text-1)

.approvals-list__head-total
  color: var(--nt-text-2)
  font-size: var(--nt-fs-caption)

.approvals-list__item
  min-height: 56px

.approvals-list__item--checked
  background: color-mix(in srgb, var(--nt-accent) 7%, transparent)

.approvals-list__check
  padding-right: var(--nt-space-1)

.approvals-list__title
  display: flex
  align-items: center
  gap: var(--nt-space-2)

.approvals-list__title-text
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap

.approvals-list__item--active
  background: var(--nt-bg-2)

.approvals-list__outcome[data-status='processing']
  color: var(--nt-info)

.approvals-list__outcome[data-status='conflict']
  color: var(--nt-warn)

.approvals-list__outcome[data-status='failed']
  color: var(--nt-danger)

.approvals-list__skeleton
  height: 56px
  border-radius: 8px

.approvals-queue__empty
  padding: var(--nt-space-8) var(--nt-space-4)
  text-align: center
  color: var(--nt-text-2)

.approvals-matrix__hint
  margin: 0 0 var(--nt-space-3)
  color: var(--nt-text-2)

.approvals-matrix__actions
  display: flex
  justify-content: space-between
  margin-top: var(--nt-space-3)

.approvals-matrix__empty
  color: var(--nt-text-2)

// 断点 1099：列表与证据轨收敛为单栏（规范断点 1200/1099/799/559/359）。
@media (max-width: 1099px)
  .approvals-workbench
    grid-template-columns: minmax(0, 1fr)

@media (max-width: 799px)
  .approvals-queue__list
    padding: var(--nt-space-2)
</style>
