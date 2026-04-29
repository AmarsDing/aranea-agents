<template>
  <section class="tool-hero">
    <div class="tool-hero__intro">
      <div class="tool-hero__kicker">{{ kicker }}</div>
      <h1 class="tool-hero__title">{{ title }}</h1>
      <p v-if="subtitle" class="tool-hero__subtitle">{{ subtitle }}</p>
    </div>
    <div v-if="$slots.actions" class="tool-hero__actions row q-gutter-sm">
      <slot name="actions" />
    </div>
  </section>
</template>

<script setup lang="ts">
defineProps<{
  kicker: string;
  title: string;
  subtitle?: string;
}>();
</script>

<style scoped lang="sass">
.tool-hero
  display: flex
  justify-content: space-between
  align-items: flex-start
  gap: 16px
  margin-bottom: 20px
  flex-wrap: wrap

.tool-hero__kicker
  color: var(--color-accent)
  font-size: 12px
  font-weight: 700
  letter-spacing: 0.14em
  text-transform: uppercase

.tool-hero__title
  font-size: 32px
  line-height: 1.1
  margin: 4px 0
  color: var(--color-text-primary)
  letter-spacing: -0.02em

.tool-hero__subtitle
  margin: 0
  max-width: 52ch
  color: var(--color-text-secondary)

.tool-hero__actions
  flex-shrink: 0
  align-items: center

@media (max-width: 720px)
  .tool-hero
    flex-direction: column
    align-items: stretch
</style>
