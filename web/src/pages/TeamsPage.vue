<template>
  <q-page :class="['app-standard-page app-entity-page teams-page', { 'is-dark': isDark }]">
    <AppPageHero
      kicker="ADK Multi-Agent"
      title="Team 管理"
      subtitle="参照 ADK Web 的 App / Session / Trace 工作台，将多个 Agent 编排成可运行、可观测的协作团队。"
    >
      <template #actions>
        <q-btn color="primary" rounded unelevated icon="add" label="新增 Team" @click="openCreate" />
      </template>
    </AppPageHero>

    <TeamToolbar
      v-model:search="search"
      v-model:mode-filter="modeFilter"
      v-model:status-filter="statusFilter"
      v-model:department-filter="departmentFilter"
      v-model:show-orchestrated="showOrchestrated"
      class="q-mt-md"
      :department-options="departmentOptions"
      :loading="loading"
      :is-dark="isDark"
      @refresh="loadRows"
    />

    <AppStatePanel
      v-if="pageState === 'loading'"
      class="q-mt-lg"
      variant="loading"
      :rows="4"
    />
    <AppStatePanel
      v-else-if="pageState === 'error'"
      class="q-mt-lg"
      variant="error"
      :title="error"
      :action-label="t('shell.common.retry')"
      action-icon="refresh"
      @action="loadRows"
    />
    <AppStatePanel
      v-else-if="pageState === 'forbidden'"
      class="q-mt-lg"
      variant="forbidden"
    />
    <template v-else-if="pageState === 'empty'">
      <AppStatePanel
        class="q-mt-lg"
        variant="empty"
        title="暂无 Team"
        description="创建一个 Team，把多个 Agent 组织成顺序、并行或评审闭环。"
        action-label="新增 Team"
        action-icon="add"
        @action="openCreate"
      />
      <div v-if="storeAgents.length === 0" class="text-body2 app-text-secondary q-mt-sm text-center">
        你还没有可用 Agent——Team 由 Agent 组成，请先到 Agent 页创建。
      </div>
      <div
        v-if="hiddenOrchestratedCount > 0"
        class="q-mt-md row items-center no-wrap rounded-borders q-px-sm q-py-xs text-caption app-text-secondary teams-page__hint-banner"
      >
        <q-icon name="visibility_off" size="16px" class="q-mr-xs" />
        <span>{{ t('teamsPage.hiddenOrchestrated', { count: hiddenOrchestratedCount }) }}</span>
        <q-btn
          flat
          dense
          no-caps
          rounded
          color="primary"
          :label="t('teamsPage.showOrchestratedAction')"
          @click="showOrchestrated = true"
        />
      </div>
    </template>

    <div
      v-else
      class="teams-workspace q-mt-lg"
      :class="{ 'teams-workspace--inspecting': inspectorOpen }"
    >
      <div class="teams-workspace__main">
        <AppStatePanel
          v-if="pageState === 'partial'"
          class="q-mb-md"
          variant="partial"
          dense
          :description="error"
          :action-label="t('shell.common.retry')"
          action-icon="refresh"
          @action="loadRows"
        />
        <AppRegistryTable
          :rows="paginatedTeams"
          :columns="teamColumns"
          :loading="loading"
          row-key="id"
          hide-pagination
          :pagination="{ rowsPerPage: 0 }"
        >
          <template #header-cell-select="slotProps">
            <q-th :props="slotProps">
              <q-checkbox
                dense
                data-test="select-all"
                :aria-label="t('shell.bulkBar.selectAllPage')"
                :model-value="teamsPageAllSelected"
                @update:model-value="toggleAllTeams"
              />
            </q-th>
          </template>
          <template #body-cell-select="slotProps">
            <q-td :props="slotProps">
              <q-checkbox
                dense
                :data-test="`row-select-${slotProps.row.id}`"
                :aria-label="t('shell.bulkBar.selectRow', { name: slotProps.row.display_name || slotProps.row.id })"
                :model-value="isTeamSelected(slotProps.row.id)"
                @update:model-value="toggleTeamSelected(slotProps.row.id, $event)"
              />
            </q-td>
          </template>
          <template #body-cell-name="slotProps">
            <q-td :props="slotProps">
              <button type="button" class="registry-table-link app-registry-cell-primary" @click="inspectTeam(slotProps.row)">
                {{ slotProps.row.display_name }}
              </button>
              <div class="app-registry-cell-sub ellipsis">{{ slotProps.row.team_key }}</div>
              <div v-if="teamDefinition(slotProps.row).description" class="app-registry-cell-desc ellipsis-2-lines">
                {{ teamDefinition(slotProps.row).description }}
              </div>
            </q-td>
          </template>
          <template #body-cell-department="slotProps">
            <q-td :props="slotProps">{{ departmentLabel(slotProps.row) }}</q-td>
          </template>
          <template #body-cell-mode="slotProps">
            <q-td :props="slotProps">{{ teamModeLabel(teamDefinition(slotProps.row).mode) }}</q-td>
          </template>
          <template #body-cell-members="slotProps">
            <q-td :props="slotProps" class="mono">{{ teamDefinition(slotProps.row).members.length }}</q-td>
          </template>
          <template #body-cell-status="slotProps">
            <q-td :props="slotProps">
              <div class="row items-center no-wrap q-gutter-xs">
                <AppStatusChip :status="slotProps.row.status" />
                <q-badge v-if="slotProps.row.has_active_run" color="positive" rounded>{{ t('shell.registry.activeRun') }}</q-badge>
              </div>
            </q-td>
          </template>
          <template #body-cell-updated_at="slotProps">
            <q-td :props="slotProps" class="mono">{{ formatDate(slotProps.row.updated_at) }}</q-td>
          </template>
          <template #body-cell-actions="slotProps">
            <q-td :props="slotProps">
              <div class="app-registry-cell-actions">
                <q-btn flat dense round icon="info" color="primary" :aria-label="t('shell.registry.inspect')" @click="inspectTeam(slotProps.row)" />
                <q-btn flat dense round icon="format_list_bulleted" color="primary" :aria-label="t('shell.registry.runs')" @click="openRuns(slotProps.row)" />
                <q-btn flat dense round icon="play_arrow" color="primary" :aria-label="t('shell.registry.runTest')" @click="openRunTest(slotProps.row)" />
                <q-btn flat dense round icon="edit" color="primary" :aria-label="t('shell.registry.edit')" @click="openEdit(slotProps.row)" />
                <q-btn-dropdown flat dense round dropdown-icon="more_vert" :aria-label="t('shell.registry.actions')">
                  <q-list dense>
                    <q-item v-if="slotProps.row.has_active_run" v-close-popup clickable @click="openTeamObservatory(slotProps.row)">
                      <q-item-section>{{ t('shell.registry.observatory') }}</q-item-section>
                    </q-item>
                    <q-item v-close-popup clickable @click="duplicate(slotProps.row)">
                      <q-item-section>{{ t('shell.registry.duplicate') }}</q-item-section>
                    </q-item>
                    <q-item v-if="canRetryTeam(slotProps.row)" v-close-popup clickable @click="retryTeam(slotProps.row)">
                      <q-item-section>{{ t('shell.registry.retry') }}</q-item-section>
                    </q-item>
                    <q-item v-if="!slotProps.row.readonly" v-close-popup clickable @click="confirmRemove(slotProps.row)">
                      <q-item-section class="text-negative">{{ t('shell.registry.remove') }}</q-item-section>
                    </q-item>
                  </q-list>
                </q-btn-dropdown>
              </div>
            </q-td>
          </template>
        </AppRegistryTable>
      </div>

      <AppRegistryInspector
        v-model="inspectorOpen"
        :title="inspectedTeam?.display_name || t('shell.registry.inspect')"
        :eyebrow="inspectedTeam ? `TEAM / ${inspectedTeam.team_key}` : undefined"
      >
        <div v-if="inspectedTeam && inspectedDefinition" class="registry-inspector-stack">
          <AppStatusChip :status="inspectedTeam.status" />
          <p class="registry-inspector-description">{{ inspectedDefinition.description || t('shell.registry.noDescription') }}</p>
          <dl class="registry-facts">
            <div><dt>{{ t('shell.registry.identifier') }}</dt><dd class="mono">{{ inspectedTeam.team_key }}</dd></div>
            <div><dt>{{ t('shell.registry.department') }}</dt><dd>{{ departmentLabel(inspectedTeam) }}</dd></div>
            <div><dt>{{ t('shell.registry.mode') }}</dt><dd>{{ teamModeLabel(inspectedDefinition.mode) }}</dd></div>
            <div><dt>{{ t('shell.registry.members') }}</dt><dd class="mono">{{ inspectedDefinition.members.length }}</dd></div>
            <div><dt>{{ t('shell.registry.updatedAt') }}</dt><dd class="mono">{{ formatDate(inspectedTeam.updated_at) }}</dd></div>
          </dl>
          <section class="registry-inspector-section">
            <h3>{{ t('shell.registry.members') }}</h3>
            <ul class="registry-member-list">
              <li v-for="member in inspectedDefinition.members" :key="`${member.agent_id}:${member.sort_order}`">
                <span>{{ member.name || agentName(storeAgents, member.agent_id) }} · {{ teamRoleLabel(member.role) }}</span>
                <AppStatusChip :status="member.enabled ? 'enabled' : 'disabled'" />
              </li>
            </ul>
          </section>
          <div class="row q-gutter-sm">
            <q-btn outline no-caps color="primary" icon="format_list_bulleted" :label="t('shell.registry.runs')" @click="openRuns(inspectedTeam)" />
            <q-btn unelevated no-caps color="primary" icon="edit" :label="t('shell.registry.edit')" @click="openEdit(inspectedTeam)" />
          </div>
        </div>
      </AppRegistryInspector>
    </div>

    <BulkActionBar
      v-if="teamsSelectedCount > 0"
      :count="teamsSelectedCount"
      :total="totalFiltered"
      :actions="teamsBulkActions"
      :busy="teamsBulkBusy"
      :error="teamsBulkError"
      @run="runTeamsBulk"
      @clear="clearTeamsSelection"
    />

    <footer v-if="totalFiltered > 0" class="app-registry-pagination app-registry-pagination--card q-mt-md">
      <div class="app-registry-pagination__summary">{{ totalFiltered }} 条</div>
      <div class="app-registry-pagination__controls row items-center no-wrap">
        <q-select
          v-model="pageSize"
          dense
          outlined
          emit-value
          map-options
          label="行"
          :options="[12, 24, 48].map((v) => ({ label: String(v), value: v }))"
          class="app-registry-pagination__page-size app-glass-control"
        />
        <span class="app-registry-pagination__page-label">第 {{ currentPage }} / {{ pageMax }} 页</span>
        <q-btn
          round
          dense
          flat
          icon="chevron_left"
          :disable="currentPage <= 1"
          @click="currentPage = Math.max(1, currentPage - 1)"
        />
        <q-btn
          round
          dense
          flat
          icon="chevron_right"
          :disable="currentPage >= pageMax"
          @click="currentPage = Math.min(pageMax, currentPage + 1)"
        />
      </div>
    </footer>

    <TeamEditorDialog
      v-model="editorOpen"
      v-model:selected-template-key="selectedTeamTemplateKey"
      v-model:form="form"
      v-model:definition="definition"
      :editing-id="editingId"
      :has-active-run="editingHasActiveRun"
      :definition-json="definitionJSON"
      :agent-options="agentOptions"
      :department-options="departmentOptions"
      :graph-options="graphOptions"
      :overwrite-baseline-key="editorOverwriteBaselineKey"
      :saving="saving"
      :can-save="canSave"
      :is-dark="isDark"
      @add-member="addMember"
      @remove-member="removeMember"
      @apply-template="applyTemplate"
      @save="save"
      @retry="retryEditingTeam"
      @reset-to-derived="resetToDerived"
    />

    <TeamRunsDialog
      v-model="runsOpen"
      :selected-team="selectedTeam"
      :runs="runs"
      :steps-by-run="stepsByRun"
      :steps-loading="stepsLoading"
      :summaries-by-run="summariesByRun"
      :summaries-loading="summariesLoading"
      :details-by-run="detailsByRun"
      :details-loading="detailsLoading"
      :dead-letters="deadLetters"
      :dead-letters-loading="deadLettersLoading"
      :agents="storeAgents"
      :loading="runsLoading"
      :error="runsError"
      :live-connected="runEventsConnected"
      :is-dark="isDark"
      @refresh="loadRuns"
      @show-steps="loadRunSteps"
      @load-summary="loadRunSummary"
      @load-detail="loadRunDetail"
      @open-observatory="openRunObservatory"
      @refresh-dead-letters="loadDeadLetters"
      @resolve-dead-letter="resolveDeadLetter"
    />

    <TeamTestDialog
      v-model="testOpen"
      :team="testTeam"
      :loading="testLoading"
      :error="testError"
      :reply="testReply"
      :run="testRun"
      :is-dark="isDark"
      @run="executeRunTest"
    />
  </q-page>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import type { QTableColumn } from 'quasar';
