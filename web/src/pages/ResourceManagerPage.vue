<template>
  <q-page :class="['q-pa-md resource-manager-page', { 'is-dark': isDark }]">
    <q-card v-if="isProviderResource" flat bordered class="provider-card">
      <q-card-section class="provider-header row items-center q-col-gutter-md">
        <div class="col-12 col-md">
          <div class="text-h5 text-weight-bold">Provider</div>
          <div class="text-body2 text-grey-7">管理 LLM Provider</div>
        </div>
        <div class="col-12 col-md-4">
          <q-input v-model="keyword" dense outlined clearable debounce="200" placeholder="搜索Provider...">
            <template #prepend><q-icon name="search" /></template>
          </q-input>
        </div>
        <div class="col-auto">
          <q-btn color="primary" unelevated rounded icon="add" label="添加Provider" @click="openCreate" />
        </div>
      </q-card-section>
      <q-separator />

      <q-card-section v-if="loading" class="q-gutter-md">
        <q-skeleton v-for="item in 4" :key="item" type="rect" height="96px" />
      </q-card-section>

      <q-list v-else-if="pagedProviderRows.length" separator class="provider-list">
        <ProviderModelRow
          v-for="row in pagedProviderRows"
          :key="row.id"
          :row="row"
          :saving="saving"
          @toggle-enabled="toggleEnabled"
          @trend="openTrend"
          @edit="openEdit"
          @delete="confirmRemoveRow"
        />
      </q-list>

      <q-card-section v-else class="empty-state">
        <q-icon name="manage_search" size="40px" color="grey-5" />
        <div class="text-subtitle1 q-mt-sm">暂无 Provider 模型</div>
        <div class="text-caption text-grey-7">添加 Provider 后，可为每个模型配置能力分类、密钥和性能指标。</div>
      </q-card-section>

      <q-separator />
      <q-card-actions class="row items-center justify-between pagination-bar">
        <div class="text-caption text-grey-7">共 {{ filteredRows.length }} 条，每页 {{ rowsPerPage }} 条</div>
        <q-pagination v-model="page" :max="pageCount" direction-links boundary-links color="primary" />
      </q-card-actions>
    </q-card>

    <q-card v-else flat bordered class="resource-card">
      <q-card-section class="row items-center q-col-gutter-md">
        <div class="col-12 col-md">
          <div class="text-h6">{{ pageTitle }}</div>
          <div class="text-caption text-grey-7">{{ pageSubtitle }}</div>
        </div>
        <div class="col-12 col-md-4">
          <q-input v-model="keyword" dense outlined clearable debounce="200" label="搜索">
            <template #prepend><q-icon name="search" /></template>
          </q-input>
        </div>
        <div class="col-auto">
          <q-btn color="primary" unelevated rounded icon="add" label="新增" @click="openCreate" />
        </div>
      </q-card-section>
      <q-separator />
      <q-table
        flat
        :rows="filteredRows"
        :columns="columns"
        row-key="id"
        :loading="loading"
        :pagination="{ rowsPerPage: 10 }"
      >
        <template #body-cell-status="props">
          <q-td :props="props">
            <q-badge :color="props.row.enabled ? 'positive' : 'grey'">
              {{ props.row.enabled ? "enabled" : "disabled" }}
            </q-badge>
          </q-td>
        </template>
        <template #body-cell-actions="props">
          <q-td :props="props" class="q-gutter-xs">
            <q-btn flat dense round icon="edit" color="primary" :aria-label="`编辑 ${props.row.name}`" @click="openEdit(props.row)" />
            <q-btn flat dense round icon="delete" color="negative" :aria-label="`删除 ${props.row.name}`" @click="confirmRemoveRow(props.row)" />
          </q-td>
        </template>
      </q-table>
    </q-card>

    <q-dialog v-model="dialogOpen" persistent>
      <q-card class="resource-dialog-card">
        <q-card-section>
          <div class="text-h6">{{ dialogTitle }}</div>
          <div class="text-caption text-grey-7">{{ dialogSubtitle }}</div>
        </q-card-section>
        <q-separator />
        <q-card-section v-if="isProviderResource" class="row q-col-gutter-md">
          <q-select
            v-model="providerPresetKey"
            class="col-12 col-md-6"
            dense
            outlined
            emit-value
            map-options
            label="供应商"
            :options="providerPresetOptions"
            @update:model-value="applyProviderPreset(String($event ?? ''))"
          >
            <template #option="scope">
              <q-item v-bind="scope.itemProps">
                <q-item-section>
                  <q-item-label>{{ scope.opt.label }}</q-item-label>
                  <q-item-label caption>{{ scope.opt.caption }}</q-item-label>
                </q-item-section>
              </q-item>
            </template>
          </q-select>
          <q-select
            v-model="providerForm.provider_type"
            class="col-12 col-md-6"
            dense
            outlined
            emit-value
            map-options
            label="Provider类型 *"
            :options="providerTypeOptions"
          />
          <q-input
            v-model="providerForm.api_key"
            class="col-12 col-md-6"
            dense
            outlined
            :type="showApiKey ? 'text' : 'password'"
            label="API 密钥"
            :hint="apiKeyFieldHint"
          >
            <template #append>
              <q-btn flat dense round :icon="showApiKey ? 'visibility_off' : 'visibility'" :aria-label="showApiKey ? '隐藏密钥' : '显示密钥'" @click="showApiKey = !showApiKey" />
            </template>
          </q-input>
          <q-select
            v-model="providerForm.model_api_id"
            class="col-12 col-md-6"
            dense
            outlined
            use-input
            fill-input
            hide-selected
            input-debounce="0"
            emit-value
            map-options
            label="模型ID"
            :options="providerModelOptions"
            @new-value="setCustomModelValue"
            @update:model-value="applyModelPreset(String($event ?? ''))"
          >
            <template #append>
              <q-btn
                flat
                dense
                rounded
                color="primary"
                label="检查"
                :loading="checkingModel"
                :disable="!canInspectProviderModel"
                @click.stop="inspectCurrentProviderModel"
              />
            </template>
            <template #option="scope">
              <q-item v-bind="scope.itemProps">
                <q-item-section>
                  <q-item-label>{{ scope.opt.label }}</q-item-label>
                  <q-item-label caption>{{ scope.opt.caption }}</q-item-label>
                </q-item-section>
              </q-item>
            </template>
          </q-select>
          <q-input
            v-model="providerForm.provider_code"
            class="col-12 col-md-6"
            dense
            outlined
            label="名称 *"
            hint="小写字母、数字、连字符，例如 openrouter"
            :rules="[providerCodeRule]"
          />
          <q-input v-model="providerForm.provider_display_name" class="col-12 col-md-6" dense outlined label="显示名称" />
          <q-input v-model="providerForm.model_display_name" class="col-12 col-md-6" dense outlined label="模型展示名" />
          <q-input v-model="providerForm.api_base_url" class="col-12 col-md-6" dense outlined label="API 基础 URL" placeholder="https://..." />
          <q-toggle v-model="providerForm.enabled" class="col-12 col-md-6" color="primary" label="已启用" />

          <q-separator class="col-12 q-my-sm" />
          <div class="col-12 section-label">模型分类（能力说明）</div>
          <q-select
            v-model="providerForm.model_category"
            class="col-12"
            dense
            outlined
            multiple
            use-chips
            label="模型类型"
            :options="categoryOptions"
            option-label="label"
            option-value="value"
          >
            <template #option="scope">
              <q-item v-bind="scope.itemProps">
                <q-item-section>
                  <q-item-label>{{ scope.opt.label }}</q-item-label>
                  <q-item-label caption>{{ scope.opt.tooltip }}</q-item-label>
                </q-item-section>
              </q-item>
            </template>
          </q-select>

          <q-input v-model="providerForm.model_size_label" class="col-12 col-md-3" dense outlined label="模型大小" placeholder="7B / 70B" />
          <q-input
            v-model.number="providerForm.context_window_k"
            class="col-12 col-md-3"
            dense
            outlined
            type="number"
            min="0"
            suffix="K"
            label="上下文大小"
            hint="单位 K，例如 128 表示 128K"
          />
          <q-input
            v-model.number="providerForm.max_output_tokens"
            class="col-12 col-md-3"
            dense
            outlined
            type="number"
            min="1"
            label="最大输出 Token"
            hint="长回复输出上限，默认 4096"
          />
          <q-input
            v-model.number="providerForm.model_rating"
            class="col-12 col-md-3"
            dense
            outlined
            type="number"
            min="1"
            max="100"
            label="模型评级"
            hint="越高表示认为模型越强"
          />
          <q-slider v-model="providerForm.model_rating" class="col-12 q-px-sm" :min="1" :max="100" label color="primary" />
          <div class="col-12 section-label">价格快照（micro USD / 1K tokens）</div>
          <q-input v-model.number="providerForm.input_price_micro_usd_per_1k" class="col-12 col-md-3" dense outlined type="number" min="0" label="输入价格" />
          <q-input v-model.number="providerForm.output_price_micro_usd_per_1k" class="col-12 col-md-3" dense outlined type="number" min="0" label="输出价格" />
          <q-input v-model.number="providerForm.cached_input_price_micro_usd_per_1k" class="col-12 col-md-3" dense outlined type="number" min="0" label="缓存输入价格" />
          <q-input v-model.number="providerForm.reasoning_price_micro_usd_per_1k" class="col-12 col-md-3" dense outlined type="number" min="0" label="推理 Token 价格" />
          <q-input v-model.number="providerForm.sort_order" class="col-12 col-md-6" dense outlined type="number" label="排序" />
          <q-input v-model="providerForm.description" class="col-12" dense outlined autogrow type="textarea" label="描述" />
        </q-card-section>

        <q-card-section v-else class="row q-col-gutter-md">
          <q-input v-model="form.key" class="col-12 col-md-6" dense outlined label="Key" />
          <q-input v-model="form.name" class="col-12 col-md-6" dense outlined label="Name" />
          <q-input v-model="form.description" class="col-12" dense outlined autogrow type="textarea" label="Description" />
          <q-input v-model="form.provider" class="col-12 col-md-6" dense outlined label="Provider" />
          <q-input v-model="form.model" class="col-12 col-md-6" dense outlined label="Model" />
          <q-input v-model="form.parent_id" class="col-12 col-md-6" dense outlined label="Parent ID" />
          <q-input v-model="form.agent_id" class="col-12 col-md-6" dense outlined label="Agent ID" />
          <q-input v-model.number="form.sort_order" class="col-12 col-md-6" dense outlined type="number" label="Sort Order" />
          <q-toggle v-model="form.enabled" class="col-12 col-md-6" color="primary" label="Enabled" />
          <q-input v-model="form.config_json" class="col-12" dense outlined autogrow type="textarea" label="Config JSON" />
          <q-input v-model="form.metadata_json" class="col-12" dense outlined autogrow type="textarea" label="Metadata JSON" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat rounded label="取消" v-close-popup />
          <q-btn
            color="primary"
            rounded
            unelevated
            :label="editingId ? '保存' : '创建'"
            :loading="saving"
            :disable="saving || !canSubmitNewProviderModel"
            @click="saveRow"
          >
            <q-tooltip v-if="isProviderResource && !editingId && !canSubmitNewProviderModel">
              请先点击「检查」并通过远程验证后再创建
            </q-tooltip>
          </q-btn>
        </q-card-actions>
      </q-card>
    </q-dialog>

    <ProviderTrendDialog v-model="trendDialogOpen" :row="trendRow" />
  </q-page>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { useQuasar, type QTableColumn } from "quasar";
