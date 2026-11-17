<template>
  <AppDialog
    :model-value="modelValue"
    title="删除 Skill"
    size="sm"
    icon="delete_outline"
    :loading="loading"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <p class="text-body2 app-text-secondary">
      确认删除「{{ skill?.name }}」？此操作不可撤销，将永久删除 Skill 及其磁盘文件。
    </p>
    <template #actions>
      <q-btn flat rounded no-caps :label="t('shell.common.cancel')" @click="emit('update:modelValue', false)" />
      <q-btn
        color="negative"
        rounded
        unelevated
        no-caps
        data-test="skill-delete-confirm"
        label="删除"
        :loading="loading"
        @click="emit('confirm')"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppDialog from '../common/AppDialog.vue';
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

const { t } = useI18n();
</script>
