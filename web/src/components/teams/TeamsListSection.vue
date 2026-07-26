<template>
  <section v-for="group in groups" :key="group.id" class="teams-industry-section q-mt-lg">
    <header class="teams-industry-section__head">
      <q-icon name="domain" size="20px" color="primary" />
      <h2 class="teams-industry-section__title">{{ group.label }}</h2>
      <q-chip dense square size="sm" class="teams-industry-section__count">{{ group.teams.length }}</q-chip>
    </header>
    <draggable
      :list="groupTeamsList[group.id]"
      item-key="id"
      class="teams-draggable-grid"
      ghost-class="team-card--ghost"
      chosen-class="team-card--chosen"
      drag-class="team-card--dragging"
      :animation="200"
      :delay="100"
      :disabled="disabled"
      @change="onGroupChange(group.id)"
    >
      <template #item="{ element: team }">
        <TeamCard
          :team="team"
          :agents="agents"
          :is-dark="isDark"
          @open-runs="$emit('open-runs', $event)"
          @open-observatory="$emit('open-observatory', $event)"
          @run-test="$emit('run-test', $event)"
          @duplicate="$emit('duplicate', $event)"
          @edit="$emit('edit', $event)"
          @remove="$emit('remove', $event)"
        />
      </template>
    </draggable>
  </section>
</template>

<script setup lang="ts">
import { reactive, watch } from 'vue';
import draggable from 'vuedraggable';
import TeamCard from './TeamCard.vue';
import type { TeamIndustryGroup } from './teamUtils';
import type { Agent } from '../../features/agents/types';
import type { Team } from '../../features/teams/types';

const props = defineProps<{
  groups: TeamIndustryGroup[];
  agents: Agent[];
  isDark: boolean;
  disabled: boolean;
}>();

const emit = defineEmits<{
  'open-runs': [team: Team];
  'open-observatory': [team: Team];
  'run-test': [team: Team];
  duplicate: [team: Team];
  edit: [team: Team];
  remove: [team: Team];
  reorder: [ids: string[]];
}>();

const groupTeamsList = reactive<Record<string, Team[]>>({});

watch(
  () => props.groups.map((g) => ({ id: g.id, teamIds: g.teams.map((t) => t.id) })),
  (groupSnapshots) => {
    const currentKeys = new Set(groupSnapshots.map((g) => g.id));
    for (const key of Object.keys(groupTeamsList)) {
      if (!currentKeys.has(key)) {
        delete groupTeamsList[key];
      }
    }
    for (const snap of groupSnapshots) {
      const existing = groupTeamsList[snap.id];
      const incomingIds = snap.teamIds;
      if (!existing || existing.length !== incomingIds.length || existing.some((t, i) => t.id !== incomingIds[i])) {
        const group = props.groups.find((g) => g.id === snap.id);
        groupTeamsList[snap.id] = group ? group.teams.slice() : [];
      }
    }
  },
  { immediate: true },
);

function onGroupChange(groupId: string) {
  const list = groupTeamsList[groupId];
  if (list) {
    emit(
      'reorder',
      list.map((t) => t.id),
    );
  }
}
</script>
