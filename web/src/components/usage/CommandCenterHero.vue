<template>
  <section class="command-center-hero">
    <div class="command-center-hero__header">
      <div class="command-center-hero__greeting">
        <div class="app-page-kicker">Command Center</div>
        <h1 class="app-page-title">你好，{{ username }}</h1>
        <p class="app-page-subtitle">
          <q-icon name="schedule" size="14px" class="q-mr-xs" />
          {{ currentTime }}
        </p>
      </div>
      <div class="command-center-hero__actions">
        <slot name="actions" />
      </div>
    </div>
    <div class="command-center-hero__stats">
      <div class="command-center-hero__stat">
        <div class="command-center-hero__stat-icon command-center-hero__stat-icon--agent">
          <q-icon name="smart_toy" size="20px" />
        </div>
        <div class="command-center-hero__stat-body">
          <div class="command-center-hero__stat-value">{{ activeAgentCount }}</div>
          <div class="command-center-hero__stat-label">活跃 Agent</div>
        </div>
      </div>
      <div class="command-center-hero__stat">
        <div class="command-center-hero__stat-icon command-center-hero__stat-icon--session">
          <q-icon name="chat" size="20px" />
        </div>
        <div class="command-center-hero__stat-body">
          <div class="command-center-hero__stat-value">{{ todaySessionCount }}</div>
          <div class="command-center-hero__stat-label">今日对话</div>
        </div>
      </div>
      <div class="command-center-hero__stat">
        <div class="command-center-hero__stat-icon command-center-hero__stat-icon--token">
          <q-icon name="data_usage" size="20px" />
        </div>
        <div class="command-center-hero__stat-body">
          <div class="command-center-hero__stat-value">{{ formattedTokenCount }}</div>
          <div class="command-center-hero__stat-label">今日 Token</div>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from "vue";
import { formatCount } from "../../features/usage/moneyFormat";

const props = defineProps<{
  username: string;
  activeAgentCount: number;
  todaySessionCount: number;
  todayTokenCount: number;
}>();

const formattedTokenCount = computed(() => formatCount(props.todayTokenCount));

const currentTime = ref(formatTime());
let timer: ReturnType<typeof setInterval> | null = null;

function formatTime() {
  return new Date().toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" });
}

onMounted(() => {
  timer = setInterval(() => {
    currentTime.value = formatTime();
  }, 60_000);
});

onUnmounted(() => {
  if (timer) clearInterval(timer);
});
</script>

<style lang="sass">
.command-center-hero
  padding: 24px 0 20px

  &__header
    display: flex
    flex-wrap: wrap
    align-items: flex-start
    justify-content: space-between
    gap: var(--space-4, 16px)
    margin-bottom: var(--space-5, 20px)

  &__greeting
    min-width: 0

  &__actions
    display: flex
    align-items: center
    gap: 8px
    flex-wrap: wrap

  &__stats
    display: grid
    gap: 16px
    grid-template-columns: repeat(3, minmax(0, 1fr))
    margin-bottom: 20px

    @media (max-width: 719px)
      grid-template-columns: repeat(2, minmax(0, 1fr))

    @media (max-width: 479px)
      grid-template-columns: 1fr

  &__stat
    display: flex
    align-items: center
    gap: 12px
    padding: 16px 20px
    border-radius: 14px
    background: var(--color-background-elevated, rgba(128, 128, 128, 0.04))
    border: 1px solid rgba(128, 128, 128, 0.08)
    transition: border-color 0.2s ease

    &:hover
      border-color: rgba(128, 128, 128, 0.18)

  &__stat-icon
    width: 40px
    height: 40px
    border-radius: 10px
    display: flex
    align-items: center
    justify-content: center
    color: white
    flex-shrink: 0
    &--agent
      background: var(--color-accent-indigo, #4F46E5)
    &--session
      background: var(--color-accent-blue, #2563EB)
    &--token
      background: var(--color-accent, #DCA03E)

  &__stat-value
    font-size: 1.72rem
    font-weight: 700
    line-height: 1.15
    color: var(--color-text-primary)
    letter-spacing: -0.02em
    font-variant-numeric: tabular-nums

  &__stat-label
    font-size: 0.78rem
    font-weight: 500
    color: var(--color-text-secondary)
    letter-spacing: -0.01em

body.body--dark
  .command-center-hero__stat
    background: rgba(255, 255, 255, 0.025)
    border-color: rgba(255, 255, 255, 0.05)
    &:hover
      border-color: rgba(255, 255, 255, 0.12)
  .command-center-hero__stat-icon--agent
    background: var(--color-accent-indigo, #818CF8)
  .command-center-hero__stat-icon--session
    background: var(--color-accent-blue, #3B82F6)
  .command-center-hero__stat-icon--token
    background: var(--color-accent, #4DD8E8)
</style>
