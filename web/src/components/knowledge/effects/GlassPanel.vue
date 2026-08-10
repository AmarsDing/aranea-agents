<template>
  <div class="kb-glass kb-glass-panel" :class="{ 'kb-glass--strong': strong, 'kb-glass-panel--glow': glow }">
    <div v-if="title" class="kb-glass-panel__header">
      <q-icon v-if="icon" :name="icon" size="16px" class="kb-glass-panel__icon" />
      <span class="kb-glass-panel__title">{{ title }}</span>
      <slot name="header-actions" />
    </div>
    <div class="kb-glass-panel__body" :class="{ 'kb-glass-panel__body--flush': flush }">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
// 液态玻璃容器（SP2 §SP2-3）：面板/浮层基座。视觉令牌来自 css/deep-space.sass（.kb-workbench 作用域）。
defineProps<{
  title?: string;
  icon?: string;
  /** 更强的玻璃底色（浮层用） */
  strong?: boolean;
  /** 呼吸辉光 */
  glow?: boolean;
  /** 去掉 body 内边距（编辑器/列表等自管理padding的内容） */
  flush?: boolean;
}>();
</script>

<style lang="sass" scoped>
.kb-glass-panel
  display: flex
  flex-direction: column
  min-height: 0

  &--glow
    animation: kb-glass-breathe 4.5s ease-in-out infinite

  &__header
    display: flex
    align-items: center
    gap: 8px
    padding: 10px 14px
    border-bottom: 1px solid var(--kb-glass-border)
    flex: none

  &__icon
    color: var(--kb-accent-cyan)

  &__title
    font-size: 12px
    font-weight: 600
    letter-spacing: 0.08em
    text-transform: uppercase
    color: var(--kb-text-dim)
    flex: 1

  &__body
    padding: 12px 14px
    flex: 1
    min-height: 0
    overflow: auto

    &--flush
      padding: 0

@keyframes kb-glass-breathe
  0%, 100%
    box-shadow: inset 0 0 24px rgba(79, 216, 255, 0.04), 0 12px 40px rgba(2, 6, 18, 0.5)
  50%
    box-shadow: inset 0 0 32px rgba(79, 216, 255, 0.1), 0 12px 48px rgba(79, 216, 255, 0.12)
</style>
