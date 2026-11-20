<template>
  <!-- Harness › 编制 › Agent 管理：编制树 · 花名册 · 检视（审查与设计：docs/aranea-agents/review/2026-11-19-Agent管理审查与重设计.md） -->
  <q-page class="app-standard-page bz-page ar-page" :class="{ 'ar-page--inspecting': !!selectedAgent }">
    <Teleport defer to="#app-tab-actions">
      <q-btn
        flat
        dense
        round
        icon="refresh"
        :title="t('cap.common.refresh')"
        :aria-label="t('cap.common.refresh')"
        :loading="loading"
        @click="runLoadList"
      />
      <q-btn
        unelevated
        dense
        no-caps
        color="primary"
        icon="add"
        :label="t('agentRoster.create')"
        @click="openCreate()"
      />
    </Teleport>

    <AppStatePanel
      v-if="listError && !agents.length"
      class="bz-full"
      variant="error"
      :title="listError"
      :action-label="t('shell.common.retry')"
      action-icon="refresh"
      @action="runLoadList"
    />

    <template v-else>
      <OrgTreeRail
        :title="t('agentRoster.railTitle')"
        :total="agents.length"
        :entries="railEntries"
        :section-label="t('agentRoster.org')"
        :companies="railCompanies"
        :selected="scope"
        @select="(key) => setScope(key as RosterScope)"
      >
        <template #foot>
          <router-link :to="{ name: 'organization' }" class="bz-rail__link">
            <q-icon name="account_tree" size="16px" aria-hidden="true" />{{ t('agentRoster.manageOrg') }}
          </router-link>
        </template>
      </OrgTreeRail>

      <!-- ============ 中：花名册 ============ -->
      <section class="ar-panel ar-list">
        <header class="ar-list__head">
          <div class="ar-list__title">
            <span class="ar-list__name">{{ scopeTitle }}</span>
            <span class="ar-num ar-list__count">{{ scoped.length }}</span>
          </div>
        </header>

        <!-- 关键数 = 按角色筛选：点了只看这一类，再点（或点「在编」）取消 -->
        <CapStats
          class="ar-stats"
          :stats="roleStats"
          :label="t('agentRoster.filterAria')"
          :active="role || 'all'"
          @pick="pickRole"
        />

        <div class="cap-bar ar-list__bar" role="search">
          <q-input
            v-model="keyword"
            class="cap-bar__search"
            dense
            outlined
            clearable
            debounce="200"
            :placeholder="t('agentRoster.search')"
            :aria-label="t('agentRoster.search')"
          >
            <template #prepend><q-icon name="search" size="18px" /></template>
          </q-input>
          <span class="cap-bar__spacer" />
          <div v-if="statusTabs.length" class="cap-seg" role="group" :aria-label="t('agentRoster.statusAria')">
            <button
              v-for="s in statusTabs"
              :key="s.value"
              type="button"
              :aria-pressed="status === s.value"
              :class="{ 'ar-bad': s.value === 'failed' && s.count > 0 }"
              @click="status = status === s.value ? '' : s.value"
            >
              {{ s.label }} <span class="ar-num">{{ s.count }}</span>
            </button>
          </div>
        </div>

        <div class="ar-cols" aria-hidden="true">
          <span class="ar-row__check">
            <q-checkbox
              v-if="selectableCount"
              dense
              :model-value="allVisibleSelected ? true : selectedAgentIds.length ? null : false"
              :aria-label="t('agentRoster.selectAll')"
              @update:model-value="toggleSelectAllVisible"
            />
          </span>
          <span>{{ t('agentRoster.col.name') }}</span>
          <span>{{ t('agentRoster.col.position') }}</span>
          <span>{{ t('agentRoster.col.role') }}</span>
          <span>{{ t('agentRoster.col.model') }}</span>
          <span>{{ t('agentRoster.col.status') }}</span>
          <span>{{ t('agentRoster.col.run') }}</span>
          <span />
        </div>

        <div class="ar-list__body">
          <AppStatePanel v-if="loading && !agents.length" variant="loading" dense :rows="8" />
          <AppStatePanel
            v-else-if="!visible.length"
            variant="empty"
            dense
            :title="t('agentRoster.emptyTitle')"
            :description="t('agentRoster.emptyHint')"
            :action-label="hasFilters ? t('agentRoster.clearFilters') : undefined"
            @action="resetFilters"
          />
          <template v-else>
            <p v-if="truncated" class="ar-hint ar-trunc">{{ t('agentRoster.truncated', { n: rosterLimit }) }}</p>
            <section v-for="g in groups" :key="g.key" class="ar-grp">
              <div class="ar-grp__head">
                <span v-if="g.company && scopeNode?.level !== 'department'" class="ar-grp__co">{{ g.company }} ›</span>
                <span class="ar-grp__name">{{ groupTitle(g.key, g.title) }}</span>
                <span class="ar-grp__n">{{ t('agentRoster.groupCount', { n: g.agents.length }) }}</span>
                <span v-if="g.lead && g.lead.id !== g.agents[0]?.id" class="ar-grp__lead">
                  {{ t('agentRoster.groupLead', { name: g.lead.display_name }) }}
                </span>
              </div>
              <AgentRosterRow
                v-for="a in g.agents"
                :key="a.id"
                :agent="a"
                :role="rosterRole(a)"
                :position-name="positionName(a)"
                :active="selectedId === a.id"
                :selected="selectedAgentIds.includes(a.id)"
                :selectable="canToggleAgent(a)"
                :deletable="canDeleteAgent(a)"
                @open="selectAgent"
                @toggle-select="toggleAgentSelected"
                @toggle-favorite="toggleFavorite"
                @copy-key="copyAgentKey"
                @duplicate="duplicateListedAgent"
                @set-status="setAgentStatus"
                @delete="confirmDelete"
              />
            </section>
          </template>
        </div>

        <BulkActionBar
          v-if="selectedAgentIds.length > 0"
          class="ar-bulk"
          :count="selectedAgentIds.length"
          :actions="batchActions"
          :busy="batchBusy"
          :error="batchError"
          @run="onBatchRun"
          @clear="clearAgentSelection"
        />
      </section>

      <!-- ============ 右：检视（选中才出现，花名册平时占满宽度） ============ -->
      <aside v-if="selectedAgent" class="ar-panel ar-insp">
        <AgentRosterInspector
          :agent="selectedAgent"
          :role="rosterRole(selectedAgent)"
          :path="orgPath(orgIndex, selectedAgent.taxonomy_position_id)"
          :boss="reportsTo(rail, orgIndex, selectedAgent)"
          :reports="reportsOf(selectedAgent)"
          :reports-scope="reportsScopeOf(selectedAgent)"
          :deletable="canDeleteAgent(selectedAgent)"
          @close="selectAgent(null)"
          @open="selectAgent"
          @scope="setScope"
          @copy-key="copyAgentKey"
          @toggle-favorite="toggleFavorite"
          @duplicate="duplicateListedAgent"
          @set-status="setAgentStatus"
          @delete="confirmDelete"
        />
      </aside>
    </template>

    <agent-create-dialog
      v-model="createOpen"
      v-model:self-evolve="selfEvolve"
      v-model:agent-kind="agentKind"
      v-model:form="form"
      v-model:a2a-proxy="a2aProxy"
      :is-a2-a-proxy="isA2AProxyCreate"
      :taxonomy-tree="taxonomyTree"
      :provider-options="providerOptions"
      :model-options="modelOptions"
      :selected-template-key="selectedTemplateKey"
      :templates="createTemplates"
      :agent-key-error="agentKeyError"
      :display-name-error="displayNameError"
      :provider-model-error="providerModelError"
      :remote-url-error="remoteUrlError"
      :create-form-error="createFormError"
      :can-create="canCreate"
      :create-disabled-hint="createDisabledHint"
      :creating="creating"
      :checking-model="checkingModel"
      @apply-template="applyTemplate"
      @check-model="checkModel"
      @create-attempt="onCreateAttempt"
      @create="onCreate"
    />

    <AppConfirmDialog
      v-model="deleteOpen"
      :title="t('agentRoster.deleteTitle')"
      :message="t('agentRoster.deleteMessage', { name: deleteTarget?.display_name ?? '' })"
      :confirm-label="t('agentRoster.actions.delete')"
      @confirm="deleteAgentTarget"
    />
  </q-page>
