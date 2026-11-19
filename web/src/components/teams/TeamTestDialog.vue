<!--
  Team 运行测试对话框：仅 props / emits。
-->
<template>
  <AppDialog
    :model-value="modelValue"
    title="运行测试"
    :subtitle="team?.display_name || 'Team'"
    size="sm"
    @update:model-value="$emit('update:modelValue', $event)"
  >
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
      运行{{ teamRunStatusLabel(run.status) }} · {{ run.duration_ms }}ms · 输入 {{ run.token_in }} / 输出
      {{ run.token_out }}
    </div>
    <template #actions>
      <q-btn flat label="关闭" @click="$emit('update:modelValue', false)" />
      <q-btn
        color="primary"
        unelevated
        icon="science"
        label="执行测试"
        :loading="loading"
        @click="$emit('run', localContent)"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import AppDialog from '../common/AppDialog.vue';
import type { Team, TeamRun } from '../../features/teams/types';
import { teamRunStatusLabel } from './teamUtils';

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
