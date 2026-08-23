<template>
  <div class="kb-glass kb-glass-panel" :class="{ 'kb-glass--strong': strong }">
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
// 标准玻璃面板（SP3 §SP3.3）：工作台面板/浮层基座。
// 已退役：SVG 折射光纹、指针追随高光、镜面边缘、glow/refract（性能与视觉减负）。
// 玻璃皮肤来自 css/knowledge-workbench.sass 的 .kb-workbench .kb-glass 作用域。
defineProps<{
  title?: string;
  icon?: string;
  /** 更强的玻璃底色（浮层用） */
  strong?: boolean;
  /** 去掉 body 内边距（编辑器/列表等自管理 padding 的内容） */
  flush?: boolean;
}>();
</script>

<style lang="sass" scoped>
.kb-glass-panel
  display: flex
  flex-direction: column
  min-height: 0

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
    position: relative
    padding: 12px 14px
    flex: 1
    min-height: 0
    overflow: auto

    &--flush
      padding: 0
</style>
