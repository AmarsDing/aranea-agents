<template>
  <!-- Harness › 编制 › Team 管理：编制树（按部门）| 关键数 + 筛选 + 团队构件卡 | 检视（结构图、最近运行） -->
  <q-page class="app-standard-page bz-page" :class="{ 'bz-page--inspecting': inspector.open.value }">
    <Teleport defer to="#app-tab-actions">
      <q-btn
        flat
        dense
        round
        icon="refresh"
        :title="t('cap.common.refresh')"
        :aria-label="t('cap.common.refresh')"
        :loading="loading"
        @click="loadRows"
      />
      <q-btn unelevated dense no-caps color="primary" icon="add" :label="t('teamsHub.newTeam')" @click="openCreate" />
    </Teleport>

    <AppStatePanel v-if="pageState === 'loading'" class="bz-full" variant="loading" :rows="4" />
    <AppStatePanel
      v-else-if="pageState === 'error'"
      class="bz-full"
      variant="error"
      :title="error"
      :action-label="t('shell.common.retry')"
      action-icon="refresh"
      @action="loadRows"
    />
    <AppStatePanel v-else-if="pageState === 'forbidden'" class="bz-full" variant="forbidden" />
    <AppStatePanel
      v-else-if="pageState === 'empty'"
      class="bz-full"
      variant="empty"
      icon="groups"
      :title="t('teamsHub.emptyTitle')"
      :description="storeAgents.length ? t('teamsHub.emptyBody') : t('teamsPage.editorNoAvailableAgents')"
      :action-label="t('teamsHub.newTeam')"
      action-icon="add"
      @action="openCreate"
    />

    <template v-else>
      <OrgTreeRail
        :title="t('teamsHub.byDepartment')"
        :entries="railEntries"
        :section-label="t('orgChart.title')"
        :companies="railCompanies"
        :selected="departmentFilter"
        @select="(key) => (departmentFilter = key)"
      >
        <template #foot>
          <router-link :to="{ name: 'organization' }" class="bz-rail__link">
            <q-icon name="account_tree" size="16px" aria-hidden="true" />{{ t('orgChart.title') }}
          </router-link>
        </template>
      </OrgTreeRail>

      <section class="bz-panel bz-main" :aria-label="t('teamsHub.title')">
        <AppStatePanel
          v-if="pageState === 'partial'"
          variant="partial"
          dense
          :description="error"
          :action-label="t('shell.common.retry')"
          action-icon="refresh"
          @action="loadRows"
        />
        <header class="bz-head">
          <div class="bz-head__main">
            <div class="org-heading">
              <h2 class="org-heading__title">{{ departmentFilter ? currentDeptLabel : t('teamsHub.allTeams') }}</h2>
            </div>
            <p class="bz-head__desc">{{ t('teamsHub.subtitle') }}</p>
          </div>
        </header>

        <CapStats :stats="stats" :label="t('teamsHub.statsLabel')" :active="statusFilter || 'all'" @pick="pickStat" />

        <div class="cap-bar" role="search">
          <q-input
            v-model="search"
            class="cap-bar__search"
            dense
            outlined
            clearable
            debounce="200"
            :placeholder="t('teamsHub.searchPlaceholder')"
            :aria-label="t('teamsHub.searchPlaceholder')"
          >
            <template #prepend><q-icon name="search" size="18px" /></template>
          </q-input>
          <div class="cap-seg" role="group" :aria-label="t('shell.registry.mode')">
            <button type="button" :aria-pressed="!modeFilter" @click="modeFilter = ''">
              {{ t('teamsHub.allModes') }}
            </button>
            <button
              v-for="m in modeOptions"
              :key="m.value"
              type="button"
              :aria-pressed="modeFilter === m.value"
              :title="m.description"
              @click="modeFilter = modeFilter === m.value ? '' : m.value"
            >
              {{ m.label }}
            </button>
          </div>
          <span class="cap-bar__spacer" />
          <q-toggle v-model="showOrchestrated" dense :label="t('teamsPage.showOrchestrated')">
            <q-tooltip>{{ t('teamsPage.showOrchestratedHint') }}</q-tooltip>
          </q-toggle>
        </div>

        <div class="bz-scroll">
          <div v-if="paginatedTeams.length" class="cap-grid">
            <CapCard
              v-for="team in paginatedTeams"
              :key="team.id"
              :icon="modeIcon(parseDefinition(team).mode)"
              :code="team.team_key"
              :title="team.display_name"
              :description="parseDefinition(team).description"
              :status="teamStatus(team)"
              :facts="teamFacts(team)"
              :tags="teamTags(team)"
              :selected="inspector.team.value?.id === team.id"
              :checked="isTeamSelected(team.id)"
              @open="inspector.inspect(team)"
              @update:checked="(v) => toggleTeamSelected(team.id, v)"
            >
              <template #relation>
                <span class="tm-avatars" :aria-label="t('teamsHub.memberCount', { n: memberAvatars(team).enabled })">
                  <span
                    v-for="m in memberAvatars(team).shown"
                    :key="m.key"
                    class="tm-avatar"
                    :class="{ 'tm-avatar--off': !m.enabled }"
                    :title="m.name"
                    >{{ m.initial }}</span
                  >
                  <span v-if="memberAvatars(team).extra" class="tm-avatar tm-avatar--more"
                    >+{{ memberAvatars(team).extra }}</span
                  >
                </span>
              </template>
              <template #actions>
                <q-btn
                  flat
                  dense
                  no-caps
                  color="primary"
                  icon="play_arrow"
                  :label="t('teamsHub.tryRun')"
                  :disable="memberAvatars(team).enabled === 0"
                  @click="openRunTest(team)"
                />
                <q-btn flat dense round size="sm" icon="more_horiz" :aria-label="t('orgChart.menu.more')">
                  <q-menu anchor="bottom right" self="top right">
                    <q-list dense class="org-menu">
                      <q-item v-close-popup clickable @click="openEdit(team)">
                        <q-item-section avatar><q-icon name="edit" size="18px" /></q-item-section>
                        <q-item-section>{{ t('shell.registry.edit') }}</q-item-section>
                      </q-item>
                      <q-item v-close-popup clickable @click="openRuns(team)">
                        <q-item-section avatar><q-icon name="format_list_bulleted" size="18px" /></q-item-section>
                        <q-item-section>{{ t('shell.registry.runs') }}</q-item-section>
                      </q-item>
                      <q-item v-close-popup clickable @click="openOrchestrate(team)">
                        <q-item-section avatar><q-icon name="account_tree" size="18px" /></q-item-section>
                        <q-item-section>{{ t('teamsHub.orchestrate') }}</q-item-section>
                      </q-item>
                      <q-item v-if="team.has_active_run" v-close-popup clickable @click="openTeamObservatory(team)">
                        <q-item-section avatar><q-icon name="monitor_heart" size="18px" /></q-item-section>
                        <q-item-section>{{ t('shell.registry.observatory') }}</q-item-section>
                      </q-item>
                      <q-item v-close-popup clickable @click="duplicate(team)">
                        <q-item-section avatar><q-icon name="content_copy" size="18px" /></q-item-section>
                        <q-item-section>{{ t('shell.registry.duplicate') }}</q-item-section>
                      </q-item>
                      <q-item v-if="canRetryTeam(team)" v-close-popup clickable @click="retryTeam(team)">
                        <q-item-section avatar><q-icon name="restart_alt" size="18px" /></q-item-section>
                        <q-item-section>{{ t('shell.registry.retry') }}</q-item-section>
                      </q-item>
                      <template v-if="canRemoveTeam(team)">
                        <q-separator />
                        <q-item v-close-popup clickable class="org-menu__danger" @click="confirmRemove(team)">
                          <q-item-section avatar><q-icon name="delete" size="18px" /></q-item-section>
                          <q-item-section>{{ t('shell.registry.remove') }}</q-item-section>
                        </q-item>
                      </template>
                    </q-list>
                  </q-menu>
                </q-btn>
              </template>
            </CapCard>
          </div>
          <div v-else class="org-empty">
            <span>{{ t('teamsHub.noMatch') }}</span>
            <q-btn
              v-if="hiddenOrchestratedCount > 0"
              flat
              dense
              no-caps
              color="primary"
              :label="t('teamsPage.hiddenOrchestrated', { count: hiddenOrchestratedCount })"
              @click="showOrchestrated = true"
            />
            <q-btn flat dense no-caps color="primary" icon="add" :label="t('teamsHub.newTeam')" @click="openCreate" />
          </div>

          <footer v-if="pageMax > 1" class="tm-pager">
            <q-select
              v-model="pageSize"
              dense
              outlined
              emit-value
              map-options
              class="tm-pager__size"
              :label="t('teamsHub.pageSize')"
              :options="[12, 24, 48].map((v) => ({ label: String(v), value: v }))"
            />
            <span class="bz-num">{{ currentPage }} / {{ pageMax }}</span>
            <q-btn
              round
              dense
              flat
              icon="chevron_left"
              :aria-label="t('teamsHub.prevPage')"
              :disable="currentPage <= 1"
              @click="currentPage = Math.max(1, currentPage - 1)"
            />
            <q-btn
              round
              dense
              flat
              icon="chevron_right"
              :aria-label="t('teamsHub.nextPage')"
              :disable="currentPage >= pageMax"
              @click="currentPage = Math.min(pageMax, currentPage + 1)"
            />
          </footer>
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
      </section>

      <AppRegistryInspector
        v-model="inspector.open.value"
        :title="inspector.team.value?.display_name || t('shell.registry.inspect')"
        :eyebrow="inspector.team.value ? `TEAM / ${inspector.team.value.team_key}` : undefined"
      >
        <TeamDetail
          v-if="inspector.team.value"
          :team="inspector.team.value"
          :definition="parseDefinition(inspector.team.value)"
          :agents="storeAgents"
          :department-label="departmentLabel(inspector.team.value)"
          :lead="inspectedLead"
          :kind-label="kindLabel(inspector.team.value)"
          :runs="inspector.recentRuns.value"
          :runs-loading="inspector.runsLoading.value"
          :runs-error="inspector.runsError.value"
          @run-test="openRunTest(inspector.team.value)"
          @edit="openEdit(inspector.team.value)"
          @runs="openRuns(inspector.team.value)"
          @orchestrate="openOrchestrate(inspector.team.value)"
          @open-agent="(a) => router.push({ name: 'agent-settings', params: { id: a.id } })"
          @open-run="(runId: string) => openRun(inspector.team.value!, runId)"
        />
      </AppRegistryInspector>
    </template>

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
import { computed, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { useRouter } from 'vue-router';
import { storeToRefs } from 'pinia';
import AppRegistryInspector from '../components/layout/AppRegistryInspector.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import BulkActionBar from '../components/common/BulkActionBar.vue';
import CapCard from '../components/capability/CapCard.vue';
import CapStats from '../components/capability/CapStats.vue';
import type { CapFact, CapStat, CapStatus, CapTag, CapTone } from '../components/capability/capTypes';
import OrgTreeRail from '../components/organization/OrgTreeRail.vue';
import type { TreeRailCompany, TreeRailEntry } from '../components/organization/treeRail';
import TeamDetail from '../components/teams/TeamDetail.vue';
import TeamEditorDialog from '../components/teams/TeamEditorDialog.vue';
import TeamRunsDialog from '../components/teams/TeamRunsDialog.vue';
import TeamTestDialog from '../components/teams/TeamTestDialog.vue';
import {
  agentName,
  modeOptions,
  parseDefinition,
  teamModeLabel,
  teamStatusMap,
  UNCategorizedGroupId,
} from '../components/teams/teamUtils';
import { departmentPath, findNode } from '../features/organization/orgChart';
import { useTeamInspector } from '../features/teams/useTeamInspector';
import { useTeamsBulk } from '../features/teams/useTeamsBulk';
import { useTeamsPage } from '../features/teams/useTeamsPage';
import type { Team } from '../features/teams/types';
import { useTeamsStore } from '../stores/teams';
import { resolvePageState } from './pageState';

const { t } = useI18n();
const router = useRouter();
const teamsStore = useTeamsStore();
const { agents: storeAgents, teams: storeTeams } = storeToRefs(teamsStore);
const inspector = useTeamInspector();

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
  taxonomyTree,
  departmentOptions,
  departmentRail,
  unassignedCount,
  departmentLabel,
  departmentLeadId,
  focusTeamId,
  baseCount,
  scopeTeams,
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
  busy: teamsBulkBusy,
  error: teamsBulkError,
  actions: teamsBulkActions,
  isSelected: isTeamSelected,
  toggleRow: toggleTeamSelected,
  clear: clearTeamsSelection,
  run: runTeamsBulk,
} = useTeamsBulk(paginatedTeams, loadRows);

