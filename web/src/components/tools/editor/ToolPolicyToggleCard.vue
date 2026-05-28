<template>
  <q-card flat bordered class="tool-policy-card">
    <q-card-section class="row items-start no-wrap q-gutter-md">
      <div class="col min-width-0">
        <div class="row items-center q-gutter-xs q-mb-xs">
          <span class="tool-policy-card__title">{{ copy.title }}</span>
          <q-icon v-if="locked" name="lock" size="xs" class="app-registry-muted-caption">
            <q-tooltip>内置工具：此项由 registry 同步，重启后可能恢复默认值</q-tooltip>
          </q-icon>
        </div>
        <p class="tool-policy-card__summary">{{ copy.summary }}</p>
        <p class="tool-policy-card__meta">{{ copy.impact }}</p>
        <p v-if="copy.note" class="tool-policy-card__meta q-mt-xs">{{ copy.note }}</p>
      </div>
      <q-toggle
        class="col-auto q-mt-xs"
        dense
        color="primary"
        :model-value="modelValue"
        :disable="disable || locked"
        @update:model-value="$emit('update:modelValue', Boolean($event))"
      />
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import type { ToolPolicyToggleCopy } from "../../../features/tools/toolEditorCopy";

defineProps<{
  copy: ToolPolicyToggleCopy;
  modelValue: boolean;
  locked?: boolean;
  disable?: boolean;
}>();

defineEmits<{ "update:modelValue": [value: boolean] }>();
</script>
