<template>
  <div class="artifact-preview">
    <div v-if="loading" class="row justify-center q-py-lg">
      <q-spinner color="primary" size="2em" />
    </div>
    <template v-else-if="preview">
      <div class="artifact-preview__header q-mb-sm row items-center q-gutter-sm">
        <q-icon :name="kindIcon" size="sm" />
        <span class="text-caption">{{ preview.meta.mime_type }}</span>
        <span class="text-caption text-grey-7">{{ formatBytes(preview.meta.size) }}</span>
        <span v-if="preview.meta.version > 0" class="text-caption text-grey-7">v{{ preview.meta.version }}</span>
        <q-space />
        <q-btn v-if="showDownload" flat dense round icon="download" size="sm" @click="$emit('download', preview.meta)">
          <q-tooltip>下载</q-tooltip>
        </q-btn>
      </div>

      <div v-if="preview.preview_kind === 'text'" class="artifact-preview__text">
        <pre class="artifact-preview__code">{{ preview.text_content }}</pre>
      </div>

      <div v-else-if="preview.preview_kind === 'image'" class="artifact-preview__image">
        <img :src="imageSrc" :alt="preview.meta.name" class="artifact-preview__img" />
      </div>

      <div v-else-if="preview.preview_kind === 'pdf'" class="artifact-preview__pdf">
        <iframe :src="pdfSrc" class="artifact-preview__iframe" :title="preview.meta.name" />
      </div>

      <div v-else class="artifact-preview__binary q-pa-md text-center text-grey-7">
        <q-icon name="description" size="3em" class="q-mb-sm" />
        <div>{{ preview.meta.name }}</div>
        <div class="text-caption">此类型暂不支持在线预览</div>
      </div>
    </template>
    <div v-else-if="error" class="text-negative q-pa-md">
      {{ error }}
    </div>
  </div>
</template>

// Container: approved because these are container components that coordinate artifact state for their parent page
<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useArtifactStore } from '../../stores/artifact';
import { formatBytes } from '../../shared/format';
import type { ArtifactMeta, ArtifactPreview } from './types';

const props = defineProps<{
  artifactId: string;
  version?: number;
  showDownload?: boolean;
}>();

defineEmits<{
  download: [meta: ArtifactMeta];
}>();

const artifactStore = useArtifactStore();
const preview = ref<ArtifactPreview | null>(null);
const loading = ref(false);
const error = ref('');

const kindIcon = computed(() => {
  if (!preview.value) return 'insert_drive_file';
  const kind = preview.value.preview_kind;
  if (kind === 'text') return 'code';
  if (kind === 'image') return 'image';
  if (kind === 'pdf') return 'picture_as_pdf';
  return 'insert_drive_file';
});

const imageSrc = computed(() => {
  if (!preview.value || !preview.value.data_base64) return '';
  return `data:${preview.value.meta.mime_type};base64,${preview.value.data_base64}`;
});

const pdfSrc = computed(() => {
  if (!preview.value || !preview.value.data_base64) return '';
  return `data:application/pdf;base64,${preview.value.data_base64}`;
});

async function loadPreview() {
  if (!props.artifactId) {
    preview.value = null;
    return;
  }
  loading.value = true;
  error.value = '';
  try {
    preview.value = await artifactStore.loadPreview(props.artifactId, props.version);
  } catch (e) {
    error.value = e instanceof Error ? e.message : '加载预览失败';
    preview.value = null;
  } finally {
    loading.value = false;
  }
}

watch(() => [props.artifactId, props.version], loadPreview, { immediate: true });
</script>
