<template>
  <div class="wb-rrow" :class="{ 'wb-await-bar': waiting }" :data-agent-row="agent.key">
    <!-- DOT：身份（角色色 × 形状双编码；吉祥物只作识别辅助） -->
    <button type="button" class="wb-rrow__card wb-rrow__dot" @click="drill('dot')">
      <i class="wb-rrow__glyph" :style="glyphStyle" />
      <span class="wb-rrow__at">@{{ agent.name }}</span>
      <span class="wb-rrow__status" :class="`is-${liveKind}`">{{ statusText }}</span>
    </button>
    <!-- JOB：使命与触发（岗位/业务线；触发方式无证据显 —） -->
    <button type="button" class="wb-rrow__card wb-rrow__job" @click="drill('job')">
      <span class="wb-rrow__micro">{{ t('workbench.row.job') }}</span>
      <span class="wb-rrow__main">{{ agent.positionName || agent.deptName || agent.name }}</span>
      <span class="wb-rrow__foot">{{ deptLine }}</span>
    </button>
    <!-- RULE：权限与批准态（☑approved / ☐needs you） -->
    <button
      type="button"
      class="wb-rrow__card wb-rrow__rule"
      :class="{ 'wb-await-frame': waiting }"
      @click="drill('rule')"
    >
      <span class="wb-rrow__micro">{{ t('workbench.row.rule') }}</span>
      <span class="wb-rrow__main" :class="{ gold: waiting }">
        {{ waiting ? t('workbench.row.needsYou') : t('workbench.row.approved') }}
      </span>
      <span class="wb-rrow__foot">{{ t('workbench.row.ruleFoot') }}</span>
    </button>
    <!-- ACTIVITY：当前任务 + 产物 + 工具行（ticker + 点链 + 咳嗽） -->
    <button
      type="button"
      class="wb-rrow__card wb-rrow__activity"
      :class="{ 'wb-await-frame': waiting }"
      @click="drill('activity')"
    >
      <span class="wb-rrow__micro">
        {{ t('workbench.row.activity') }}
        <span class="wb-rrow__chain" :class="{ on: activeNow }">
          <i v-for="n in 4" :key="n" :style="{ animationDelay: `${(n - 1) * 120}ms` }" />
        </span>
      </span>
      <span class="wb-rrow__main ellipsis">{{ activityText }}</span>
      <span class="wb-rrow__ticker" :class="{ cough: hasError }">{{ tickerText }}</span>
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { toolBeatText } from '../../features/workbench/logic';
import type { AgentLiveState, DrillCard, RosterAgent } from '../../features/workbench/types';

const props = defineProps<{
  agent: RosterAgent;
  live?: AgentLiveState;
  /** 走廊点亮窗口内（最近 3s 有真实事件） */
  activeNow: boolean;
  /** 等批归因（confirm step 或 lane 行 agentKey 命中） */
  waiting: boolean;
}>();

const emit = defineEmits<{
  drill: [agentKey: string, card: DrillCard];
}>();

const { t } = useI18n();

function drill(card: DrillCard): void {
  emit('drill', props.agent.key, card);
}

const liveKind = computed(() => (props.waiting ? 'waiting' : (props.live?.statusKind ?? 'idle')));

const statusText = computed(() => {
  if (props.live?.statusWord) return props.live.statusWord;
  return t(`workbench.row.${liveKind.value}`);
});

const deptLine = computed(
  () => [props.agent.deptName, props.agent.positionName].filter(Boolean).join(' · ') || t('workbench.row.noEvidence'),
);

const activityText = computed(() => {
  const l = props.live;
  if (!l?.lastToolName) return t('workbench.row.noEvidence');
  return toolBeatText({ toolName: l.lastToolName, durationMs: l.lastDurationMs, errorCode: l.lastErrorCode, at: '' });
});

const tickerText = computed(() => {
  const beat = props.live?.recentTools?.[0];
  if (!beat) return '—';
  return toolBeatText(beat);
});

const hasError = computed(() => !!props.live?.recentTools?.[0]?.errorCode);

const glyphStyle = computed(() => {
  let h = 0;
  const key = props.agent.key;
  for (let i = 0; i < key.length; i += 1) h = (h * 31 + key.charCodeAt(i)) >>> 0;
  return { background: `var(--agent-palette-${h % 9}, var(--wb-flow))` };
});
</script>

<style scoped lang="sass">
.wb-rrow
  position: relative
  display: grid
  grid-template-columns: 1.1fr 1fr 1fr 1.4fr
  gap: 8px
  padding: 4px 0 4px 8px
  align-items: stretch
.wb-rrow__card
  position: relative
  display: flex
  flex-direction: column
  align-items: flex-start
  gap: 2px
  min-width: 0
  padding: 6px 8px
  border: 1px solid var(--wb-line-hairline)
  border-radius: var(--wb-radius)
  background: color-mix(in srgb, var(--color-text-primary) 3%, transparent)
  color: inherit
  font-size: var(--wb-caption)
  text-align: left
  cursor: pointer
  &:hover
    border-color: color-mix(in srgb, var(--color-text-primary) 20%, transparent)
.wb-rrow__dot
  flex-direction: row
  align-items: center
  gap: 8px
.wb-rrow__glyph
  flex: none
  width: 14px
  height: 14px
  border-radius: 4px
.wb-rrow__at
  font-weight: 600
  white-space: nowrap
  overflow: hidden
  text-overflow: ellipsis
.wb-rrow__status
  margin-left: auto
  font-size: 10px
  color: var(--color-text-secondary)
  &.is-waiting
    color: var(--wb-await)
  &.is-error
    color: var(--wb-block)
  &.is-working, &.is-thinking
    color: var(--wb-flow)
.wb-rrow__micro
  display: inline-flex
  align-items: center
  gap: 6px
  font-size: var(--wb-micro-label)
  letter-spacing: 0.14em
  text-transform: uppercase
  color: var(--color-text-tertiary, var(--color-text-secondary))
.wb-rrow__main
  font-size: var(--wb-body)
  font-weight: 600
  max-width: 100%
  &.gold
    color: var(--wb-await)
  &.ellipsis
    white-space: nowrap
    overflow: hidden
    text-overflow: ellipsis
.wb-rrow__foot, .wb-rrow__ticker
  font-size: 10px
  color: var(--color-text-secondary)
  white-space: nowrap
  overflow: hidden
  text-overflow: ellipsis
  max-width: 100%
.wb-rrow__ticker.cough
  animation: wb-cough 140ms ease 2
// 点链行级版（设计 §5.3：行级点亮即可，无 join key 不画源连线）
.wb-rrow__chain
  display: inline-flex
  gap: 3px
  i
    width: 3px
    height: 3px
    border-radius: 50%
    background: var(--wb-flow)
    opacity: 0.2
.wb-rrow__chain.on i
  animation: wb-dot-pulse 1.2s ease-in-out infinite
</style>
