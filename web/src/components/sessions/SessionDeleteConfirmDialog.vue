<template>
  <AppDialog
    :model-value="modelValue"
    title="确认永久删除"
    size="sm"
    icon="delete_forever"
    persistent
    :loading="loading"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <p class="app-text-secondary">{{ message }}</p>
    <template #actions>
      <q-btn flat rounded no-caps :label="t('shell.common.cancel')" @click="emit('update:modelValue', false)" />
      <q-btn
        unelevated
        rounded
        no-caps
        color="negative"
        data-test="session-delete-confirm"
        label="永久删除"
        :loading="loading"
        @click="emit('confirm')"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import AppDialog from '../common/AppDialog.vue';

const props = defineProps<{
  modelValue: boolean;
  count?: number;
  loading?: boolean;
}>();

const emit = defineEmits<{
  'update:modelValue': [v: boolean];
  confirm: [];
}>();

const { t } = useI18n();

const message = computed(() => {
  const n = props.count ?? 1;
  if (n <= 1) {
    return '删除后无法在历史列表中恢复（usage 统计仍保留）。确定要永久删除该会话吗？';
  }
  return `将永久删除 ${n} 个会话，删除后无法在历史列表中恢复（usage 统计仍保留）。确定继续吗？`;
});
</script>
