<template>
  <pre class="tool-json-block">{{ text }}</pre>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    text?: string;
  }>(),
  { text: "" }
);
</script>

<style scoped lang="sass">
.tool-json-block
  margin: 0
  padding: 12px
  border-radius: 12px
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace
  font-size: 12px
  line-height: 1.45
  white-space: pre-wrap
  word-break: break-word
  max-height: 320px
  overflow: auto
  color: var(--color-text-primary)
  border: 1px solid rgba(208, 192, 168, 0.55)
  background: rgba(255, 253, 245, 0.92)
  box-shadow: inset var(--glass-inner-highlight, inset 0 1px 0 rgba(255, 255, 255, 0.35))

body.body--dark .tool-json-block
  border-color: var(--glass-border)
  background: rgba(12, 18, 28, 0.94)
  color: var(--color-text-primary)
  box-shadow: none
</style>
