<template>
  <section class="overview-hero">
    <div class="overview-hero__intro">
      <div class="overview-hero__kicker">Usage</div>
      <h1 class="overview-hero__title">模型消耗概览</h1>
      <p class="overview-hero__subtitle">Token、费用、调用次数与异常请求运营看板</p>
    </div>
    <div v-if="$slots.actions" class="overview-hero__actions row q-gutter-sm items-center">
      <slot name="actions" />
    </div>
  </section>
</template>

<script setup lang="ts"></script>

<style scoped lang="sass">
.overview-hero
  display: flex
  justify-content: space-between
  align-items: flex-start
  gap: 16px
  margin-bottom: 16px
  flex-wrap: wrap

.overview-hero__kicker
  color: var(--color-accent)
  font-size: 12px
  font-weight: 700
  letter-spacing: 0.14em
  text-transform: uppercase

.overview-hero__title
  font-size: clamp(1.35rem, 2.5vw, 1.75rem)
  line-height: 1.15
  margin: 4px 0
  color: var(--color-text-primary)
  font-weight: 700
  letter-spacing: -0.02em

.overview-hero__subtitle
  margin: 0
  max-width: 52ch
  color: var(--color-text-secondary)
  font-size: 0.875rem

.overview-hero__actions
  flex-shrink: 0

@media (max-width: 720px)
  .overview-hero
    flex-direction: column
    align-items: stretch
</style>
