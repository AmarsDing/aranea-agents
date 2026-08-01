<template>
  <q-page class="app-standard-page app-registry-page self-improvement-page">
    <AppPageHero
      :kicker="t('selfImprovementPage.kicker')"
      :title="t('selfImprovementPage.title')"
      :subtitle="t('selfImprovementPage.subtitle')"
    >
      <template #actions>
        <q-btn
          unelevated
          rounded
          no-caps
          icon="refresh"
          :label="t('selfImprovementPage.refresh')"
          :loading="loading"
          @click="loadRows"
        />
      </template>
    </AppPageHero>

    <OutcomeStatsPanel :stats="outcomeStats" class="q-mb-md" />

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
        <q-btn flat rounded no-caps icon="restart_alt" :label="t('selfImprovementPage.reset')" @click="resetFilters" />
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

    <q-banner v-if="error" rounded class="app-page-error-banner q-mb-md">
      {{ error }}
      <template #action>
        <q-btn flat dense :label="t('selfImprovementPage.retry')" class="text-white" @click="loadRows" />
      </template>
    </q-banner>

    <q-card v-if="!loading && rows.length === 0" flat class="app-registry-empty app-empty-state-center">
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-avatar size="72px" color="primary" text-color="white" icon="precision_manufacturing" />
        <div class="text-h6 q-mt-md">{{ t('selfImprovementPage.emptyTitle') }}</div>
        <div class="text-body2 text-grey-7 q-mt-sm">{{ t('selfImprovementPage.emptyHint') }}</div>
      </q-card-section>
    </q-card>

    <template v-else>
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
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppPageToolbar from '../components/layout/AppPageToolbar.vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
import AppRegistryPagination from '../components/layout/AppRegistryPagination.vue';
import OutcomeStatsPanel from '../components/self-improvement/OutcomeStatsPanel.vue';
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
  error,
  pageMax,
  statusOptions,
  riskOptions,
  triggerOptions,
  loadRows,
  resetFilters,
  outcomeStats,
  drawerOpen,
  detail,
  detailLoading,
  openDetail,
  actionRunning,
  approveRunAction,
  rejectRunAction,
  rollbackRunAction,
  closeRunAction,
} = useSelfImprovementPage();

const columns = computed(() => createSIRunColumns(t));
</script>
