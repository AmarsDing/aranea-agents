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

    <q-banner v-if="error" rounded class="bg-negative text-white q-mt-md">
      {{ error }}
      <template #action><q-btn flat color="white" label="重试" @click="loadRows" /></template>
    </q-banner>

    <section v-for="group in teamGroups" :key="group.id" class="teams-industry-section q-mt-lg">
      <header class="teams-industry-section__head">
        <q-icon :name="group.id === '__builtin__' ? 'verified_user' : 'domain'" size="20px" color="primary" />
        <h2 class="teams-industry-section__title">{{ group.label }}</h2>
        <q-chip dense square size="sm" class="teams-industry-section__count">{{ group.teams.length }}</q-chip>
      </header>
      <div class="teams-grid">
        <TeamCard
          v-for="team in group.teams"
          :key="team.id"
          :team="team"
          :agents="storeAgents"
          :is-dark="isDark"
          @open-runs="openRuns"
          @open-observatory="openTeamObservatory"
          @run-test="openRunTest"
          @duplicate="duplicate"
          @edit="openEdit"
          @remove="confirmRemove"
          @retry="retryTeam"
        />
      </div>
    </section>

    <q-card
      v-if="!loading && teamGroups.length === 0"
      flat
      bordered
      :class="['app-entity-empty', { 'is-dark': isDark }, 'q-mt-lg']"
    >
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-avatar size="72px" color="primary" text-color="white" icon="hub" />
        <div class="text-h6 q-mt-md">暂无 Team</div>
        <div class="text-body2 app-text-secondary q-mt-sm">
          创建一个 Team，把多个 Agent 组织成顺序、并行或评审闭环。
        </div>
        <div v-if="storeAgents.length === 0" class="text-body2 app-text-secondary q-mt-sm">
          你还没有可用 Agent——Team 由 Agent 组成，请先到 Agent 页创建。
        </div>
        <div
          v-if="hiddenOrchestratedCount > 0"
          class="q-mt-md row items-center no-wrap rounded-borders q-px-sm q-py-xs text-caption app-text-secondary"
          style="border: 1px dashed var(--q-primary)"
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
        <q-btn class="q-mt-md" color="primary" rounded unelevated icon="add" label="新增 Team" @click="openCreate" />
      </q-card-section>
    </q-card>

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
import AppPageHero from '../components/layout/AppPageHero.vue';
import TeamCard from '../components/teams/TeamCard.vue';
import TeamEditorDialog from '../components/teams/TeamEditorDialog.vue';
import TeamRunsDialog from '../components/teams/TeamRunsDialog.vue';
import TeamTestDialog from '../components/teams/TeamTestDialog.vue';
import TeamToolbar from '../components/teams/TeamToolbar.vue';
import { useTeamsPage } from '../features/teams/useTeamsPage';
import { useTeamsStore } from '../stores/teams';
import { storeToRefs } from 'pinia';
import { useI18n } from 'vue-i18n';

const { t } = useI18n();
const teamsStore = useTeamsStore();
const { agents: storeAgents } = storeToRefs(teamsStore);

const {
  isDark,
  loading,
  saving,
  error,
  search,
  modeFilter,
  statusFilter,
  departmentFilter,
  showOrchestrated,
  hiddenOrchestratedCount,
  departmentOptions,
  teamGroups,
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
</script>
