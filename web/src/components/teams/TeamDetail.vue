<template>
  <div class="org-detail">
    <div class="org-detail__tags">
      <AppStatusChip :status="team.status" />
      <span v-if="kindLabel" class="org-tag org-tag--gov">{{ kindLabel }}</span>
      <span v-if="team.has_active_run" class="org-tag">
        <span class="org-dot org-dot--run q-mr-xs" aria-hidden="true" />{{ t('shell.registry.activeRun') }}
      </span>
    </div>
    <p class="org-detail__desc">{{ definition.description || t('shell.registry.noDescription') }}</p>

    <div class="org-detail__actions">
      <q-btn
        unelevated
        no-caps
        color="primary"
        icon="play_arrow"
        :label="t('teamsHub.tryRun')"
        :disable="!definition.members.some((m) => m.enabled !== false)"
        @click="emit('run-test')"
      />
      <q-btn outline no-caps color="primary" icon="edit" :label="t('shell.registry.edit')" @click="emit('edit')" />
      <q-btn
        flat
        no-caps
        color="primary"
        icon="account_tree"
        :label="t('teamsHub.orchestrate')"
        @click="emit('orchestrate')"
      />
    </div>

    <section class="org-detail__section">
      <h3>{{ t('teamsHub.structure') }}</h3>
      <TeamStructure :definition="definition" :agents="agents" />
    </section>

    <dl class="registry-facts">
      <div>
        <dt>{{ t('shell.registry.department') }}</dt>
        <dd>{{ departmentLabel || t('teamsHub.unassigned') }}</dd>
      </div>
      <div v-if="lead">
        <dt>{{ t('orgChart.deptLead') }}</dt>
        <dd><OrgAgentChip :agent="lead" compact @open="(a) => emit('open-agent', a)" /></dd>
      </div>
      <div>
        <dt>{{ t('shell.registry.identifier') }}</dt>
        <dd class="org-mono">{{ team.team_key }}</dd>
      </div>
      <div v-if="definition.linked_graph_id">
        <dt>{{ t('teamsHub.linkedGraph') }}</dt>
        <dd class="org-mono">{{ definition.linked_graph_id }}</dd>
      </div>
      <div>
        <dt>{{ t('shell.registry.updatedAt') }}</dt>
        <dd class="org-mono">{{ formatDate(team.updated_at) }}</dd>
      </div>
    </dl>

    <section class="org-detail__section">
      <h3>{{ t('teamsHub.recentRuns') }}</h3>
      <AppStatePanel v-if="runsLoading" variant="loading" dense :rows="2" />
      <p v-else-if="runsError" class="org-note" role="alert">{{ runsError }}</p>
      <ul v-else-if="runs.length" class="tm-runs" role="list">
        <li v-for="run in runs" :key="run.id">
          <button type="button" class="tm-run org-team-row" @click="emit('open-run', run.id)">
            <AppStatusChip :status="run.status" />
            <span class="tm-run__preview">{{ run.input_preview || run.id }}</span>
            <span class="tm-run__meta org-mono">{{ formatDate(run.created_at) }}</span>
          </button>
        </li>
      </ul>
      <p v-else class="org-note">{{ t('teamsHub.noRuns') }}</p>
      <q-btn
        flat
        dense
        no-caps
        color="primary"
        icon="format_list_bulleted"
        :label="t('teamsHub.allRuns')"
        class="self-start"
        @click="emit('runs')"
      />
    </section>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppStatePanel from '../common/AppStatePanel.vue';
import AppStatusChip from '../common/AppStatusChip.vue';
import OrgAgentChip from '../organization/OrgAgentChip.vue';
import TeamStructure from './TeamStructure.vue';
import { formatDate } from './teamUtils';
import type { Agent } from '../../features/agents/types';
import type { Team, TeamDefinition, TeamRun } from '../../features/teams/types';

defineProps<{
  team: Team;
  definition: TeamDefinition;
  agents: Agent[];
  departmentLabel: string;
  lead: Agent | null;
  kindLabel: string;
  runs: TeamRun[];
  runsLoading: boolean;
  runsError: string;
}>();
const emit = defineEmits<{
  'run-test': [];
  edit: [];
  runs: [];
  orchestrate: [];
  'open-agent': [agent: Agent];
  'open-run': [runId: string];
}>();
const { t } = useI18n();
</script>
