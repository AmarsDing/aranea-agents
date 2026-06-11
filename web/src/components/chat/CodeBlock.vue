<template>
  <div class="code-block">
    <!-- Header row -->
    <div class="code-block__header">
      <span class="code-block__lang">{{ displayLang }}</span>
      <button
        class="code-block__copy-btn"
        :title="copied ? t('chat.codeBlock.copied') : t('chat.codeBlock.copy')"
        @click="handleCopy"
      >
        <q-icon :name="copied ? 'check' : 'content_copy'" size="14px" />
      </button>
    </div>

    <!-- Collapsed state -->
    <div
      v-if="isCollapsed"
      class="code-block__collapsed"
      @click="isCollapsed = false"
    >
      ▶ {{ t('chat.codeBlock.expandLine', { count: lineCount }) }}
    </div>

    <!-- Expanded code body -->
    <pre v-else class="code-block__body"><code v-html="highlightedHtml" /></pre>

    <!-- Collapse toggle (only when expanded and > 20 lines) -->
    <div
      v-if="!isCollapsed && lineCount > 20"
      class="code-block__collapse"
      @click="isCollapsed = true"
    >
      {{ t('chat.codeBlock.collapseLine') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { detectLanguage, highlight } from '../../features/chat/lib/detectCodeLanguage'

const props = withDefaults(
  defineProps<{
    code: string
    lang?: string
    defaultCollapsed?: boolean
  }>(),
  {
    lang: undefined,
    defaultCollapsed: undefined,
  },
)

const { t } = useI18n()

const displayLang = computed(() => detectLanguage(props.code, props.lang))
const lineCount = computed(() => props.code.split('\n').length)
const isCollapsed = ref(props.defaultCollapsed ?? lineCount.value > 20)

const highlightedHtml = computed(() => highlight(props.code, displayLang.value))

const copied = ref(false)
let copyTimer: ReturnType<typeof setTimeout> | undefined

async function handleCopy() {
  try {
    await navigator.clipboard.writeText(props.code)
    copied.value = true
    clearTimeout(copyTimer)
    copyTimer = setTimeout(() => {
      copied.value = false
    }, 2000)
  } catch {
    // clipboard API unavailable — silently ignore
  }
}
</script>

<style scoped lang="sass">
.code-block
  background: var(--color-bg-elevated, var(--glass-surface))
  border: 1px solid var(--glass-border, var(--color-border))
  border-radius: var(--radius, 6px)
  overflow: hidden
  font-family: var(--font-family-mono, 'Cascadia Code', 'Fira Code', monospace)
  font-size: var(--font-size-base, 13px)

.code-block__header
  display: flex
  align-items: center
  justify-content: space-between
  padding: 4px 10px
  border-bottom: 1px solid var(--glass-border, var(--color-border))

.code-block__lang
  font-size: 10px
  text-transform: uppercase
  letter-spacing: 0.05em
  color: var(--color-text-tertiary)

.code-block__copy-btn
  display: inline-flex
  align-items: center
  justify-content: center
  background: none
  border: none
  cursor: pointer
  padding: 2px
  color: var(--color-text-tertiary)
  border-radius: 4px
  transition: color 0.15s ease, background 0.15s ease
  &:hover
    color: var(--color-text-primary)
    background: var(--glass-surface-hover)

.code-block__body
  overflow-x: auto
  padding: 8px 12px
  margin: 0
  background: transparent
  code
    font-family: inherit
    font-size: inherit

.code-block__collapsed
  padding: 8px 12px
  cursor: pointer
  color: var(--color-text-secondary)
  user-select: none
  &:hover
    color: var(--color-text-primary)

.code-block__collapse
  padding: 4px 12px
  cursor: pointer
  color: var(--color-text-secondary)
  text-align: center
  border-top: 1px solid var(--glass-border, var(--color-border))
  user-select: none
  &:hover
    color: var(--color-text-primary)
</style>
