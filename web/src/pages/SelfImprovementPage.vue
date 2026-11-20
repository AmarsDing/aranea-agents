<template>
  <!--
    Harness › 运行与治理 › 自我改进（2026-11-19 重排）：平台自身的补丁闭环。
    未启用时给开启引导 + 三项前置条件的实时核对（开关 / 归因模型 / 仓库目录），不只贴一段配置。
  -->
  <q-page class="app-standard-page app-registry-page op-page self-improvement-page">
    <AppPageHero :title="t('ops.nav.selfImprovement')" :subtitle="t('selfImprovementPage.subtitle')">
      <template #actions>
        <!-- 风险规则是流水线治理配置，功能未启用时无意义（GetRiskRules 亦 503） -->
        <q-btn
          v-if="!featureDisabled"
          flat
          rounded
          no-caps
          icon="tune"
          :label="t('selfImprovementPage.rules.action')"
          @click="openRulesDialog"
        />
        <q-btn
          flat
          round
          dense
          icon="refresh"
          :aria-label="t('selfImprovementPage.refresh')"
          :loading="loading"
          @click="loadRows"
        />
      </template>
    </AppPageHero>

    <!-- 功能未启用：说明 + 三项前置条件逐项核对（数据来自 GET /v1/self-improvement/status） -->
    <OpsGuide
      v-if="featureDisabled"
      data-state="empty"
      icon="precision_manufacturing"
      :title="t('selfImprovementPage.disabledTitle')"
      :description="t('selfImprovementPage.disabledDesc')"
    >
      <ul class="op-list" :aria-label="t('ops.si.prereq')">
        <li v-for="c in prereqs" :key="c.key" class="op-row">
          <span class="op-dot" :class="c.ok ? 'op-dot--ok' : 'op-dot--alert'" aria-hidden="true" />
          <span class="op-row__main">
            <span class="op-row__title">{{ c.label }}</span>
            <span class="op-row__sub op-num">{{ c.detail }}</span>
          </span>
          <span class="op-tag" :class="c.ok ? 'op-tag--ok' : 'op-tag--alert'">
            {{ c.ok ? t('ops.si.ready') : t('ops.si.missing') }}
          </span>
        </li>
      </ul>
      <p class="op-note">{{ t('selfImprovementPage.disabledConfigTitle') }}</p>
      <pre class="si-disabled-config">{{ t('selfImprovementPage.disabledConfigSnippet') }}</pre>
      <template #actions>
        <q-btn
          outline
          no-caps
          color="primary"
          icon="refresh"
          :label="t('selfImprovementPage.disabledRecheck')"
          :loading="loading"
          @click="recheckAction"
        />
      </template>
    </OpsGuide>

    <template v-else>
      <OutcomeStatsPanel :stats="outcomeStats" :failed="statsFailed" class="q-mb-md" />

      <AppPageToolbar>
        <q-select
          v-model="status"
          class="app-page-toolbar__field"
          dense
          outlined
          clearable
          emit-value
          map-options
          :label="t('selfImprovementPage.filterStatus')"
          :options="statusOptions"
        />
        <q-select
          v-model="riskLevel"
          class="app-page-toolbar__field"
          dense
          outlined
          clearable
          emit-value
          map-options
          :label="t('selfImprovementPage.filterRisk')"
          :options="riskOptions"
        />
        <q-select
          v-model="triggerSource"
          class="app-page-toolbar__field"
          dense
          outlined
          clearable
          emit-value
          map-options
          :label="t('selfImprovementPage.filterTrigger')"
          :options="triggerOptions"
        />
        <template #actions>
          <q-btn
            flat
            rounded
            no-caps
            icon="restart_alt"
            :label="t('selfImprovementPage.reset')"
            @click="resetFilters"
          />
          <q-btn
            flat
            rounded
            no-caps
            icon="refresh"
            :label="t('selfImprovementPage.refresh')"
            :loading="loading"
            @click="loadRows"
          />
        </template>
      </AppPageToolbar>

      <!-- 五态：已有数据但刷新失败 → 局部失败横幅，列表照常渲染 -->
      <AppStatePanel
        v-if="errorMessage && rows.length > 0"
        variant="partial"
        dense
        class="q-mb-md"
        :description="errorMessage"
        :action-label="errorRetryable ? t('selfImprovementPage.retry') : ''"
        action-icon="refresh"
        @action="loadRows"
      />

      <!-- 五态：加载失败且无数据 → 403 走无重试的 forbidden，其余为可重试 error；不与空态并存 -->
      <AppStatePanel
        v-else-if="errorMessage && errorKind === 'forbidden'"
        variant="forbidden"
        class="q-mb-md"
        :description="errorMessage"
      />
      <AppStatePanel
        v-else-if="errorMessage"
        variant="error"
        class="q-mb-md"
        :description="errorMessage"
        :action-label="errorRetryable ? t('selfImprovementPage.retry') : ''"
        action-icon="refresh"
        @action="loadRows"
      />

      <!-- 前置条件自检（P5.5）：enabled 但闭环无法真正运行时给出警告 -->
      <q-banner v-if="prereqLlmMissing || prereqRepoInvalid" rounded class="app-banner-warning q-mb-md">
        <div>{{ t('selfImprovementPage.prereqTitle') }}</div>
        <ul class="si-prereq-list">
          <li v-if="prereqLlmMissing">{{ t('selfImprovementPage.prereqLlm') }}</li>
          <li v-if="prereqRepoInvalid">
            {{ t('selfImprovementPage.prereqRepo', { path: statusInfo?.repoRoot || '—' }) }}
          </li>
        </ul>
      </q-banner>

      <!-- 五态：首屏加载无数据 → 骨架面板 -->
      <AppStatePanel v-if="loading && !errorMessage && rows.length === 0" variant="loading" :rows="5" />

      <!-- 五态：成功但无数据 → 空态 -->
      <AppStatePanel
        v-else-if="!loading && !errorMessage && rows.length === 0"
        variant="empty"
        :title="t('selfImprovementPage.emptyTitle')"
        :description="emptyDescription"
        icon="precision_manufacturing"
      />

      <template v-else-if="rows.length > 0">
        <AppRegistryTable
          :rows="rows"
          :columns="columns"
          row-key="id"
          :loading="loading"
          column-persist-key="self-improvement-runs"
        >
          <template #body-cell-id="props">
            <q-td :props="props">
              <span
                class="app-registry-cell-primary ellipsis cursor-pointer"
                :title="props.row.id"
                @click="openDetail(props.row)"
              >
                {{ props.row.id }}
              </span>
            </q-td>
          </template>

          <template #body-cell-status="props">
            <q-td :props="props">
              <q-chip dense square :color="siStatusColor(props.row.status)" text-color="white">
                {{ siStatusLabel(t, props.row.status) }}
              </q-chip>
            </q-td>
          </template>

          <template #body-cell-riskLevel="props">
            <q-td :props="props">
              <q-chip dense square :color="siRiskColor(props.row.riskLevel)" text-color="white">
                {{ siRiskLabel(t, props.row.riskLevel) }}
              </q-chip>
            </q-td>
          </template>

          <template #body-cell-triggerSource="props">
            <q-td :props="props">
              <q-chip dense square outline :color="siTriggerColor(props.row.triggerSource)">
                {{ siTriggerLabel(t, props.row.triggerSource) }}
              </q-chip>
            </q-td>
          </template>

          <template #body-cell-patchKind="props">
            <q-td :props="props">
              <span class="app-registry-cell-sub">{{ siKindLabel(t, props.row.patchKind) }}</span>
            </q-td>
          </template>

          <template #body-cell-diffStats="props">
            <q-td :props="props">
              <span class="app-registry-cell-sub">
                {{
                  t('selfImprovementPage.diffSummary', {
                    files: props.row.diffStats.files,
                    additions: props.row.diffStats.additions,
                    deletions: props.row.diffStats.deletions,
                  })
                }}
              </span>
            </q-td>
          </template>

          <template #body-cell-createdAt="props">
            <q-td :props="props">
              <span class="app-registry-cell-sub">{{ formatSITime(props.row.createdAt) }}</span>
            </q-td>
          </template>

          <template #body-cell-actions="props">
            <q-td :props="props">
              <div class="app-registry-cell-actions">
                <q-btn flat dense round icon="visibility" color="primary" @click="openDetail(props.row)">
                  <q-tooltip>{{ t('selfImprovementPage.actionView') }}</q-tooltip>
                </q-btn>
                <template v-if="canApprove(props.row.status)">
                  <q-btn
                    flat
                    dense
                    round
                    icon="check"
                    color="positive"
                    :loading="actionRunning === 'approve'"
                    @click="approveRunAction(props.row)"
                  >
                    <q-tooltip>{{ t('selfImprovementPage.actionApprove') }}</q-tooltip>
                  </q-btn>
                  <q-btn flat dense round icon="close" color="negative" @click="rejectRunAction(props.row)">
                    <q-tooltip>{{ t('selfImprovementPage.actionReject') }}</q-tooltip>
                  </q-btn>
                </template>
                <q-btn
                  v-if="canRollback(props.row.status)"
                  flat
                  dense
                  round
                  icon="undo"
                  color="warning"
                  @click="rollbackRunAction(props.row)"
                >
                  <q-tooltip>{{ t('selfImprovementPage.actionRollback') }}</q-tooltip>
                </q-btn>
                <q-btn
                  v-if="canClose(props.row.status)"
                  flat
                  dense
                  round
                  icon="task_alt"
                  color="primary"
                  @click="closeRunAction(props.row)"
                >
                  <q-tooltip>{{ t('selfImprovementPage.actionClose') }}</q-tooltip>
                </q-btn>
              </div>
            </q-td>
          </template>
        </AppRegistryTable>

        <AppRegistryPagination
          v-model:page="page"
          v-model:page-size="pageSize"
          :page-max="pageMax"
          :total="total"
          :loading="loading"
          :label="t('selfImprovementPage.paginationLabel')"
        />
      </template>
    </template>

    <RunDetailDrawer
      :open="drawerOpen"
      :run="detail"
      :loading="detailLoading"
      :action-running="actionRunning"
      @update:open="drawerOpen = $event"
      @approve="approveRunAction"
      @reject="rejectRunAction"
      @rollback="rollbackRunAction"
      @close="closeRunAction"
      @control="controlRunAction"
    />

    <RiskRulesDialog
      v-model="rulesDialogOpen"
      :configured="configuredRules"
      :effective="effectiveRules"
      :saving="rulesSaving"
      @submit="saveRiskRulesAction"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import OpsGuide from '../components/ops/OpsGuide.vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import OutcomeStatsPanel from '../components/self-improvement/OutcomeStatsPanel.vue';
