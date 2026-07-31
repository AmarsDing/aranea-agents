<template>
  <q-dialog :model-value="open" @update:model-value="emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--sm app-glass-dialog">
      <q-card-section class="app-glass-dialog__head row items-center justify-between">
        <div class="app-glass-dialog__title">{{ t('shopPage.replyTitle', { author: item?.author ?? '' }) }}</div>
        <q-btn v-close-popup flat round dense icon="close" />
      </q-card-section>
      <q-separator />
      <q-card-section class="app-glass-dialog__body">
        <div v-if="item" class="reply-dialog__quote q-mb-md">
          <rating-stars :rating="item.rating" size="13px" class="q-mb-xs" />
          <div class="text-body2">{{ item.content }}</div>
        </div>
        <q-input
          v-model="content"
          outlined
          dense
          autogrow
          type="textarea"
          :placeholder="t('shopPage.replyPlaceholder')"
        />
      </q-card-section>
      <q-separator />
      <q-card-actions align="right" class="app-glass-dialog__actions">
        <q-btn v-close-popup flat no-caps :label="t('common.cancel')" />
        <q-btn
          color="primary"
          unelevated
          no-caps
          :label="t('shopPage.replySend')"
          :disable="!content.trim()"
          :loading="sending"
          @click="send"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { StudioInboxItem } from '../../features/ecosystem/types';
import RatingStars from './RatingStars.vue';

const props = defineProps<{
  open: boolean;
  item: StudioInboxItem | null;
  sending?: boolean;
}>();

const emit = defineEmits<{
  'update:open': [value: boolean];
  submit: [content: string];
}>();

const { t } = useI18n();
const content = ref('');

watch(
  () => props.open,
  (v) => {
    if (v) content.value = '';
  },
);

function send() {
  emit('submit', content.value.trim());
}
</script>

<style scoped>
.reply-dialog__quote {
  padding: 10px 12px;
  border-radius: 10px;
  border-left: 3px solid var(--color-accent);
  background: var(--interaction-surface-hover);
}
body.body--dark .reply-dialog__quote {
  background: rgba(255, 255, 255, 0.06);
}
</style>
