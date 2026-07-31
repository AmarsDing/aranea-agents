<template>
  <q-dialog :model-value="open" @update:model-value="$emit('update:open', !!$event)">
    <q-card style="min-width: 400px">
      <q-card-section>
        <div class="text-h6">{{ t('knowledgePage.moveConflictTitle') }}</div>
        <div class="text-caption text-grey-7">
          {{ t('knowledgePage.moveConflictMessage', { dir: targetLabel, name: fileName }) }}
        </div>
      </q-card-section>
      <q-card-section class="q-pt-none">
        <q-list dense>
          <q-item v-ripple clickable :disable="loading" @click="$emit('resolve', 'overwrite')">
            <q-item-section avatar><q-icon name="find_replace" color="warning" /></q-item-section>
            <q-item-section>
              <q-item-label>{{ t('knowledgePage.moveConflictOverwrite') }}</q-item-label>
              <q-item-label caption>{{ t('knowledgePage.moveConflictOverwriteHint') }}</q-item-label>
            </q-item-section>
          </q-item>
          <q-item v-ripple clickable :disable="loading" @click="$emit('resolve', 'rename')">
            <q-item-section avatar><q-icon name="content_copy" color="primary" /></q-item-section>
            <q-item-section>
              <q-item-label>{{ t('knowledgePage.moveConflictRename') }}</q-item-label>
              <q-item-label caption>{{ t('knowledgePage.moveConflictRenameHint') }}</q-item-label>
            </q-item-section>
          </q-item>
        </q-list>
        <q-linear-progress v-if="loading" indeterminate color="primary" class="q-mt-sm" />
      </q-card-section>
      <q-card-actions align="right">
        <q-btn v-close-popup flat no-caps :disable="loading" :label="t('knowledgePage.moveCancel')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n';

// G3-F1 拖拽移动同名冲突弹窗（V12.5）：覆盖（旧版入回收站）/ 保留两份（自动改名）/ 取消。
defineProps<{
  open: boolean;
  /** 冲突文件名。 */
  fileName: string;
  /** 目标目录展示名（末段目录名；根目录由调用方传 i18n 文案）。 */
  targetLabel: string;
  loading: boolean;
}>();

defineEmits<{
  'update:open': [value: boolean];
  resolve: [policy: 'overwrite' | 'rename'];
}>();

const { t } = useI18n();
</script>
