<template>
  <section class="tool-console" :aria-label="title">
    <header class="tool-console__head">
      <q-icon name="terminal" size="16px" aria-hidden="true" />
      <span>{{ title }}</span>
      <q-space />
      <q-spinner v-if="busy" size="14px" />
    </header>
    <pre ref="box" class="tool-console__body" aria-live="polite">{{ text || placeholder }}</pre>
  </section>
</template>

<script setup lang="ts">
import { nextTick, ref, watch } from 'vue';

// 共用执行日志面板：添加、测试都往这里追加文本，自动滚到最底
const props = defineProps<{ title: string; text: string; placeholder?: string; busy?: boolean }>();
const box = ref<HTMLElement | null>(null);

watch(
  () => props.text,
  async () => {
    await nextTick();
    if (box.value) box.value.scrollTop = box.value.scrollHeight;
  },
);
</script>

<style scoped lang="sass">
.tool-console
  display: flex
  flex-direction: column
  min-height: 0
  border: 1px solid var(--nt-border-1)
  border-radius: var(--nt-radius-m)
  background: var(--nt-data-bg)
.tool-console__head
  display: flex
  align-items: center
  gap: var(--nt-space-2)
  padding: var(--nt-space-2) var(--nt-space-3)
  border-bottom: 1px solid var(--nt-border-1)
  color: var(--nt-text-2)
  font-size: var(--nt-fs-caption)
.tool-console__body
  flex: 1
  min-height: 160px
  max-height: 280px
  margin: 0
  padding: var(--nt-space-2) var(--nt-space-3)
  overflow: auto
  color: var(--nt-text-1)
  font-family: var(--nt-font-mono)
  font-size: var(--nt-fs-micro)
  font-variant-numeric: tabular-nums
  line-height: 1.6
  white-space: pre-wrap
  overflow-wrap: anywhere
</style>
