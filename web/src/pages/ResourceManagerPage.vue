<template>
  <q-page :class="['app-standard-page app-registry-page resource-manager-page', { 'is-dark': isDark }]">
    <template v-if="isProviderResource">
      <AppPageHero kicker="LLM Provider" :title="pageTitle" :subtitle="pageSubtitle">
        <template #actions>
          <q-btn color="primary" unelevated rounded icon="add" label="添加 Provider" @click="openCreate" />
        </template>
      </AppPageHero>

      <q-banner
        v-if="credentialEncryptionAvailable === false"
        dense
        rounded
        class="app-banner-warning q-mx-md q-mt-sm"
      >
        <template #avatar>
          <q-icon name="lock_open" color="warning" />
        </template>
        凭据加密密钥未配置，API 密钥将以明文存储。请在「系统设置」中初始化加密密钥，或设置 ARANEA_CREDENTIAL_KEY 环境变量。
      </q-banner>

      <q-card flat bordered class="app-entity-glass-panel provider-card">
        <q-card-section class="app-page-toolbar__body">
          <q-input
            v-model="keyword"
            class="app-page-toolbar__search provider-control"
            dense
            outlined
            clearable
            debounce="200"
            placeholder="搜索 Provider、模型或类型..."
          >
            <template #prepend><q-icon name="search" /></template>
          </q-input>
          <q-select
            v-model="providerTypeFilter"
            class="app-page-toolbar__field provider-control"
            dense
            outlined
            multiple
            use-chips
            emit-value
            map-options
            label="Provider 类型"
            :options="providerTypeFilterOptions"
          />
        </q-card-section>
        <q-separator />

        <div v-if="!loading && !pagedProviderRows.length" class="app-registry-empty empty-state q-card-section">
          <q-icon name="manage_search" size="40px" color="grey-5" />
          <div class="text-subtitle1 q-mt-sm">暂无 Provider 模型</div>
          <div class="text-caption text-grey-7">添加 Provider 后，可为每个模型配置能力分类、密钥和性能指标。</div>
          <q-btn class="q-mt-md" color="primary" unelevated rounded icon="add" label="添加 Provider" @click="openCreate" />
        </div>

        <div v-else class="app-registry-table-shell provider-table-shell">
          <ProviderModelsTable
            :rows="pagedProviderRows"
            :loading="loading"
            :saving="saving"
            :list-key-state="listKeyState"
            @toggle-enabled="toggleEnabled"
            @toggle-reveal-key="toggleListKeyReveal"
            @trend="openTrend"
            @edit="openEdit"
            @delete="confirmRemoveRow"
          />

          <AppRegistryPagination
            v-model:page="page"
            v-model:page-size="rowsPerPage"
            :page-max="pageCount"
            :total="filteredRows.length"
            :loading="loading"
            label="条模型"
            :page-size-options="[10, 20, 50]"
          />
        </div>
      </q-card>
    </template>

    <q-card v-else flat bordered class="resource-card">
      <q-card-section class="row items-center q-col-gutter-md">
        <div class="col-12 col-md">
          <div class="text-h6">{{ pageTitle }}</div>
          <div class="text-caption text-grey-7">{{ pageSubtitle }}</div>
        </div>
        <div class="app-field-md">
          <q-input v-model="keyword" dense outlined clearable debounce="200" label="搜索">
            <template #prepend><q-icon name="search" /></template>
          </q-input>
        </div>
        <div class="col-auto">
          <q-btn color="primary" unelevated rounded icon="add" label="新增" @click="openCreate" />
        </div>
      </q-card-section>
      <q-separator />
      <AppRegistryTable
        :shell="false"
        :rows="filteredRows"
        :columns="columns"
        row-key="id"
        :loading="loading"
        :pagination="{ rowsPerPage: 10 }"
      >
        <template #body-cell-status="props">
          <q-td :props="props">
            <q-badge :color="props.row.enabled ? 'positive' : 'grey'">
              {{ props.row.enabled ? "已启用" : "已禁用" }}
            </q-badge>
          </q-td>
        </template>
        <template #body-cell-actions="props">
          <q-td :props="props" class="q-gutter-xs">
            <q-btn flat dense round icon="edit" color="primary" :aria-label="`编辑 ${props.row.name}`" @click="openEdit(props.row)" />
            <q-btn flat dense round icon="delete" color="negative" :aria-label="`删除 ${props.row.name}`" @click="confirmRemoveRow(props.row)" />
          </q-td>
        </template>
      </AppRegistryTable>
    </q-card>

    <q-dialog v-model="dialogOpen" persistent>
      <q-card class="resource-dialog-card app-dialog-card app-dialog-card--xl app-glass-dialog provider-dialog">
        <q-card-section class="app-glass-dialog__head provider-dialog__head row items-start justify-between no-wrap">
          <div class="col min-width-0">
            <div class="app-glass-dialog__title provider-dialog__title">{{ dialogTitle }}</div>
            <div class="app-glass-dialog__subtitle provider-dialog__subtitle">{{ dialogSubtitle }}</div>
          </div>
          <q-btn flat dense round icon="close" aria-label="关闭" v-close-popup />
        </q-card-section>
        <q-separator />

        <template v-if="isProviderResource">
          <nav class="provider-wizard-nav" role="tablist" aria-label="Provider 配置步骤">
            <button
              v-for="step in providerWizardSteps"
              :key="step.id"
              type="button"
              role="tab"
              class="provider-wizard-step"
              :class="{ 'is-active': providerStep === step.id, 'is-done': providerStep > step.id }"
              :aria-selected="providerStep === step.id"
              @click="providerStep = step.id"
            >
              <span class="provider-wizard-step__index">{{ step.id }}</span>
              <span class="provider-wizard-step__text">
                <span class="provider-wizard-step__label">{{ step.title }}</span>
                <span class="provider-wizard-step__caption">{{ step.caption }}</span>
              </span>
            </button>
          </nav>

          <div class="provider-wizard-scroll">
            <q-card-section class="provider-wizard-body">
              <div v-show="providerStep === 1" class="provider-wizard-panel">
                <h3 class="provider-step-heading">连接与身份</h3>
                <p class="provider-step-hint">从 models.dev 目录选择或手动添加本地/自定义 Provider。</p>
                <div v-if="!editingId" class="app-grid-span-full q-mb-md">
                  <q-btn-toggle
                    v-model="providerAddMode"
                    spread
                    no-caps
                    toggle-color="primary"
                    :options="[
                      { label: '目录选择', value: 'catalog' },
                      { label: '自定义', value: 'custom' }
                    ]"
                    @update:model-value="setProviderAddMode($event === 'custom' ? 'custom' : 'catalog')"
                  />
                </div>
                <div class="app-form-field-grid app-form-field-grid--2col">
          <q-input
            v-if="providerAddMode === 'catalog'"
            v-model="catalogProviderSearch"
            dense
            outlined
            clearable
            debounce="300"
            label="搜索供应商"
            class="app-grid-span-full"
            @update:model-value="reloadCatalogProviders()"
          />
          <q-select
            v-if="providerAddMode === 'catalog'"
            v-model="catalogProviderId"
            dense
            outlined
            emit-value
            map-options
            label="供应商（models.dev）"
            :loading="catalogLoading"
            :options="catalogProviderOptions"
            @update:model-value="applyCatalogProvider(String($event ?? ''))"
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
          <div
            v-if="providerAddMode === 'catalog' && (catalogProviderHint || catalogProviderDocUrl)"
            class="app-grid-span-full row items-center q-gutter-sm q-mb-sm"
          >
            <span v-if="catalogProviderHint" class="text-caption text-grey-7">{{ catalogProviderHint }}</span>
            <a
              v-if="catalogProviderDocUrl"
              :href="catalogProviderDocUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="text-caption text-primary"
            >
              查看 Provider 文档 ↗
            </a>
          </div>
          <q-select
            v-else
            v-model="providerPresetKey"
            dense
            outlined
            emit-value
            map-options
            label="供应商预设（可选）"
            :options="providerPresetOptions"
            clearable
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
            v-if="providerRuntimeLocked"
            dense
            outlined
            readonly
            disable
            label="运行时类型"
            :model-value="providerRuntimeSummary"
            hint="由 models.dev 目录 / runtime overlay 自动决定，无需手动选择"
          />
          <q-select
            v-else
            v-model="providerForm.provider_type"
            dense
            outlined
            emit-value
            map-options
            label="Provider类型 *"
            :options="providerTypeOptions"
          />
          <q-input
            v-model="providerForm.api_key"
            dense
            outlined
            :type="showApiKey ? 'text' : 'password'"
            label="API 密钥"
            :hint="apiKeyFieldHint"
            :placeholder="apiKeyMaskedPlaceholder"
            :loading="revealingCredentials"
          >
            <template #append>
              <q-btn
                flat
                dense
                round
                :icon="showApiKey ? 'visibility_off' : 'visibility'"
                :aria-label="showApiKey ? '隐藏密钥' : '显示密钥'"
                :disable="revealingCredentials"
                @click="toggleApiKeyVisibility"
              />
            </template>
          </q-input>
          <div
            v-if="catalogModelsHint"
            class="app-grid-span-full text-caption text-grey-7 q-mb-xs"
          >
            {{ catalogModelsHint }}
          </div>
          <q-select
            v-model="providerForm.model_api_id"
            dense
            outlined
            :use-input="useCatalogModelPicker"
            :fill-input="false"
            :hide-selected="false"
            input-debounce="0"
            emit-value
            map-options
            label="模型"
            :loading="catalogModelsLoading"
            :options="providerModelOptions"
            @filter="(val, update) => useCatalogModelPicker ? filterCatalogModelsLocal(val, update) : update(() => {})"
            @new-value="setCustomModelValue"
            @update:model-value="useCatalogModelPicker ? applyCatalogModel(String($event ?? '')) : applyModelPreset(String($event ?? ''))"
          >
            <template #append>
              <q-btn
                flat
                dense
                no-caps
                class="provider-inspect-btn"
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
            dense
            outlined
            label="Provider ID *"
            hint="厂商 ID（如 deepseek），勿填模型名；目录模式下为 models.dev 供应商 id"
            :readonly="providerAddMode === 'catalog'"
            :rules="[providerCodeRule]"
          />
          <q-banner
            v-if="providerRuntimeBindingPreview"
            dense
            rounded
            class="app-grid-span-full bg-info text-white text-caption"
          >
            {{ providerRuntimeBindingPreview }}
          </q-banner>
          <q-banner
            v-if="editingId && providerIdentityChanged"
            dense
            rounded
            class="app-grid-span-full app-banner-warning text-caption"
          >
            已修改 Provider ID 或模型 ID，保存前请点击模型旁的「检查」验证连通性。
          </q-banner>
          <q-input
            v-model="providerForm.provider_display_name"
            dense
            outlined
            label="供应商名称"
            :readonly="providerAddMode === 'catalog'"
            hint="来自 catalog 的 name 字段"
          />
          <q-input v-model="providerForm.model_display_name" dense outlined label="模型展示名" />
          <q-input v-model="providerForm.api_base_url" dense outlined label="API 基础 URL" placeholder="https://..." />
          <q-toggle v-model="providerForm.enabled" label="已启用" />
          <q-select
            v-if="!providerRuntimeLocked && providerForm.provider_type === 'openai'"
            v-model="providerForm.variant"
            dense
            outlined
            emit-value
            map-options
            label="Variant"
            :options="variantOptions"
          />
          <template v-if="currentAuthType === 'secret_id_key'">
            <q-input v-model="providerForm.secret_id" dense outlined label="Secret ID" />
            <q-input
              v-model="providerForm.secret_key"
              dense
              outlined
              :type="showSecretKey ? 'text' : 'password'"
              label="Secret Key"
              :hint="editingId ? '留空表示不修改' : undefined"
              :placeholder="secretKeyMaskedPlaceholder"
              :loading="revealingCredentials"
            >
              <template #append>
                <q-btn
                  flat
                  dense
                  round
                  :icon="showSecretKey ? 'visibility_off' : 'visibility'"
                  :aria-label="showSecretKey ? '隐藏 Secret Key' : '显示 Secret Key'"
                  :disable="revealingCredentials"
                  @click="toggleSecretKeyVisibility"
                />
              </template>
            </q-input>
          </template>
          <q-input
            v-if="currentAuthType === 'aws_config'"
            v-model="providerForm.aws_region"
            dense
            outlined
            label="AWS Region"
            placeholder="us-east-1"
          />
                </div>
              </div>

              <div v-show="providerStep === 2" class="provider-wizard-panel">
                <h3 class="provider-step-heading">模型规格</h3>
                <p class="provider-step-hint">能力分类、上下文窗口、评级与价格快照。</p>
                <div class="app-form-field-grid">
          <div class="section-label app-grid-span-full">模型分类（能力说明）</div>
          <q-select
            v-model="providerForm.model_category"
            class="app-grid-span-full"
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

          <q-input v-model="providerForm.model_size_label" dense outlined label="模型大小" placeholder="7B / 70B" />
          <q-input
            v-model.number="providerForm.context_window_k"
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
            dense
            outlined
            type="number"
            min="1"
            label="最大输出 Token"
            hint="长回复输出上限，默认 4096"
          />
          <q-input
            v-model.number="providerForm.model_rating"
            dense
            outlined
            type="number"
            min="1"
            max="100"
            label="模型评级"
            hint="越高表示认为模型越强"
          />
          <q-slider v-model="providerForm.model_rating" class="app-grid-span-full q-px-sm provider-rating-slider" :min="1" :max="100" label />
          <div class="section-label app-grid-span-full">目录能力标签</div>
          <div v-if="providerForm.capability_chips.length" class="app-grid-span-full q-gutter-xs">
            <q-chip
              v-for="chip in providerForm.capability_chips"
              :key="chip.key"
              dense
              square
              color="blue-grey-1"
              text-color="blue-grey-9"
            >
              {{ chip.label }}
            </q-chip>
          </div>
          <div v-else class="text-caption text-grey-7 app-grid-span-full q-mb-sm">从目录选择模型后自动填充；自定义模型可留空。</div>

          <div class="section-label app-grid-span-full">价格快照（USD / 1M tokens）</div>
          <q-banner
            v-if="providerAddMode === 'catalog' && catalogPricingMissing"
            dense
            rounded
            class="app-banner-warning app-grid-span-full q-mb-sm"
          >
            目录中该模型未提供定价（或尚未同步 models.dev）。请前往「系统设置 → 模型目录」执行同步，或在此手动填写价格。
          </q-banner>
          <q-banner v-else-if="showPricingWarning" dense rounded class="app-banner-warning app-grid-span-full q-mb-sm">
            {{ pricingWarningMessage() }}
          </q-banner>
          <q-input v-model.number="providerForm.input_price_usd_per_1m" dense outlined type="number" min="0" step="0.0001" label="输入价格" />
          <q-input v-model.number="providerForm.output_price_usd_per_1m" dense outlined type="number" min="0" step="0.0001" label="输出价格" />
          <q-input v-model.number="providerForm.cache_read_usd_per_1m" dense outlined type="number" min="0" step="0.0001" label="缓存读取价格" />
          <q-input v-model.number="providerForm.reasoning_price_usd_per_1m" dense outlined type="number" min="0" step="0.0001" label="推理 Token 价格" />
          <q-input v-model.number="providerForm.embedding_price_usd_per_1m" dense outlined type="number" min="0" step="0.0001" label="Embedding 价格" />
          <q-input v-model.number="providerForm.sort_order" dense outlined type="number" label="排序" />
          <q-input v-model="providerForm.description" class="app-grid-span-full" dense outlined autogrow type="textarea" label="描述" />
                </div>
              </div>

              <div v-show="providerStep === 3" class="provider-wizard-panel">
                <h3 class="provider-step-heading">高可用</h3>
                <p class="provider-step-hint">配置 failover 链路与备用 Provider。</p>
                <ProviderHAConfig
                  v-model="providerHAForm"
                  :provider-type-options="providerTypeFilterOptions"
                />
              </div>

              <div v-show="providerStep === 4" class="provider-wizard-panel">
                <h3 class="provider-step-heading">高级选项</h3>
                <p class="provider-step-hint">Token tailoring、缓存优化与速率限制。</p>
                <div class="app-form-field-grid app-form-field-grid--2col">
                <q-toggle v-model="providerForm.enable_token_tailoring" label="Token Tailoring" />
                <q-toggle
                  v-if="providerForm.provider_type === 'openai'"
                  v-model="providerForm.optimize_for_cache"
                  label="Prompt Cache 优化"
                />
                <q-toggle
                  v-if="providerForm.provider_type === 'openai' && providerForm.variant === 'deepseek'"
                  v-model="providerForm.reasoning_backfill"
                  label="Reasoning 回填"
                />
                <q-toggle
                  v-if="['openai', 'anthropic'].includes(providerForm.provider_type)"
                  v-model="providerForm.show_tool_call_delta"
                  label="Tool Call Delta"
                />
                <q-input
                  v-model.number="providerForm.rate_limit_rpm"
                  dense
                  outlined
                  type="number"
                  min="0"
                  label="速率限制 (RPM)"
                />
                <q-input
                  v-if="providerForm.provider_type === 'ollama'"
                  v-model.number="providerForm.keep_alive_minutes"
                  dense
                  outlined
                  type="number"
                  min="0"
                  label="Keep Alive (分钟)"
                />
                </div>
              </div>
            </q-card-section>
          </div>
        </template>

        <q-card-section v-else class="app-form-field-grid app-form-field-grid--2col">
          <q-input v-model="form.key" dense outlined label="标识" />
          <q-input v-model="form.name" dense outlined label="名称" />
          <q-input v-model="form.description" class="app-grid-span-full" dense outlined autogrow type="textarea" label="描述" />
          <q-input v-model="form.provider" dense outlined label="Provider" />
          <q-input v-model="form.model" dense outlined label="模型" />
          <q-input v-model="form.parent_id" dense outlined label="父级 ID" />
          <q-input v-model="form.agent_id" dense outlined label="Agent ID" />
          <q-input v-model.number="form.sort_order" dense outlined type="number" label="排序" />
          <q-toggle v-model="form.enabled" color="primary" label="启用" />
          <q-input v-model="form.config_json" class="app-grid-span-full" dense outlined autogrow type="textarea" label="配置 JSON" />
          <q-input v-model="form.metadata_json" class="app-grid-span-full" dense outlined autogrow type="textarea" label="元数据 JSON" />
        </q-card-section>
        <q-separator v-if="isProviderResource" />
        <q-card-actions class="app-actions-bar app-glass-dialog__actions provider-dialog-actions">
          <q-btn flat no-caps label="取消" v-close-popup />
          <q-space />
          <template v-if="isProviderResource">
            <q-btn v-if="providerStep > 1" flat no-caps label="上一步" @click="providerStep -= 1" />
            <q-btn v-if="providerStep < 4" flat no-caps label="下一步" @click="providerStep += 1" />
          </template>
          <q-btn
            unelevated
            no-caps
            class="provider-dialog-save"
            :label="editingId ? '保存' : '创建'"
            :loading="saving"
            :disable="saving || !canSubmitNewProviderModel"
            @click="saveRow"
          >
            <q-tooltip v-if="isProviderResource && !canSubmitNewProviderModel">
              {{
                editingId && providerIdentityChanged
                  ? "修改 Provider/模型 ID 后需先「检查」"
                  : "远程模型需先点击「检查」并通过验证后再创建；本地自定义模型可直接创建"
              }}
            </q-tooltip>
          </q-btn>
        </q-card-actions>
      </q-card>
    </q-dialog>

    <ProviderTrendDialog
      v-model="trendDialogOpen"
      :row="trendRow"
      :metric="trendMetric"
      :metric-options="trendMetricOptions"
      :overview="trendOverview"
      :loading="trendOverviewLoading"
      @update:metric="trendMetric = $event"
    />
  </q-page>