import RiskRulesDialog from '../components/self-improvement/RiskRulesDialog.vue';
import RunDetailDrawer from '../components/self-improvement/RunDetailDrawer.vue';
import {
  canApprove,
  canClose,
  canRollback,
  createSIRunColumns,
  formatSITime,
  siKindLabel,
  siRiskColor,
  siRiskLabel,
  siStatusColor,
  siStatusLabel,
  siTriggerColor,
  siTriggerLabel,
} from '../components/self-improvement/selfImprovementUi';
import { useSelfImprovementPage } from '../features/self-improvement/useSelfImprovementPage';

const { t } = useI18n();

const {
  status,
  riskLevel,
  triggerSource,
  page,
  pageSize,
  rows,
  total,
  loading,
  errorMessage,
  errorKind,
  errorRetryable,
  pageMax,
  statusOptions,
  riskOptions,
  triggerOptions,
  loadRows,
  resetFilters,
  featureDisabled,
  statusInfo,
  prereqLlmMissing,
  prereqRepoInvalid,
  recheckAction,
  outcomeStats,
  statsFailed,
  drawerOpen,
  detail,
  detailLoading,
  openDetail,
  actionRunning,
  approveRunAction,
  rejectRunAction,
  rollbackRunAction,
  closeRunAction,
  controlRunAction,
  rulesDialogOpen,
  rulesSaving,
  configuredRules,
  effectiveRules,
  openRulesDialog,
  saveRiskRulesAction,
} = useSelfImprovementPage();

