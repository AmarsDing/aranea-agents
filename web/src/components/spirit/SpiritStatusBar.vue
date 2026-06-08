<template>
  <div v-if="visible" class="spirit-status-bar">
    <div class="row items-center no-wrap q-gutter-sm spirit-status-bar__inner">
      <div v-if="complexityLevel" class="spirit-status-bar__item">
        <q-icon :name="complexityIcon" size="14px" :style="{ color: complexityColor }" />
        <span>{{ complexityLabel }}</span>
        <q-tooltip v-if="complexityReason" :delay="300">{{ complexityReason }}</q-tooltip>
      </div>
      <div v-if="runningTeamCount > 0" class="spirit-status-bar__item spirit-status-bar__item--clickable" @click="emit('click-running')">
        <q-icon name="bolt" size="14px" :style="{ color: 'var(--color-accent)' }" />
        <span>{{ runningTeamCount }} 运行中</span>
      </div>
      <div v-if="interruptedTeamCount > 0" class="spirit-status-bar__item spirit-status-bar__item--clickable" @click="emit('click-interrupted')">
        <q-icon name="pause_circle" size="14px" :style="{ color: 'var(--color-warning)' }" />
        <span>{{ interruptedTeamCount }} 已中断</span>
      </div>
      <div v-if="checkpointStep" class="spirit-status-bar__item spirit-status-bar__item--hide-sm">
        <q-icon name="flag" size="14px" :style="{ color: 'var(--color-text-tertiary)' }" />
        <span class="ellipsis">{{ checkpointStep }}</span>
      </div>
      <div v-if="quotaMax > 0" class="spirit-status-bar__item spirit-status-bar__item--hide-sm">
        <q-icon name="bar_chart" size="14px" :style="{ color: 'var(--color-text-tertiary)' }" />
        <span>{{ quotaUsed }}/{{ quotaMax }} 配额</span>
      </div>
      <div v-if="tokenUsage" class="spirit-status-bar__item spirit-status-bar__item--hide-sm">
        <q-icon name="data_usage" size="14px" :style="{ color: 'var(--color-text-tertiary)' }" />
        <span>{{ tokenLabel }}</span>
      </div>
      <div v-if="dqScore != null" class="spirit-status-bar__item spirit-status-bar__item--hide-sm">
        <q-icon name="verified" size="14px" :style="{ color: dqScoreColor }" />
        <span :style="{ color: dqScoreColor }">DQ: {{ dqScore.toFixed(2) }}</span>
        <q-tooltip :delay="300">部署质量评分</q-tooltip>
      </div>
      <div v-if="lastEvent" class="spirit-status-bar__item spirit-status-bar__last-event spirit-status-bar__item--clickable" @click="emit('click-last-event')">
        <q-icon
          :name="lastEvent.type === 'completed' ? 'check_circle' : 'error'"
          :style="{ color: lastEvent.type === 'completed' ? 'var(--color-success)' : 'var(--color-danger)' }"
          size="14px"
        />
        <span class="ellipsis">{{ lastEvent.teamName }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { COMPLEXITY_CONFIG, dqScoreColor as getDqScoreColor, formatTokenCount } from '../../features/spirit/spiritUi';

const props = defineProps<{
  runningTeamCount: number;
  interruptedTeamCount: number;
  quotaUsed: number;
  quotaMax: number;
  tokenUsage?: { in: number; out: number } | null;
  lastEvent?: { type: 'completed' | 'failed'; teamName: string; teamId?: string } | null;
  /** Complexity level from spirit_plan_created event (simple/moderate/complex). */
  complexityLevel?: string | null;
  /** Strategy reason from spirit_plan_created event. */
  complexityReason?: string | null;
  /** Current orchestration checkpoint step from spirit_orchestration_checkpoint event. */
  checkpointStep?: string | null;
  /** Last DQ score from spirit_team_completed event. */
  dqScore?: number | null;
}>();

const emit = defineEmits<{
  'click-running': [];
  'click-interrupted': [];
  'click-last-event': [];
}>();

const visible = computed(() => props.runningTeamCount > 0 || props.interruptedTeamCount > 0 || props.quotaMax > 0 || !!props.lastEvent || !!props.complexityLevel || !!props.checkpointStep || props.dqScore != null);

const tokenLabel = computed(() => formatTokenCount(props.tokenUsage?.in, props.tokenUsage?.out));

const complexityLabel = computed(() => {
  if (!props.complexityLevel) return '';
  return COMPLEXITY_CONFIG[props.complexityLevel]?.label ?? props.complexityLevel;
});

const complexityIcon = computed(() => {
  if (!props.complexityLevel) return 'tune';
  return COMPLEXITY_CONFIG[props.complexityLevel]?.icon ?? 'tune';
});

const complexityColor = computed(() => {
  if (!props.complexityLevel) return 'var(--color-text-tertiary)';
  return COMPLEXITY_CONFIG[props.complexityLevel]?.color ?? 'var(--color-text-tertiary)';
});

const dqScoreColor = computed(() => getDqScoreColor(props.dqScore));
</script>

<style scoped lang="sass">
.spirit-status-bar
  height: 24px
  flex-shrink: 0
  border-top: 1px solid color-mix(in srgb, var(--glass-border) 50%, transparent)
  background: color-mix(in srgb, var(--glass-surface) 40%, transparent)
  backdrop-filter: blur(var(--glass-blur-default))
  -webkit-backdrop-filter: blur(var(--glass-blur-default))

.spirit-status-bar__inner
  height: 24px
  padding: 0 var(--space-3)
  font-size: 11px
  color: var(--color-text-secondary)
  overflow: hidden

.spirit-status-bar__item
  display: flex
  align-items: center
  gap: 3px
  white-space: nowrap
  flex-shrink: 0

.spirit-status-bar__item--clickable
  cursor: pointer
  border-radius: 4px
  padding: 1px 4px
  transition: background 0.15s ease

  &:hover
    background: color-mix(in srgb, var(--color-accent) 12%, transparent)

.spirit-status-bar__last-event
  margin-left: auto
  max-width: 160px

.spirit-status-bar__item--hide-sm
  @media (max-width: 600px)
    display: none
</style>