// 筛选/分页变化必须清空选择：否则批量动作会作用到用户已看不见的行。
watch([search, modeFilter, statusFilter, departmentFilter, showOrchestrated, currentPage, pageSize], () => {
  clearTeamsSelection();
});

// 列表刷新后检视面板跟上最新数据；团队被删了就关掉。
watch(storeTeams, (rows) => {
  const current = inspector.team.value;
  if (!current) return;
  const fresh = rows.find((row) => row.id === current.id);
  if (!fresh) inspector.close();
  else if (fresh !== current) inspector.team.value = fresh;
});

// 从组织页带 ?team= 过来：数据到了就打开它的检视面板。
watch([focusTeamId, storeTeams], ([id, rows]) => {
  const team = id ? rows.find((row) => row.id === id) : undefined;
  if (!team) return;
  void inspector.inspect(team);
  focusTeamId.value = '';
});

const pageState = computed(() =>
  resolvePageState({
    loading: loading.value,
    error: error.value,
    count: storeTeams.value.length,
    forbidden: forbidden.value,
  }),
);

// ——— 编制树：全部 / 未分部门 + 公司 → 有团队的部门 ———
const railEntries = computed<TreeRailEntry[]>(() => [
  { key: '', icon: 'apps', label: t('teamsHub.allTeams'), count: baseCount.value },
  ...(unassignedCount.value || departmentFilter.value === UNCategorizedGroupId
    ? [
        {
          key: UNCategorizedGroupId,
          icon: 'help_outline',
          label: t('teamsHub.unassigned'),
          count: unassignedCount.value,
        },
      ]
    : []),
]);
const railCompanies = computed<TreeRailCompany[]>(() =>
  departmentRail.value.map((g) => ({
    id: g.id,
    name: g.name,
    enabled: true,
    count: g.items.reduce((sum, d) => sum + d.count, 0),
    departments: g.items.map((d) => ({ id: d.id, name: d.name, enabled: d.enabled, count: d.count })),
  })),
);

