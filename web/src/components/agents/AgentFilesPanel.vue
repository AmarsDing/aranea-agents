<template>
  <q-splitter v-model="splitterModel" class="agent-files-splitter fit">
    <template #before>
      <q-list bordered separator class="agent-file-list">
        <q-item v-for="file in files" :key="file.name" clickable :active="activeFile === file.name" active-class="agent-file-item--active" class="agent-file-item" @click="$emit('update:activeFile', file.name)">
          <q-item-section>
            <q-item-label>{{ file.name }}</q-item-label>
            <q-item-label caption>{{ fileTokenLabel(file.name, file.body) }}</q-item-label>
          </q-item-section>
        </q-item>
      </q-list>
    </template>

    <template #after>
      <div class="agent-file-editor q-pa-md column fit">
        <div class="row items-start justify-between q-gutter-md flex-shrink-0">
          <div>
            <div class="text-h6">{{ activeFile }}</div>
            <div class="text-caption text-grey-7">{{ activeFileMeta.caption }}</div>
          </div>
          <div class="row q-gutter-sm">
            <q-btn outline rounded icon="refresh" label="重新召唤" @click="$emit('reload')" />
            <!-- PGO-3-WEB-03: AIRefineButton for file Tab editor. -->
            <AIRefineButton
              scope="agent.file"
              :file-name="activeFile"
              :resource-id="agentId || undefined"
              :text="bodyModel"
              outline
              @apply="(v: string) => emit('update-file-body', activeFile, v)"
            />
            <q-btn color="primary" rounded unelevated icon="save" label="保存" :disable="!dirty" @click="$emit('save')" />
          </div>
        </div>
        <q-input v-model="bodyModel" class="q-mt-md app-markdown-editor" outlined type="textarea" label="Markdown" />
        <div class="agent-file-editor__footer flex-shrink-0">Token 估算：{{ activeTokenCount }}</div>
      </div>
    </template>
  </q-splitter>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { AgentFile } from "./agentUi";
import { tokenEstimateFor, tokenText } from "./agentUi";
import AiRefineButton from "./AIRefineButton.vue";

const props = defineProps<{
  files: AgentFile[];
  activeFile: string;
  splitter: number;
  dirty: boolean;
  fileTokenByName?: Record<string, number>;
  agentId?: string;
}>(); 

function fileTokenLabel(name: string, body: string) {
  const n = props.fileTokenByName?.[name];
  if (n != null && n > 0) return `约 ${n} token（服务端）`;
  return tokenText(body);
}

const activeTokenCount = computed(() => {
  const name = props.activeFile;
  const n = props.fileTokenByName?.[name];
  if (n != null && n > 0) return n;
  return tokenEstimateFor(bodyModel.value);
});

const emit = defineEmits<{
  "update:activeFile": [value: string];
  "update:splitter": [value: number];
  "update-file-body": [fileName: string, body: string];
  reload: [];
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
