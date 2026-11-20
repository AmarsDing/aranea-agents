<template>
  <!-- Harness › 编制 › 组织架构：编制树 | 公司概览（部门卡）/ 部门（岗位卡 + 团队）| 检视。记录：docs/aranea-agents/design/2026-11-17-三大模块重设计/11-组织架构与团队.md -->
  <q-page class="app-standard-page bz-page" :class="{ 'bz-page--inspecting': inspectorOpen }">
    <Teleport defer to="#app-tab-actions">
      <q-btn
        flat
        dense
        round
        icon="refresh"
        :title="t('cap.common.refresh')"
        :aria-label="t('cap.common.refresh')"
        :loading="loading"
        @click="load()"
      />
      <q-btn
        unelevated
        dense
        no-caps
        color="primary"
        icon="add"
        :label="t('orgChart.newCompany')"
        @click="openCreate('company', null)"
      />
    </Teleport>

    <AppStatePanel v-if="forbidden" class="bz-full" variant="forbidden" />
    <AppStatePanel
      v-else-if="loadError && !companies.length"
      class="bz-full"
      variant="error"
      :description="loadError"
      :action-label="t('shell.common.retry')"
      action-icon="refresh"
      @action="load()"
    />
    <AppStatePanel v-else-if="loading && !companies.length" class="bz-full" variant="loading" :rows="6" />

    <template v-else>
      <OrgTreeRail
        :title="t('orgChart.title')"
        :total="companies.length"
        :companies="railCompanies"
        :selected="selectedKey"
        :empty-dept-text="keyword ? t('orgChart.noMatchShort') : t('orgChart.emptyDepts')"
        @select="onRailSelect"
      >
        <template #foot>
          <router-link :to="{ name: 'agents' }" class="bz-rail__link">
            <q-icon name="smart_toy" size="16px" aria-hidden="true" />{{ t('orgChart.toAgents') }}
          </router-link>
          <router-link :to="{ name: 'team' }" class="bz-rail__link">
            <q-icon name="groups" size="16px" aria-hidden="true" />{{ t('orgChart.toTeams') }}
          </router-link>
        </template>
      </OrgTreeRail>

      <section class="bz-panel bz-main" :aria-label="current?.name || t('orgChart.title')">
        <AppStatePanel
          v-if="loadError || sideError"
          variant="partial"
          dense
          :description="loadError || sideError"
          :action-label="t('shell.common.retry')"
          action-icon="refresh"
          @action="load()"
        />

        <AppStatePanel
          v-if="!company"
          variant="empty"
          icon="account_tree"
          :title="t('orgChart.emptyTitle')"
          :description="t('orgChart.emptyBody')"
          :action-label="t('orgChart.newCompany')"
          action-icon="add"
          @action="openCreate('company', null)"
        />

        <template v-else-if="current">
          <!-- 页头：公司概览时是公司；进了部门时上面一行可以点回公司 -->
          <header class="bz-head">
            <div class="bz-head__main">
              <div v-if="dept" class="bz-crumb">
                <button type="button" class="bz-crumb__link" @click="selectCompany(company.id)">
                  {{ company.name }}
                </button>
                <q-icon name="chevron_right" size="14px" aria-hidden="true" />
              </div>
              <div class="org-heading">
                <h2 class="org-heading__title">{{ current.name }}</h2>
                <span v-if="deptIsOffice" class="org-tag org-tag--gov">{{ t('orgChart.office') }}</span>
                <span class="org-tag" :class="{ 'org-tag--gov': isSystem(current) }">
                  {{ t(isSystem(current) ? 'orgChart.system' : 'orgChart.custom') }}
                </span>
                <span v-if="!current.enabled" class="org-tag org-tag--off">{{ t('orgChart.disabledTag') }}</span>
              </div>
              <p class="bz-head__desc">{{ current.description || t('orgChart.inspector.noDescription') }}</p>
            </div>
            <div v-if="!deptIsOffice" class="bz-head__lead">
              <span class="org-label">{{ t(dept ? 'orgChart.deptLead' : 'orgChart.companyLead') }}</span>
              <OrgAgentChip
                :agent="dept ? deptLead : companyLead"
                :empty-label="t('orgChart.noLead')"
                @open="openAgent"
              />
            </div>
            <div class="bz-head__actions">
              <q-btn
                outline
                dense
                no-caps
                color="primary"
                icon="add"
                :label="t(dept ? 'orgChart.newPosition' : 'orgChart.newDept')"
                @click="dept ? openCreate('position', dept) : openCreate('department', company)"
              />
              <q-btn
                flat
                dense
                round
                icon="info_outline"
                :title="t('orgChart.details')"
                :aria-label="t('orgChart.details')"
                @click="inspect(current)"
              />
              <OrgNodeMenu
                :node="current"
                :system="isSystem(current)"
                :can-up="canMove(current, -1)"
                :can-down="canMove(current, 1)"
                @edit="openEdit(current)"
                @move="(d) => move(current!, d)"
                @toggle="toggleEnabled(current)"
                @remove="remove(current)"
              />
            </div>
          </header>

          <CapStats :stats="stats" :label="t('orgChart.statsLabel')" :active="statFilter" @pick="toggleStatFilter" />

          <div class="cap-bar" role="search">
            <q-input
              v-model="keyword"
              class="cap-bar__search"
              dense
              outlined
              clearable
              debounce="150"
              :placeholder="t('orgChart.searchPlaceholder')"
              :aria-label="t('orgChart.searchPlaceholder')"
            >
              <template #prepend><q-icon name="search" size="18px" /></template>
            </q-input>
            <span class="cap-bar__spacer" />
            <span class="cap-bar__meta">{{ shownLabel }}</span>
          </div>

          <div class="bz-scroll">
            <!-- 公司概览：一个部门一张卡，点进部门 -->
            <template v-if="!dept">
              <div v-if="overviewDepts.length" class="cap-grid">
                <CapCard
                  v-for="d in overviewDepts"
                  :key="d.node.id"
                  :icon="d.office ? 'shield' : 'lan'"
                  :code="d.node.key"
                  :title="d.node.name"
                  :description="d.node.description"
                  :status="deptStatus(d)"
                  :facts="deptFacts(d)"
                  :tags="d.office ? [{ label: t('orgChart.office'), tone: 'watch' }] : []"
                  @open="selectDept(d.node.id)"
                >
                  <template #relation>
                    <!-- 主管 Agent 的名字本身就带角色（「部门主管-xx部」），不再加标签 -->
                    <span class="org-relation" :title="deptLeadName(d)">
                      <q-icon name="shield_person" size="14px" aria-hidden="true" />
                      <span class="org-relation__who">{{ deptLeadName(d) }}</span>
                    </span>
                  </template>
                  <template #actions>
                    <q-btn
                      flat
                      dense
                      no-caps
                      color="primary"
                      icon-right="chevron_right"
                      :label="t('orgChart.enter')"
                      @click="selectDept(d.node.id)"
                    />
                    <OrgNodeMenu
                      :node="d.node"
                      :system="isSystem(d.node)"
                      :can-up="!keyword && canMove(d.node, -1)"
                      :can-down="!keyword && canMove(d.node, 1)"
                      @edit="openEdit(d.node)"
                      @move="(delta) => move(d.node, delta)"
                      @toggle="toggleEnabled(d.node)"
                      @remove="remove(d.node)"
                    />
                  </template>
                </CapCard>
              </div>
              <div v-else class="org-empty">
                <span>{{ keyword || statFilter ? t('orgChart.noMatchShort') : t('orgChart.emptyDepts') }}</span>
                <q-btn
                  v-if="!keyword && !statFilter"
                  flat
                  dense
                  no-caps
                  color="primary"
                  icon="add"
                  :label="t('orgChart.newDept')"
                  @click="openCreate('department', company)"
                />
              </div>
            </template>

            <!-- 部门：一个岗位一张卡（坐岗 Agent 的 emoji、模型、最近运行），下面是本部门的团队 -->
            <template v-else>
              <div v-if="positions.length" class="cap-grid">
                <CapCard
                  v-for="item in positions"
                  :key="item.node.id"
                  :icon="item.agents.length ? 'person' : 'person_add_alt'"
                  :glyph="agentGlyph(item.agents[0])"
                  :code="item.agents[0]?.agent_key || item.node.key"
                  :title="item.node.name"
                  :description="item.node.description"
                  :status="positionStatus(item.node, item.agents)"
                  :facts="positionFacts(item.agents)"
                  :tags="
                    item.agents.length > 1
                      ? [{ label: t('orgChart.multiSeat', { n: item.agents.length }), tone: 'watch' }]
                      : []
                  "
                  :selected="inspected?.id === item.node.id"
                  class="org-pos-card"
                  @open="inspect(item.node)"
                >
                  <template #relation>
                    <span v-if="item.agents.length" class="org-relation" :title="item.agents[0].display_name">
                      <q-icon name="smart_toy" size="14px" aria-hidden="true" />
                      <span class="org-relation__who">{{ item.agents.map((a) => a.display_name).join('、') }}</span>
                    </span>
                    <router-link v-else :to="{ name: 'agents' }" class="org-vacant" @click.stop>
                      <q-icon name="person_add" size="14px" />{{ t('orgChart.vacantSeat') }}
                    </router-link>
                  </template>
                  <template #actions>
                    <q-btn
                      v-if="item.agents.length"
                      flat
                      dense
                      round
                      size="sm"
                      color="primary"
                      icon="open_in_new"
                      :title="t('orgChart.openAgent')"
                      :aria-label="t('orgChart.openAgent')"
                      @click="openAgent(item.agents[0])"
                    />
                    <OrgNodeMenu
                      :node="item.node"
                      :system="isSystem(item.node)"
                      :can-up="!keyword && !statFilter && canMove(item.node, -1)"
                      :can-down="!keyword && !statFilter && canMove(item.node, 1)"
                      @edit="openEdit(item.node)"
                      @move="(delta) => move(item.node, delta)"
                      @toggle="toggleEnabled(item.node)"
                      @remove="remove(item.node)"
                    />
                  </template>
                </CapCard>
              </div>
              <div v-else class="org-empty">
                <span>{{
                  keyword
                    ? t('orgChart.noMatch', { q: keyword })
                    : statFilter
                      ? t('orgChart.noMatchShort')
                      : t('orgChart.noPositions')
                }}</span>
                <q-btn
                  v-if="!keyword && !statFilter"
                  flat
                  dense
                  no-caps
                  color="primary"
                  icon="add"
                  :label="t('orgChart.newPosition')"
                  @click="openCreate('position', dept)"
                />
              </div>

              <div class="bz-sec">
                <h3 class="bz-sec__title">
                  {{ t('orgChart.teams') }} <span class="bz-num org-label">{{ deptTeams.length }}</span>
                </h3>
                <q-btn
                  flat
                  dense
                  no-caps
                  color="primary"
                  icon="groups"
                  :label="t('orgChart.viewTeams')"
                  @click="openTeams(dept)"
                />
              </div>
              <div v-if="deptTeams.length" class="org-team-strip">
                <button
                  v-for="team in deptTeams"
                  :key="team.id"
                  type="button"
                  class="org-team-pill"
                  @click="openTeams(dept, team.id)"
                >
                  <q-icon name="hub" size="16px" />
                  <span class="org-team-pill__name">{{ team.display_name }}</span>
                  <span class="org-team-pill__meta">{{ teamModeLabel(parseDefinition(team).mode) }}</span>
                </button>
              </div>
              <p v-else class="org-note">{{ t('orgChart.noTeams') }}</p>
            </template>
          </div>
        </template>
      </section>

      <AppRegistryInspector v-model="inspectorOpen" :title="inspectorTitle" :eyebrow="inspectorEyebrow">
        <OrgNodeForm
          v-if="inspector.mode === 'edit' || inspector.mode === 'create'"
          v-model:form="form"
          :level="inspectedLevel"
          :parent="inspectorParent"
          :node-id="inspected?.id ?? ''"
          :node-key="inspected?.key ?? ''"
          :system="inspected ? isSystem(inspected) : false"
          :saving="saving"
          :refine-fn="refinePromptField"
          @save="save"
          @cancel="inspected ? inspect(inspected) : closeInspector()"
          @refine-error="(m: string) => $q.notify({ type: 'negative', message: m })"
        />
        <OrgNodeDetail
          v-else-if="inspected"
          :node="inspected"
          :parent="inspectorParent"
          :agents="inspectedAgents"
          :lead="inspectedLead"
          :counts="inspectedCounts"
          :teams="inspected.level === 'department' ? (byDept.get(inspected.id) ?? []) : []"
          :system="isSystem(inspected)"
          :can-up="canMove(inspected, -1)"
          :can-down="canMove(inspected, 1)"
          @edit="openEdit(inspected)"
          @move="(d) => move(inspected!, d)"
          @toggle="toggleEnabled(inspected)"
          @remove="remove(inspected)"
          @open-agent="openAgent"
          @open-teams="(teamId?: string) => openTeams(inspected!, teamId)"
        />
      </AppRegistryInspector>
    </template>
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';
import AppRegistryInspector from '../components/layout/AppRegistryInspector.vue';
import AppStatePanel from '../components/common/AppStatePanel.vue';
import CapCard from '../components/capability/CapCard.vue';
import CapStats from '../components/capability/CapStats.vue';
import type { CapFact, CapStat, CapStatus } from '../components/capability/capTypes';
import OrgAgentChip from '../components/organization/OrgAgentChip.vue';
import OrgNodeDetail from '../components/organization/OrgNodeDetail.vue';
import OrgNodeForm from '../components/organization/OrgNodeForm.vue';
import OrgNodeMenu from '../components/organization/OrgNodeMenu.vue';
import OrgTreeRail from '../components/organization/OrgTreeRail.vue';
import { parseDefinition, teamModeLabel } from '../components/teams/teamUtils';
import { refinePromptField } from '../features/agents/aiRefine';
import type { Agent } from '../features/agents/types';
import {
  agentGlyph,
  companyCounts,
  companyLeadOf,
  deptCounts,
  deptLeadOf,
  positionsOf,
  type OrgCounts,
  type OrgNode,
} from '../features/organization/orgChart';
import { useOrgChart } from '../features/organization/useOrgChart';

