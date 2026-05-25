// Container: approved — artifact detail + preview dialog.
<template>
  <q-dialog :model-value="open" @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--md">
      <q-card-section class="text-h6">{{ meta?.name }}</q-card-section>
      <q-card-section v-if="meta" class="app-dialog-body q-gutter-sm q-pt-none text-body2">
        <div><b>ID：</b>{{ meta.id }}</div>
        <div><b>Session：</b>{{ meta.session_id }}</div>
        <div><b>SHA256：</b><span class="text-caption">{{ meta.sha256 }}</span></div>
        <div><b>存储：</b>{{ meta.storage_kind }} — {{ meta.storage_uri }}</div>
        <div><b>大小：</b>{{ formatBytes(meta.size) }} · v{{ meta.version }}</div>
        <div v-if="versions.length > 1" class="q-mt-sm">
          <div class="text-caption text-grey-7 q-mb-xs">版本历史</div>
          <div class="row q-gutter-xs">
            <q-chip
              v-for="v in versions"
              :key="`${v.id}-v${v.version}`"
              dense
              clickable
              :color="v.version === selectedVersion ? 'primary' : undefined"
              :outline="v.version !== selectedVersion"
              @click="$emit('select-version', v)"
            >
              v{{ v.version }}
            </q-chip>
          </div>
        </div>
      </q-card-section>
      <q-card-section v-if="artifactId">
        <ArtifactPreview
          :artifact-id="artifactId"
          :version="selectedVersion"
          :show-download="true"
          @download="$emit('download', $event)"
        />
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar">
        <q-btn flat no-caps label="关闭" v-close-popup />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import ArtifactPreview from "../ArtifactPreview.vue";
import type { ArtifactMeta } from "../types";

defineProps<{
  open: boolean;
  meta: ArtifactMeta | null;
  artifactId: string;
  selectedVersion?: number;
  versions: ArtifactMeta[];
  formatBytes: (n: number) => string;
}>();

defineEmits<{
  "update:open": [value: boolean];
  "select-version": [meta: ArtifactMeta];
  download: [meta: ArtifactMeta];
}>();
</script>
