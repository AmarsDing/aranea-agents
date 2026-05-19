<template>
  <q-card flat bordered class="q-mb-md">
    <q-card-section>
      <div class="text-subtitle2 q-mb-sm">Embedder 配置</div>
      <div class="row q-col-gutter-md">
        <div class="col-12 col-md-3">
          <q-select
            v-model="form.provider"
            dense
            outlined
            label="Provider"
            :options="providerOptions"
            emit-value
            map-options
          />
        </div>
        <div class="col-12 col-md-5">
          <q-input v-model="form.base_url" dense outlined label="Base URL" placeholder="https://api.openai.com" />
        </div>
        <div class="col-12 col-md-4">
          <q-input v-model="form.model" dense outlined label="Model" />
        </div>
        <div class="col-12 col-md-3">
          <q-input v-model.number="form.dim" dense outlined type="number" label="Dim" />
        </div>
        <div class="col-12 col-md-5">
          <q-input
            v-model="form.api_key"
            dense
            outlined
            label="API Key"
            type="password"
            :placeholder="config?.has_api_key ? '已配置（留空不修改）' : 'sk-...'"
          />
        </div>
        <div class="col-12 col-md-4 flex items-center q-gutter-sm">
          <q-badge :color="config?.configured ? 'positive' : 'warning'">
            {{ config?.configured ? "已配置" : "未配置" }}
          </q-badge>
          <q-btn color="primary" unelevated label="保存" :loading="saving" @click="save" />
        </div>
      </div>
      <div class="text-caption text-grey-7 q-mt-sm">
        运行时生效；启动时仍可通过环境变量 KRATOS_KNOWLEDGE_EMBED_* 初始化。
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { reactive, watch } from "vue";
import type { EmbedderConfig, UpdateEmbedderConfigInput } from "../../features/knowledge/types";

const props = defineProps<{
  config: EmbedderConfig | null;
  saving?: boolean;
}>();

const emit = defineEmits<{
  save: [input: UpdateEmbedderConfigInput];
}>();

const providerOptions = [
  { label: "OpenAI / 兼容", value: "openai" },
  { label: "Ollama", value: "ollama" }
];

const form = reactive({
  provider: "openai",
  base_url: "",
  model: "text-embedding-3-small",
  dim: 1536,
  api_key: ""
});

watch(
  () => props.config,
  (cfg) => {
    if (!cfg) return;
    form.provider = cfg.provider || "openai";
    form.base_url = cfg.base_url || "";
    form.model = cfg.model || "text-embedding-3-small";
    form.dim = cfg.dim || 1536;
    form.api_key = "";
  },
  { immediate: true }
);

function save() {
  emit("save", {
    provider: form.provider,
    base_url: form.base_url.trim(),
    model: form.model.trim(),
    dim: form.dim,
    api_key: form.api_key.trim() || undefined
  });
}
</script>
