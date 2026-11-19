<!--
  RunTrack：一条运行的一维时间轨迹（纯展示）。
  按时间排序的步骤条 + 每条步骤的相对时间条，失败步骤可一键定位，
  解决「瀑布图里满屏色块、不知道死在哪一步」的问题。
  数据来自 monitor_trace_spans（归一化交给 features/monitor/runTrack.ts）。
-->
<template>
  <section
    class="run-track"
    data-test="run-track"
    :aria-busy="loading ? 'true' : 'false'"
    role="group"
    :aria-label="title"
  >
    <AppStatePanel v-if="loading && !track.steps.length" variant="loading" dense :rows="3" />

    <AppStatePanel
      v-else-if="!track.steps.length"
      variant="empty"
      dense
      icon="timeline"
      :title="t('monitorPage.traces.runTrack.emptyTitle')"
      :description="t('monitorPage.traces.runTrack.emptyDesc')"
    />

    <template v-else>
      <header class="run-track__head">
        <p class="run-track__summary" data-test="run-track-summary" aria-live="polite">
          <span>{{ t('monitorPage.traces.runTrack.steps', { n: track.steps.length }) }}</span>
          <span class="run-track__dot" aria-hidden="true">·</span>
          <span :class="track.failedCount ? 'run-track__failed' : ''">
            {{ t('monitorPage.traces.runTrack.failed', { n: track.failedCount }) }}
          </span>
          <span class="run-track__dot" aria-hidden="true">·</span>
          <span class="run-track__total">{{
            t('monitorPage.traces.runTrack.total', { value: formatSpanDuration(track.totalMs) })
          }}</span>
        </p>
        <q-btn
          v-if="track.firstFailedId"
          flat
          dense
          no-caps
          icon="error_outline"
          data-test="run-track-jump"
          :label="t('monitorPage.traces.runTrack.jumpFirstFailure')"
          @click="emit('select', track.firstFailedId)"
        />
      </header>

      <ol class="run-track__list">
        <li v-for="step in track.steps" :key="step.id">
          <button
            type="button"
            class="run-track__step"
            :class="[`run-track__step--${step.tone}`, { 'run-track__step--active': step.id === selectedId }]"
            data-test="run-track-step"
            :data-step-id="step.id"
            :data-tone="step.tone"
            :data-depth="Math.min(step.depth, 4)"
            :aria-current="step.id === selectedId ? 'true' : undefined"
            :aria-label="stepLabel(step)"
            @click="emit('select', step.id)"
            @keydown.enter.prevent="emit('select', step.id)"
            @keydown.space.prevent="emit('select', step.id)"
          >
            <span class="run-track__marker" aria-hidden="true" />
            <span class="run-track__body">
              <span class="run-track__line">
                <span class="run-track__name">{{ step.name }}</span>
                <span v-if="step.kind" class="run-track__kind">{{ step.kind }}</span>
                <span v-if="step.failed || step.tone === 'warn' || step.tone === 'running'" class="run-track__tone">
                  {{ toneLabel(step.tone) }}
                </span>
                <span class="run-track__time">{{ formatSpanDuration(step.durationMs) }}</span>
              </span>
              <span class="run-track__rail">
                <svg class="run-track__bar" viewBox="0 0 100 6" preserveAspectRatio="none" aria-hidden="true">
                  <rect
                    :x="step.offsetPct"
                    y="0"
                    :width="step.widthPct"
                    height="6"
                    :class="`run-track__rect run-track__rect--${step.tone}`"
                  />
                </svg>
                <span v-if="step.error" class="run-track__error">{{ step.error }}</span>
              </span>
            </span>
          </button>
        </li>
      </ol>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppStatePanel from '../common/AppStatePanel.vue';
import { formatSpanDuration, type SpanStatusTone } from '../../features/monitor/spans';
import { buildRunTrack, type RunTrackStep } from '../../features/monitor/runTrack';

const props = withDefaults(
  defineProps<{
    /** monitor_trace_spans 原始数组（扁平 parent_id 或旧嵌套 children 均可）。 */
    spans: unknown[];
    loading?: boolean;
    selectedId?: string;
    title?: string;
  }>(),
  { loading: false, selectedId: '', title: '运行轨迹' },
);

const emit = defineEmits<{ select: [id: string] }>();
const { t } = useI18n();

const track = computed(() => buildRunTrack(props.spans));

function toneLabel(tone: SpanStatusTone): string {
  const map: Record<SpanStatusTone, string> = {
    error: t('monitorPage.traces.runTrack.toneError'),
    ok: t('monitorPage.traces.runTrack.toneOk'),
    running: t('monitorPage.traces.runTrack.toneRunning'),
    warn: t('monitorPage.traces.runTrack.toneWarn'),
    idle: t('monitorPage.traces.runTrack.toneIdle'),
  };
  return map[tone];
}