const currentDeptLabel = computed(() => {
  const f = departmentFilter.value;
  if (f === UNCategorizedGroupId) return t('teamsHub.unassigned');
  const node = findNode(taxonomyTree.value, f);
  if (node?.level === 'company') return node.name;
  return departmentPath(taxonomyTree.value, f) || t('teamsHub.allTeams');
});

// ——— 关键数：按当前部门范围数各状态，点了按状态筛，再点取消 ———
const stats = computed<Array<CapStat & { clickable?: boolean }>>(() => {
  const rows = scopeTeams.value;
  const n = (...s: string[]) => rows.filter((team) => s.includes(team.status)).length;
  const failed = n('failed', 'partial_failure');
  return [
    { key: 'all', label: t('teamsHub.stat.all'), value: rows.length, clickable: true },
    {
      key: 'running',
      label: t('teamsHub.stat.running'),
      value: n('running'),
      tone: n('running') ? 'running' : undefined,
      clickable: true,
    },
    { key: 'pending', label: t('teamsHub.stat.pending'), value: n('pending'), clickable: true },
    {
      key: 'failed',
      label: t('teamsHub.stat.failed'),
      value: failed,
      tone: failed ? 'alert' : undefined,
      clickable: true,
    },
    { key: 'completed', label: t('teamsHub.stat.completed'), value: n('completed'), clickable: true },
  ];
});

