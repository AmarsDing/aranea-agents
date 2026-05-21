<template>
  <q-dialog :model-value="modelValue" persistent @update:model-value="$emit('update:modelValue', $event)">
    <q-card class="sessions-dialog-card" style="min-width: 400px; max-width: 480px">
      <q-card-section>
        <div class="text-h6 text-weight-bold">{{ title }}</div>
        <div class="text-caption q-mt-xs" style="color: var(--color-text-secondary)">{{ subtitle }}</div>
        <q-input
          v-model.number="days"
          type="number"
          dense
          outlined
          class="q-mt-md sessions-field"
          label="保留最近（天）"
          :min="1"
        />
        <q-checkbox
          v-if="mode === 'delete'"
          v-model="includeArchived"
          dense
          class="q-mt-sm"
          label="包含已归档会话"
        />
        <div v-if="preview" class="q-mt-md q-pa-sm sessions-retention-preview">
          <div>将{{ actionVerb }} <strong>{{ preview.matched }}</strong> 个会话</div>
          <div class="text-caption q-mt-xs">
            保留最近 {{ days }} 天
            <span v-if="preview.skipped_running">；运行中将跳过（{{ preview.skipped_running }}）</span>
            <span v-if="preview.skipped_not_found">；未找到（{{ preview.skipped_not_found }}）</span>
            <span v-if="preview.truncated">；扫描达上限，请分批执行</span>
          </div>
        </div>
        <div v-else-if="previewLoading" class="q-mt-md row items-center q-gutter-sm">
          <q-spinner size="20px" color="primary" />
          <span class="text-caption">正在预览…</span>
        </div>
      </q-card-section>
      <q-card-actions align="right" class="q-pa-md q-pt-none">
        <q-btn flat rounded label="取消" class="sessions-btn-ghost" @click="$emit('update:modelValue', false)" />
        <q-btn
          flat
          rounded
          label="预览"
          class="sessions-btn-ghost"
          :loading="previewLoading"
          @click="$emit('preview', { days: Math.max(1, Number(days) || 30), includeArchived })"
        />
        <q-btn
          unelevated
          rounded
          :color="mode === 'delete' ? 'negative' : 'primary'"
          :label="confirmLabel"
          :disable="!preview || preview.matched === 0"
          :loading="loading"
          @click="$emit('confirm', { days: Math.max(1, Number(days) || 30), includeArchived })"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { BatchPreviewResult, RetentionDialogMode } from "../../features/session/types";

const props = defineProps<{
  modelValue: boolean;
  mode: RetentionDialogMode;
  preview: BatchPreviewResult | null;
  previewLoading?: boolean;
  loading?: boolean;
}>();

defineEmits<{
  "update:modelValue": [v: boolean];
  preview: [payload: { days: number; includeArchived: boolean }];
  confirm: [payload: { days: number; includeArchived: boolean }];
}>();

const days = ref(30);
const includeArchived = ref(false);

const title = computed(() => (props.mode === "archive" ? "按天数批量归档" : "按天数批量删除"));
const subtitle = computed(() =>
  props.mode === "archive"
    ? "归档 cutoff 之前的会话，保留最近 N 天。"
    : "永久删除 cutoff 之外的会话，保留最近 N 天。"
);
const actionVerb = computed(() => (props.mode === "archive" ? "归档" : "删除"));
const confirmLabel = computed(() => (props.mode === "archive" ? "确认归档" : "确认删除"));

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      days.value = 30;
      includeArchived.value = false;
    }
  }
);
</script>

<style scoped>
.sessions-retention-preview {
  border-radius: 10px;
  border: 1px solid var(--glass-border);
  background: color-mix(in srgb, var(--canvas-base) 6%, transparent);
}
</style>
