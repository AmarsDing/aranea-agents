<!--
  Cron 域展示组件：仅 props / emits（aranea-frontend-guide SKILL §1 红线 #1）。
  路径：SKILL §3.3 → web/src/components/cron/。
  浮层外壳：共享 AppDialog（标题栏 + 滚动 body + #actions 底部条）。
-->
<template>
  <AppDialog
    :model-value="modelValue"
    :title="row ? '编辑定时任务' : '创建定时任务'"
    subtitle="安排定期 Agent 任务，计划字段会保存到 config_json。"
    persistent
    :loading="Boolean(submitting)"
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <CronTaskFormFields
      ref="fieldsRef"
      v-model:form="form"
      :agents="agents"
      :teams="teams"
      :editing="!!row"
      :server-error="serverError"
      @submit="onFormSubmit"
    />
    <template #actions>
      <q-btn
        flat
        rounded
        class="app-dialog-muted-btn"
        label="取消"
        :disable="submitting"
        @click="$emit('update:modelValue', false)"
      />
      <q-btn
        rounded
        unelevated
        class="app-dialog-accent-btn"
        :label="row ? '保存' : '创建'"
        :loading="submitting"
        :disable="!canSave || submitting"
        type="button"
        @click="onFormSubmit"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import type { Agent } from '../../features/agents/types';
import type { Team } from '../../features/teams/types';
import type { PlatformResourceInput } from '../../features/platform/types';
import type { CronTaskFormValue, CronTaskRow } from '../../features/cron/types';
import CronTaskFormFields from './CronTaskFormFields.vue';
import AppDialog from '../common/AppDialog.vue';
import { applyCronRowToForm, buildCronTaskPayload, canSaveCronForm, emptyCronTaskForm } from './cronTaskUtils';

const props = defineProps<{
  modelValue: boolean;
  row: CronTaskRow | null;
  agents: Agent[];
  teams: Team[];
  submitting?: boolean;
  serverError?: string;
}>();

const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  submit: [payload: PlatformResourceInput];
}>();

const fieldsRef = ref<InstanceType<typeof CronTaskFormFields> | null>(null);
const form = reactive<CronTaskFormValue>(emptyCronTaskForm());

const canSave = computed(() => canSaveCronForm(form, !!props.row));

watch(
  () => props.modelValue,
  (open) => {
    if (open) applyCronRowToForm(props.row, form);
  },
);

async function onFormSubmit() {
  const raw = await fieldsRef.value?.validate?.();
  if (raw === false) return;
  emit('submit', buildCronTaskPayload(form, props.row));
}
</script>
