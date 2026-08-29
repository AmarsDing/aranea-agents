<template>
  <q-page
    class="app-page-cream chat-page chat-workspace-shell fit column no-wrap"
    :class="{
      'chat-workspace-shell--compact': compact,
      'chat-workspace-shell--dark': isDark,
    }"
    style="min-height: 0"
  >
    <header v-if="!compact" class="chat-workspace-hero">
      <div class="chat-workspace-hero__text">
        <div class="chat-workspace-kicker">{{ t('chat.workspaceKicker') }}</div>
        <h1 class="chat-workspace-title">{{ t('chat.workspaceTitle') }}</h1>
        <p class="chat-workspace-subtitle">{{ t('chat.workspaceSubtitle') }}</p>
      </div>
    </header>

    <div class="row no-wrap col chat-page__row chat-workspace-shell__main" style="min-height: 0">
      <slot />
    </div>

    <slot name="dialogs" />
  </q-page>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useQuasar } from 'quasar';
import { useI18n } from 'vue-i18n';

withDefaults(
  defineProps<{
    compact?: boolean;
  }>(),
  {
    compact: true,
  },
);

const { t } = useI18n();
const $q = useQuasar();
const isDark = computed(() => $q.dark.isActive);
</script>

<style scoped lang="sass">
.chat-workspace-shell
  padding: var(--space-3) var(--space-4) var(--space-3)

.chat-workspace-shell--compact
  padding: var(--space-2) var(--space-3) var(--space-2)

.chat-workspace-hero
  flex: 0 0 auto
  margin-bottom: var(--space-4)

.chat-workspace-hero__text
  max-width: 52rem

.chat-workspace-kicker
  display: inline-flex
  padding: var(--space-1) var(--space-3)
  border-radius: 999px
  font-size: var(--text-xs)
  font-weight: 700
  letter-spacing: 0.08em
  text-transform: uppercase
  border: 1px solid var(--glass-border)
  background: var(--glass-surface)
  color: var(--color-text-secondary)
  backdrop-filter: blur(var(--glass-blur-default))

.chat-workspace-title
  margin: var(--space-3) 0 0
  font-size: clamp(1.35rem, 2.5vw, 1.75rem)
  font-weight: 700
  letter-spacing: -0.03em
  line-height: 1.15
  color: var(--color-text-primary)

.chat-workspace-subtitle
  margin: var(--space-2) 0 0
  font-size: 0.9rem
  line-height: 1.55
  color: var(--color-text-secondary)

.chat-workspace-shell--dark .chat-workspace-kicker
  color: var(--color-text-secondary)

.chat-workspace-shell--dark .chat-workspace-title
  text-shadow: 0 0 12px color-mix(in srgb, var(--color-accent) 12%, transparent)
</style>
