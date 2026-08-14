<template>
  <q-dialog :model-value="modelValue" @update:model-value="emit('update:modelValue', $event)">
    <q-card class="skill-ux-dialog-card app-dialog-card app-dialog-card--sm">
      <q-card-section>
        <div class="text-h6">删除 Skill</div>
        <div class="text-body2 text-grey-7 q-mt-sm">
          确认删除「{{ skill?.name }}」？此操作不可撤销，将永久删除 Skill 及其磁盘文件。
        </div>
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar">
        <q-btn v-close-popup flat rounded no-caps label="取消" />
        <q-btn color="negative" rounded unelevated no-caps label="删除" :loading="loading" @click="emit('confirm')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import type { Skill } from '../../features/skills/types';

defineProps<{
  modelValue: boolean;
  skill: Skill | null;
  loading?: boolean;
}>();

const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  confirm: [];
}>();
</script>
