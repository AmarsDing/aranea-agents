<template>
  <q-dialog
    :model-value="open"
    persistent
    maximized
    transition-show="slide-up"
    transition-hide="slide-down"
    @update:model-value="$emit('update:open', $event)"
  >
    <q-card class="app-dialog-card app-glass-dialog">
      <q-card-section class="app-glass-dialog__head row items-center justify-between">
        <div class="app-glass-dialog__title">进化建议详情</div>
        <q-btn v-close-popup flat round dense icon="close" />
      </q-card-section>
      <q-separator />
      <div class="app-glass-dialog__scroll">
        <q-card-section class="app-dialog-body app-glass-dialog__body q-gutter-md">
          <template v-if="suggestion">
            <div class="row q-col-gutter-sm text-body2">
              <div class="col-6">
                <span class="text-weight-medium text-grey-7">目标类型：</span>
                <q-chip dense square :color="evoTargetTypeColor(suggestion.targetType)" text-color="white">
                  {{ evoTargetTypeLabel(suggestion.targetType) }}
                </q-chip>
              </div>
              <div class="col-6">
                <span class="text-weight-medium text-grey-7">目标 ID：</span>{{ suggestion.targetId || '—' }}
              </div>
              <div class="col-6">
                <span class="text-weight-medium text-grey-7">操作类型：</span>
                <q-chip dense square :color="evoActionTypeColor(suggestion.actionType)" text-color="white">
                  {{ evoActionTypeLabel(suggestion.actionType) }}
                </q-chip>
              </div>
              <div class="col-6">
                <span class="text-weight-medium text-grey-7">状态：</span>
                <q-chip dense square :color="evoSuggestionStatusColor(suggestion.status)" text-color="white">
                  {{ evoSuggestionStatusLabel(suggestion.status) }}
                </q-chip>
              </div>
              <div class="col-6">
                <span class="text-weight-medium text-grey-7">生命周期：</span>
                <q-chip dense square :color="evoLifecycleStatusColor(suggestion.lifecycleStatus)" text-color="white">
                  {{ evoLifecycleStatusLabel(suggestion.lifecycleStatus) }}
                </q-chip>
              </div>
              <div class="col-6">
                <span class="text-weight-medium text-grey-7">沙箱验证：</span>
                <template v-if="suggestion.sandboxPassed === true">
                  <q-icon name="check_circle" color="positive" size="sm" class="q-mr-xs" />通过
                </template>
                <template v-else-if="suggestion.sandboxPassed === false">
                  <q-icon name="cancel" color="negative" size="sm" class="q-mr-xs" />未通过
                </template>
                <template v-else>—</template>
              </div>
            </div>

            <div v-if="suggestion.triggerReason" class="text-body2">
              <span class="text-weight-medium text-grey-7">触发原因：</span>{{ suggestion.triggerReason }}
            </div>

            <q-expansion-item
              v-if="suggestion.draftBody"
              dense-toggle
              default-opened
              label="Draft Body"
              class="evolution-detail-section"
            >
              <div class="q-pa-sm">
                <pre class="evolution-detail-pre">{{ suggestion.draftBody }}</pre>
              </div>
            </q-expansion-item>

            <q-expansion-item
              v-if="suggestion.sandboxResult && hasKeys(suggestion.sandboxResult)"
              dense-toggle
              default-opened
              label="沙箱验证结果"
              class="evolution-detail-section"
            >
              <div class="q-pa-sm">
                <pre class="evolution-detail-pre">{{ formatJson(suggestion.sandboxResult) }}</pre>
              </div>
            </q-expansion-item>
          </template>
        </q-card-section>
      </div>
      <q-separator />
      <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
        <q-btn v-close-popup flat no-caps label="关闭" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import type { SkillEvolutionView } from '../../features/skills/types';
import {
  evoSuggestionStatusColor,
  evoSuggestionStatusLabel,
  evoLifecycleStatusColor,
  evoLifecycleStatusLabel,
  evoTargetTypeColor,
  evoTargetTypeLabel,
  evoActionTypeColor,
  evoActionTypeLabel,
} from './evolutionSuggestionTableUi';

defineProps<{
  open: boolean;
  suggestion: SkillEvolutionView | null;
}>();

defineEmits<{
  'update:open': [value: boolean];
}>();

function hasKeys(obj: Record<string, unknown> | undefined): boolean {
  return !!obj && Object.keys(obj).length > 0;
}

function formatJson(obj: Record<string, unknown>): string {
  try {
    return JSON.stringify(obj, null, 2);
  } catch {
    return String(obj);
  }
}
</script>

<style scoped lang="sass">
.evolution-detail-section
  border: 1px solid var(--glass-border, rgba(0,0,0,0.08))
  border-radius: 8px
  margin-bottom: 8px

.evolution-detail-pre
  background: var(--glass-elevated, rgba(0,0,0,0.03))
  border-radius: 6px
  padding: 12px
  font-size: 13px
  line-height: 1.5
  white-space: pre-wrap
  word-break: break-word
  max-height: 400px
  overflow-y: auto
  margin: 0
</style>
