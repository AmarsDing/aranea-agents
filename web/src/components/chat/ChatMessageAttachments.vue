// Container: approved — chat message inline attachment chips (ART-01).
<template>
  <div v-if="attachments.length" class="chat-message-attachments row q-gutter-xs q-mt-xs">
    <div
      v-for="item in attachments"
      :key="item.id"
      class="chat-message-attachments__chip row items-center no-wrap"
      role="button"
      tabindex="0"
      @click="openPreview(item)"
      @keydown.enter="openPreview(item)"
    >
      <q-icon :name="mimeIcon(item.mime_type)" size="16px" class="q-mr-xs" />
      <span class="chat-message-attachments__name ellipsis">{{ item.name }}</span>
      <span v-if="item.size" class="chat-message-attachments__size text-caption q-ml-xs">{{ formatBytes(item.size) }}</span>
    </div>

    <q-dialog v-model="previewOpen" transition-show="slide-up" transition-hide="slide-down">
      <q-card class="app-dialog-card app-dialog-card--sm">
        <q-card-section class="row items-center q-pb-none">
          <div class="text-subtitle1 ellipsis col">{{ previewMeta?.name }}</div>
          <q-btn flat round dense icon="close" v-close-popup />
        </q-card-section>
        <q-card-section v-if="previewMeta" class="q-pt-sm text-caption text-grey-7">
          {{ previewMeta.mime_type }}
          <span v-if="previewMeta.size"> · {{ formatBytes(previewMeta.size) }}</span>
        </q-card-section>
        <q-card-section v-if="previewId">
          <ArtifactPreview :artifact-id="previewId" :show-download="true" @download="onDownload" />
        </q-card-section>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";
import ArtifactPreview from "../../features/artifact/ArtifactPreview.vue";
import { signDownloadUrl, artifactDownloadHref } from "../../features/artifact/api";
import type { ArtifactMeta } from "../../features/artifact/types";
import {
  attachmentMimeIcon,
  formatAttachmentBytes,
  type MessageAttachmentRef,
} from "../../features/chat/messageAttachments";

defineProps<{
  attachments: MessageAttachmentRef[];
}>();

const previewOpen = ref(false);
const previewMeta = ref<MessageAttachmentRef | null>(null);
const previewId = ref("");

function mimeIcon(mime: string) {
  return attachmentMimeIcon(mime);
}

function formatBytes(n?: number) {
  return formatAttachmentBytes(n);
}

function openPreview(item: MessageAttachmentRef) {
  previewMeta.value = item;
  previewId.value = item.id;
  previewOpen.value = true;
}

async function onDownload(meta: ArtifactMeta) {
  try {
    const signed = await signDownloadUrl(meta.id, meta.version);
    window.open(artifactDownloadHref(signed.url), "_blank", "noopener,noreferrer");
  } catch {
    // user can retry from preview toolbar
  }
}
</script>

<style scoped>
.chat-message-attachments__chip {
  max-width: 220px;
  padding: 4px 8px;
  border-radius: 8px;
  background: rgb(0 0 0 / 6%);
  cursor: pointer;
  font-size: 12px;
  line-height: 1.3;
  transition: background 0.15s;
}

.chat-message-attachments__chip:hover {
  background: rgb(0 0 0 / 10%);
}

.chat-message-attachments__name {
  font-weight: 500;
  min-width: 0;
}

.chat-message-attachments__size {
  opacity: 0.75;
  flex-shrink: 0;
}

.ellipsis {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