</template>

<script setup lang="ts">
/** 路由容器：组装编制树 / 花名册 / 检视三栏；用例在 features/agents/useAgentsPage.ts，口径在 roster.ts。 */
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AgentCreateDialog from '../components/agents/AgentCreateDialog.vue';
import AgentRosterRow from '../components/agents/AgentRosterRow.vue';
import AgentRosterInspector from '../components/agents/AgentRosterInspector.vue';
import CapStats from '../components/capability/CapStats.vue';
import type { CapStat } from '../components/capability/capTypes';
import OrgTreeRail from '../components/organization/OrgTreeRail.vue';
import type { TreeRailCompany, TreeRailEntry } from '../components/organization/treeRail';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import AppConfirmDialog from '../components/common/AppConfirmDialog.vue';
import BulkActionBar from '../components/common/BulkActionBar.vue';
import type { BulkAction } from '../components/common/bulkAction';
import { useAgentsPage } from '../features/agents/useAgentsPage';
import {
  canDeleteAgent,
  canToggleAgent,
  orgPath,
  placementOf,
  reportsTo,
  rosterRole,
  type RosterRole,
  type RosterScope,
  type RosterStatus,
} from '../features/agents/roster';
import type { Agent } from '../features/agents/types';

const { t } = useI18n();

const {
  agents,
  truncated,
  rosterLimit,
  loading,
  listError,
  taxonomyTree,
  orgIndex,
  rail,
  scoped,
  visible,
  groups,
  roleCounts,
  statusCounts,
  scope,
  scopeNode,
  keyword,
  role,
  status,
  selectedId,
  selectedAgent,
  selectAgent,
  setScope,
  resetFilters,
  runLoadList,
  selectedAgentIds,
  deletableSelected,
  allVisibleSelected,
  toggleSelectAllVisible,
  toggleAgentSelected,
  clearAgentSelection,
  batchBusy,
  batchError,
  runBatch,
  setAgentStatus,
  toggleFavorite,
  copyAgentKey,
  duplicateListedAgent,
  deleteOpen,
  deleteTarget,
  confirmDelete,
  deleteAgentTarget,
  createOpen,
  creating,
  checkingModel,
  selfEvolve,
  agentKind,
  a2aProxy,
  isA2AProxyCreate,
  form,
  selectedTemplateKey,
  createTemplates,
  providerOptions,
  modelOptions,
  agentKeyError,
  displayNameError,
  providerModelError,
  remoteUrlError,
  createFormError,
  canCreate,
  createDisabledHint,
  openCreate,
  checkModel,
  onCreate,
  onCreateAttempt,
  applyTemplate,
} = useAgentsPage();

