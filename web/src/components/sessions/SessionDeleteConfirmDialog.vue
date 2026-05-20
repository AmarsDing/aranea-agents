<template>
  <q-dialog :model-value="modelValue" persistent @update:model-value="$emit('update:modelValue', $event)">
    <q-card class="sessions-dialog-card" style="min-width: 360px; max-width: 440px">
      <q-card-section>
        <div class="text-h6 text-weight-bold">确认永久删除</div>
        <div class="text-body2 q-mt-sm" style="color: var(--color-text-secondary)">
          {{ message }}
        </div>
      </q-card-section>
      <q-card-actions align="right" class="q-pa-md q-pt-none">
        <q-btn flat rounded label="取消" class="sessions-btn-ghost" @click="$emit('update:modelValue', false)" />
        <q-btn unelevated rounded color="negative" label="永久删除" :loading="loading" @click="$emit('confirm')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  modelValue: boolean;
  count?: number;
  loading?: boolean;
}>();

defineEmits<{
  "update:modelValue": [v: boolean];
  confirm: [];
}>();

const message = computed(() => {
  const n = props.count ?? 1;
  if (n <= 1) {
    return "删除后无法在历史列表中恢复（usage 统计仍保留）。确定要永久删除该会话吗？";
  }
  return `将永久删除 ${n} 个会话，删除后无法在历史列表中恢复（usage 统计仍保留）。确定继续吗？`;
});
</script>
