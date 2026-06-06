<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', $event)">
    <q-card class="app-dialog-card app-dialog-card--sm app-glass-dialog">
      <q-card-section class="app-glass-dialog__head row items-center justify-between">
        <div class="app-glass-dialog__title">文档入库</div>
        <q-btn v-close-popup flat round dense icon="close" />
      </q-card-section>
      <q-separator />
      <div class="app-glass-dialog__scroll">
        <q-card-section class="app-dialog-body app-glass-dialog__body q-gutter-md">
          <q-input
            :model-value="source"
            class="app-field-md"
            dense
            outlined
            label="来源标识"
            @update:model-value="$emit('update:source', String($event ?? ''))"
          />
          <q-input
            :model-value="mimeType"
            class="app-field-sm"
            dense
            outlined
            label="MIME 类型"
            placeholder="text/plain"
            @update:model-value="$emit('update:mimeType', String($event ?? ''))"
          />
          <q-file
            :model-value="file"
            label="选择文件"
            outlined
            dense
            accept=".txt,.md,.json,.csv,.log,.html,.htm,.xml,.yaml,.yml,.toml,.pdf,.doc,.docx,.pptx,.xlsx"
            hint="文本类型可在下方预览编辑；二进制（PDF/DOCX/…）按原字节上传，依赖后端解析支持"
            @update:model-value="$emit('update:file', $event)"
          />
          <q-input
            :model-value="text"
            class="app-field-long"
            dense
            outlined
            type="textarea"
            label="或粘贴文本"
            autogrow
            @update:model-value="$emit('update:text', String($event ?? ''))"
          />
          <q-expansion-item
            dense
            dense-toggle
            label="高级分块参数"
            header-class="text-caption text-grey-7"
            expand-icon-class="text-grey-6"
          >
            <div class="q-gutter-sm q-pa-sm">
              <div class="row q-gutter-sm items-center">
                <q-select
                  :model-value="chunkStrategy"
                  dense
                  outlined
                  label="分块策略"
                  :options="chunkStrategyOptions"
                  emit-value
                  map-options
                  style="min-width: 140px"
                  @update:model-value="$emit('update:chunkStrategy', String($event ?? ''))"
                />
                <q-input
                  :model-value="chunkSize"
                  dense
                  outlined
                  type="number"
                  label="分块大小"
                  hint="0 = 默认 512"
                  style="max-width: 120px"
                  @update:model-value="$emit('update:chunkSize', Number($event) || 0)"
                />
                <q-input
                  :model-value="chunkOverlap"
                  dense
                  outlined
                  type="number"
                  label="分块重叠"
                  hint="0 = 默认 64"
                  style="max-width: 120px"
                  @update:model-value="$emit('update:chunkOverlap', Number($event) || 0)"
                />
              </div>
            </div>
          </q-expansion-item>
        </q-card-section>
      </div>
      <q-separator />
      <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions">
        <q-btn v-close-popup flat no-caps label="取消" />
        <q-btn color="primary" unelevated no-caps label="入库" :loading="loading" @click="$emit('submit')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { KNOWLEDGE_CHUNK_STRATEGY_OPTIONS } from '../../features/knowledge/knowledgeUi';

defineProps<{
  open: boolean;
  source: string;
  mimeType: string;
  text: string;
  file: File | null;
  chunkStrategy: string;
  chunkSize: number;
  chunkOverlap: number;
  loading: boolean;
}>();
defineEmits<{
  'update:open': [value: boolean];
  'update:source': [value: string];
  'update:mimeType': [value: string];
  'update:text': [value: string];
  'update:file': [value: File | null];
  'update:chunkStrategy': [value: string];
  'update:chunkSize': [value: number];
  'update:chunkOverlap': [value: number];
  submit: [];
}>();

const chunkStrategyOptions = KNOWLEDGE_CHUNK_STRATEGY_OPTIONS;
</script>