const $q = useQuasar();
const { t } = useI18n();

const {
  loading,
  loadError,
  forbidden,
  sideError,
  saving,
  keyword,
  companies,
  company,
  companyLead,
  companyStats,
  departments,
  dept,
  deptIsOffice,
  deptLead,
  deptTeams,
  positions,
  positionTotal,
  railCompanies,
  selectedKey,
  statFilter,
  toggleStatFilter,
  overviewDepts,
  inspector,
  inspected,
  inspectedAgents,
  inspectedLevel,
  inspectorParent,
  form,
  byDept,
  byNode,
  agents,
  load,
  selectCompany,
  selectDept,
  onRailSelect,
  inspect,
  openEdit,
  openCreate,
  closeInspector,
  save,
  toggleEnabled,
  canMove,
  move,
  remove,
  openAgent,
  openTeams,
  isSystem,
} = useOrgChart();

/** 页头说的是谁：进了部门是部门，否则是公司 */
const current = computed<OrgNode | null>(() => dept.value ?? company.value);

// 关键数：概览按公司算、筛部门；部门里按部门算、筛岗位。空缺、已停用可以点来筛选
const stats = computed<Array<CapStat & { clickable?: boolean }>>(() => {
  const counts: OrgCounts | null = dept.value ? deptCounts(dept.value, byNode.value, byDept.value) : companyStats.value;
  if (!counts) return [];
  const off = dept.value
    ? positionsOf(dept.value).filter((p) => !p.enabled).length
    : departments.value.filter((d) => !d.node.enabled).length;
  return [
    ...(dept.value ? [] : [{ key: 'departments', label: t('orgChart.departments'), value: counts.departments }]),
    { key: 'positions', label: t('orgChart.positions'), value: counts.positions },
    { key: 'agents', label: t('orgChart.onDuty'), value: counts.agents },
    {
      key: 'vacant',
      label: t(dept.value ? 'orgChart.vacant' : 'orgChart.vacantDepts'),
      value: dept.value ? counts.vacant : departments.value.filter((d) => d.counts.vacant > 0).length,
      tone: counts.vacant ? 'watch' : undefined,
      clickable: true,
    },
    { key: 'off', label: t('orgChart.disabled'), value: off, tone: off ? 'off' : undefined, clickable: true },
    { key: 'teams', label: t('orgChart.teams'), value: counts.teams },
  ];
});

