<template>
  <AppPromptDialog
    :model-value="promptState.visible"
    data-test="app-prompt-host"
    :title="promptState.title"
    :message="promptState.message"
    :label="promptState.label"
    :placeholder="promptState.placeholder"
    :type="promptState.type"
    :items="promptState.items"
    :default-value="promptState.defaultValue"
    :required="promptState.required"
    :confirm-label="promptState.confirmLabel"
    :cancel-label="promptState.cancelLabel"
    :tone="promptState.tone"
    :persistent="promptState.persistent"
    :loading="promptState.loading"
    @submit="promptAccept"
    @update:model-value="onUpdateVisible"
  />
</template>

<script setup lang="ts">
import AppPromptDialog from './AppPromptDialog.vue';
import { promptAccept, promptCancel, promptState } from '../../composables/useAppPrompt';

/** 任何非确认的关闭（取消键、Esc、遮罩）都按取消处理，保证 Promise 一定 settle。 */
function onUpdateVisible(visible: boolean) {
  if (!visible) promptCancel();
}
</script>
