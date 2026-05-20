<template>
  <q-card flat bordered class="q-pa-md q-gutter-md" style="max-width: 640px">
    <q-input v-model="calleeAgentId" dense outlined label="Callee Agent ID" />
    <q-input v-model="capability" dense outlined label="Capability" />
    <q-input v-model="payloadJson" dense outlined type="textarea" rows="6" label="Payload JSON" />
    <q-input v-model.number="timeoutSeconds" dense outlined type="number" label="Timeout (秒)" />
    <q-btn color="primary" unelevated icon="send" label="Invoke" :loading="loading" @click="$emit('invoke')" />
    <q-card v-if="result" flat bordered class="q-pa-sm">
      <div class="text-caption">invoke_id: {{ result.invoke_id }}</div>
      <div class="text-caption">status: {{ result.status }} · {{ result.duration_ms }}ms</div>
      <pre class="a2a-result">{{ result.result_json || result.error_message }}</pre>
    </q-card>
  </q-card>
</template>

<script setup lang="ts">
import type { A2AInvokeResult } from "../../features/a2a/types";

defineProps<{
  loading: boolean;
  result: A2AInvokeResult | null;
}>();

const calleeAgentId = defineModel<string>("calleeAgentId", { default: "" });
const capability = defineModel<string>("capability", { default: "" });
const payloadJson = defineModel<string>("payloadJson", { default: "{}" });
const timeoutSeconds = defineModel<number>("timeoutSeconds", { default: 30 });

defineEmits<{ invoke: [] }>();
</script>

<style scoped>
.a2a-result {
  margin: 0.5rem 0 0;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 0.85rem;
}
</style>
