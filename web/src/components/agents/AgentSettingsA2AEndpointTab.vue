<template>
  <section class="settings-section">
    <div class="section-heading">
      <div>
        <div class="text-subtitle1 text-weight-bold">A2A Endpoint</div>
        <div class="text-caption text-grey-7">将本 Agent 暴露为 A2A 服务，供同工作区 call_agent 或外部客户端调用。</div>
      </div>
    </div>

    <q-inner-loading :showing="loading" />

    <div v-if="card" class="row q-col-gutter-md">
      <q-toggle v-model="card.enabled" class="col-12" color="primary" label="启用 A2A" />
      <div class="col-12">
        <div class="text-caption text-grey-7 q-mb-sm">Capabilities（JSON 名称列表，每行一个能力名）</div>
        <q-input v-model="capabilityLines" outlined type="textarea" rows="4" hint="例如 chat、summarize" />
      </div>
      <div class="col-12">
        <q-btn color="primary" rounded unelevated label="保存 AgentCard" :loading="saving" @click="saveEndpoint" />
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useQuasar } from "quasar";
import type { A2AAgentCard } from "../../features/a2a/types";
import { useA2AStore } from "../../stores/a2a";

const props = defineProps<{ agentId: string }>();

const $q = useQuasar();
const a2aStore = useA2AStore();
const loading = ref(false);
const saving = ref(false);
const card = ref<A2AAgentCard | null>(null);

const capabilityLines = computed({
  get: () => (card.value?.capabilities ?? []).map((c) => c.name).join("\n"),
  set: (text: string) => {
    if (!card.value) return;
    const names = text
      .split("\n")
      .map((s) => s.trim())
      .filter(Boolean);
    card.value.capabilities = names.map((name) => ({
      name,
      description: name,
      input_schema_json: "{}",
      output_schema_json: "{}"
    }));
  }
});

onMounted(async () => {
  loading.value = true;
  try {
    card.value = await a2aStore.refreshCard(props.agentId);
  } catch {
    card.value = {
      agent_id: props.agentId,
      display_name: "",
      workspace: "",
      enabled: false,
      capabilities: [],
      updated_at: ""
    };
  } finally {
    loading.value = false;
  }
});

async function saveEndpoint() {
  if (!card.value) return;
  if (card.value.enabled && card.value.capabilities.length === 0) {
    card.value.capabilities = [
      {
        name: "chat",
        description: "Default chat capability",
        input_schema_json: "{}",
        output_schema_json: "{}"
      }
    ];
  }
  saving.value = true;
  try {
    card.value = await a2aStore.updateCard(props.agentId, {
      enabled: card.value.enabled,
      capabilities: card.value.capabilities
    });
    $q.notify({ type: "positive", message: "A2A AgentCard 已更新" });
  } catch (error) {
    $q.notify({ type: "negative", message: error instanceof Error ? error.message : "保存失败" });
  } finally {
    saving.value = false;
  }
}
</script>
