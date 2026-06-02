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
      v-model:industry-filter="industryFilter"
      class="q-mt-md"
      :industry-options="industryOptions"
      :loading="loading"
      :is-dark="isDark"
      @refresh="loadRows"
    />

    <q-banner v-if="error" rounded class="bg-negative text-white q-mt-md">
      {{ error }}
      <template #action><q-btn flat color="white" label="重试" @click="loadRows" /></template>
    </q-banner>

    <section v-for="group in teamIndustryGroups" :key="group.id" class="teams-industry-section q-mt-lg">
      <header class="teams-industry-section__head">
        <q-icon :name="group.id === '__builtin__' ? 'verified_user' : 'domain'" size="20px" color="primary" />
        <h2 class="teams-industry-section__title">{{ group.label }}</h2>
        <q-chip dense square size="sm" class="teams-industry-section__count">{{ group.teams.length }}</q-chip>
      </header>
      <draggable
        v-model="draggableTeamsMap[group.id]"
        item-key="id"
        class="teams-draggable-grid"
        ghost-class="team-card--ghost"
        chosen-class="team-card--chosen"
        drag-class="team-card--dragging"
        :animation="200"
        :delay="100"
      >
        <template #item="{ element: team }">
          <TeamCard
            :team="team"
            :agents="agents"
            :is-dark="isDark"
            @copy-key="copyKey"
            @open-runs="openRuns"
            @open-observatory="openTeamObservatory"
            @run-test="openRunTest"
            @duplicate="duplicate"
            @edit="openEdit"
            @remove="confirmRemove"
          />
        </template>
      </draggable>
    </section>

    <q-card v-if="!loading && teamIndustryGroups.length === 0" flat bordered :class="['app-entity-empty', { 'is-dark': isDark }, 'q-mt-lg']">
      <q-card-section class="column items-center text-center q-pa-xl">
        <q-avatar size="72px" color="primary" text-color="white" icon="hub" />
        <div class="text-h6 q-mt-md">暂无 Team</div>
        <div class="text-body2 app-text-secondary q-mt-sm">创建一个 Team，把多个 Agent 组织成顺序、并行或评审闭环。</div>
        <q-btn class="q-mt-md" color="primary" rounded unelevated icon="add" label="新增 Team" @click="openCreate" />
      </q-card-section>
    </q-card>

    <TeamEditorDialog
      v-model="editorOpen"
      v-model:selected-template-key="selectedTeamTemplateKey"
      :editing-id="editingId"
      :form="form"
      :definition="definition"
      :definition-json="definitionJSON"
      :agent-options="agentOptions"
      :industry-options="industryOptions"
      :saving="saving"
      :can-save="canSave"
      :is-dark="isDark"
      :is-platform-admin="isPlatformAdmin"
      @add-member="addMember"
      @remove-member="removeMember"
      @apply-template="applyTemplate"
      @save="save"
    />

    <TeamRunsDialog
      v-model="runsOpen"
      :selected-team="selectedTeam"
      :runs="runs"
      :steps-by-run="stepsByRun"
      :steps-loading="stepsLoading"
      :summaries-by-run="summariesByRun"
      :summaries-loading="summariesLoading"
      :agents="agents"
      :loading="runsLoading"
      :error="runsError"
      :live-connected="runEventsConnected"
      :live-replaying="runEventsReplaying"
      :is-dark="isDark"
      @refresh="loadRuns"
      @show-steps="loadRunSteps"
      @load-summary="loadRunSummary"
      @open-observatory="openRunObservatory"
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
import { computed, reactive, watchEffect, type WritableComputedRef } from "vue";
import draggable from "vuedraggable";
import AppPageHero from "../components/layout/AppPageHero.vue";
import TeamCard from "../components/teams/TeamCard.vue";
import TeamEditorDialog from "../components/teams/TeamEditorDialog.vue";
import TeamRunsDialog from "../components/teams/TeamRunsDialog.vue";
import TeamTestDialog from "../components/teams/TeamTestDialog.vue";
import TeamToolbar from "../components/teams/TeamToolbar.vue";
import { useTeamsPage } from "../features/teams/useTeamsPage";
import { storeToRefs } from "pinia";
import { useAuthStore } from "../stores/auth";
import type { Team } from "../features/teams/types";

const authStore = useAuthStore();
const { isPlatformAdmin } = storeToRefs(authStore);

const {
  isDark,
  rows,
  agents,
  loading,
  saving,
  error,
  search,
  modeFilter,
  statusFilter,
  industryFilter,
  industryOptions,
  teamIndustryGroups,
  editorOpen,
  selectedTeamTemplateKey,
  editingId,
  runsOpen,
  runsLoading,
  runsError,
  runEventsConnected,
  runEventsReplaying,
  selectedTeam,
  runs,
  stepsByRun,
  stepsLoading,
  summariesByRun,
  summariesLoading,
  testOpen,
  testTeam,
  testLoading,
  testError,
  testReply,
  testRun,
  form,
  definition,
  agentOptions,
  definitionJSON,
  canSave,
  loadRows,
  openCreate,
  openEdit,
  addMember,
  removeMember,
  applyTemplate,
  save,
  duplicate,
  confirmRemove,
  copyKey,
  openRuns,
  openRunTest,
  executeRunTest,
  loadRunSummary,
  openRunObservatory,
  openTeamObservatory,
  loadRuns,
  loadRunSteps,
  reorderTeams
} = useTeamsPage();

const draggableTeamsMap = reactive<Record<string, WritableComputedRef<Team[]>>>({});

watchEffect(() => {
  for (const group of teamIndustryGroups.value) {
    if (!draggableTeamsMap[group.id]) {
      draggableTeamsMap[group.id] = computed({
        get: () => teamIndustryGroups.value.find((g) => g.id === group.id)?.teams ?? [],
        set: (val: Team[]) => reorderTeams(val.map((t) => t.id)),
      });
    }
  }
});
</script>