import AppPageHero from '../components/layout/AppPageHero.vue';
import AppRegistryInspector from '../components/layout/AppRegistryInspector.vue';
import AppRegistryTable from '../components/layout/AppRegistryTable.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import AppStatusChip from '../components/common/AppStatusChip.vue';
import BulkActionBar from '../components/common/BulkActionBar.vue';
import TeamEditorDialog from '../components/teams/TeamEditorDialog.vue';
import TeamRunsDialog from '../components/teams/TeamRunsDialog.vue';
import TeamTestDialog from '../components/teams/TeamTestDialog.vue';
import TeamToolbar from '../components/teams/TeamToolbar.vue';
import { useTeamsPage } from '../features/teams/useTeamsPage';
import { useTeamsBulk } from '../features/teams/useTeamsBulk';
import { REGISTRY_COL_W, registryCol } from '../features/ui/registryTableColumns';
import { useTeamsStore } from '../stores/teams';
import { storeToRefs } from 'pinia';
import { useI18n } from 'vue-i18n';
import { agentName, formatDate, parseDefinition, teamModeLabel, teamRoleLabel } from '../components/teams/teamUtils';
import type { Team } from '../features/teams/types';
import { resolvePageState } from './pageState';

const { t } = useI18n();
const inspectorOpen = ref(false);
const inspectedTeam = ref<Team | null>(null);
const inspectedDefinition = computed(() => (inspectedTeam.value ? parseDefinition(inspectedTeam.value) : null));

