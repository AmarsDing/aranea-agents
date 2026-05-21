<!--
  Team 运行测试对话框：仅 props / emits。
-->
<template>
  <q-dialog :model-value="modelValue" @update:model-value="$emit('update:modelValue', $event)">
    <q-card :class="['team-test-dialog', { 'is-dark': isDark }]" style="min-width: min(520px, 94vw)">
      <q-card-section class="row items-center justify-between">
        <div>
          <div class="text-h6">运行测试</div>
          <div class="text-caption text-grey-7">{{ team?.display_name || "Team" }}</div>
        </div>
        <q-btn flat round icon="close" @click="$emit('update:modelValue', false)" />
      </q-card-section>
      <q-separator />
      <q-card-section>
        <q-input
          v-model="localContent"
          type="textarea"
          autogrow
          outlined
          label="测试消息"
          hint="留空则使用默认英文探针"
          :disable="loading"
        />
        <q-banner v-if="error" rounded class="bg-negative text-white q-mt-md">{{ error }}</q-banner>
        <div v-if="reply" class="q-mt-md">
          <div class="text-caption text-grey-7 q-mb-xs">Team 回复</div>
          <div class="team-test-reply">{{ reply }}</div>
        </div>
        <div v-if="run" class="q-mt-sm text-caption text-grey-7">
          Run {{ run.status }} · {{ run.duration_ms }}ms · in {{ run.token_in }} / out {{ run.token_out }}
        </div>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn flat label="关闭" @click="$emit('update:modelValue', false)" />
        <q-btn color="primary" unelevated icon="science" label="执行测试" :loading="loading" @click="$emit('run', localContent)" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from "vue";
import type { Team, TeamRun } from "../../features/teams/types";

const props = defineProps<{
  modelValue: boolean;
  team: Team | null;
  loading: boolean;
  error: string;
  reply: string;
  run: TeamRun | null;
  isDark: boolean;
}>();

defineEmits<{
  "update:modelValue": [value: boolean];
  run: [content: string];
}>();

const localContent = ref("");

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      localContent.value = "";
    }
  }
);
</script>

<style scoped>
.team-test-dialog {
  border-radius: 20px;
  border: 1px solid rgb(15 23 42 / 8%);
  background: rgb(255 255 255 / 96%);
  backdrop-filter: blur(16px);
}

.team-test-reply {
  white-space: pre-wrap;
  padding: 12px;
  border-radius: 14px;
  border: 1px solid rgb(15 23 42 / 8%);
  background: var(--color-page-tint);
  line-height: 1.55;
}

.team-test-dialog.is-dark {
  border-color: rgb(148 163 184 / 16%);
  background: rgb(17 24 39 / 94%);
}

.team-test-dialog.is-dark .team-test-reply {
  border-color: rgb(148 163 184 / 14%);
  background: rgb(30 41 59 / 76%);
}
</style>
