<template>
  <div class="json-code-viewer">
    <div v-if="showToolbar" class="json-code-viewer__toolbar row items-center q-gutter-xs q-mb-xs">
      <q-btn flat dense no-caps size="sm" icon="auto_fix_high" label="格式化" @click="formatInPlace" />
      <q-btn flat dense no-caps size="sm" icon="content_copy" label="复制" @click="copyText" />
      <q-space />
      <span v-if="parseError" class="text-caption text-negative">{{ parseError }}</span>
    </div>
    <q-scroll-area :style="{ height: scrollHeight }" class="json-code-viewer__scroll">
      <pre class="json-code-viewer__pre" v-html="highlightedHtml"></pre>
    </q-scroll-area>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { formatJsonText, highlightJsonHtml } from '../../utils/jsonFormat';

const props = withDefaults(
  defineProps<{
    text?: string;
    scrollHeight?: string;
    showToolbar?: boolean;
  }>(),
  {
    text: '',
    scrollHeight: '320px',
    showToolbar: true,
  },
);

const emit = defineEmits<{
  (e: 'copy-success'): void;
  (e: 'copy-error', message: string): void;
}>();
const displayText = ref('');
const parseError = ref('');

watch(
  () => props.text,
  (value) => {
    displayText.value = formatJsonText(value || '');
    parseError.value = '';
  },
  { immediate: true },
);

const highlightedHtml = computed(() => highlightJsonHtml(displayText.value));

function formatInPlace() {
  const next = formatJsonText(displayText.value);
  if (next === displayText.value && displayText.value.trim()) {
    try {
      JSON.parse(displayText.value);
    } catch (e) {
      parseError.value = e instanceof Error ? e.message : 'JSON 格式错误';
      return;
    }
  }
  displayText.value = next;
  parseError.value = '';
}

async function copyText() {
  try {
    await navigator.clipboard.writeText(displayText.value);
    emit('copy-success');
  } catch {
    emit('copy-error', '复制失败');
  }
}
</script>
