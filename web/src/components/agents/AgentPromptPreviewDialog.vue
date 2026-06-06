<template>
  <q-dialog :model-value="open" @update:model-value="emit('update:open', $event)">
    <q-card class="prompt-dialog app-dialog-card">
      <q-card-section class="row items-center justify-between prompt-dialog__header">
        <div>
          <div class="text-h6">系统提示词</div>
          <div class="text-caption prompt-dialog__stats">
            构建期约 {{ staticTokens }} tokens · 运行时追加约 {{ runtimeTokens }} tokens
          </div>
        </div>
        <q-btn v-close-popup flat round icon="close" />
      </q-card-section>
      <q-tabs :model-value="mode" dense align="left" narrow-indicator class="prompt-dialog__mode-tabs" @update:model-value="emit('update:mode', $event)">
        <q-tab v-for="m in modes" :key="m.value" :name="m.value" :label="m.label" />
      </q-tabs>
      <q-separator />
      <q-card-section class="prompt-dialog__body">
        <p class="prompt-dialog__hint">
          下方为<strong>构建期</strong>写入模型的 System Prompt（Description、Prompt 文件、运行时策略）。
          实际对话时还会按开关追加记忆、Skills、Intent 等，可在「Token 分解」中查看估算。
        </p>
        <pre class="agent-prompt-preview">{{ instructionText }}</pre>
        <q-expansion-item
          v-if="sections.length"
          dense
          expand-separator
          icon="analytics"
          label="Token 分解（估算）"
          caption="构建期已含于上文；运行时按每轮对话追加"
          class="prompt-dialog__breakdown"
        >
          <AppRegistryTable
            :shell="false"
            :data-shell="true"
            hide-bottom
            row-key="key"
            :rows="sections"
            :columns="AGENT_PROMPT_ASSEMBLY_TABLE_COLUMNS"
            hide-pagination
            :pagination="{ rowsPerPage: 0 }"
          >
            <template #body-cell-source="props">
              <q-td :props="props">
                <q-chip
                  dense
                  size="sm"
                  :color="props.row.source === 'build' ? 'primary' : 'secondary'"
                  text-color="white"
                >
                  {{ props.row.source === 'build' ? '构建期' : '运行时' }}
                </q-chip>
              </q-td>
            </template>
            <template #body-cell-est_tokens="props">
              <q-td :props="props" class="text-right">
                {{ props.row.est_tokens > 0 ? props.row.est_tokens : '—' }}
              </q-td>
            </template>
          </AppRegistryTable>
        </q-expansion-item>
      </q-card-section>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import AppRegistryTable from '../layout/AppRegistryTable.vue';
import { AGENT_PROMPT_ASSEMBLY_TABLE_COLUMNS } from './agentTableUi';
import type { PromptModeOption } from '../../components/agents/agentUi';

defineProps<{
  open: boolean;
  mode: string;
  modes: PromptModeOption[];
  instructionText: string;
  staticTokens: number;
  runtimeTokens: number;
  sections: { source: string; est_tokens: number }[];
}>();

const emit = defineEmits<{
  'update:open': [value: boolean];
  'update:mode': [value: string];
}>();
</script>
