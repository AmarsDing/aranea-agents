<template>
  <div class="command-center-quick-actions">
    <router-link v-for="action in actions" :key="action.to" :to="action.to" class="command-center-quick-action">
      <div class="command-center-quick-action__icon" :class="action.iconClass">
        <q-icon :name="action.icon" size="24px" />
      </div>
      <div class="command-center-quick-action__body">
        <div class="command-center-quick-action__title">{{ action.title }}</div>
        <div class="command-center-quick-action__desc">{{ action.desc }}</div>
      </div>
      <q-icon name="chevron_right" size="18px" class="command-center-quick-action__arrow" />
    </router-link>
  </div>
</template>

<script setup lang="ts">
const actions = [
  {
    to: '/chat',
    icon: 'chat',
    iconClass: 'command-center-quick-action__icon--chat',
    title: '新建对话',
    desc: '与 AI 智能体对话',
  },
  {
    to: '/agents',
    icon: 'smart_toy',
    iconClass: 'command-center-quick-action__icon--agent',
    title: '创建 Agent',
    desc: '配置新的智能体',
  },
  {
    to: '/cron',
    icon: 'schedule',
    iconClass: 'command-center-quick-action__icon--cron',
    title: '定时任务',
    desc: '管理自动化任务',
  },
  {
    to: '/settings',
    icon: 'settings',
    iconClass: 'command-center-quick-action__icon--settings',
    title: '系统设置',
    desc: '系统参数配置',
  },
];
</script>

<style lang="sass">
.command-center-quick-actions
  display: grid
  grid-template-columns: repeat(4, 1fr)
  gap: 12px
  margin-bottom: 24px

  @media (max-width: 1023px)
    grid-template-columns: repeat(2, 1fr)

  @media (max-width: 599px)
    grid-template-columns: 1fr

.command-center-quick-action
  display: flex
  align-items: center
  gap: 12px
  padding: 14px 16px
  border-radius: 14px
  background: var(--color-background-elevated)
  border: 1px solid var(--glass-border)
  text-decoration: none
  color: inherit
  transition: border-color 0.2s ease, background 0.2s ease
  cursor: pointer

  &:hover
    border-color: var(--glass-border-hover)

  &__icon
    width: 40px
    height: 40px
    border-radius: 10px
    display: flex
    align-items: center
    justify-content: center
    color: white
    flex-shrink: 0
    &--chat
      background: var(--color-accent-blue)
    &--agent
      background: var(--color-accent-indigo)
    &--cron
      background: var(--color-accent)
    &--settings
      background: var(--color-text-secondary)

  &__body
    flex: 1
    min-width: 0

  &__title
    font-size: 0.88rem
    font-weight: 600
    line-height: 1.3
    color: var(--color-text-primary)

  &__desc
    font-size: 0.75rem
    color: var(--color-text-secondary)
    line-height: 1.3

  &__arrow
    color: var(--color-text-secondary)
    flex-shrink: 0

body.body--dark
  .command-center-quick-action
    background: var(--color-background-elevated)
    border-color: var(--glass-border)
    &:hover
      border-color: var(--glass-border-hover)

  .command-center-quick-action__icon--chat
    background: var(--color-accent-blue)

  .command-center-quick-action__icon--agent
    background: var(--color-accent-indigo)

  .command-center-quick-action__icon--cron
    background: var(--color-accent)

  .command-center-quick-action__icon--settings
    background: var(--color-text-secondary)
</style>
