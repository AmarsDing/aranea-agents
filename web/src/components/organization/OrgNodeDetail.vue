<template>
  <div class="org-detail">
    <div class="org-detail__tags">
      <span class="org-tag">{{ t(`orgChart.level.${node.level}`) }}</span>
      <span class="org-tag" :class="{ 'org-tag--gov': system }">{{
        t(system ? 'orgChart.system' : 'orgChart.custom')
      }}</span>
      <span class="org-tag" :class="{ 'org-tag--off': !node.enabled }">
        {{ t(node.enabled ? 'orgChart.enabled' : 'orgChart.disabledTag') }}
      </span>
    </div>
    <p class="org-detail__desc">{{ node.description || t('orgChart.inspector.noDescription') }}</p>

    <section v-if="node.level === 'position'" class="org-detail__section">
      <h3>{{ t('orgChart.inspector.seated') }}</h3>
      <div class="org-detail__agents">
        <OrgAgentChip v-for="a in agents" :key="a.id" :agent="a" @open="(x) => emit('open-agent', x)" />
        <router-link v-if="!agents.length" :to="{ name: 'agents' }" class="org-vacant">
          <q-icon name="person_add" size="16px" />{{ t('orgChart.vacantSeat') }}
        </router-link>
      </div>
    </section>
    <section v-else class="org-detail__section">
      <h3>{{ t(node.level === 'company' ? 'orgChart.companyLead' : 'orgChart.deptLead') }}</h3>
      <OrgAgentChip :agent="lead" :empty-label="t('orgChart.noLead')" @open="(x) => emit('open-agent', x)" />
      <p class="org-note">{{ t(node.level === 'company' ? 'orgChart.companyLeadHint' : 'orgChart.deptLeadHint') }}</p>
    </section>

    <dl class="registry-facts">
      <div v-if="parent">
        <dt>{{ t('orgChart.inspector.parent') }}</dt>
        <dd>{{ parent.name }}</dd>
      </div>
      <template v-if="counts">
        <div v-if="node.level === 'company'">
          <dt>{{ t('orgChart.departments') }}</dt>
          <dd class="org-mono">{{ counts.departments }}</dd>
        </div>
        <div>
          <dt>{{ t('orgChart.positions') }}</dt>
          <dd class="org-mono">{{ counts.positions }}</dd>
        </div>
        <div>
          <dt>{{ t('orgChart.vacant') }}</dt>
          <dd class="org-mono">{{ counts.vacant }}</dd>
        </div>
        <div>
          <dt>{{ t('orgChart.teams') }}</dt>
          <dd class="org-mono">{{ counts.teams }}</dd>
        </div>
      </template>
      <div>
        <dt>{{ t('orgChart.inspector.key') }}</dt>
        <dd class="org-mono">{{ node.key }}</dd>
      </div>
      <div v-if="node.updated_at">
        <dt>{{ t('orgChart.inspector.updatedAt') }}</dt>
        <dd class="org-mono">{{ formatDate(node.updated_at) }}</dd>
      </div>
    </dl>

    <section v-if="node.level === 'department'" class="org-detail__section">
      <h3>{{ t('orgChart.teams') }}</h3>
      <ul v-if="teams.length" class="org-team-list" role="list">
        <li v-for="team in teams" :key="team.id">
          <button type="button" class="org-team-row" @click="emit('open-teams', team.id)">
            <span class="org-team-row__name">{{ team.display_name }}</span>
            <AppStatusChip :status="team.status" />
          </button>
        </li>
      </ul>
      <p v-else class="org-note">{{ t('orgChart.noTeams') }}</p>
    </section>

    <div class="org-detail__actions">
      <q-btn
        unelevated
        no-caps
        color="primary"
        icon="edit"
        :label="t('orgChart.inspector.edit')"
        @click="emit('edit')"
      />
      <q-btn
        v-if="node.level === 'department'"
        outline
        no-caps
        color="primary"
        icon="groups"
        :label="t('orgChart.viewTeams')"
        @click="emit('open-teams')"
      />
      <OrgNodeMenu
        :node="node"
        :system="system"
        :can-up="canUp"
        :can-down="canDown"
        @edit="emit('edit')"
        @move="(d) => emit('move', d)"
        @toggle="emit('toggle')"
        @remove="emit('remove')"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppStatusChip from '../common/AppStatusChip.vue';
import OrgAgentChip from './OrgAgentChip.vue';
import OrgNodeMenu from './OrgNodeMenu.vue';
import { formatDate } from '../teams/teamUtils';
import type { Agent } from '../../features/agents/types';
import type { Team } from '../../features/teams/types';
import type { OrgCounts, OrgNode } from '../../features/organization/orgChart';

defineProps<{
  node: OrgNode;
  parent: OrgNode | null;
  agents: Agent[];
  lead: Agent | null;
  counts: OrgCounts | null;
  teams: Team[];
  system: boolean;
  canUp: boolean;
  canDown: boolean;
}>();
const emit = defineEmits<{
  edit: [];
  move: [delta: -1 | 1];
  toggle: [];
  remove: [];
  'open-agent': [agent: Agent];
  'open-teams': [teamId?: string];
}>();
const { t } = useI18n();
</script>