import {
  createPlatformResource,
  deletePlatformResource,
  inspectProviderModel,
  listPlatformResources,
  updatePlatformResource,
  type PlatformResource,
  type PlatformResourceInput,
  type PlatformResourceName
} from "../features/platform/api";
import ProviderModelRow from "../components/platform/ProviderModelRow.vue";
import ProviderTrendDialog from "../components/platform/ProviderTrendDialog.vue";
import {
  PROVIDER_PRESETS,
  findModelPreset,
  findProviderPreset,
  type ProviderModelPreset
} from "../config/providerPresets";

const route = useRoute();
const $q = useQuasar();
const isDark = computed(() => $q.dark.isActive);

const rows = ref<PlatformResource[]>([]);
const loading = ref(false);
const saving = ref(false);
const checkingModel = ref(false);
const keyword = ref("");
const dialogOpen = ref(false);
const editingId = ref("");
const page = ref(1);
const rowsPerPage = ref(20);
const showApiKey = ref(false);
const trendDialogOpen = ref(false);
const trendRow = ref<PlatformResource | null>(null);
const providerPresetKey = ref("");
/** 新建 Provider：最近一次「检查」成功时的连接指纹；改动 code/model/base/key/type 后需重新检查 */
const providerCreateInspectFingerprint = ref("");

