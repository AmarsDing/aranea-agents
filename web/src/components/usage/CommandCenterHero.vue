<template>
  <section class="command-center-hero">
    <div class="command-center-hero__header">
      <div class="command-center-hero__greeting">
        <div class="app-page-kicker">Command Center</div>
        <h1 class="app-page-title">{{ t('overviewPage.greeting', { username }) }}</h1>
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
      <router-link to="/agents" class="command-center-hero__stat command-center-hero__stat--link">
        <div class="command-center-hero__stat-icon command-center-hero__stat-icon--agent">
          <q-icon name="smart_toy" size="20px" />
        </div>
        <div class="command-center-hero__stat-body">
          <div class="command-center-hero__stat-value command-center-hero__stat-value--tech">
            {{ activeAgentCount }}
          </div>
          <div class="command-center-hero__stat-label">{{ t('overviewPage.statActiveAgents') }}</div>
        </div>
      </router-link>
      <router-link to="/models" class="command-center-hero__stat command-center-hero__stat--link">
        <div class="command-center-hero__stat-icon command-center-hero__stat-icon--provider">
          <q-icon name="dns" size="20px" />
        </div>
        <div class="command-center-hero__stat-body">
          <div class="command-center-hero__stat-value command-center-hero__stat-value--tech">{{ providerCount }}</div>
          <div class="command-center-hero__stat-label">{{ t('overviewPage.statProviders') }}</div>
        </div>
      </router-link>
      <router-link to="/settings/taxonomy" class="command-center-hero__stat command-center-hero__stat--link">
        <div class="command-center-hero__stat-icon command-center-hero__stat-icon--category">
          <q-icon name="category" size="20px" />
        </div>
        <div class="command-center-hero__stat-body">
          <div class="command-center-hero__stat-value command-center-hero__stat-value--tech">{{ categoryCount }}</div>
          <div class="command-center-hero__stat-label">{{ t('overviewPage.statCategories') }}</div>
        </div>
      </router-link>
      <router-link to="/team" class="command-center-hero__stat command-center-hero__stat--link">
        <div class="command-center-hero__stat-icon command-center-hero__stat-icon--team">
          <q-icon name="groups" size="20px" />
        </div>
        <div class="command-center-hero__stat-body">
          <div class="command-center-hero__stat-value command-center-hero__stat-value--tech">{{ teamCount }}</div>
          <div class="command-center-hero__stat-label">{{ t('overviewPage.statTeams') }}</div>
        </div>
      </router-link>
      <router-link to="/chat" class="command-center-hero__stat command-center-hero__stat--link">
        <div class="command-center-hero__stat-icon command-center-hero__stat-icon--session">
          <q-icon name="chat" size="20px" />
        </div>
        <div class="command-center-hero__stat-body">
          <div class="command-center-hero__stat-value command-center-hero__stat-value--tech">
            {{ todaySessionCount }}
          </div>
          <div class="command-center-hero__stat-label">{{ t('overviewPage.statTodayChats') }}</div>
        </div>
      </router-link>
      <a class="command-center-hero__stat command-center-hero__stat--link" @click.prevent="$emit('navigate', 'tokens')">
        <div class="command-center-hero__stat-icon command-center-hero__stat-icon--token">
          <q-icon name="data_usage" size="20px" />
        </div>
        <div class="command-center-hero__stat-body">
          <div class="command-center-hero__stat-value command-center-hero__stat-value--tech">
            {{ formattedTokenCount }}
          </div>
          <div class="command-center-hero__stat-label">{{ t('overviewPage.statTodayTokens') }}</div>
        </div>
      </a>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue';
import { useI18n } from 'vue-i18n';
import { formatCount } from '../../features/usage/moneyFormat';

const { t } = useI18n();

const props = defineProps<{
  username: string;
  activeAgentCount: number;
  providerCount: number;
  categoryCount: number;
  teamCount: number;
  todaySessionCount: number;
  todayTokenCount: number;
}>();

defineEmits<{
  navigate: [action: string];
}>();

const formattedTokenCount = computed(() => {
  return formatCount(props.todayTokenCount);
});

const currentTime = ref(formatTime());
let timer: ReturnType<typeof setInterval> | null = null;

function formatTime() {
  return new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
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
    gap: 12px
    grid-template-columns: repeat(6, minmax(0, 1fr))
    margin-bottom: 20px

    @media (max-width: 1199px)
      grid-template-columns: repeat(3, minmax(0, 1fr))

    @media (max-width: 599px)
      grid-template-columns: repeat(2, minmax(0, 1fr))

  &__stat
    display: flex
    align-items: center
    gap: 14px
    padding: 18px 16px
    min-height: 88px
    border-radius: 14px
    background: var(--color-background-elevated, rgba(128, 128, 128, 0.04))
    border: 1px solid rgba(128, 128, 128, 0.08)
    transition: border-color 0.2s ease, background 0.2s ease, transform 0.15s ease
    text-decoration: none
    cursor: pointer

    &--link:hover
      border-color: rgba(128, 128, 128, 0.22)
      transform: translateY(-1px)

  &__stat-icon
    width: 42px
    height: 42px
    border-radius: 11px
    display: flex
    align-items: center
    justify-content: center
    color: white
    flex-shrink: 0
    &--agent
      background: var(--color-accent-indigo, #4F46E5)
    &--provider
      background: var(--color-accent-blue, #2563EB)
    &--category
      background: var(--color-accent-indigo-dark, #7C3AED)
    &--team
      background: var(--color-accent-cyan, #0891B2)
    &--session
      background: var(--color-accent-green, #059669)
    &--token
      background: var(--color-accent, #D4891A)

  &__stat-body
    display: flex
    flex-direction: column
    align-items: center
    text-align: center
    min-width: 0

  &__stat-value
    font-size: 2rem
    font-weight: 700
    line-height: 1.15
    color: var(--color-text-primary)
    letter-spacing: -0.02em
    font-variant-numeric: tabular-nums

    &--tech
      background: linear-gradient(135deg, var(--color-accent-indigo, #4F46E5), var(--color-accent-blue, #2563EB))
      -webkit-background-clip: text
      -webkit-text-fill-color: transparent
      background-clip: text

  &__stat-label
    font-size: 0.75rem
    font-weight: 500
    color: var(--color-text-secondary)
    letter-spacing: -0.01em
    margin-top: 4px
    align-self: stretch
    text-align: right

body.body--dark
  .command-center-hero__stat
    background: rgba(255, 255, 255, 0.025)
    border-color: rgba(255, 255, 255, 0.05)
    &--link:hover
      border-color: rgba(255, 255, 255, 0.14)

  .command-center-hero__stat-icon--agent
    background: var(--color-accent-indigo-lighter, #818CF8)
  .command-center-hero__stat-icon--provider
    background: var(--color-accent-blue, #3B82F6)
  .command-center-hero__stat-icon--category
    background: var(--color-accent-indigo-light, #A78BFA)
  .command-center-hero__stat-icon--team
    background: var(--color-accent-cyan, #22D3EE)
  .command-center-hero__stat-icon--session
    background: var(--color-accent-green-light, #34D399)
  .command-center-hero__stat-icon--token
    background: var(--color-accent, #4DD8E8)

  .command-center-hero__stat-value--tech
    background: linear-gradient(135deg, var(--color-accent-indigo-lighter, #818CF8), var(--color-accent-cyan, #22D3EE))
    -webkit-background-clip: text
    -webkit-text-fill-color: transparent
    background-clip: text
</style>
