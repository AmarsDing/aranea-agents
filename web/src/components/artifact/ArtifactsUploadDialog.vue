// Container: approved — artifact upload dialog; controlled by page composable.
<template>
  <AppDialog
    :model-value="open"
    :title="t('artifact.upload.title')"
    size="sm"
    persistent
    :loading="loading"
    @update:model-value="$emit('update:open', $event)"
  >
    <div class="q-gutter-md">
      <q-select
        :model-value="sessionId"
        class="app-field-md"
        dense
        outlined
        use-input
        input-debounce="0"
        new-value-mode="add-unique"
        emit-value
        map-options
        :options="filteredSessionOptions"
        :label="t('artifact.upload.sessionId')"
        @filter="filterSessionOptions"
        @update:model-value="$emit('update:sessionId', String($event ?? ''))"
      >
        <template #option="scope">
          <q-item v-bind="scope.itemProps">
            <q-item-section>
              <q-item-label>{{ scope.opt.label }}</q-item-label>
              <q-item-label caption>{{ scope.opt.caption }}</q-item-label>
            </q-item-section>
          </q-item>
        </template>
      </q-select>
      <q-input
        :model-value="name"
        class="app-field-md"
        dense
        outlined
        :label="t('artifact.upload.fileName')"
        @update:model-value="$emit('update:name', String($event ?? ''))"
      />
      <q-input
        :model-value="mimeType"
        class="app-field-sm"
        dense
        outlined
        label="MIME"
        placeholder="application/octet-stream"
        @update:model-value="$emit('update:mimeType', String($event ?? ''))"
      />
      <q-file
        :model-value="file"
        :label="t('artifact.upload.pickFile')"
        outlined
        dense
        @update:model-value="$emit('update:file', $event)"
      />
      <div class="text-caption text-grey-7">{{ maxSizeHint }}</div>
    </div>
    <template #actions>
      <q-btn flat no-caps :label="t('common.cancel')" @click="$emit('update:open', false)" />
      <q-btn
        color="primary"
        unelevated
        no-caps
        :label="t('artifact.page.upload')"
        :loading="loading"
        @click="$emit('submit')"
      />
    </template>
  </AppDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import AppDialog from '../common/AppDialog.vue';
import type { SessionSelectOption } from '../../features/artifact/useArtifactsPage';

const props = defineProps<{
  open: boolean;
  loading: boolean;
  file: File | null;
  sessionId: string;
  name: string;
  mimeType: string;
  maxSizeHint: string;
  sessionOptions: SessionSelectOption[];
}>();

defineEmits<{
  'update:open': [value: boolean];
  'update:file': [value: File | null];
  'update:sessionId': [value: string];
  'update:name': [value: string];
  'update:mimeType': [value: string];
  submit: [];
}>();

const { t } = useI18n();

/** 下拉过滤为纯展示逻辑：按标题或 UUID 子串过滤。 */
const filteredSessionOptions = ref<SessionSelectOption[]>(props.sessionOptions);

watch(
  () => props.sessionOptions,
  (opts) => {
    filteredSessionOptions.value = opts;
  },
);

function filterSessionOptions(val: string, update: (fn: () => void) => void) {
  update(() => {
    const needle = val.trim().toLowerCase();
    filteredSessionOptions.value = needle
      ? props.sessionOptions.filter(
          (o) => o.label.toLowerCase().includes(needle) || o.value.toLowerCase().includes(needle),
        )
      : props.sessionOptions;
  });
}
</script>
