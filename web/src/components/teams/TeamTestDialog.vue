<!--
  Team 运行测试对话框：仅 props / emits。
-->
<template>
  <q-dialog :model-value="modelValue" @update:model-value="$emit('update:modelValue', $event)">
    <q-card :class="['team-test-dialog app-dialog-card app-dialog-card--sm', { 'is-dark': isDark }]">
      <q-card-section class="row items-center justify-between">
        <div>
          <div class="text-h6">运行测试</div>
          <div class="text-caption text-grey-7">{{ team?.display_name || 'Team' }}</div>
        </div>
        <q-btn flat round icon="close" @click="$emit('update:modelValue', false)" />
      </q-card-section>
      <q-separator />
      <q-card-section>
        <q-input
          v-model="localContent"
          class="app-field-long"
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
          <div class="app-code-block team-test-reply">{{ reply }}</div>
        </div>
        <div v-if="run" class="q-mt-sm text-caption text-grey-7">
          Run {{ run.status }} · {{ run.duration_ms }}ms · in {{ run.token_in }} / out {{ run.token_out }}
        </div>
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar">
        <q-btn flat label="关闭" @click="$emit('update:modelValue', false)" />
        <q-btn
          color="primary"
          unelevated
          icon="science"
          label="执行测试"
          :loading="loading"
          @click="$emit('run', localContent)"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import type { Team, TeamRun } from '../../features/teams/types';

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
  'update:modelValue': [value: boolean];
  run: [content: string];
}>();

const localContent = ref('');

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      localContent.value = '';
    }
  },
);
</script>
