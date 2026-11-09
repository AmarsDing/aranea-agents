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
      <!-- ============ 待办队列（左列表右详情） ============ -->
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

        <q-banner v-if="error" rounded class="app-page-error-banner q-mb-md" role="alert">
          {{ error }}
          <template #action>
            <q-btn flat dense :label="t('approvalsPage.retry')" @click="loadList" />
          </template>
        </q-banner>

        <div class="approvals-queue">
          <section class="approvals-queue__list" :aria-label="t('approvalsPage.tabQueue')">
            <q-list separator class="approvals-list">
              <q-item
                v-for="it in items"
                :key="`${it.kind}:${it.id}`"
                v-ripple
                clickable
                class="approvals-list__item"
                :active="isSelected(it)"
                active-class="approvals-list__item--active"
                @click="select(it)"
              >
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
                </q-item-section>
                <q-item-section side>
                  <q-icon name="chevron_right" />
                </q-item-section>
              </q-item>
            </q-list>
            <div v-if="!loading && items.length === 0" class="approvals-queue__empty">
              {{ t('approvalsPage.emptyQueue') }}
            </div>
            <div v-if="loading" class="q-pa-md">
              <q-skeleton v-for="i in 3" :key="i" type="rect" class="q-mb-sm approvals-list__skeleton" />
            </div>
            <AppRegistryPagination
              v-model:page="page"
              v-model:page-size="pageSize"
              :page-max="pageMax"
              :total="total"
              :loading="loading"
              :label="t('approvalsPage.paginationLabel')"
            />
          </section>

          <section class="approvals-queue__detail" :aria-label="t('approvalsPage.detailTitle')">
            <q-card v-if="selected" flat bordered class="approvals-detail">
              <q-card-section>
                <div class="approvals-detail__badges">
                  <q-badge outline color="primary" :label="t(`approvalsPage.kind_${selected.kind}`)" />
                  <q-badge
                    v-if="selected.risk_level"
                    :color="riskColor(selected.risk_level)"
                    :label="riskLabel(selected.risk_level)"
                  />
                </div>
                <div class="approvals-detail__title">{{ selected.title || selected.id }}</div>
              </q-card-section>
              <q-separator />
              <q-card-section class="approvals-detail__meta">
                <div>
                  <span class="approvals-detail__label">{{ t('approvalsPage.labelRequester') }}</span>
                  <span>{{ selected.requester || '—' }}</span>
                </div>
                <div>
                  <span class="approvals-detail__label">{{ t('approvalsPage.labelCreated') }}</span>
                  <span class="mono">{{ fmtTime(selected.created_at) }}</span>
                </div>
                <div>
                  <span class="approvals-detail__label">{{ t('approvalsPage.labelId') }}</span>
                  <span class="mono">{{ selected.id }}</span>
                </div>
              </q-card-section>
              <q-card-section v-if="selected.summary">
                <div class="approvals-detail__label">{{ t('approvalsPage.labelSummary') }}</div>
                <p class="approvals-detail__summary">{{ selected.summary }}</p>
              </q-card-section>
              <q-card-section v-if="contextEntries.length">
                <div class="approvals-detail__label">{{ t('approvalsPage.labelContext') }}</div>
                <dl class="approvals-detail__ctx">
                  <template v-for="e in contextEntries" :key="e.key">
                    <dt class="mono">{{ e.key }}</dt>
                    <dd class="mono">{{ e.value }}</dd>
                  </template>
                </dl>
              </q-card-section>
              <q-card-section v-else-if="selected.context_raw">
                <div class="approvals-detail__label">{{ t('approvalsPage.labelContext') }}</div>
                <pre class="approvals-detail__raw mono">{{ selected.context_raw }}</pre>
              </q-card-section>
              <q-separator />
              <q-card-section>
                <q-input
                  v-model="note"
                  type="textarea"
                  autogrow
                  outlined
                  dense
                  :label="t('approvalsPage.notePlaceholder')"
                />
              </q-card-section>
              <q-card-actions align="right" class="q-px-md q-pb-md">
                <q-btn
                  v-if="selected.kind === 'task_review'"
                  flat
                  no-caps
                  icon="archive"
                  :label="t('approvalsPage.btnArchive')"
                  :disable="deciding"
                  @click="decide('archive')"
                />
                <q-btn
                  outline
                  color="negative"
                  no-caps
                  icon="close"
                  :label="t('approvalsPage.btnReject')"
                  :disable="deciding"
                  @click="decide('reject')"
                />
                <q-btn
                  unelevated
                  color="primary"
                  no-caps
                  icon="check"
                  :label="t('approvalsPage.btnApprove')"
                  :loading="deciding"
                  @click="decide('approve')"
                />
              </q-card-actions>
            </q-card>
            <div v-else class="approvals-queue__empty-detail">
              <q-icon name="approval" size="48px" />
              <div>{{ t('approvalsPage.emptyDetail') }}</div>
            </div>
          </section>
        </div>
      </q-tab-panel>

      <!-- ============ 风险矩阵（A-4；approval.policy 门） ============ -->
      <q-tab-panel v-if="canPolicy" name="matrix" class="q-pa-none">
        <p class="approvals-matrix__hint">{{ t('approvalsPage.matrixHint') }}</p>

        <q-banner v-if="matrixError" rounded class="app-page-error-banner q-mb-md" role="alert">
          {{ matrixError }}
          <template #action>
            <q-btn flat dense :label="t('approvalsPage.retry')" @click="loadMatrix" />
          </template>
        </q-banner>

        <div class="app-registry-table-shell">
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
            <template #no-data>
              <div class="full-width text-center q-pa-lg approvals-matrix__empty">
                {{ t('approvalsPage.matrixEmpty') }}
              </div>
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
 * 左列表右详情两栏 + kind 分组 + 风险级标识；决议出口走各领域 RPC
 * （见 features/approval/api.ts decideApproval），并发决议 409 呈现为
 * 「已被处理」提示并刷新。风险矩阵 tab 仅 approval.policy 持有者可见
 * （展示层过滤；后端 PutRiskMatrix 硬判同码）。
 */
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import { useApprovalsPage } from '../features/approval/useApprovalsPage';
import {
  APPROVAL_KINDS,
  MATRIX_MODES,
  RISK_LEVELS,
  type ApprovalItemRow,
  type ApprovalKindKey,
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
  summaryCounts,
  summaryTotal,
  selected,
  note,
  deciding,
  loadList,
  select,
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

const contextEntries = computed(() => {
  const c = selected.value?.context;
  if (!c) return [];
  return Object.entries(c).map(([key, v]) => ({
    key,
    value: v === null || v === undefined || v === '' ? '—' : typeof v === 'object' ? JSON.stringify(v) : String(v),
  }));
});
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
  margin-bottom: 12px

.approvals-kind-filter
  min-width: 240px

.approvals-queue
  display: grid
  grid-template-columns: minmax(320px, 400px) minmax(0, 1fr)
  gap: 16px
  align-items: start

.approvals-list__item
  min-height: 56px

.approvals-list__title
  display: flex
  align-items: center
  gap: 8px

.approvals-list__title-text
  overflow: hidden
  text-overflow: ellipsis
  white-space: nowrap

.approvals-list__item--active
  background: var(--nt-bg-2)

.approvals-list__skeleton
  height: 56px
  border-radius: 8px

.approvals-queue__empty,
.approvals-queue__empty-detail
  padding: 32px 16px
  text-align: center
  color: var(--nt-text-2)

.approvals-queue__empty-detail
  display: flex
  flex-direction: column
  align-items: center
  gap: 12px
  border: 1px dashed var(--nt-border, currentColor)
  border-radius: 12px
  opacity: 0.7

.approvals-detail__badges
  display: flex
  gap: 8px
  margin-bottom: 8px

.approvals-detail__title
  font-size: 16px
  font-weight: 600
  color: var(--nt-text-0)
  word-break: break-all

.approvals-detail__meta
  display: flex
  flex-direction: column
  gap: 6px

.approvals-detail__label
  display: inline-block
  min-width: 72px
  margin-right: 8px
  color: var(--nt-text-2)
  font-size: 12px

.approvals-detail__summary
  margin: 4px 0 0
  white-space: pre-wrap
  word-break: break-word

.approvals-detail__ctx
  margin: 4px 0 0
  display: grid
  grid-template-columns: max-content 1fr
  gap: 4px 12px
  dt
    color: var(--nt-text-2)
  dd
    margin: 0
    word-break: break-all

.approvals-detail__raw
  margin: 4px 0 0
  padding: 8px
  border-radius: 6px
  background: var(--nt-bg-2)
  white-space: pre-wrap
  word-break: break-all

.approvals-matrix__hint
  margin: 0 0 12px
  color: var(--nt-text-2)

.approvals-matrix__actions
  display: flex
  justify-content: space-between
  margin-top: 12px

.approvals-matrix__empty
  color: var(--nt-text-2)

// 断点 799：两栏收敛为单栏堆叠（规范断点 1200/1099/799/559/359）。
@media (max-width: 799px)
  .approvals-queue
    grid-template-columns: 1fr
</style>
