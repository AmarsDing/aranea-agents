<template>
  <q-card v-if="team.status === 'interrupted'" flat bordered class="interrupted-team-card">
    <q-card-section>
      <div class="row items-center q-mb-sm">
        <q-icon name="pause_circle" size="24px" :style="{ color: 'var(--color-warning)' }" class="q-mr-sm" />
        <span class="text-subtitle2">团队已中断</span>
      </div>
      <div class="text-body2 q-mb-xs">{{ team.teamName }} 因{{ interruptReason || '未知原因' }}而中断</div>
      <div v-if="team.totalSteps > 0" class="text-caption text-grey-7">
        已完成 {{ team.completedSteps }}/{{ team.totalSteps }} 步骤
      </div>
    </q-card-section>
    <q-card-actions align="right">
      <q-btn v-if="canResume" label="恢复执行" :style="{ color: 'var(--color-accent)' }" flat @click="$emit('resume', team.id)" />
      <q-btn v-else label="不支持断点恢复" :style="{ color: 'var(--color-text-tertiary)' }" flat disable />
      <q-btn label="取消团队" :style="{ color: 'var(--color-danger)' }" flat @click="$emit('cancel', team.id)" />
    </q-card-actions>
  </q-card>
</template>

<script setup lang="ts">
import type { SpiritTeam } from '../../features/spirit/types';

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