type ModelCategory = {
  value: string;
  label: string;
  tooltip: string;
};

type ProviderConfig = {
  provider_type?: string;
  provider_display_name?: string;
  api_base_url?: string;
  api_key?: string;
  api_key_set?: boolean;
  model_category?: ModelCategory[];
  model_size_label?: string;
  context_window_k?: number | string | null;
  max_output_tokens?: number | string | null;
  tokens_per_second?: number | string | null;
  model_hotness_score?: number | string | null;
  usage_call_count_30d?: number | string | null;
  usage_total_tokens_30d?: number | string | null;
  usage_cost_micro_usd_30d?: number | string | null;
  success_rate_30d?: number | string | null;
  avg_latency_ms_30d?: number | string | null;
  input_price_micro_usd_per_1k?: number | string | null;
  output_price_micro_usd_per_1k?: number | string | null;
  cached_input_price_micro_usd_per_1k?: number | string | null;
  reasoning_price_micro_usd_per_1k?: number | string | null;
  embedding_price_micro_usd_per_1k?: number | string | null;
  raw_metadata_json?: string;
  metadata_source?: string;
  last_used_at?: string;
  model_rating?: number | string | null;
};

const categoryOptions: ModelCategory[] = [
  { value: "general", label: "通用对话", tooltip: "均衡，适合日常问答与轻任务" },
  { value: "reasoning", label: "推理 / 复杂问题", tooltip: "数学、逻辑、多步推导" },
  { value: "code", label: "代码", tooltip: "生成、解释、重构代码" },
  { value: "long_context", label: "长上下文", tooltip: "大文档、长会话摘要" },
  { value: "vision", label: "视觉 / 多模态", tooltip: "图像理解" },
  { value: "embedding", label: "向量嵌入", tooltip: "记忆、检索" },
  { value: "fast", label: "低延迟", tooltip: "优先响应速度" },
  { value: "creative", label: "创意写作", tooltip: "文案、故事、营销" }
];

const providerTypeOptions = [
  { label: "OpenAI Compatible", value: "openai" },
  { label: "Anthropic", value: "anthropic" },
  { label: "Google Gemini", value: "gemini" },
  { label: "Azure OpenAI", value: "openai" },
  { label: "Ollama", value: "ollama" },
  { label: "Hunyuan", value: "hunyuan" },
  { label: "自定义", value: "openai" }
];

