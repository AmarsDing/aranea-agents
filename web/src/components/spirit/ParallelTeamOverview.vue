<template>
  <div class="parallel-team-overview">
    <div class="row items-center q-gutter-sm q-mb-sm">
      <div class="parallel-team-overview__icon">
        <q-icon name="hub" size="18px" />
      </div>
      <div class="col min-width-0">
        <div class="parallel-team-overview__title">{{ t('spirit.parallelTeams') }}</div>
      </div>
      <div class="parallel-team-overview__stats row items-center q-gutter-md">
        <span class="parallel-team-overview__stat">
          <span class="parallel-team-overview__stat-value">{{ activeCount }}</span>
          <span class="parallel-team-overview__stat-label">{{ t('spirit.inProgress') }}</span>
        </span>
        <span class="parallel-team-overview__stat">
          <span class="parallel-team-overview__stat-value">{{ completedCount }}</span>
          <span class="parallel-team-overview__stat-label">{{ t('spirit.completed') }}</span>
        </span>
      </div>
    </div>

    <div v-if="maxParallel > 0" class="parallel-team-overview__quota q-mb-sm">
      <q-linear-progress
        :value="activeCount / maxParallel"
        size="4px"
        rounded
        :color="activeCount >= maxParallel ? 'negative' : 'accent'"      />
      <div class="text-caption text-grey q-mt-xs">{{ t('spirit.parallelQuota', { active: activeCount, max: maxParallel }) }}</div>
    </div>

    <div v-if="allCompleted" class="parallel-team-overview__all-done q-mb-sm">
      <q-icon name="check_circle" size="16px" color="positive" class="q-mr-xs" />
      <span class="text-caption">{{ t('spirit.allTeamsCompleted') }}</span>
      <span v-if="completionStats" class="text-caption text-grey q-ml-sm">
        ({{ completionStats.completedTeams }}/{{ completionStats.totalTeams }} {{ t('spirit.success') }}<span
          v-if="completionStats.failedTeams > 0"
          >, {{ completionStats.failedTeams }} {{ t('spirit.failed') }}</span
        >)
      </span>
    </div>

    <DAGDiagramCard v-if="hasDagTeams" :teams="teams" class="q-mb-sm" />

    <SynthesisResultCard
      v-if="synthesisResult"
      :result="synthesisResult"
      :rendered-content="renderChatMarkdown(synthesisResult.content)"
      :evolution-suggestion="evolutionSuggestion"
      class="q-mb-sm"
    />

    <div class="parallel-team-overview__cards">
      <TeamProgressCard
        v-for="team in teams"
        :key="team.id"
        :team="team"
        @click="$emit('select-team', team.id)"
        @cancel="$emit('cancel-team', team.id)"
        @retry="$emit('retry-team', team.id)"
        @archive="$emit('archive-team', team.id)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import TeamProgressCard from './TeamProgressCard.vue';
import SynthesisResultCard from './SynthesisResultCard.vue';
import DAGDiagramCard from './DAGDiagramCard.vue';
import { renderChatMarkdown } from '../../features/chat/chatMessageMarkdown';
import type { SpiritTeam, SynthesisOutput, EvolutionSuggestion, CompletionStats } from '../../features/spirit/types';

const { t } = useI18n();

const props = defineProps<{
  teams: SpiritTeam[];
  maxParallel: number;
  allCompleted: boolean;
  completionStats?: CompletionStats | null;
  synthesisResult?: SynthesisOutput | null;
  evolutionSuggestion?: EvolutionSuggestion | null;
}>();

defineEmits<{
  'select-team': [teamId: string];
  'cancel-team': [teamId: string];
  'retry-team': [teamId: string];
  'archive-team': [teamId: string];
}>();

const activeCount = computed(
  () =>
    props.teams.filter(
      (t) => t.status !== 'completed' && t.status !== 'failed' && t.status !== 'cancelled' && t.status !== 'archived',
    ).length,
);

const completedCount = computed(() => props.teams.filter((t) => t.status === 'completed').length);

const hasDagTeams = computed(() => props.teams.some((t) => t.dagNodeId || (t.dependsOn && t.dependsOn.length > 0)));
</script>

<style scoped lang="sass">
.parallel-team-overview
  padding: var(--space-4)
  border-radius: 16px
  border: 1px solid var(--glass-border)
  background: color-mix(in srgb, var(--glass-surface) 55%, transparent)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))

.parallel-team-overview__icon
  display: flex
  align-items: center
  justify-content: center
  width: 28px
  height: 28px
  border-radius: 8px
  background: color-mix(in srgb, var(--color-accent) 10%, var(--glass-surface))
  color: var(--color-accent)
  flex-shrink: 0

.parallel-team-overview__title
  font-size: var(--text-sm)
  font-weight: 700
  color: var(--color-text-primary)

.parallel-team-overview__stat
  display: flex
  align-items: baseline
  gap: 4px

.parallel-team-overview__stat-value
  font-size: var(--text-sm)
  font-weight: 700
  color: var(--color-text-primary)

.parallel-team-overview__stat-label
  font-size: var(--text-xs)
  color: var(--color-text-tertiary)

.parallel-team-overview__quota
  padding-top: var(--space-2)
  border-top: 1px solid color-mix(in srgb, var(--glass-border) 50%, transparent)

.parallel-team-overview__all-done
  display: flex
  align-items: center
  padding: var(--space-2) 0

.parallel-team-overview__cards
  display: flex
  flex-direction: column
  gap: var(--space-2)
  padding-top: var(--space-2)
  border-top: 1px solid color-mix(in srgb, var(--glass-border) 50%, transparent)
</style>