</template>

<script setup lang="ts">
import AppPageHero from "../components/layout/AppPageHero.vue";
import AppRegistryTable from "../components/layout/AppRegistryTable.vue";
import AppRegistryPagination from "../components/layout/AppRegistryPagination.vue";
import ProviderHAConfig from "../components/platform/ProviderHAConfig.vue";
import ProviderModelsTable from "../components/platform/ProviderModelsTable.vue";
import ProviderTrendDialog from "../components/platform/ProviderTrendDialog.vue";
import { useResourceManagerPage } from "../features/platform/useResourceManagerPage";

const providerWizardSteps = [
  { id: 1, title: "连接", caption: "密钥与身份" },
  { id: 2, title: "规格", caption: "能力与定价" },
  { id: 3, title: "高可用", caption: "Failover" },
  { id: 4, title: "高级", caption: "限速与优化" }
] as const;

const {
  isDark,
  rows,
  loading,
  saving,
  checkingModel,
  keyword,
  dialogOpen,
  editingId,
  page,
  rowsPerPage,
  showApiKey,
  showSecretKey,
  revealingCredentials,
  trendDialogOpen,
  trendRow,
  providerPresetKey,
  providerStep,
  providerAddMode,
  catalogProviderId,
    catalogProviderHint,
    catalogProviderDocUrl,
  catalogModelsHint,
  catalogProviderSearch,
  reloadCatalogProviders,
  useCatalogModelPicker,
  providerRuntimeLocked,
  providerRuntimeSummary,
  catalogProviderOptions,
    catalogLoading,
    catalogModelsLoading,
    filterCatalogModelsLocal,
  providerTypeFilter,
  categoryOptions,
  providerTypeOptions,
  providerTypeFilterOptions,
  variantOptions,
  form,
  providerForm,
  providerHAForm,
  columns,
  isProviderResource,
  pageTitle,
  pageSubtitle,
  filteredRows,
  pageCount,
  pagedProviderRows,
  providerPresetOptions,
  currentAuthType,
  isLocalProviderModel,
  canInspectProviderModel,
  apiKeyFieldHint,
  apiKeyMaskedPlaceholder,
  secretKeyMaskedPlaceholder,
  showPricingWarning,
  canSubmitNewProviderModel,
  providerIdentityChanged,
  providerRuntimeBindingPreview,
  catalogPricingMissing,
  providerModelOptions,
  dialogTitle,
  dialogSubtitle,
  pricingWarningMessage,
  openCreate,
  openEdit,
  saveRow,
  applyProviderPreset,
  applyModelPreset,
  applyCatalogProvider,
  applyCatalogModel,
  setProviderAddMode,
  setCustomModelValue,
  inspectCurrentProviderModel,
  toggleApiKeyVisibility,
  toggleSecretKeyVisibility,
  toggleEnabled,
  confirmRemoveRow,
  openTrend,
  listKeyState,
  toggleListKeyReveal,
  trendOverview,
  trendOverviewLoading,
  trendMetric,
  trendMetricOptions,
  providerCodeRule,
  getCategories,
  metadataLabel,
  credentialEncryptionAvailable
} = useResourceManagerPage();
</script>