function pickStat(key: string) {
  statusFilter.value = key === 'all' || statusFilter.value === key ? '' : key;
}

// ——— 团队卡 ———
const MODE_ICON: Record<string, string> = {
  sequential: 'trending_flat',
  parallel: 'call_split',
  coordinator: 'account_tree',
  critic_loop: 'sync',
};
const modeIcon = (mode: string) => MODE_ICON[String(mode || '').toLowerCase()] ?? 'hub';

const STATUS_TONE: Record<string, CapTone> = {
  running: 'running',
  pending: 'pending',
  completed: 'ok',
  active: 'ok',
  partial_failure: 'watch',
  interrupted: 'watch',
  failed: 'alert',
  cancelled: 'off',
  archived: 'off',
};

function teamStatus(team: Team): CapStatus {
  return { tone: STATUS_TONE[team.status] ?? 'off', label: teamStatusMap[team.status]?.label ?? team.status };
}

function teamFacts(team: Team): CapFact[] {
  const def = parseDefinition(team);
  const dept = departmentLabel(team);
  return [
    { label: t('shell.registry.mode'), value: teamModeLabel(def.mode) },
    { label: t('shell.registry.members'), value: def.members.filter((m) => m.enabled !== false).length },
    {
      label: t('shell.registry.department'),
      value: dept ? dept.split(' / ').pop()! : t('teamsHub.unassigned'),
      hint: dept || undefined,
    },
  ];
}