const form = reactive<PlatformResourceInput>({
  key: "",
  name: "",
  description: "",
  enabled: true,
  sort_order: 0,
  parent_id: "",
  level: "",
  agent_id: "",
  provider: "",
  model: "",
  config_json: "{}",
  metadata_json: "{}"
});

const providerForm = reactive({
  provider_type: "openai",
  model_api_id: "",
  provider_code: "",
  provider_display_name: "",
  model_display_name: "",
  api_base_url: "",
  api_key: "",
  api_key_set: false,
  enabled: true,
  model_category: [] as ModelCategory[],
  model_size_label: "",
  context_window_k: null as number | null,
  max_output_tokens: 4096,
  model_rating: 60,
  input_price_micro_usd_per_1k: 0,
  output_price_micro_usd_per_1k: 0,
  cached_input_price_micro_usd_per_1k: 0,
  reasoning_price_micro_usd_per_1k: 0,
  embedding_price_micro_usd_per_1k: 0,
  raw_metadata_json: "",
  metadata_source: "",
  sort_order: 0,
  description: ""
});

const columns: QTableColumn<PlatformResource>[] = [
  { name: "name", field: "name", label: "Name", align: "left", sortable: true },
  { name: "key", field: "key", label: "Key", align: "left", sortable: true },
  { name: "provider", field: "provider", label: "Provider", align: "left" },
  { name: "model", field: "model", label: "Model", align: "left" },
  { name: "status", field: "status", label: "Status", align: "left" },
  { name: "actions", field: "id", label: "Actions", align: "right" }
];

const resource = computed(() => route.meta.resource as PlatformResourceName);
const isProviderResource = computed(() => resource.value === "llm-provider-models");
const pageTitle = computed(() => (route.meta.title as string) || "资源管理");
const pageSubtitle = computed(() => (route.meta.subtitle as string) || "管理平台资源、启用状态与运行配置。");
const filteredRows = computed(() => {
  const q = keyword.value.trim().toLowerCase();
  if (!q) return rows.value;
  return rows.value.filter((row) =>
    [
      row.key,
      row.name,
      row.description,
      row.provider,
      row.model,
      row.agent_id,
      getConfig(row).provider_display_name,
      ...getCategories(row).map((category) => category.label)
    ].some((value) =>
      (value || "").toLowerCase().includes(q)
    )
  );
});
const pageCount = computed(() => Math.max(1, Math.ceil(filteredRows.value.length / rowsPerPage.value)));
const pagedProviderRows = computed(() => {
  const start = (page.value - 1) * rowsPerPage.value;
  return filteredRows.value.slice(start, start + rowsPerPage.value);
});
const providerPresetOptions = computed(() =>
  PROVIDER_PRESETS.map((preset) => ({
    label: preset.label,
    value: preset.key,
    caption: `${preset.apiBaseUrl || "手动配置"} · ${metadataLabel(preset.metadataApi)}`
  }))
);
const currentProviderPreset = computed(() => findProviderPreset(providerPresetKey.value || providerForm.provider_code));

