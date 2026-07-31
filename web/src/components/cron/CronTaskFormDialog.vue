<!--
  Cron 域展示组件：仅 props / emits（aranea-frontend-guide SKILL §1 红线 #1）。
  路径：SKILL §3.3 → web/src/components/cron/。
  浮层皮肤：app-glass-dialog（--glass-elevated、backdrop-filter 双前缀、accent CTA）。
-->
<template>
  <q-dialog :model-value="modelValue" persistent @update:model-value="$emit('update:modelValue', $event)">
    <q-card class="app-dialog-card app-dialog-card--md app-glass-dialog">
      <q-card-section class="app-glass-dialog__head row items-start justify-between no-wrap">
        <div class="col min-width-0">
          <div class="app-glass-dialog__title">{{ row ? '编辑定时任务' : '创建定时任务' }}</div>
          <div class="app-glass-dialog__subtitle">安排定期 Agent 任务，计划字段会保存到 config_json。</div>
        </div>
        <q-btn
          flat
          dense
          round
          icon="close"
          class="app-dialog-icon-btn"
          aria-label="关闭"
          @click="$emit('update:modelValue', false)"
        />
      </q-card-section>
      <q-separator />

      <div class="app-glass-dialog__scroll">
        <q-card-section class="app-dialog-body app-glass-dialog__body">
          <CronTaskFormFields
            ref="fieldsRef"
            v-model:form="form"
            :agents="agents"
            :teams="teams"
            :editing="!!row"
            :server-error="serverError"
            @submit="onFormSubmit"
          />
        </q-card-section>
      </div>

      <q-separator />
      <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
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
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import type { Agent } from '../../features/agents/types';
import type { Team } from '../../features/teams/types';
import type { PlatformResourceInput } from '../../features/platform/types';
import type { CronTaskFormValue, CronTaskRow } from '../../features/cron/types';
import CronTaskFormFields from './CronTaskFormFields.vue';
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
