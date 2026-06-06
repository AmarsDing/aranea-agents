<template>
  <div v-if="dagTeams.length > 0" class="dag-diagram-card">
    <div class="row items-center q-gutter-sm q-mb-sm">
      <div class="dag-diagram-card__icon">
        <q-icon name="account_tree" size="18px" />
      </div>
      <div class="col min-width-0">
        <div class="dag-diagram-card__title">任务依赖图</div>
      </div>
    </div>

    <div class="dag-diagram-card__nodes">
      <div v-for="node in dagNodes" :key="node.teamId" class="dag-diagram-card__node">
        <span class="dag-diagram-card__prefix">{{ node.prefix }}</span>
        <span class="dag-diagram-card__name">{{ node.teamName }}</span>
        <span v-if="node.dependsOn.length > 0" class="dag-diagram-card__deps text-caption text-grey-6">
          (依赖: {{ node.dependsOn.join(', ') }})
        </span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { SpiritTeam } from '../../features/spirit/types';

const props = defineProps<{
  teams: SpiritTeam[];
}>();

const dagTeams = computed(() => props.teams.filter((t) => t.dagNodeId || (t.dependsOn && t.dependsOn.length > 0)));

const dagNodes = computed(() =>
  dagTeams.value.map((t) => ({
    teamId: t.id,
    teamName: t.teamName || t.taskSummary,
    prefix: t.dependsOn && t.dependsOn.length > 0 ? '⏳' : '▶',
    dependsOn: t.dependsOn ?? [],
  })),
);
</script>

<style scoped lang="sass">
.dag-diagram-card
  padding: var(--space-4)
  border-radius: 16px
  border: 1px solid var(--glass-border)
  background: color-mix(in srgb, var(--glass-surface) 55%, transparent)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))

.dag-diagram-card__icon
  display: flex
  align-items: center
  justify-content: center
  width: 28px
  height: 28px
  border-radius: 8px
  background: color-mix(in srgb, var(--color-accent) 10%, var(--glass-surface))
  color: var(--color-accent)
  flex-shrink: 0

.dag-diagram-card__title
  font-size: var(--text-sm)
  font-weight: 700
  color: var(--color-text-primary)

.dag-diagram-card__nodes
  display: flex
  flex-direction: column
  gap: var(--space-1)

.dag-diagram-card__node
  display: flex
  align-items: baseline
  gap: 6px
  font-size: var(--text-xs)
  color: var(--color-text-secondary)

.dag-diagram-card__prefix
  flex-shrink: 0

.dag-diagram-card__name
  font-weight: 600
  color: var(--color-text-primary)

.dag-diagram-card__deps
  white-space: nowrap
  overflow: hidden
  text-overflow: ellipsis
</style>