function teamTags(team: Team): CapTag[] {
  const tags: CapTag[] = [];
  const kind = kindLabel(team);
  if (kind) tags.push({ label: kind, tone: 'watch' });
  if (team.has_active_run) tags.push({ label: t('shell.registry.activeRun'), tone: 'running' });
  return tags;
}

const MAX_AVATARS = 5;
function memberAvatars(team: Team) {
  const members = [...parseDefinition(team).members]
    .sort((a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0))
    .map((m, i) => {
      const name = m.name || agentName(storeAgents.value, m.agent_id);
      return {
        key: `${m.agent_id}:${i}`,
        name,
        initial: name.trim().slice(0, 1).toUpperCase() || '?',
        enabled: m.enabled !== false,
      };
    });
  return {
    shown: members.slice(0, MAX_AVATARS),
    extra: Math.max(0, members.length - MAX_AVATARS),
    enabled: members.filter((m) => m.enabled).length,
  };
}

const inspectedLead = computed(() => {
  const team = inspector.team.value;
  const id = team ? departmentLeadId(team) : '';
  return (id && storeAgents.value.find((a) => a.id === id)) || null;
});

function kindLabel(team: Team) {
  if (team.readonly || team.kind === 'system_builtin') return t('teamsHub.kind.builtin');
  if (team.kind === 'ecosystem_preset') return t('teamsHub.kind.preset');
  if (String(team.spirit_session_id || '').trim()) return t('teamsHub.kind.orchestrated');
  return '';
}

// 与后端状态机一致：失败 / 取消 / 部分失败都能重置为待执行。
function canRetryTeam(team: Team) {
  return ['failed', 'cancelled', 'partial_failure'].includes(team.status) && !team.readonly;
}

// 后端拒删：系统内置、预设（要走行业包卸载）、默认团队、只读团队——菜单里就不给这个选项。
function canRemoveTeam(team: Team) {
  return !team.readonly && !team.is_default && team.kind !== 'system_builtin' && team.kind !== 'ecosystem_preset';
}

function openOrchestrate(team: Team) {
  void router.push({ name: 'team-orchestrate', params: { teamId: team.id } });
}

function openRun(team: Team, runId: string) {
  void router.push({ name: 'team-run-observatory', params: { teamId: team.id, runId } });
}
</script>
