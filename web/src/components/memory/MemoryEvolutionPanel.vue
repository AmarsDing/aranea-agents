// Container: approved — feature-local panel/dialog; data from Page composable via props.
<template>
  <div class="row q-col-gutter-md">
    <div v-for="panel in panels" :key="panel.title" class="col-12 col-md-6">
      <q-card flat bordered class="memory-card memory-evolution-card full-height">
        <q-card-section class="memory-evolution-card__head">
          <div class="memory-evolution-card__icon" :class="`memory-evolution-card__icon--${panel.color}`">
            <q-icon :name="panel.icon" size="20px" />
          </div>
          <div class="memory-evolution-card__heading">
            <div class="memory-evolution-card__title">{{ panel.title }}</div>
            <div class="memory-evolution-card__caption">{{ panel.caption }}</div>
          </div>
        </q-card-section>
        <q-card-section class="memory-evolution-card__body">
          <div class="memory-evolution-card__state">
            <span
              class="memory-evolution-card__state-dot"
              :class="`memory-evolution-card__state-dot--${panel.color}`"
            />
            <span class="memory-evolution-card__state-text">{{ panel.state }}</span>
          </div>
          <ul v-if="panel.items?.length" class="memory-evolution-card__items">
            <li v-for="item in panel.items" :key="item" class="memory-evolution-card__item">
              <span class="memory-evolution-card__item-text">{{ item }}</span>
            </li>
          </ul>
        </q-card-section>
      </q-card>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  panels: Array<{
    title: string;
    caption: string;
    state: string;
    icon: string;
    color: string;
    items?: string[];
  }>;
}>();
</script>

<style scoped lang="sass">
.memory-evolution-card
  display: flex
  flex-direction: column

  &__head
    display: flex
    align-items: flex-start
    gap: var(--space-3)
    padding: var(--space-4) var(--space-4) 0

  &__icon
    display: flex
    align-items: center
    justify-content: center
    width: 36px
    height: 36px
    border-radius: 11px
    flex-shrink: 0
    background: color-mix(in srgb, var(--color-accent) 14%, transparent)
    color: var(--color-accent)

    &--teal
      background: color-mix(in srgb, var(--kv-teal) 16%, transparent)
      color: var(--kv-teal)

    &--primary
      background: color-mix(in srgb, var(--color-accent) 14%, transparent)
      color: var(--color-accent)

    &--deep-purple
      background: color-mix(in srgb, var(--kv-violet) 16%, transparent)
      color: var(--kv-violet)

    &--orange
      background: color-mix(in srgb, var(--kv-orange) 16%, transparent)
      color: var(--kv-orange)

  &__heading
    min-width: 0

  &__title
    font-size: var(--text-md)
    font-weight: 600
    line-height: 1.3
    color: var(--color-text-primary)

  &__caption
    margin-top: 2px
    font-size: var(--text-sm)
    line-height: 1.45
    color: var(--color-text-secondary)

  &__body
    display: flex
    flex-direction: column
    flex: 1
    padding: var(--space-3) var(--space-4) var(--space-4)

  &__state
    display: flex
    align-items: center
    gap: var(--space-2)
    font-size: var(--text-sm)
    line-height: 1.4
    color: var(--color-text-secondary)
    min-width: 0

  &__state-dot
    width: 6px
    height: 6px
    border-radius: 50%
    flex-shrink: 0
    background: var(--color-accent)

    &--teal
      background: var(--kv-teal)

    &--primary
      background: var(--color-accent)

    &--deep-purple
      background: var(--kv-violet)

    &--orange
      background: var(--kv-orange)

  &__state-text
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap

  &__items
    list-style: none
    margin: var(--space-3) 0 0
    padding: var(--space-3) 0 0
    border-top: 1px solid var(--glass-border)
    display: flex
    flex-direction: column
    gap: 6px

  &__item
    display: flex
    align-items: center
    gap: var(--space-2)
    font-size: var(--text-sm)
    line-height: 1.4
    color: var(--color-text-primary)
    min-width: 0

    &::before
      content: ''
      width: 4px
      height: 4px
      border-radius: 50%
      flex-shrink: 0
      background: var(--color-icon-muted)

  &__item-text
    overflow: hidden
    text-overflow: ellipsis
    white-space: nowrap
</style>
