<template>
  <aside v-if="target" class="wb-drawer" role="complementary" :aria-label="t('workbench.drawer.title')">
    <header class="wb-drawer__head">
      <span class="wb-drawer__name">@{{ target.agentName }}</span>
      <button type="button" class="wb-drawer__x" :title="t('workbench.drawer.close')" @click="$emit('close')">×</button>
    </header>
    <div class="wb-drawer__body">
      <section class="wb-drawer__sec">
        <div class="wb-drawer__label">L1 · {{ t('workbench.drawer.task') }}</div>
        <div class="wb-drawer__kv">
          <span class="wb-drawer__k">{{ t('workbench.drawer.status') }}</span>
          <span class="wb-drawer__v" :class="`is-${live?.statusKind ?? 'idle'}`">{{ statusText }}</span>
        </div>
        <div class="wb-drawer__kv">
          <span class="wb-drawer__k">{{ t('workbench.drawer.progress') }}</span>
          <span class="wb-drawer__v">{{ lastToolText }}</span>
        </div>
        <div class="wb-drawer__think">
          <span class="wb-drawer__k">{{ t('workbench.drawer.thinking') }}</span>
          <p class="wb-drawer__think-text">{{ live?.lastThinking || t('workbench.drawer.noData') }}</p>
        </div>
      </section>
      <section class="wb-drawer__sec">
        <div class="wb-drawer__label">L2 · {{ t('workbench.drawer.tools') }}</div>
        <template v-if="live?.recentTools?.length">
          <div v-for="(b, i) in live.recentTools" :key="i" class="wb-drawer__tool" :class="{ err: b.errorCode }">
            <span class="wb-drawer__tool-name">{{ b.toolName }}</span>
            <span class="wb-drawer__tool-dur">{{ (b.durationMs / 1000).toFixed(1).replace(/\.0$/, '') }}s</span>
            <span v-if="b.errorCode" class="wb-drawer__tool-err">{{ b.errorCode }}</span>
          </div>
        </template>
        <div v-else class="wb-drawer__nodata">{{ t('workbench.drawer.noData') }}</div>
      </section>
      <section class="wb-drawer__sec">
        <div class="wb-drawer__label">L3 · {{ t('workbench.drawer.l3pending') }}</div>
        <div class="wb-drawer__nodata">{{ t('workbench.sources.pending') }}</div>
      </section>
    </div>
  </aside>
</template>

<script setup lang="ts">
/**
 * 下钻抽屉（420px，设计 §2.5.1）：L1 当前任务+思考 / L2 工具面（错误「咳嗽」标记）；
 * L3 决策生命线 P1（设计 §9——本抽屉显式待接入，禁占位假数据）。
 * 非模态：抽屉来时主区 scale(.97)+brightness(.85) 由 shell 承担。
 */
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { toolBeatText } from '../../features/workbench/logic';
import type { AgentLiveState, DrillTarget } from '../../features/workbench/types';

const props = defineProps<{
  target: DrillTarget;
  live?: AgentLiveState;
}>();

defineEmits<{
  close: [];
}>();

const { t } = useI18n();

const statusText = computed(() => t(`workbench.row.${props.live?.statusKind ?? 'idle'}`));
const lastToolText = computed(() => {
  const l = props.live;
  if (!l?.lastToolName) return t('workbench.drawer.noData');
  return toolBeatText({ toolName: l.lastToolName, durationMs: l.lastDurationMs, errorCode: l.lastErrorCode, at: '' });
});
</script>

<style scoped lang="sass">
.wb-drawer
  position: absolute
  top: 0
  right: 0
  bottom: 0
  z-index: 25
  width: 420px
  max-width: 92%
  display: flex
  flex-direction: column
  background: var(--color-surface-solid, var(--canvas-base))
  border-left: 1px solid var(--wb-line-hairline)
  box-shadow: -8px 0 32px color-mix(in srgb, var(--color-shadow) 30%, transparent)
  transition: transform var(--wb-dur-view) ease
.wb-drawer__head
  flex: none
  display: flex
  align-items: center
  justify-content: space-between
  padding: 12px 14px
  border-bottom: 1px solid var(--wb-line-hairline)
.wb-drawer__name
  font-size: 13px
  font-weight: 700
.wb-drawer__x
  border: none
  background: transparent
  color: var(--color-text-secondary)
  font-size: 16px
  cursor: pointer
  &:hover
    color: var(--color-text-primary)
.wb-drawer__body
  flex: 1
  min-height: 0
  overflow-y: auto
  padding: 12px 14px
.wb-drawer__sec
  margin-bottom: 18px
.wb-drawer__label
  font-size: var(--wb-micro-label)
  letter-spacing: 0.14em
  text-transform: uppercase
  color: var(--color-text-tertiary, var(--color-text-secondary))
  margin-bottom: 8px
.wb-drawer__kv
  display: flex
  justify-content: space-between
  padding: 3px 0
  font-size: var(--wb-caption)
.wb-drawer__k
  color: var(--color-text-secondary)
.wb-drawer__v
  font-variant-numeric: tabular-nums
  &.is-working, &.is-thinking
    color: var(--wb-flow)
  &.is-waiting
    color: var(--wb-await)
  &.is-error
    color: var(--wb-block)
.wb-drawer__think
  margin-top: 6px
.wb-drawer__think-text
  margin: 4px 0 0
  font-size: var(--wb-caption)
  color: var(--color-text-secondary)
  line-height: 1.6
  word-break: break-all
.wb-drawer__tool
  display: flex
  gap: 8px
  padding: 3px 0
  font-size: var(--wb-caption)
  &.err
    animation: wb-cough 140ms ease 2
.wb-drawer__tool-name
  flex: 1
  font-weight: 600
.wb-drawer__tool-dur
  font-variant-numeric: tabular-nums
  color: var(--color-text-secondary)
.wb-drawer__tool-err
  color: var(--wb-block)
  font-size: 10px
.wb-drawer__nodata
  font-size: var(--wb-caption)
  color: var(--color-text-tertiary, var(--color-text-secondary))
</style>