const shownLabel = computed(() =>
  dept.value
    ? t('orgChart.shownPositions', { n: positions.value.length, total: positionTotal.value })
    : t('orgChart.shownDepts', { n: overviewDepts.value.length, total: departments.value.length }),
);

type DeptRow = { node: OrgNode; office: boolean; counts: OrgCounts };

function deptStatus(d: DeptRow): CapStatus {
  if (!d.node.enabled) return { tone: 'off', label: t('orgChart.disabledTag') };
  if (d.counts.vacant) return { tone: 'watch', label: t('orgChart.vacantShort', { n: d.counts.vacant }) };
  return { tone: 'ok', label: t('orgChart.fullyStaffed') };
}

function deptFacts(d: DeptRow): CapFact[] {
  return [
    { label: t('orgChart.positions'), value: d.counts.positions },
    { label: t('orgChart.onDuty'), value: d.counts.agents },
    { label: t('orgChart.teams'), value: d.counts.teams },
  ];
}

function deptLeadName(d: DeptRow) {
  const lead =
    d.office && company.value
      ? companyLeadOf(company.value, agents.value)
      : deptLeadOf(d.node, agents.value, byNode.value);
  return lead?.display_name || t('orgChart.noLead');
}

function positionStatus(node: OrgNode, seated: Agent[]): CapStatus {
  if (!node.enabled) return { tone: 'off', label: t('orgChart.disabledTag') };
  if (!seated.length) return { tone: 'watch', label: t('orgChart.vacant') };
  return seated[0].status === 'active'
    ? { tone: 'ok', label: t('orgChart.agentStatus.active') }
    : { tone: 'off', label: t('orgChart.agentStatus.inactive') };
}