const columns = computed(() => createSIRunColumns(t));
/** 闭环真正能跑的三个前提，逐项核对（未启用时也能读到状态） */
const prereqs = computed(() => {
  const s = statusInfo.value;
  return [
    {
      key: 'switch',
      label: t('ops.si.switch'),
      detail: 'self_improvement.enabled',
      ok: s?.enabled === true,
    },
    {
      key: 'llm',
      label: t('ops.si.llm'),
      detail: s?.refineLlmConfigured ? `${s.refineLlmProvider} / ${s.refineLlmModel}` : t('ops.si.llmMissing'),
      ok: s?.refineLlmConfigured === true,
    },
    {
      key: 'repo',
      label: t('ops.si.repo'),
      detail: s?.repoRoot || t('ops.si.repoCwd'),
      ok: s?.repoRootValid === true,
    },
  ];
});

const emptyDescription = computed(
  () => `${t('selfImprovementPage.emptyHint')} ${t('selfImprovementPage.emptySignalHint')}`,
);
</script>

<style scoped lang="sass">
.si-disabled-config
  margin: 0
  padding: 12px 16px
  border-radius: 8px
  background: var(--nt-data-inset)
  color: var(--nt-text-1)
  font-family: var(--nt-font-mono)
  font-size: 12px
  line-height: 1.6
  text-align: left
  white-space: pre-wrap
  word-break: break-all
  max-width: 560px

.si-prereq-list
  margin: 4px 0 0
  padding-left: 20px
</style>
