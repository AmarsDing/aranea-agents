<template>
  <div
    class="team-task-card"
    :class="{
      'team-task-card--active': active,
      'team-task-card--expanded': expanded,
    }"
    role="button"
    tabindex="0"
    @click="$emit('click')"
    @keydown.enter="$emit('click')"
    @keydown.space.prevent="$emit('click')"
  >
    <div class="row items-center no-wrap q-gutter-sm">
      <div class="team-task-card__icon">
        <q-icon name="groups" size="18px" />
      </div>
      <div class="col min-width-0">
        <div class="team-task-card__name ellipsis">{{ team.teamName }}</div>
        <div class="team-task-card__summary ellipsis">{{ team.taskSummary }}</div>
      </div>
      <q-icon
        name="expand_more"
        size="16px"
        class="team-task-card__expand"
        :class="{ 'team-task-card__expand--collapsed': !expanded }"
        @click.stop="$emit('toggle-expand')"
      />
    </div>

    <div v-if="expanded" class="team-task-card__detail q-mt-sm">
      <div class="row items-center q-gutter-xs q-mb-xs">
        <SessionStatusBadge :status="mappedStatus" :status-reason="undefined" :status-changed-at="undefined" />
        <q-chip v-if="team.mode" dense size="sm" outline :label="modeLabel" class="team-task-card__mode" />
      </div>

      <div v-if="team.memberAvatars.length > 0" class="team-task-card__avatars row items-center q-gutter-xs q-mb-xs">
        <q-avatar v-for="(url, idx) in team.memberAvatars.slice(0, 5)" :key="idx" size="22px">
          <img v-if="url" :src="url" alt="" />
          <q-icon v-else name="person" size="14px" color="grey-6" />
        </q-avatar>
        <span v-if="team.memberAvatars.length > 5" class="text-caption text-grey-6">
          +{{ team.memberAvatars.length - 5 }}
        </span>
      </div>

      <div v-if="team.totalSteps > 0" class="team-task-card__progress">
        <q-linear-progress
          :value="team.completedSteps / team.totalSteps"
          size="4px"
          rounded
          color="accent"
          class="q-mt-xs"
        />
        <div class="text-caption text-grey-6 q-mt-xs">{{ team.completedSteps }} / {{ team.totalSteps }} 步骤</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import SessionStatusBadge from '../sessions/SessionStatusBadge.vue';
import type { SpiritTeam } from '../../features/spirit/types';
import { mapSpiritStatusToSession, spiritModeLabel } from '../../features/spirit/spiritUi';

const props = defineProps<{
  team: SpiritTeam;
  expanded: boolean;
  active: boolean;
}>();

defineEmits<{
  click: [];
  'toggle-expand': [];
}>();

const mappedStatus = computed(() => mapSpiritStatusToSession(props.team.status));

const modeLabel = computed(() => spiritModeLabel(props.team.mode));
</script>

<style scoped lang="sass">
.team-task-card
  padding: var(--space-3)
  border-radius: 12px
  border: 1px solid transparent
  background: color-mix(in srgb, var(--glass-surface) 40%, transparent)
  cursor: pointer
  transition: background 0.15s ease, border-color 0.15s ease

.team-task-card:hover
  background: color-mix(in srgb, var(--glass-surface) 65%, transparent)
  border-color: var(--glass-border)

.team-task-card--active
  background: color-mix(in srgb, var(--color-accent) 8%, var(--glass-surface))
  border-color: color-mix(in srgb, var(--color-accent) 30%, var(--glass-border))

.team-task-card__icon
  display: flex
  align-items: center
  justify-content: center
  width: 28px
  height: 28px
  border-radius: 8px
  background: color-mix(in srgb, var(--color-accent) 10%, var(--glass-surface))
  color: var(--color-accent)
  flex-shrink: 0

.team-task-card__name
  font-size: var(--text-sm)
  font-weight: 600
  color: var(--color-text-primary)
  line-height: 1.3

.team-task-card__summary
  font-size: var(--text-xs)
  color: var(--color-text-tertiary)
  line-height: 1.3

.team-task-card__expand
  color: var(--color-text-tertiary)
  transition: transform 0.2s ease
  cursor: pointer
  flex-shrink: 0

.team-task-card__expand--collapsed
  transform: rotate(-90deg)

.team-task-card__detail
  padding-top: var(--space-2)
  border-top: 1px solid color-mix(in srgb, var(--glass-border) 50%, transparent)

.team-task-card__mode
  font-size: var(--text-xs)

.team-task-card__avatars
  flex-wrap: nowrap

.team-task-card__progress
  margin-top: var(--space-1)
</style>
