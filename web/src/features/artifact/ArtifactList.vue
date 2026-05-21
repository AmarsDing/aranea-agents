<template>
  <div class="artifact-list">
    <div v-if="loading" class="row justify-center q-py-sm">
      <q-spinner color="primary" size="1.5em" />
    </div>
    <template v-else-if="items.length">
      <div class="artifact-list__items">
        <div v-for="item in items" :key="item.id" class="artifact-list__item row items-center q-gutter-xs" clickable @click="onOpen(item)">
          <q-icon :name="mimeIcon(item.mime_type)" size="18px" color="grey-7" />
          <div class="col" style="min-width: 0">
            <div class="artifact-list__name ellipsis">{{ item.name }}</div>
            <div class="artifact-list__meta text-caption text-grey-7">
              {{ formatBytes(item.size) }}
              <span v-if="item.version > 0"> · v{{ item.version }}</span>
            </div>
          </div>
          <q-btn flat dense round icon="download" size="xs" @click.stop="onDownload(item)">
            <q-tooltip>下载</q-tooltip>
          </q-btn>
        </div>
      </div>
    </template>
    <div v-else class="text-caption text-grey-7 q-pa-sm">
      暂无制品
    </div>

    <q-dialog v-model="previewOpen" transition-show="slide-up" transition-hide="slide-down">
      <q-card style="min-width: 480px; max-width: 94vw">
        <q-card-section class="row items-center q-pb-none">
          <div class="text-subtitle1 ellipsis col">{{ previewMeta?.name }}</div>
          <q-btn flat round dense icon="close" v-close-popup />
        </q-card-section>
        <q-card-section v-if="previewMeta" class="q-pt-sm text-caption text-grey-7">
          {{ previewMeta.mime_type }} · {{ formatBytes(previewMeta.size) }}
          <span v-if="previewMeta.version > 0"> · v{{ previewMeta.version }}</span>
        </q-card-section>
        <q-card-section v-if="previewArtifactId">
          <ArtifactPreview :artifact-id="previewArtifactId" :show-download="true" @download="onPreviewDownload" />
        </q-card-section>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { signDownloadUrl, artifactDownloadHref } from "./api";
import ArtifactPreview from "./ArtifactPreview.vue";
import type { ArtifactMeta } from "./types";

const props = defineProps<{
  items: ArtifactMeta[];
  loading?: boolean;
}>();

const emit = defineEmits<{
  open: [id: string];
}>();

const previewOpen = ref(false);
const previewMeta = ref<ArtifactMeta | null>(null);
const previewArtifactId = ref("");

function mimeIcon(mime: string): string {
  if (!mime) return "insert_drive_file";
  const m = mime.toLowerCase();
  if (m.startsWith("image/")) return "image";
  if (m === "application/pdf") return "picture_as_pdf";
  if (m.startsWith("text/") || m.includes("json") || m.includes("xml") || m.includes("javascript") || m.includes("yaml")) return "code";
  if (m.startsWith("video/")) return "videocam";
  if (m.startsWith("audio/")) return "audiotrack";
  if (m.includes("zip") || m.includes("tar") || m.includes("gzip") || m.includes("compressed")) return "folder_zip";
  return "insert_drive_file";
}

function formatBytes(n: number) {
  if (!n) return "0 B";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

function onOpen(item: ArtifactMeta) {
  previewMeta.value = item;
  previewArtifactId.value = item.id;
  previewOpen.value = true;
  emit("open", item.id);
}

async function onDownload(item: ArtifactMeta) {
  try {
    const signed = await signDownloadUrl(item.id, item.version);
    window.open(artifactDownloadHref(signed.url), "_blank", "noopener,noreferrer");
  } catch {
    // silent — user can retry
  }
}

async function onPreviewDownload(meta: ArtifactMeta) {
  await onDownload(meta);
}
</script>

<style scoped>
.artifact-list__items {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.artifact-list__item {
  padding: 6px 8px;
  border-radius: 6px;
  cursor: pointer;
  transition: background 0.15s;
}
.artifact-list__item:hover {
  background: rgba(0, 0, 0, 0.04);
}
.artifact-list__name {
  font-size: 13px;
  font-weight: 500;
  line-height: 1.3;
}
.artifact-list__meta {
  line-height: 1.2;
}
.ellipsis {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
