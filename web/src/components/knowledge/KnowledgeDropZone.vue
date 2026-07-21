<template>
  <div
    class="knowledge-drop-zone"
    :class="{ 'knowledge-drop-zone--active': dragOver }"
    role="button"
    tabindex="0"
    :aria-label="t('knowledgePage.dropzoneAria')"
    @dragenter.prevent="onDragEnter"
    @dragover.prevent="onDragOver"
    @dragleave.prevent="onDragLeave"
    @drop.prevent="onDrop"
    @click="openPicker"
    @keydown.enter.prevent="openPicker"
  >
    <q-icon name="cloud_upload" size="28px" color="primary" />
    <div class="text-body2 q-mt-xs">{{ t('knowledgePage.dropzoneTitle') }}</div>
    <div class="text-caption text-grey-7 q-mt-xs">{{ t('knowledgePage.dropzoneHint') }}</div>
    <input ref="fileInput" type="file" multiple hidden :accept="accept" @change="onPick" />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue';
import { useI18n } from 'vue-i18n';

const emit = defineEmits<{
  'files-selected': [files: File[]];
}>();

const { t } = useI18n();

// 与 KnowledgeIngestDialog 的可解析格式保持一致（图片经 Phase 9 VisionExtractor 入库）。
const accept = '.txt,.md,.json,.csv,.log,.html,.htm,.xml,.yaml,.yml,.toml,.pdf,.doc,.docx,.pptx,.xlsx,.png,.jpg,.jpeg,.webp';

const fileInput = ref<HTMLInputElement | null>(null);
const dragOver = ref(false);
// 嵌套子元素会触发成对的 dragenter/dragleave，用计数器避免高亮闪烁。
let dragDepth = 0;

function onDragEnter() {
  dragDepth += 1;
  dragOver.value = true;
}

function onDragOver() {
  dragOver.value = true;
}

function onDragLeave() {
  dragDepth = Math.max(0, dragDepth - 1);
  if (dragDepth === 0) dragOver.value = false;
}

function onDrop(e: DragEvent) {
  dragDepth = 0;
  dragOver.value = false;
  const files = Array.from(e.dataTransfer?.files ?? []);
  if (files.length) emit('files-selected', files);
}

function openPicker() {
  fileInput.value?.click();
}

function onPick(e: Event) {
  const input = e.target as HTMLInputElement;
  const files = Array.from(input.files ?? []);
  if (files.length) emit('files-selected', files);
  input.value = '';
}
</script>

<style scoped>
.knowledge-drop-zone {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 20px 16px;
  margin-bottom: 12px;
  border: 1.5px dashed var(--glass-border);
  border-radius: 16px;
  background: var(--glass-surface);
  cursor: pointer;
  text-align: center;
  transition:
    border-color 0.18s ease,
    background 0.18s ease;
}
.knowledge-drop-zone:hover,
.knowledge-drop-zone--active {
  border-color: var(--q-primary);
  background: var(--glass-surface-hover);
}
.knowledge-drop-zone:focus-visible {
  outline: 2px solid var(--q-primary);
  outline-offset: 2px;
}
</style>
