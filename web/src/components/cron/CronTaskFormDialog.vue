<!--
  Cron 域展示组件：仅 props / emits（vue-design.md §0.2）。
  路径：vue-design.md §2 → web/src/components/cron/。
  浮层皮肤：UX.md §5.2a（--glass-elevated、backdrop-filter 双前缀、accent CTA）。
-->
<template>
  <q-dialog :model-value="modelValue" persistent @update:model-value="$emit('update:modelValue', $event)">
    <q-card class="cron-form-dialog app-dialog-card app-dialog-card--md" :class="{ 'is-dark': $q.dark.isActive }">
      <q-card-section class="row items-start justify-between q-gutter-md">
        <div>
          <div class="text-h6 text-primary-contrast">{{ row ? "编辑定时任务" : "创建定时任务" }}</div>
          <div class="text-caption cron-form-subtitle">安排定期 Agent 任务，计划字段会保存到 config_json。</div>
        </div>
        <q-btn flat dense round icon="close" class="cron-form-icon-btn" aria-label="关闭" @click="$emit('update:modelValue', false)" />
      </q-card-section>
      <q-separator class="cron-form-sep" />

      <q-card-section>
        <CronTaskFormFields
          ref="fieldsRef"
          v-model:form="form"
          :agents="agents"
          :teams="teams"
          :server-error="serverError"
          @submit="onFormSubmit"
        />
      </q-card-section>

      <q-separator class="cron-form-sep" />
      <q-card-actions align="right">
        <q-btn flat rounded class="cron-form-cancel" label="取消" :disable="submitting" @click="$emit('update:modelValue', false)" />
        <q-btn
          rounded
          unelevated
          class="cron-form-submit"
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
import { computed, reactive, ref, watch } from "vue";
import { useQuasar } from "quasar";
import type { Agent } from "../../features/agents/api";
import type { Team } from "../../features/teams/api";
import type { PlatformResourceInput } from "../../features/platform/api";
import type { CronTaskFormValue, CronTaskRow } from "../../features/cron/types";
import CronTaskFormFields from "./CronTaskFormFields.vue";
import {
  applyCronRowToForm,
  buildCronTaskPayload,
  canSaveCronForm,
  emptyCronTaskForm
} from "./cronTaskUtils";

const props = defineProps<{
  modelValue: boolean;
  row: CronTaskRow | null;
  agents: Agent[];
  teams: Team[];
  submitting?: boolean;
  serverError?: string;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
  submit: [payload: PlatformResourceInput];
}>();

const $q = useQuasar();
const fieldsRef = ref<InstanceType<typeof CronTaskFormFields> | null>(null);
const form = reactive<CronTaskFormValue>(emptyCronTaskForm());

const canSave = computed(() => canSaveCronForm(form));

watch(
  () => props.modelValue,
  (open) => {
    if (open) applyCronRowToForm(props.row, form);
  }
);

async function onFormSubmit() {
  const raw = await fieldsRef.value?.validate?.();
  if (raw === false) return;
  emit("submit", buildCronTaskPayload(form, props.row));
}
</script>

<style scoped>
.cron-form-dialog {
  /* glass + width from app-dialog-card */
}

.cron-form-subtitle {
  color: var(--color-text-secondary);
}

.cron-form-sep {
  opacity: 55%;
}

.cron-form-icon-btn {
  color: var(--color-icon-muted);
}

.cron-form-icon-btn:hover {
  color: var(--color-accent);
}

.text-primary-contrast {
  color: var(--color-text-primary);
}

.cron-form-cancel {
  color: var(--color-text-secondary);
}

.cron-form-submit {
  background: var(--color-accent) !important;
  color: var(--color-on-accent) !important;
}

.cron-form-submit:hover {
  background: var(--color-accent-hover) !important;
}
</style>
