<template>
  <div class="team-assembly-card" :class="`team-assembly-card--${status}`">
    <div class="row items-center q-gutter-sm">
      <div class="team-assembly-card__icon">
        <q-spinner v-if="status === 'assembling'" size="20px" color="accent" />
        <q-icon v-else-if="status === 'assembled'" name="groups" size="20px" color="accent" />
        <q-icon v-else-if="status === 'completed'" name="check_circle" size="20px" color="positive" />
        <q-icon v-else name="error" size="20px" color="negative" />
      </div>
      <div class="col min-width-0">
        <div class="team-assembly-card__name ellipsis">{{ teamName }}</div>
        <div v-if="taskSummary" class="team-assembly-card__summary ellipsis">{{ taskSummary }}</div>
      </div>
      <q-chip v-if="mode" dense size="sm" outline :label="modeLabel" class="team-assembly-card__mode" />
    </div>

    <div v-if="members.length > 0" class="team-assembly-card__members q-mt-sm">
      <div class="row items-center q-gutter-xs">
        <q-avatar v-for="member in members.slice(0, 4)" :key="member.agentId" size="24px">
          <img v-if="member.avatarUrl" :src="member.avatarUrl" :alt="member.displayName" />
          <q-icon v-else name="person" size="14px" color="grey-6" />
        </q-avatar>
        <span v-if="members.length > 4" class="text-caption text-grey-6"> +{{ members.length - 4 }} </span>
      </div>
      <div class="text-caption text-grey-6 q-mt-xs">
        {{ members.map((m) => m.displayName).join(' · ') }}
      </div>
    </div>

    <q-btn
      v-if="status === 'assembled' || status === 'completed'"
      flat
      dense
      no-caps
      icon="visibility"
      label="查看团队"
      color="accent"
      class="q-mt-sm"
      @click="$emit('view-team')"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import type { SpiritMember } from '../../features/spirit/types';
import { spiritModeLabel } from '../../features/spirit/spiritUi';

const props = defineProps<{
  status: 'assembling' | 'assembled' | 'completed' | 'failed';
  teamName: string;
  taskSummary: string;
  mode: string;
  members: SpiritMember[];
}>();

defineEmits<{
  'view-team': [];
}>();

const modeLabel = computed(() => spiritModeLabel(props.mode));
</script>

<style scoped lang="sass">
.team-assembly-card
  padding: var(--space-4)
  border-radius: 16px
  border: 1px solid var(--glass-border)
  background: color-mix(in srgb, var(--glass-surface) 55%, transparent)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))

.team-assembly-card--assembling
  border-color: color-mix(in srgb, var(--color-accent) 25%, var(--glass-border))

.team-assembly-card--assembled
  border-color: color-mix(in srgb, var(--color-accent) 30%, var(--glass-border))
  background: color-mix(in srgb, var(--color-accent) 5%, var(--glass-surface))

.team-assembly-card--completed
  border-color: color-mix(in srgb, var(--color-success) 25%, var(--glass-border))

.team-assembly-card--failed
  border-color: color-mix(in srgb, var(--color-danger) 25%, var(--glass-border))

.team-assembly-card__icon
  display: flex
  align-items: center
  justify-content: center
  width: 32px
  height: 32px
  border-radius: 8px
  background: color-mix(in srgb, var(--color-accent) 10%, var(--glass-surface))
  flex-shrink: 0

.team-assembly-card__name
  font-size: var(--text-sm)
  font-weight: 700
  color: var(--color-text-primary)

.team-assembly-card__summary
  font-size: var(--text-xs)
  color: var(--color-text-tertiary)

.team-assembly-card__mode
  font-size: var(--text-xs)

.team-assembly-card__members
  padding-top: var(--space-2)
  border-top: 1px solid color-mix(in srgb, var(--glass-border) 50%, transparent)
</style>
