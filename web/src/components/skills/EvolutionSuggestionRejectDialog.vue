<template>
  <AppDialog
    :model-value="open"
    title="拒绝进化建议"
    size="md"
    icon="block"
    persistent
    :loading="loading"
    @update:model-value="$emit('update:open', $event)"
  >
    <div class="reject-dialog">
      <div v-if="target" class="text-body2">
        <div class="q-mb-sm"><span class="app-text-secondary">目标 ID：</span>{{ target.targetId || '—' }}</div>
        <div class="q-mb-sm">
          <span class="app-text-secondary">操作类型：</span>{{ evoActionTypeLabel(target.actionType) }}
        </div>
        <div><span class="app-text-secondary">触发原因：</span>{{ target.triggerReason || '—' }}</div>
      </div>
      <q-input
        :model-value="reason"
        dense
        outlined
        type="textarea"
        autogrow
        label="拒绝原因"
        placeholder="请输入拒绝原因（可选）"
        data-test="reject-reason"
        @update:model-value="(v: string | number | null) => $emit('update:reason', String(v ?? ''))"
      />
    </div>
    <template #actions>
      <q-btn flat no-caps :label="t('shell.common.cancel')" @click="$emit('update:open', false)" />
      <q-btn
        color="negative"
        unelevated
        no-caps
        data-test="reject-confirm"
        label="确认拒绝"
        :loading="loading"
        @click="$emit('confirm')"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import AppDialog from '../common/AppDialog.vue';
import type { SkillEvolutionView } from '../../features/skills/types';
import { evoActionTypeLabel } from './evolutionSuggestionTableUi';

defineProps<{
  open: boolean;
  target: SkillEvolutionView | null;
  reason: string;
  loading: boolean;
}>();

defineEmits<{
  'update:open': [value: boolean];
  'update:reason': [value: string];
  confirm: [];
}>();

const { t } = useI18n();
</script>

<style scoped lang="sass">
.reject-dialog
  display: flex
  flex-direction: column
  gap: var(--nt-space-3)
</style>
