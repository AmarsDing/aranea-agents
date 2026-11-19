<template>
  <AppDialog
    :model-value="open"
    maximized
    persistent
    title="进化建议详情"
    @update:model-value="$emit('update:open', $event)"
  >
    <template v-if="suggestion">
      <div class="row q-col-gutter-sm text-body2">
        <div class="col-6">
          <span class="text-weight-medium text-grey-7">目标类型：</span>
          <q-chip dense square :color="evoTargetTypeColor(suggestion.targetType)" text-color="white">
            {{ evoTargetTypeLabel(suggestion.targetType) }}
          </q-chip>
        </div>
        <div class="col-6">
          <span class="text-weight-medium text-grey-7">目标 ID：</span
          >{{ suggestion.targetName || suggestion.targetId || '—' }}
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

      <div v-if="suggestion.triggerReason" class="text-body2 q-mt-md">
        <span class="text-weight-medium text-grey-7">触发原因：</span>{{ suggestion.triggerReason }}
      </div>

      <q-expansion-item
        v-if="suggestion.draftBody"
        dense-toggle
        default-opened
        label="Draft Body"
        class="evolution-detail-section q-mt-md"
      >
        <div class="q-pa-sm">
          <JsonCodeViewer :text="suggestion.draftBody" :show-toolbar="false" scroll-height="240px" />
        </div>
      </q-expansion-item>

      <q-expansion-item
        v-if="suggestion.sandboxResult && hasKeys(suggestion.sandboxResult)"
        dense-toggle
        default-opened
        label="沙箱验证结果"
        class="evolution-detail-section q-mt-md"
      >
        <div class="q-pa-sm">
          <JsonCodeViewer :text="formatJson(suggestion.sandboxResult)" :show-toolbar="false" scroll-height="240px" />
        </div>
      </q-expansion-item>
    </template>

    <template #actions>
      <q-btn flat no-caps label="关闭" @click="$emit('update:open', false)" />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import AppDialog from '../common/AppDialog.vue';
import type { SkillEvolutionView } from '../../features/skills/types';
import JsonCodeViewer from '../common/JsonCodeViewer.vue';
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
  border: 1px solid var(--glass-border)
  border-radius: 8px
  margin-bottom: 8px
</style>
