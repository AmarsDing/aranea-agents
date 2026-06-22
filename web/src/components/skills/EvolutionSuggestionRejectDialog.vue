<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-glass-dialog">
      <q-card-section class="app-glass-dialog__head row items-center justify-between">
        <div class="app-glass-dialog__title">拒绝进化建议</div>
        <q-btn v-close-popup flat round dense icon="close" />
      </q-card-section>
      <q-separator />
      <div class="app-glass-dialog__scroll">
        <q-card-section class="app-dialog-body app-glass-dialog__body q-gutter-md">
          <div v-if="target" class="text-body2">
            <div class="q-mb-sm"><span class="text-grey-7">目标 ID：</span>{{ target.targetId || '—' }}</div>
            <div class="q-mb-sm">
              <span class="text-grey-7">操作类型：</span>{{ evoActionTypeLabel(target.actionType) }}
            </div>
            <div><span class="text-grey-7">触发原因：</span>{{ target.triggerReason || '—' }}</div>
          </div>
          <q-input
            :model-value="reason"
            dense
            outlined
            type="textarea"
            autogrow
            label="拒绝原因"
            placeholder="请输入拒绝原因（可选）"
            @update:model-value="(v: string | number | null) => $emit('update:reason', String(v ?? ''))"
          />
        </q-card-section>
      </div>
      <q-separator />
      <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
        <q-btn v-close-popup flat no-caps label="取消" />
        <q-btn color="negative" unelevated no-caps label="确认拒绝" :loading="loading" @click="$emit('confirm')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
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
</script>
