<template>
  <div class="column q-gutter-sm">
    <p class="app-registry-muted-caption">LLM 调用此工具时可传的参数（JSON Schema）。</p>
    <tool-json-block :text="prettyJSON(tool?.parameters_schema_json)" />
    <q-expansion-item v-if="tool?.result_schema_json && tool.result_schema_json !== '{}'" dense-toggle label="返回结构">
      <tool-json-block class="q-mt-sm" :text="prettyJSON(tool?.result_schema_json)" />
    </q-expansion-item>
  </div>
</template>

<script setup lang="ts">
import type { Tool } from '../../features/tools/types';
import ToolJsonBlock from './ToolJsonBlock.vue';
import { prettyJSON } from './toolUi';

defineProps<{ tool: Tool | null }>();
</script>