function inspectTeam(team: Team) {
  inspectedTeam.value = team;
  inspectorOpen.value = true;
}

function teamDefinition(team: Team) {
  return parseDefinition(team);
}

function canRetryTeam(team: Team) {
  return ['failed', 'cancelled'].includes(team.status) && !team.readonly;
}
const teamsStore = useTeamsStore();
const { agents: storeAgents } = storeToRefs(teamsStore);

const {
  isDark,
  loading,
  saving,
  error,
  forbidden,
  search,
  modeFilter,
  statusFilter,
  departmentFilter,
  showOrchestrated,
  hiddenOrchestratedCount,
  departmentOptions,
  teamGroups,
  paginatedTeams,
  currentPage,
  pageSize,
  totalFiltered,
  pageMax,
  editorOpen,
  selectedTeamTemplateKey,
  editingId,
  editingHasActiveRun,
  runsOpen,
  runsLoading,
  runsError,
  runEventsConnected,
  selectedTeam,
  runs,
  stepsByRun,
  stepsLoading,
  summariesByRun,
  summariesLoading,
  detailsByRun,
  detailsLoading,
  deadLetters,
  deadLettersLoading,
  testOpen,
  testTeam,
  testLoading,
  testError,
  testReply,
  testRun,
  form,
  definition,
  agentOptions,
  graphOptions,
  editorOverwriteBaselineKey,
  definitionJSON,
  canSave,
  loadRows,
  openCreate,
  openEdit,
  resetToDerived,
  addMember,
  removeMember,
  applyTemplate,
  save,
  duplicate,
  confirmRemove,
  openRuns,
  openRunTest,
  executeRunTest,
  loadRunSummary,
  loadRunDetail,
  openRunObservatory,
  openTeamObservatory,
  loadRuns,
  loadRunSteps,
  loadDeadLetters,
  resolveDeadLetter,
  retryTeam,
  retryEditingTeam,
} = useTeamsPage();

