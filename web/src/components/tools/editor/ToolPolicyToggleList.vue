<template>
  <div class="tool-editor-policy-list">
    <div v-for="item in toggles" :key="item.id" class="tool-policy-card">
      <q-toggle
        :model-value="Boolean(form[item.id])"
        :disable="isDisabled(item.id)"
        dense
        class="tool-policy-card__switch"
        @update:model-value="$emit('patch-form', { [item.id]: Boolean($event) })"
      />
      <div class="tool-policy-card__body">
        <div class="tool-policy-card__title">
          {{ item.title }}
          <span v-if="isDisabled(item.id)" class="tool-policy-card__lock">
            {{ $t('toolsPage.policy.registryMaintained') }}
          </span>
        </div>
        <p class="tool-policy-card__summary">{{ item.summary }}</p>
        <p class="tool-policy-card__meta">
          {{ item.impact }}<template v-if="item.note">&nbsp;{{ item.note }}</template>
        </p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { toolPolicyToggles, type ToolPolicyToggleId } from '../../../features/tools/toolEditorCopy';
import type { ToolUpsertInput } from '../../../features/tools/types';

const props = defineProps<{
  form: ToolUpsertInput;
  registryLocked: boolean;
}>();

defineEmits<{ 'patch-form': [p: Record<string, unknown>] }>();

const toggles = computed(() => toolPolicyToggles());

function isDisabled(id: ToolPolicyToggleId): boolean {
  // 「全局启用」始终可改（日常启停入口）；其余目录标记在内置/只读工具上由 registry 维护。
  return id !== 'enabled' && props.registryLocked;
}
</script>
