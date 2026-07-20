<template>
  <q-card v-if="tasks.length" flat class="knowledge-upload-queue">
    <div class="row items-center justify-between q-px-md q-pt-sm">
      <div class="text-caption text-grey-7">
        {{ queueHeader }}
      </div>
      <q-btn
        flat
        dense
        no-caps
        size="sm"
        :label="t('knowledgePage.queueClearFinished')"
        :disable="!hasFinished"
        @click="$emit('clear-finished')"
      />
    </div>
    <q-list dense class="q-pb-sm">
      <q-item v-for="task in tasks" :key="task.id" class="knowledge-upload-queue__item">
        <q-item-section avatar style="min-width: 36px">
          <q-spinner v-if="task.status === 'reading' || task.status === 'uploading'" color="primary" size="18px" />
          <q-icon v-else-if="task.status === 'success'" name="check_circle" color="positive" size="20px" />
          <q-icon v-else name="error" color="negative" size="20px" />
        </q-item-section>
        <q-item-section>
          <q-item-label class="ellipsis">{{ task.name }}</q-item-label>
          <q-item-label caption class="ellipsis" :class="{ 'text-negative': task.status === 'error' }">
            {{ task.message || statusText(task) }}
          </q-item-label>
        </q-item-section>
        <q-item-section side class="text-caption text-grey-7">
          {{ formatKnowledgeDocSize(task.size) }}
        </q-item-section>
        <q-item-section v-if="task.status === 'success' || task.status === 'error'" side>
          <q-btn
            flat
            dense
            round
            icon="close"
            size="sm"
            :aria-label="t('knowledgePage.queueRemoveAria')"
            @click="$emit('remove-task', task.id)"
          />
        </q-item-section>
      </q-item>
    </q-list>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { formatKnowledgeDocSize } from '../../features/knowledge/knowledgeUi';
import type { KnowledgeUploadTask } from '../../features/knowledge/types';

const props = defineProps<{
  tasks: KnowledgeUploadTask[];
}>();

defineEmits<{
  'clear-finished': [];
  'remove-task': [id: string];
}>();

const { t } = useI18n();

const activeCount = computed(
  () => props.tasks.filter((task) => task.status === 'reading' || task.status === 'uploading').length,
);
const hasFinished = computed(() => props.tasks.some((task) => task.status === 'success' || task.status === 'error'));
const queueHeader = computed(() => {
  const base = t('knowledgePage.queueTitle');
  if (!activeCount.value) return base;
  return base + t('knowledgePage.queueActiveSuffix', { active: activeCount.value, total: props.tasks.length });
});

function statusText(task: KnowledgeUploadTask): string {
  if (task.status === 'reading') return t('knowledgePage.queueStatusReading');
  if (task.status === 'uploading') return t('knowledgePage.queueStatusUploading');
  if (task.status === 'success') return t('knowledgePage.queueStatusSuccess');
  return t('knowledgePage.queueStatusFailed');
}
</script>

<style scoped>
.knowledge-upload-queue {
  margin-bottom: 12px;
  border: 1px solid var(--glass-border);
  border-radius: 16px;
  background: var(--glass-surface);
}
.knowledge-upload-queue__item {
  min-height: 44px;
}
</style>