/** 本地 / 内网模型：检查列表通常可不填密钥（如 Ollama、localhost OpenAI 兼容） */
const isLocalProviderModel = computed(() => {
  if (providerForm.provider_type === "ollama") return true;
  const raw = providerForm.api_base_url.trim();
  if (!raw) return false;
  return /^(https?|wss?):\/\/(localhost|127\.0\.0\.1|\[::1\])([/:?#]|$)/i.test(raw);
});

const hasInspectApiKey = computed(() => {
  if (providerForm.api_key.trim()) return true;
  if (editingId.value && providerForm.api_key_set) return true;
  return false;
});

const canInspectProviderModel = computed(() => {
  if (!providerForm.provider_code.trim() || !providerForm.model_api_id.trim()) return false;
  if (isLocalProviderModel.value) return true;
  return hasInspectApiKey.value;
});

const apiKeyFieldHint = computed(() => {
  const parts: string[] = [];
  if (editingId.value) parts.push("留空表示不修改");
  if (!isLocalProviderModel.value) parts.push("远程 Provider 检查模型前需填写密钥");
  return parts.join("；") || undefined;
});

function providerCreateInspectFingerprintValue(): string {
  return [
    providerForm.provider_code.trim(),
    providerForm.model_api_id.trim(),
    providerForm.api_base_url.trim(),
    providerForm.api_key.trim(),
    providerForm.provider_type.trim()
  ].join("\0");
}

const canSubmitNewProviderModel = computed(() => {
  if (!isProviderResource.value) return true;
  if (editingId.value) return true;
  const saved = providerCreateInspectFingerprint.value;
  if (!saved) return false;
  return saved === providerCreateInspectFingerprintValue();
});

const providerModelOptions = computed(() =>
  (currentProviderPreset.value?.models ?? []).map((model) => ({
    label: model.label,
    value: model.id,
    caption: `${model.id}${model.contextWindowK ? ` · ${model.contextWindowK}K ctx` : ""}`
  }))
);
const dialogTitle = computed(() => {
  if (!isProviderResource.value) return editingId.value ? "编辑资源" : "新增资源";
  return editingId.value ? "编辑Provider" : "添加Provider";
});
const dialogSubtitle = computed(() => {
  if (!isProviderResource.value) return "Key 和 Name 为必填，其他字段按模块需要填写。";
  if (!editingId.value) return "配置 LLM Provider 连接。新建需先点击「检查」并通过验证后再创建。";
  return "配置 LLM Provider 连接";
});

onMounted(loadRows);

watch(resource, () => {
  keyword.value = "";
  page.value = 1;
  void loadRows();
});

watch(filteredRows, () => {
  if (page.value > pageCount.value) page.value = pageCount.value;
});

async function loadRows() {
  if (!resource.value) return;
  loading.value = true;
  try {
    rows.value = await listPlatformResources(resource.value);
  } finally {
    loading.value = false;
  }
}

function resetForm() {
  Object.assign(form, {
    key: "",
    name: "",
    description: "",
    enabled: true,
    sort_order: 0,
    parent_id: "",
    level: "",
    agent_id: "",
    provider: "",
    model: "",
    config_json: "{}",
    metadata_json: "{}"
  });
  Object.assign(providerForm, {
    provider_type: "openai",
    model_api_id: "",
    provider_code: "",
    provider_display_name: "",
    model_display_name: "",
    api_base_url: "",
    api_key: "",
    api_key_set: false,
    enabled: true,
    model_category: [],
    model_size_label: "",
    context_window_k: null,
    max_output_tokens: 4096,
    model_rating: 60,
    input_price_micro_usd_per_1k: 0,
    output_price_micro_usd_per_1k: 0,
    cached_input_price_micro_usd_per_1k: 0,
    reasoning_price_micro_usd_per_1k: 0,
    embedding_price_micro_usd_per_1k: 0,
    raw_metadata_json: "",
    metadata_source: "",
    sort_order: 0,
    description: ""
  });
  showApiKey.value = false;
  providerPresetKey.value = "";
  providerCreateInspectFingerprint.value = "";
}

function openCreate() {
  editingId.value = "";
  resetForm();
  dialogOpen.value = true;
}

function openEdit(row: PlatformResource) {
  editingId.value = row.id;
  Object.assign(form, {
    key: row.key,
    name: row.name,
    description: row.description,
    enabled: row.enabled,
    sort_order: row.sort_order,
    parent_id: row.parent_id,
    level: row.level,
    agent_id: row.agent_id,
    provider: row.provider,
    model: row.model,
    config_json: row.config_json || "{}",
    metadata_json: row.metadata_json || "{}"
  });
  if (isProviderResource.value) {
    const config = getConfig(row);
    providerPresetKey.value = findProviderPreset(row.provider)?.key || "";
    Object.assign(providerForm, {
      provider_type: normalizeProviderType(config.provider_type),
      model_api_id: row.model,
      provider_code: row.provider,
      provider_display_name: config.provider_display_name || row.provider,
      model_display_name: row.name,
      api_base_url: config.api_base_url || "",
      api_key: "",
      api_key_set: Boolean(config.api_key_set),
      enabled: row.enabled,
      model_category: getCategories(row),
      model_size_label: config.model_size_label || "",
      context_window_k: toNullableNumber(config.context_window_k),
      max_output_tokens: toNumber(config.max_output_tokens, 4096),
      model_rating: toNumber(config.model_rating, 60),
      input_price_micro_usd_per_1k: toNumber(config.input_price_micro_usd_per_1k, 0),
      output_price_micro_usd_per_1k: toNumber(config.output_price_micro_usd_per_1k, 0),
      cached_input_price_micro_usd_per_1k: toNumber(config.cached_input_price_micro_usd_per_1k, 0),
      reasoning_price_micro_usd_per_1k: toNumber(config.reasoning_price_micro_usd_per_1k, 0),
      embedding_price_micro_usd_per_1k: toNumber(config.embedding_price_micro_usd_per_1k, 0),
      raw_metadata_json: config.raw_metadata_json || "",
      metadata_source: config.metadata_source || "",
      sort_order: row.sort_order,
      description: row.description
    });
  }
  providerCreateInspectFingerprint.value = "";
  dialogOpen.value = true;
}

async function saveRow() {
  if (isProviderResource.value) {
    await saveProviderRow();
    return;
  }
  if (!form.key || !form.name) {
    $q.notify({ type: "negative", message: "Key 和 Name 必填" });
    return;
  }
  saving.value = true;
  try {
    if (editingId.value) {
      const updated = await updatePlatformResource(resource.value, editingId.value, form);
      rows.value = rows.value.map((row) => (row.id === updated.id ? updated : row));
    } else {
      const created = await createPlatformResource(resource.value, form);
      rows.value = [created, ...rows.value];
    }
    dialogOpen.value = false;
    $q.notify({ type: "positive", message: "已保存" });
  } finally {
    saving.value = false;
  }
}

async function saveProviderRow() {
  const code = providerForm.provider_code.trim();
  const model = providerForm.model_api_id.trim();
  if (!code || !model || !isProviderCodeValid(code)) {
    $q.notify({ type: "negative", message: "Provider 名称和模型ID必填，名称仅支持小写字母、数字、连字符" });
    return;
  }
  if (!editingId.value && !canSubmitNewProviderModel.value) {
    $q.notify({ type: "warning", message: "请先点击「检查」并通过验证后再创建" });
    return;
  }

  const payload = buildProviderPayload();
  saving.value = true;
  try {
    if (editingId.value) {
      const updated = await updatePlatformResource(resource.value, editingId.value, payload);
      rows.value = rows.value.map((row) => (row.id === updated.id ? updated : row));
    } else {
      const created = await createPlatformResource(resource.value, payload);
      rows.value = [created, ...rows.value];
    }
    dialogOpen.value = false;
    $q.notify({ type: "positive", message: "已保存" });
  } finally {
    saving.value = false;
  }
}

function applyProviderPreset(key: string) {
  const preset = findProviderPreset(key);
  if (!preset) return;
  providerPresetKey.value = preset.key;
  providerForm.provider_type = preset.providerType;
  providerForm.provider_code = preset.providerCode;
  providerForm.provider_display_name = preset.label;
  providerForm.api_base_url = preset.apiBaseUrl;
  if (!providerForm.model_api_id && preset.models[0]) {
    providerForm.model_api_id = preset.models[0].id;
    applyModelPreset(preset.models[0].id);
  } else {
    applyModelPreset(providerForm.model_api_id);
  }
}

function applyModelPreset(modelId: string) {
  const preset = findModelPreset(providerPresetKey.value || providerForm.provider_code, modelId);
  if (!preset) return;
  applyModelPresetValues(preset, false);
}

function applyModelPresetValues(preset: ProviderModelPreset, overwrite = false) {
  providerForm.model_display_name = overwrite || !providerForm.model_display_name ? preset.label : providerForm.model_display_name;
  providerForm.model_size_label = overwrite || !providerForm.model_size_label ? preset.sizeLabel || providerForm.model_size_label : providerForm.model_size_label;
  providerForm.context_window_k = overwrite || !providerForm.context_window_k ? preset.contextWindowK ?? providerForm.context_window_k : providerForm.context_window_k;
  providerForm.max_output_tokens = overwrite || !providerForm.max_output_tokens ? preset.maxOutputTokens ?? providerForm.max_output_tokens : providerForm.max_output_tokens;
  providerForm.input_price_micro_usd_per_1k = overwrite || !providerForm.input_price_micro_usd_per_1k ? preset.inputPriceMicroUsdPer1K ?? providerForm.input_price_micro_usd_per_1k : providerForm.input_price_micro_usd_per_1k;
  providerForm.output_price_micro_usd_per_1k = overwrite || !providerForm.output_price_micro_usd_per_1k ? preset.outputPriceMicroUsdPer1K ?? providerForm.output_price_micro_usd_per_1k : providerForm.output_price_micro_usd_per_1k;
  providerForm.cached_input_price_micro_usd_per_1k = overwrite || !providerForm.cached_input_price_micro_usd_per_1k ? preset.cachedInputPriceMicroUsdPer1K ?? providerForm.cached_input_price_micro_usd_per_1k : providerForm.cached_input_price_micro_usd_per_1k;
  providerForm.reasoning_price_micro_usd_per_1k = overwrite || !providerForm.reasoning_price_micro_usd_per_1k ? preset.reasoningPriceMicroUsdPer1K ?? providerForm.reasoning_price_micro_usd_per_1k : providerForm.reasoning_price_micro_usd_per_1k;
  providerForm.embedding_price_micro_usd_per_1k = overwrite || !providerForm.embedding_price_micro_usd_per_1k ? preset.embeddingPriceMicroUsdPer1K ?? providerForm.embedding_price_micro_usd_per_1k : providerForm.embedding_price_micro_usd_per_1k;
}

function setCustomModelValue(value: string, done: (value: string, mode?: "add" | "add-unique" | "toggle") => void) {
  done(value, "add-unique");
  providerForm.model_api_id = value;
}

async function inspectCurrentProviderModel() {
  const code = providerForm.provider_code.trim();
  const model = providerForm.model_api_id.trim();
  if (!code || !model) {
    $q.notify({ type: "negative", message: "请先填写 Provider 名称和模型ID" });
    return;
  }
  if (!canInspectProviderModel.value) {
    $q.notify({ type: "warning", message: "非本地模型需填写 API 密钥后才能检查" });
    return;
  }
  checkingModel.value = true;
  try {
    const result = await inspectProviderModel({
      resource_id: editingId.value,
      provider_code: code,
      provider_type: providerForm.provider_type,
      model_api_id: model,
      api_base_url: providerForm.api_base_url.trim(),
      api_key: providerForm.api_key.trim()
    });
    if (!result.ok) {
      if (!editingId.value) {
        providerCreateInspectFingerprint.value = "";
      }
      const preset = findModelPreset(providerPresetKey.value || code, model);
      if (preset) {
        applyModelPresetValues(preset, true);
        providerForm.metadata_source = `${currentProviderPreset.value?.key || code}-preset`;
        providerForm.raw_metadata_json = JSON.stringify({ source: "frontend-provider-preset", provider: providerPresetKey.value || code, model });
        $q.notify({ type: "warning", message: `${result.message || "未获取到模型参数"}；已使用前端预设参数回填` });
        return;
      }
      $q.notify({ type: "warning", message: result.message || "未获取到模型参数，也没有匹配的预设参数" });
      return;
    }
    if (!editingId.value) {
      providerCreateInspectFingerprint.value = providerCreateInspectFingerprintValue();
    }
    providerForm.provider_type = result.provider_type || providerForm.provider_type;
    providerForm.model_display_name = result.model_display_name || providerForm.model_display_name || model;
    providerForm.model_size_label = result.model_size_label || providerForm.model_size_label;
    providerForm.context_window_k = result.context_window_k || providerForm.context_window_k;
    providerForm.max_output_tokens = result.max_output_tokens || providerForm.max_output_tokens;
    providerForm.input_price_micro_usd_per_1k = result.input_price_micro_usd_per_1k || 0;
    providerForm.output_price_micro_usd_per_1k = result.output_price_micro_usd_per_1k || 0;
    providerForm.cached_input_price_micro_usd_per_1k = result.cached_input_price_micro_usd_per_1k || 0;
    providerForm.reasoning_price_micro_usd_per_1k = result.reasoning_price_micro_usd_per_1k || 0;
    providerForm.embedding_price_micro_usd_per_1k = result.embedding_price_micro_usd_per_1k || 0;
    providerForm.raw_metadata_json = result.raw_metadata_json || "";
    providerForm.metadata_source = result.source || "";
    $q.notify({ type: "positive", message: result.message || "已获取模型参数" });
  } catch (error) {
    if (!editingId.value) {
      providerCreateInspectFingerprint.value = "";
    }
    $q.notify({ type: "negative", message: errorMessage(error) });
  } finally {
    checkingModel.value = false;
  }
}

async function toggleEnabled(row: PlatformResource, enabled: boolean) {
  saving.value = true;
  try {
    const updated = await updatePlatformResource(resource.value, row.id, { ...row, enabled });
    rows.value = rows.value.map((item) => (item.id === updated.id ? updated : item));
  } finally {
    saving.value = false;
  }
}

function confirmRemoveRow(row: PlatformResource) {
  $q.dialog({
    title: "确认删除",
    message: `确定删除「${row.name}」吗？`,
    cancel: true,
    persistent: true
  }).onOk(() => {
    void removeRow(row);
  });
}

function openTrend(row: PlatformResource) {
  trendRow.value = row;
  trendDialogOpen.value = true;
}

async function removeRow(row: PlatformResource) {
  await deletePlatformResource(resource.value, row.id);
  rows.value = rows.value.filter((item) => item.id !== row.id);
  $q.notify({ type: "positive", message: "已删除" });
}

function buildProviderPayload(): PlatformResourceInput {
  const editingRow = editingId.value ? rows.value.find((row) => row.id === editingId.value) : undefined;
  const existingConfig = editingRow ? getConfig(editingRow) : {};
  const existingApiKey = existingConfig.api_key || "";
  const nextApiKey = providerForm.api_key.trim() || existingApiKey;
  const config: ProviderConfig = {
    provider_type: providerForm.provider_type,
    provider_display_name: providerForm.provider_display_name.trim(),
    api_base_url: providerForm.api_base_url.trim(),
    api_key_set: providerForm.api_key_set || Boolean(nextApiKey),
    model_category: providerForm.model_category,
    model_size_label: providerForm.model_size_label.trim(),
    context_window_k: providerForm.context_window_k,
    max_output_tokens: providerForm.max_output_tokens,
    tokens_per_second: existingConfig.tokens_per_second,
    model_hotness_score: existingConfig.model_hotness_score,
    usage_call_count_30d: existingConfig.usage_call_count_30d,
    usage_total_tokens_30d: existingConfig.usage_total_tokens_30d,
    usage_cost_micro_usd_30d: existingConfig.usage_cost_micro_usd_30d,
    success_rate_30d: existingConfig.success_rate_30d,
    avg_latency_ms_30d: existingConfig.avg_latency_ms_30d,
    input_price_micro_usd_per_1k: providerForm.input_price_micro_usd_per_1k,
    output_price_micro_usd_per_1k: providerForm.output_price_micro_usd_per_1k,
    cached_input_price_micro_usd_per_1k: providerForm.cached_input_price_micro_usd_per_1k,
    reasoning_price_micro_usd_per_1k: providerForm.reasoning_price_micro_usd_per_1k,
    embedding_price_micro_usd_per_1k: providerForm.embedding_price_micro_usd_per_1k,
    raw_metadata_json: providerForm.raw_metadata_json,
    metadata_source: providerForm.metadata_source,
    last_used_at: existingConfig.last_used_at,
    model_rating: providerForm.model_rating
  };
  if (nextApiKey) {
    config.api_key = nextApiKey;
  }

  const code = providerForm.provider_code.trim();
  const model = providerForm.model_api_id.trim();
  return {
    key: `${code}:${model}`,
    name: providerForm.model_display_name.trim() || model,
    description: providerForm.description.trim(),
    enabled: providerForm.enabled,
    sort_order: providerForm.sort_order,
    provider: code,
    model,
    config_json: JSON.stringify(config),
    metadata_json: JSON.stringify({ model_rating: providerForm.model_rating })
  };
}

function normalizeProviderType(raw: string | undefined): string {
  const v = (raw || "").trim().toLowerCase();
  if (v === "anthropic") return "anthropic";
  if (v === "gemini" || v === "google gemini") return "gemini";
  if (v === "ollama") return "ollama";
  if (v === "hunyuan") return "hunyuan";
  return "openai";
}

function getConfig(row: PlatformResource): ProviderConfig {
  if (!row.config_json) return {};
  try {
    const value = JSON.parse(row.config_json) as ProviderConfig;
    return value && typeof value === "object" ? value : {};
  } catch {
    return {};
  }
}

function getCategories(row: PlatformResource): ModelCategory[] {
  const categories = getConfig(row).model_category;
  if (!Array.isArray(categories)) return [];
  return categories.filter((category) => category?.value && category?.label && category?.tooltip);
}

function toNullableNumber(value: unknown) {
  if (value === "" || value === null || value === undefined) return null;
  const numberValue = Number(value);
  return Number.isFinite(numberValue) ? numberValue : null;
}

function toNumber(value: unknown, fallback: number) {
  const numberValue = toNullableNumber(value);
  return numberValue === null ? fallback : numberValue;
}

function providerCodeRule(value: string) {
  return isProviderCodeValid(value) || "仅支持小写字母、数字、连字符";
}

function isProviderCodeValid(value: string) {
  return /^[a-z0-9-]+$/.test(value);
}

function metadataLabel(value: string) {
  if (value === "full") return "可查询参数";
  if (value === "partial") return "可验证模型";
  if (value === "limited") return "有限查询";
  return "手动维护";
}

function errorMessage(error: unknown) {
  if (typeof error === "object" && error && "response" in error) {
    const response = (error as { response?: { data?: { error?: string } } }).response;
    if (response?.data?.error) return response.data.error;
  }
  return error instanceof Error ? error.message : "模型检查失败";
}
</script>

<style scoped>
.resource-manager-page {
  min-height: 100%;
  background: #f7f8fb;
}

.resource-card,
.provider-card {
  border-radius: 18px;
  overflow: hidden;
}

.provider-header {
  background: linear-gradient(135deg, #ffffff 0%, #f3f6ff 100%);
}

.provider-list {
  background: #fff;
}

.empty-state {
  padding: 48px 16px;
  text-align: center;
}

.pagination-bar {
  padding: 12px 20px;
}

.section-label {
  color: #374151;
  font-size: 14px;
  font-weight: 700;
}

.resource-dialog-card {
  width: 860px;
  max-width: 94vw;
}

.resource-manager-page.is-dark {
  background:
    radial-gradient(circle at 86% 0%, rgba(59, 130, 246, 0.16), transparent 30%),
    radial-gradient(circle at 10% 16%, rgba(245, 158, 11, 0.08), transparent 24%),
    linear-gradient(160deg, #0b1220 0%, #111827 48%, #0f172a 100%);
  color: #e5e7eb;
}

.resource-manager-page.is-dark .provider-card,
.resource-manager-page.is-dark .resource-card,
.resource-manager-page.is-dark .resource-dialog-card {
  border-color: rgba(148, 163, 184, 0.16);
  background: rgba(17, 24, 39, 0.9);
  box-shadow: 0 14px 38px rgba(0, 0, 0, 0.32);
}

.resource-manager-page.is-dark .provider-header {
  background:
    linear-gradient(180deg, rgba(17, 24, 39, 0.96), rgba(15, 23, 42, 0.9)),
    radial-gradient(circle at top right, rgba(59, 130, 246, 0.14), transparent 34%);
}

.resource-manager-page.is-dark .provider-list {
  background: rgba(15, 23, 42, 0.86);
}

.resource-manager-page.is-dark :deep(.q-field__control) {
  background: rgba(30, 41, 59, 0.76);
}

.resource-manager-page.is-dark :deep(.q-field__control::before) {
  border-color: rgba(148, 163, 184, 0.18);
}

.resource-manager-page.is-dark :deep(.q-table__container) {
  background: rgba(17, 24, 39, 0.9);
  color: #e5e7eb;
}

.resource-manager-page.is-dark :deep(.q-table th) {
  background: rgba(15, 23, 42, 0.92);
  color: #cbd5e1;
}

.resource-manager-page.is-dark :deep(.q-table td) {
  color: #e2e8f0;
}

.resource-manager-page.is-dark :deep(.q-table tbody tr:hover) {
  background: rgba(51, 65, 85, 0.46);
}

.resource-manager-page.is-dark .pagination-bar,
.resource-manager-page.is-dark :deep(.q-table__bottom) {
  border-top-color: rgba(148, 163, 184, 0.14);
  background: rgba(15, 23, 42, 0.72);
  color: #cbd5e1;
}

.resource-manager-page.is-dark .section-label {
  color: #cbd5e1;
}

.resource-manager-page.is-dark .empty-state {
  color: #cbd5e1;
}
</style>