const scopeTitle = computed(() => {
  if (scopeNode.value) return scopeNode.value.name;
  if (['butlers', 'unassigned', 'favorites'].includes(scope.value)) return t(`agentRoster.${scope.value}`);
  return t('agentRoster.all');
});

// 关键数：在编 + 各角色人数（没有人的角色不占格，除非正按它筛）；点哪格按哪类筛
const roleStats = computed<Array<CapStat & { clickable?: boolean }>>(() => {
  const rows: Array<CapStat & { clickable?: boolean }> = [
    { key: 'all', label: t('agentRoster.role.all'), value: roleCounts.value.all, clickable: true },
  ];
  for (const r of ['company_lead', 'dept_lead', 'specialist', 'custom', 'butler'] as RosterRole[]) {
    if (roleCounts.value[r] || role.value === r)
      rows.push({
        key: r,
        label: t(`agentRoster.role.${r}`),
        value: roleCounts.value[r],
        hint: t(`agentRoster.roleHint.${r}`),
        clickable: true,
      });
  }
  return rows;
});

function pickRole(key: string) {
  role.value = key === 'all' || role.value === key ? '' : (key as RosterRole);
}

// ——— 编制树：固定入口（全部 / 收藏 / 管家 / 未挂岗）+ 公司 → 部门，数字是在编人数 ———
const railEntries = computed<TreeRailEntry[]>(() =>
  [
    { key: 'all', icon: 'groups', label: t('agentRoster.all'), count: agents.value.length },
    { key: 'favorites', icon: 'star', label: t('agentRoster.favorites'), count: rail.value.favorites.length },
    { key: 'butlers', icon: 'verified_user', label: t('agentRoster.butlers'), count: rail.value.butlers.length },
    { key: 'unassigned', icon: 'person_off', label: t('agentRoster.unassigned'), count: rail.value.unassigned.length },
  ].filter((e) => e.count > 0 || e.key === 'all' || e.key === 'butlers' || e.key === scope.value),
);
const railCompanies = computed<TreeRailCompany[]>(() =>
  rail.value.companies.map((c) => ({
    id: c.node.id,
    name: c.node.name,
    enabled: c.node.enabled,
    count: c.count,
    departments: c.departments.map((d) => ({
      id: d.node.id,
      name: d.node.name,
      enabled: d.node.enabled,
      count: d.count,
      tag: d.vacant.length ? t('agentRoster.vacant', { n: d.vacant.length }) : undefined,
      tagTitle: d.vacant.length ? t('agentRoster.vacantTitle', { n: d.vacant.length }) : undefined,
    })),
  })),
);

