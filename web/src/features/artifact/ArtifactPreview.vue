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

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { previewArtifact } from "./api";
import type { ArtifactMeta, ArtifactPreview } from "./types";

const props = defineProps<{
  artifactId: string;
  version?: number;
  showDownload?: boolean;
}>();

defineEmits<{
  download: [meta: ArtifactMeta];
}>();

const preview = ref<ArtifactPreview | null>(null);
const loading = ref(false);
const error = ref("");

const kindIcon = computed(() => {
  if (!preview.value) return "insert_drive_file";
  const kind = preview.value.preview_kind;
  if (kind === "text") return "code";
  if (kind === "image") return "image";
  if (kind === "pdf") return "picture_as_pdf";
  return "insert_drive_file";
});

const imageSrc = computed(() => {
  if (!preview.value || !preview.value.data_base64) return "";
  return `data:${preview.value.meta.mime_type};base64,${preview.value.data_base64}`;
});

const pdfSrc = computed(() => {
  if (!preview.value || !preview.value.data_base64) return "";
  return `data:application/pdf;base64,${preview.value.data_base64}`;
});

function formatBytes(n: number) {
  if (!n) return "0 B";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

async function loadPreview() {
  if (!props.artifactId) {
    preview.value = null;
    return;
  }
  loading.value = true;
  error.value = "";
  try {
    preview.value = await previewArtifact(props.artifactId, props.version);
  } catch (e) {
    error.value = e instanceof Error ? e.message : "加载预览失败";
    preview.value = null;
  } finally {
    loading.value = false;
  }
}

watch(() => [props.artifactId, props.version], loadPreview, { immediate: true });
</script>

<style scoped>
.artifact-preview__text {
  border: 1px solid rgba(0, 0, 0, 0.12);
  border-radius: 8px;
  overflow: hidden;
}
.artifact-preview__code {
  margin: 0;
  padding: 12px;
  max-height: 480px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: "SF Mono", "Fira Code", "Consolas", monospace;
  font-size: 13px;
  line-height: 1.5;
  background: var(--q-dark-page, #fafafa);
  color: var(--q-dark, #333);
}
.artifact-preview__image {
  text-align: center;
}
.artifact-preview__img {
  max-width: 100%;
  max-height: 480px;
  border-radius: 8px;
}
.artifact-preview__pdf {
  width: 100%;
}
.artifact-preview__iframe {
  width: 100%;
  height: 480px;
  border: 1px solid rgba(0, 0, 0, 0.12);
  border-radius: 8px;
}
</style>
