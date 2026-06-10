<template>
  <q-card v-if="team.status === 'interrupted'" flat bordered class="interrupted-team-card">
    <q-card-section>
      <div class="row items-center q-mb-sm">
        <q-icon name="pause_circle" size="24px" :style="{ color: 'var(--color-warning)' }" class="q-mr-sm" />
        <span class="text-subtitle2">{{ t('spirit.teamInterrupted') }}</span>
      </div>
      <div class="text-body2 q-mb-xs">{{ t('spirit.interruptedBecause', { name: team.teamName, reason: interruptReason || t('spirit.unknownReason') }) }}</div>
      <div v-if="team.totalSteps > 0" class="text-caption text-grey-7">
        {{ t('spirit.completedSteps', { completed: team.completedSteps, total: team.totalSteps }) }}
      </div>
    </q-card-section>
    <q-card-actions align="right">
      <q-btn v-if="canResume" :label="t('spirit.resumeExecution')" :style="{ color: 'var(--color-accent)' }" flat @click="$emit('resume', team.id)" />
      <q-btn v-else :label="t('spirit.checkpointRecoveryNotSupported')" :style="{ color: 'var(--color-text-tertiary)' }" flat disable />
      <q-btn :label="t('spirit.cancelTeam')" :style="{ color: 'var(--color-danger)' }" flat @click="$emit('cancel', team.id)" />
    </q-card-actions>
  </q-card>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import type { SpiritTeam } from '../../features/spirit/types';

const { t } = useI18n();

const props = defineProps<{
  team: SpiritTeam;
  /** Whether the team supports checkpoint recovery (has graphExecutionId). */
  canResume: boolean;
  /** Reason for the interruption. */
  interruptReason: string;
}>();

defineEmits<{
  resume: [teamId: string];
  cancel: [teamId: string];
}>();
</script>

<style scoped lang="sass">
.interrupted-team-card
  border-radius: 12px
  border: 1px solid color-mix(in srgb, var(--color-warning) 30%, var(--glass-border))
  background: color-mix(in srgb, var(--color-warning) 5%, var(--glass-surface))
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))
</style>
