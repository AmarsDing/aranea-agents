<template>
  <template v-if="kind === 'Button'">
    <q-btn
      :color="ctx.buttonPrimary ? 'primary' : undefined"
      :outline="!ctx.buttonPrimary"
      unelevated
      rounded
      dense
      no-caps
      :disable="!ctx.actionName"
      :label="ctx.buttonLabel"
      @click="ctx.onButtonClick"
    />
  </template>
  <template v-else-if="kind === 'TextField'">
    <q-input
      :model-value="ctx.textFieldValue"
      :label="ctx.textFieldLabel"
      :type="ctx.textFieldInputType as any"
      readonly
      outlined
      dense
    />
  </template>
  <template v-else-if="kind === 'CheckBox'">
    <q-checkbox :model-value="ctx.checkBoxChecked" :label="ctx.checkBoxLabel" disable />
  </template>
</template>

<script setup lang="ts">
import type { A2UIComponentCtx } from "../../../../features/chat/a2ui/useA2UIComponent";

defineProps<{
  kind: string;
  ctx: A2UIComponentCtx;
}>();
</script>