const statusTabs = computed(() => {
  const rows: Array<{ value: RosterStatus; label: string; count: number }> = [];
  for (const s of ['inactive', 'failed'] as const) {
    if (statusCounts.value[s] || status.value === s)
      rows.push({ value: s, label: t(`agentRoster.status.${s}`), count: statusCounts.value[s] });
  }
  return rows;
});

const hasFilters = computed(() => Boolean(keyword.value || role.value || status.value));
const selectableCount = computed(() => visible.value.filter(canToggleAgent).length);

function groupTitle(key: string, title: string) {
  if (key === 'butlers') return t('agentRoster.butlers');
  if (key === 'unassigned') return t('agentRoster.unassigned');
  return title;
}

function positionName(a: Agent) {
  const place = placementOf(orgIndex.value, a);
  return place.position?.name ?? (rosterRole(a) === 'dept_lead' ? (place.department?.name ?? '') : '');
}

/** 领导岗的下属人数（同部门 / 同公司在编减去本人） */
function reportsOf(a: Agent): number | null {
  const role = rosterRole(a);
  const place = placementOf(orgIndex.value, a);
  if (role === 'dept_lead' && place.department) {
    const d = rail.value.companies.flatMap((c) => c.departments).find((x) => x.node.id === place.department?.id);
    return d ? Math.max(0, d.count - 1) : null;
  }
  if (role === 'company_lead' && place.company) {
    const c = rail.value.companies.find((x) => x.node.id === place.company?.id);
    return c ? Math.max(0, c.count - 1) : null;
  }
  return null;
}

function reportsScopeOf(a: Agent): RosterScope {
  const place = placementOf(orgIndex.value, a);
  return (rosterRole(a) === 'company_lead' ? place.company?.id : place.department?.id) ?? 'all';
}

const batchActions = computed<BulkAction[]>(() => [
  { key: 'enable', label: t('agentRoster.batch.enable'), icon: 'play_circle', tone: 'primary' },
  { key: 'disable', label: t('agentRoster.batch.disable'), icon: 'pause_circle', tone: 'warn' },
  {
    key: 'delete',
    label: t('agentRoster.batch.delete'),
    icon: 'delete',
    tone: 'danger',
    requiresConfirm: true,
    confirmLabel: t('agentRoster.batch.deleteConfirm', { n: deletableSelected.value.length }),
    disabled: deletableSelected.value.length === 0,
    hint: deletableSelected.value.length === 0 ? t('agentRoster.batch.deleteNone') : undefined,
  },
]);

function onBatchRun(key: string) {
  if (key === 'enable' || key === 'disable' || key === 'delete') void runBatch(key);
}
</script>
