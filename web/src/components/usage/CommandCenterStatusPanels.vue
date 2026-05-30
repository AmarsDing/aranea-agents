<template>
  <div class="command-center-status-panels">
    <StatusPanelAgent :active="agentStats.active" :total="agentStats.total" :loading="loading" />
    <StatusPanelSession :active-count="sessionActiveCount" :sparkline="sessionSparkline" :loading="loading" />
    <StatusPanelProvider :active="providerHealth.active" :degraded="providerHealth.degraded" :total="providerHealth.total" :loading="loading" />
    <StatusPanelRunner :total-runs="runnerStats.totalRuns" :error-runs="runnerStats.errorRuns" :success-rate="runnerStats.successRate" :error-rate="runnerStats.errorRate" :loading="loading" />
  </div>
</template>

<script setup lang="ts">
import StatusPanelAgent from "./StatusPanelAgent.vue";
import StatusPanelSession from "./StatusPanelSession.vue";
import StatusPanelProvider from "./StatusPanelProvider.vue";
import StatusPanelRunner from "./StatusPanelRunner.vue";

defineProps<{
  agentStats: { active: number; total: number };
  sessionActiveCount: number;
  sessionSparkline: number[];
  providerHealth: { active: number; degraded: number; total: number };
  runnerStats: { totalRuns: number; errorRuns: number; successRate: number; errorRate: number };
  loading: boolean;
}>();
</script>

<style lang="sass">
.command-center-status-panels
  display: grid
  gap: 16px
  grid-template-columns: repeat(4, minmax(0, 1fr))
  margin-bottom: 24px

  @media (max-width: 1023px)
    grid-template-columns: repeat(2, minmax(0, 1fr))

  @media (max-width: 599px)
    grid-template-columns: 1fr

.command-center-stat-panel
  border-radius: 14px
  background: var(--color-background-elevated, rgba(128, 128, 128, 0.04))
  border: 1px solid rgba(128, 128, 128, 0.08)
  transition: border-color 0.2s ease, background 0.2s ease

  &:hover
    border-color: rgba(128, 128, 128, 0.18)

  &__header
    display: flex
    align-items: center
    gap: 8px

  &__title
    font-size: 0.78rem
    font-weight: 500
    color: var(--color-text-secondary)
    letter-spacing: -0.01em

  &__detail
    font-size: 0.73rem
    color: var(--color-text-secondary)
    display: flex
    align-items: center
    justify-content: center
    gap: 4px

  &__dot
    width: 8px
    height: 8px
    border-radius: 50%
    display: inline-block
    &--active
      background: var(--color-success)
    &--inactive
      background: var(--color-text-secondary)

  &__ring-wrap
    position: relative
    width: 80px
    height: 80px
    margin: 0 auto 8px

  &__ring
    width: 100%
    height: 100%
    transform: rotate(-90deg)

  &__ring-text
    position: absolute
    top: 50%
    left: 50%
    transform: translate(-50%, -50%)
    text-align: center

  &__ring-value
    font-size: 1.1rem
    font-weight: 700
    display: block
    line-height: 1.2
    color: var(--color-text-primary)

  &__ring-label
    font-size: 0.7rem
    color: var(--color-text-secondary)

  &__big-value
    font-size: 2.25rem
    font-weight: 800
    line-height: 1.1
    text-align: center
    background: linear-gradient(135deg, #00E5FF, #A78BFA)
    -webkit-background-clip: text
    -webkit-text-fill-color: transparent
    background-clip: text
    filter: drop-shadow(0 0 12px rgba(0, 229, 255, 0.35))
    letter-spacing: -0.03em
    font-variant-numeric: tabular-nums

  &__caption
    font-size: 0.73rem
    color: var(--color-text-secondary)
    text-align: center
    margin-top: 12px
    margin-bottom: 8px
    flex-direction: column
    box-sizing: content-box

  &__sparkline
    height: 24px
    margin-top: 4px

  &__sparkline-svg
    width: 100%
    height: 100%

  &__health-row
    display: flex
    align-items: center
    gap: 6px
    font-size: 0.85rem
    padding: 2px 0

  &__health-dot
    width: 8px
    height: 8px
    border-radius: 50%
    &--ok
      background: var(--color-success)
    &--degraded
      background: var(--color-warning)

  &__health-label
    color: var(--color-text-secondary)
    flex: 1

  &__health-value
    font-weight: 600
    &--danger
      color: var(--color-danger)

  &__health-warn
    display: flex
    align-items: center
    gap: 4px
    font-size: 0.73rem
    color: var(--color-warning)
    margin-top: 6px

  &__gauge
    display: flex
    flex-direction: column
    gap: 10px

  &__gauge-item
    display: flex
    flex-direction: column
    gap: 4px

  &__gauge-bar
    height: 6px
    border-radius: 999px
    background: rgba(128, 128, 128, 0.10)
    overflow: hidden

  &__gauge-fill
    height: 100%
    border-radius: 999px
    transition: width 0.6s cubic-bezier(0.22, 0.61, 0.36, 1)
    &--ok
      background: var(--color-success)
    &--warn
      background: var(--color-warning)
    &--danger
      background: var(--color-danger)

  &__gauge-info
    display: flex
    justify-content: space-between
    font-size: 0.73rem
    color: var(--color-text-secondary)

  &__gauge-value
    font-weight: 600

  &__runner-summary
    font-size: 0.73rem
    color: var(--color-text-secondary)
    text-align: center
    margin-top: 6px
    display: flex
    justify-content: center
    gap: 8px

  &__runner-error
    color: var(--color-danger)
    font-weight: 500

body.body--dark
  .command-center-stat-panel
    background: rgba(255, 255, 255, 0.025)
    border-color: rgba(255, 255, 255, 0.05)
    &:hover
      border-color: rgba(255, 255, 255, 0.12)

  .command-center-stat-panel__gauge-bar
    background: rgba(255, 255, 255, 0.06)
</style>