/** 可访问名称：步骤名 + 状态 + 耗时 + 错误，键盘/读屏都能拿到定位信息。 */
function stepLabel(step: RunTrackStep): string {
  const parts = [step.name, toneLabel(step.tone), formatSpanDuration(step.durationMs)];
  if (step.error) parts.push(step.error);
  return parts.join(' · ');
}
</script>

<style scoped lang="sass">
.run-track
  display: flex
  flex-direction: column
  gap: var(--nt-space-2)

  &__head
    display: flex
    align-items: center
    justify-content: space-between
    gap: var(--nt-space-3)
    flex-wrap: wrap

  &__summary
    margin: 0
    display: inline-flex
    align-items: center
    gap: var(--nt-space-1)
    color: var(--nt-text-2)
    font-size: var(--nt-fs-body)
    font-variant-numeric: tabular-nums

  &__dot
    color: var(--nt-text-3)

  &__failed
    color: var(--nt-danger)
    font-weight: 650

  &__total
    color: var(--nt-text-1)
    font-family: var(--nt-font-mono)

  &__list
    margin: 0
    padding: 0
    list-style: none
    display: flex
    flex-direction: column
    gap: var(--nt-space-1)
    max-height: 420px
    overflow: auto

  &__step
    width: 100%
    display: flex
    align-items: flex-start
    gap: var(--nt-space-2)
    padding: var(--nt-space-1) var(--nt-space-2)
    border: 1px solid transparent
    border-radius: var(--nt-radius-m)
    background: transparent
    color: var(--nt-text-1)
    text-align: left
    cursor: pointer
    transition: background 0.12s ease, border-color 0.12s ease

    &:hover
      background: var(--nt-data-raised)

    &:focus-visible
      outline: 2px solid var(--nt-accent)
      outline-offset: 2px

    &--active
      border-color: var(--nt-accent)
      background: var(--nt-data-raised)

    &[data-depth="1"]
      padding-left: calc(var(--nt-space-2) + var(--nt-space-3))

    &[data-depth="2"]
      padding-left: calc(var(--nt-space-2) + var(--nt-space-6))

    &[data-depth="3"]
      padding-left: calc(var(--nt-space-2) + var(--nt-space-8))

    &[data-depth="4"]
      padding-left: calc(var(--nt-space-2) + var(--nt-space-10))

  &__marker
    flex: 0 0 auto
    width: 8px
    height: 8px
    margin-top: var(--nt-space-2)
    border-radius: var(--nt-radius-pill)
    background: var(--nt-text-3)

    .run-track__step--ok &
      background: var(--nt-success)

    .run-track__step--error &
      background: var(--nt-danger)

    .run-track__step--warn &
      background: var(--nt-warn)

    .run-track__step--running &
      background: var(--nt-accent)

  &__body
    display: flex
    flex-direction: column
    gap: var(--nt-space-1)
    min-width: 0
    flex: 1

  &__line
    display: flex
    align-items: baseline
    gap: var(--nt-space-1)
    min-width: 0

  &__name
    font-size: var(--nt-fs-body)
    font-weight: 600
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap

  &__kind
    flex: 0 0 auto
    padding: 0 var(--nt-space-2)
    border-radius: var(--nt-radius-s)
    background: var(--nt-data-inset)
    color: var(--nt-text-2)
    font-size: var(--nt-fs-caption)

  &__tone
    flex: 0 0 auto
    color: var(--nt-warn)
    font-size: var(--nt-fs-caption)

    .run-track__step--error &
      color: var(--nt-danger)

  &__time
    margin-left: auto
    flex: 0 0 auto
    color: var(--nt-text-2)
    font-family: var(--nt-font-mono)
    font-size: var(--nt-fs-caption)
    font-variant-numeric: tabular-nums

  &__rail
    display: flex
    align-items: center
    gap: var(--nt-space-2)
    min-width: 0

  &__bar
    flex: 1
    height: 6px
    min-width: 0
    overflow: visible

  &__rect
    fill: var(--nt-text-3)

    &--ok
      fill: var(--nt-success)

    &--error
      fill: var(--nt-danger)

    &--warn
      fill: var(--nt-warn)

    &--running
      fill: var(--nt-accent)

  &__error
    flex: 0 1 auto
    max-width: 42ch
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap
    color: var(--nt-danger)
    font-size: var(--nt-fs-caption)
</style>
