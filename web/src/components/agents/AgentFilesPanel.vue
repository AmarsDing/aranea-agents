<template>
  <q-splitter v-model="splitterModel" class="files-splitter">
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
      <div class="file-editor q-pa-md">
        <div class="row items-start justify-between q-gutter-md">
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
        <div class="file-editor__footer">Token 估算：{{ tokenEstimateFor(bodyModel) }}</div>
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
  min-height: 650px;
  border: 1px solid rgba(15, 23, 42, 0.08);
  border-radius: 22px;
  overflow: hidden;
  background: #ffffff;
  box-shadow: 0 18px 48px rgba(16, 24, 40, 0.06);
}

.file-list {
  height: 100%;
  border-radius: 0;
  background: #f8fafc;
  color: #344054;
  border-color: rgba(15, 23, 42, 0.08);
}

.file-item {
  margin: 8px;
  border-radius: 14px;
  color: #475467;
}

.file-item--active {
  background: #eaf3ff;
  color: #155ebc;
  font-weight: 700;
}

.file-editor {
  min-height: 650px;
  color: #1d2939;
  background:
    radial-gradient(circle at top right, rgba(25, 118, 210, 0.08), transparent 32%),
    #ffffff;
}

.markdown-editor :deep(.q-field__control) {
  border-radius: 18px;
  background: #fbfcff;
}

.markdown-editor :deep(textarea) {
  min-height: 440px;
  color: #1d2939;
  line-height: 1.65;
  font-family: ui-monospace, SFMono-Regular, Consolas, "Liberation Mono", monospace;
}

.file-editor__footer {
  margin-top: 10px;
  color: #667085;
  font-size: 12px;
  font-weight: 600;
}
</style>