const {
  count: teamsSelectedCount,
  pageAllSelected: teamsPageAllSelected,
  busy: teamsBulkBusy,
  error: teamsBulkError,
  actions: teamsBulkActions,
  isSelected: isTeamSelected,
  toggleRow: toggleTeamSelected,
  toggleAll: toggleAllTeams,
  clear: clearTeamsSelection,
  run: runTeamsBulk,
} = useTeamsBulk(paginatedTeams, loadRows);

// 筛选/分页变化必须清空选择：否则批量动作会作用到用户已看不见的行。
watch([search, modeFilter, statusFilter, departmentFilter, showOrchestrated, currentPage, pageSize], () => {
  clearTeamsSelection();
});

const pageState = computed(() =>
  resolvePageState({
    loading: loading.value,
    error: error.value,
    count: paginatedTeams.value.length,
    forbidden: forbidden.value,
  }),
);

const teamColumns = computed<QTableColumn<Team>[]>(() => [
  registryCol<Team>('select', '', 'id', 'left', REGISTRY_COL_W.select, { sortable: false }),
  { name: 'name', label: t('shell.registry.name'), field: 'display_name', align: 'left' },
  { name: 'department', label: t('shell.registry.department'), field: 'taxonomy_industry_id', align: 'left' },
  { name: 'mode', label: t('shell.registry.mode'), field: (row) => teamDefinition(row).mode, align: 'left' },
  { name: 'members', label: t('shell.registry.members'), field: (row) => teamDefinition(row).members.length, align: 'right' },
  { name: 'status', label: t('shell.registry.status'), field: 'status', align: 'left' },
  { name: 'updated_at', label: t('shell.registry.updatedAt'), field: 'updated_at', align: 'left' },
  { name: 'actions', label: t('shell.registry.actions'), field: 'id', align: 'right' },
]);

function departmentLabel(team: Team) {
  return teamGroups.value.find((group) => group.teams.some((candidate) => candidate.id === team.id))?.label || '—';
}
</script>