function shortTime(iso: string) {
  const d = new Date(iso);
  return Number.isNaN(d.getTime())
    ? '—'
    : d.toLocaleString(undefined, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
}

function positionFacts(seated: Agent[]): CapFact[] {
  const a = seated[0];
  if (!a) return [];
  // 坐岗人的名字放在卡片底部（多半和岗位同名，放进事实栏只会被截断）
  return [
    {
      label: t('orgChart.fact.model'),
      value: a.model || '—',
      hint: a.provider ? `${a.provider} / ${a.model}` : undefined,
    },
    {
      label: t('orgChart.fact.lastRun'),
      value: a.last_run_at ? shortTime(a.last_run_at) : t('orgChart.neverRun'),
      tone: a.last_run_status === 'failed' ? 'alert' : undefined,
    },
  ];
}

const inspectorOpen = computed({
  get: () => inspector.value.mode !== 'closed',
  set: (open: boolean) => {
    if (!open) closeInspector();
  },
});

const inspectorTitle = computed(() => {
  const ins = inspector.value;
  const level = t(`orgChart.level.${inspectedLevel.value}`);
  if (ins.mode === 'create') return t('orgChart.inspector.createTitle', { level });
  if (ins.mode === 'edit') return t('orgChart.inspector.editTitle', { level });
  return inspected.value?.name ?? '';
});

// 眉标只写上级（标题已经是节点名，不重复）。
const inspectorEyebrow = computed(() => inspectorParent.value?.name || undefined);

const inspectedLead = computed(() => {
  const node = inspected.value;
  if (!node) return null;
  if (node.level === 'company') return companyLeadOf(node, agents.value);
  if (node.level === 'department') return deptLeadOf(node, agents.value, byNode.value);
  return null;
});

const inspectedCounts = computed(() => {
  const node = inspected.value;
  if (!node || node.level === 'position') return null;
  return node.level === 'company'
    ? companyCounts(node, byNode.value, byDept.value)
    : deptCounts(node, byNode.value, byDept.value);
});
</script>
