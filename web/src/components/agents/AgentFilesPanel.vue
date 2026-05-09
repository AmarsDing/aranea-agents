<template>
  <q-splitter v-model="splitterModel" class="files-splitter fit">
    <template #before>
      <q-list bordered separator class="file-list">
        <q-item v-for="file in files" :key="file.name" clickable :active="activeFile === file.name" active-class="file-item--active" class="file-item" @click="$emit('update:activeFile', file.name)">
          <q-item-section>
            <q-item-label>{{ file.name }}</q-item-label>
            <q-item-label caption>{{ tokenText(file.body) }}</q-item-label>
          </q-item-section>
        </q-item>
      </q-list>
    </template>

    <template #after>
      <div class="file-editor q-pa-md column fit">
        <div class="row items-start justify-between q-gutter-md flex-shrink-0">
          <div>
            <div class="text-h6">{{ activeFile }}</div>
            <div class="text-caption text-grey-7">{{ activeFileMeta.caption }}</div>
          </div>
          <div class="row q-gutter-sm">
            <q-btn outline rounded icon="refresh" label="重新召唤" @click="$emit('reload')" />
            <q-btn outline rounded color="primary" icon="auto_fix_high" label="AI 编辑" @click="$emit('ai-edit')" />
            <q-btn color="primary" rounded unelevated icon="save" label="保存" :disable="!dirty" @click="$emit('save')" />
          </div>
        </div>
        <q-input v-model="bodyModel" class="q-mt-md markdown-editor" outlined type="textarea" label="Markdown" />
        <div class="file-editor__footer flex-shrink-0">Token 估算：{{ tokenEstimateFor(bodyModel) }}</div>
      </div>
    </template>
  </q-splitter>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { AgentFile } from "./agentUi";
import { tokenEstimateFor, tokenText } from "./agentUi";

const props = defineProps<{
  files: AgentFile[];
  activeFile: string;
  splitter: number;
  dirty: boolean;
}>();

const emit = defineEmits<{
  "update:activeFile": [value: string];
  "update:splitter": [value: number];
  "update-file-body": [fileName: string, body: string];
  reload: [];
  "ai-edit": [];
  save: [];
}>();

const splitterModel = computed({
  get: () => props.splitter,
  set: (value: number) => emit("update:splitter", value)
});

const activeFileMeta = computed(() => props.files.find((file) => file.name === props.activeFile) ?? props.files[0]);

const bodyModel = computed({
  get: () => activeFileMeta.value?.body ?? "",
  set: (value: string) => emit("update-file-body", props.activeFile, value)
});
</script>

<style scoped>
.files-splitter {
  flex: 1 1 auto;
  min-height: 0;
  height: 100%;
  border: 1px solid var(--glass-border);
  border-radius: 22px;
  overflow: hidden;
  background: var(--glass-surface);
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
  box-shadow: none;
}

.files-splitter :deep(.q-splitter__separator) {
  background: var(--glass-border);
}

.files-splitter :deep(.q-splitter__before),
.files-splitter :deep(.q-splitter__after) {
  min-height: 0;
}

.file-list {
  height: 100%;
  border-radius: 0;
  background: var(--glass-elevated);
  color: var(--color-text-primary);
  border-color: var(--glass-border);
}

.file-item {
  margin: 8px;
  border-radius: 14px;
  color: var(--color-text-secondary);
}

.file-item:hover {
  background: var(--glass-surface-hover);
}

.file-item--active {
  background: color-mix(in srgb, var(--color-accent) 14%, transparent);
  color: var(--color-text-primary);
  font-weight: 700;
  box-shadow: var(--glass-inner-highlight);
}

.file-editor {
  flex: 1 1 auto;
  min-height: 0;
  color: var(--color-text-primary);
  background:
    radial-gradient(circle at top right, color-mix(in srgb, var(--color-accent) 12%, transparent), transparent 42%),
    var(--glass-surface);
  backdrop-filter: blur(var(--glass-blur-default));
  -webkit-backdrop-filter: blur(var(--glass-blur-default));
}

.markdown-editor {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.markdown-editor :deep(.q-field__inner) {
  flex: 1 1 auto;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.markdown-editor :deep(.q-field__control) {
  flex: 1 1 auto;
  min-height: 120px;
  border-radius: 18px;
  background: var(--glass-elevated);
  box-shadow: var(--glass-inner-highlight);
}

.markdown-editor :deep(textarea.q-field__native) {
  flex: 1 1 auto;
  min-height: 160px;
  color: var(--color-text-primary);
  line-height: 1.65;
  font-family: ui-monospace, SFMono-Regular, Consolas, "Liberation Mono", monospace;
  resize: vertical;
}

.file-editor__footer {
  margin-top: 10px;
  color: var(--color-text-secondary);
  font-size: 12px;
  font-weight: 600;
}

body.body--dark .file-item--active {
  box-shadow:
    0 0 0 1px color-mix(in srgb, var(--color-accent) 32%, transparent),
    var(--glass-inner-highlight);
}
</style>
