<template>
  <AppConfirmDialog
    v-model="state.visible"
    data-test="app-confirm-host"
    :title="state.title"
    :message="state.message"
    :tone="state.tone"
    :confirm-label="state.confirmLabel"
    :cancel-label="state.cancelLabel"
    :icon="state.icon"
    :loading="state.loading"
    @confirm="confirmAccept"
    @update:model-value="onUpdateVisible"
  />
</template>

<script setup lang="ts">
import AppConfirmDialog from './AppConfirmDialog.vue';
import { confirmAccept, confirmCancel, state } from '../../composables/useAppConfirm';

/** 任何非确认的关闭（取消键、Esc、遮罩）都按取消处理，保证 Promise 一定 settle。 */
function onUpdateVisible(visible: boolean) {
  if (!visible) confirmCancel();
}
</script>
