<template>
  <q-dialog :model-value="modelValue" @update:model-value="emit('update:modelValue', $event)">
    <q-card style="width: 420px; max-width: 92vw">
      <q-card-section>
        <div class="text-h6">删除 Skill</div>
        <div class="text-body2 text-grey-7 q-mt-sm">确认删除「{{ skill?.name }}」？此操作会软删除，列表中不再显示。</div>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn flat rounded label="取消" v-close-popup />
        <q-btn color="negative" rounded unelevated label="删除" :loading="loading" @click="emit('confirm')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import type { Skill } from "../types";

defineProps<{
  modelValue: boolean;
  skill: Skill | null;
  loading?: boolean;
}>();

const emit = defineEmits<{
  "update:modelValue": [value: boolean];
  confirm: [];
}>();
</script>
