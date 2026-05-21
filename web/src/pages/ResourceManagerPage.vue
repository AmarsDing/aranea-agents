<template>
  <q-page :class="['app-registry-page resource-manager-page', { 'is-dark': isDark }]">
    <div class="app-page-shell">
    <template v-if="isProviderResource">
      <section class="app-page-hero provider-page-hero">
        <div>
          <div class="app-page-kicker">LLM Provider</div>
          <h1 class="app-page-title">{{ pageTitle }}</h1>
          <p class="app-page-subtitle">{{ pageSubtitle }}</p>
        </div>
        <q-btn color="primary" unelevated rounded icon="add" label="添加 Provider" @click="openCreate" />
      </section>

      <q-card flat bordered class="app-entity-glass-panel provider-card">
        <q-card-section class="provider-toolbar__inner">
          <q-input
            v-model="keyword"
            class="provider-toolbar__search provider-control"
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
            class="provider-toolbar__filter provider-control"
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

        <div v-if="loading" class="provider-list-shell">
          <q-card-section class="q-gutter-md">
            <q-skeleton v-for="item in 4" :key="item" type="rect" height="96px" />
          </q-card-section>
        </div>

        <div v-else-if="pagedProviderRows.length" class="provider-table">
          <ProviderModelListHeader :is-dark="isDark" />
          <div class="provider-table__body provider-list">
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
          </div>
        </div>

        <q-card-section v-else class="app-registry-empty empty-state">
          <q-icon name="manage_search" size="40px" color="grey-5" />
          <div class="text-subtitle1 q-mt-sm">暂无 Provider 模型</div>
          <div class="text-caption text-grey-7">添加 Provider 后，可为每个模型配置能力分类、密钥和性能指标。</div>
          <q-btn class="q-mt-md" color="primary" unelevated rounded icon="add" label="添加 Provider" @click="openCreate" />
        </q-card-section>

        <q-separator />
        <q-card-actions class="app-registry-pagination pagination-bar">
          <div class="text-caption text-grey-7">共 {{ filteredRows.length }} 条，每页 {{ rowsPerPage }} 条</div>
          <q-pagination v-model="page" :max="pageCount" direction-links boundary-links color="primary" />
        </q-card-actions>
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
      <q-card class="resource-dialog-card app-dialog-card app-dialog-card--xl provider-dialog">
        <q-card-section class="provider-dialog__head row items-start justify-between no-wrap">
          <div class="col min-width-0">
            <div class="provider-dialog__title">{{ dialogTitle }}</div>
            <div class="provider-dialog__subtitle">{{ dialogSubtitle }}</div>
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
                <p class="provider-step-hint">选择供应商预设并填写 API 密钥、模型 ID 与显示名称。</p>
                <div class="app-form-field-grid app-form-field-grid--2col">
          <q-select
            v-model="providerPresetKey"
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
          <q-select
            v-model="providerForm.model_api_id"
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
            label="名称 *"
            hint="小写字母、数字、连字符，例如 openrouter"
            :rules="[providerCodeRule]"
          />
          <q-input v-model="providerForm.provider_display_name" dense outlined label="显示名称" />
          <q-input v-model="providerForm.model_display_name" dense outlined label="模型展示名" />
          <q-input v-model="providerForm.api_base_url" dense outlined label="API 基础 URL" placeholder="https://..." />
          <q-toggle v-model="providerForm.enabled" label="已启用" />
          <q-select
            v-if="providerForm.provider_type === 'openai'"
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
          <div class="section-label app-grid-span-full">价格快照（micro USD / 1K tokens）</div>
          <q-banner v-if="showPricingWarning" dense rounded class="app-banner-warning app-grid-span-full q-mb-sm">
            {{ pricingWarningMessage() }}
          </q-banner>
          <q-input v-model.number="providerForm.input_price_micro_usd_per_1k" dense outlined type="number" min="0" label="输入价格" />
          <q-input v-model.number="providerForm.output_price_micro_usd_per_1k" dense outlined type="number" min="0" label="输出价格" />
          <q-input v-model.number="providerForm.cached_input_price_micro_usd_per_1k" dense outlined type="number" min="0" label="缓存输入价格" />
          <q-input v-model.number="providerForm.reasoning_price_micro_usd_per_1k" dense outlined type="number" min="0" label="推理 Token 价格" />
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
          <q-input v-model="form.key" dense outlined label="Key" />
          <q-input v-model="form.name" dense outlined label="Name" />
          <q-input v-model="form.description" class="app-grid-span-full" dense outlined autogrow type="textarea" label="Description" />
          <q-input v-model="form.provider" dense outlined label="Provider" />
          <q-input v-model="form.model" dense outlined label="Model" />
          <q-input v-model="form.parent_id" dense outlined label="Parent ID" />
          <q-input v-model="form.agent_id" dense outlined label="Agent ID" />
          <q-input v-model.number="form.sort_order" dense outlined type="number" label="Sort Order" />
          <q-toggle v-model="form.enabled" color="primary" label="Enabled" />
          <q-input v-model="form.config_json" class="app-grid-span-full" dense outlined autogrow type="textarea" label="Config JSON" />
          <q-input v-model="form.metadata_json" class="app-grid-span-full" dense outlined autogrow type="textarea" label="Metadata JSON" />
        </q-card-section>
        <q-separator v-if="isProviderResource" />
        <q-card-actions class="app-actions-bar provider-dialog-actions">
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
            <q-tooltip v-if="isProviderResource && !editingId && !canSubmitNewProviderModel">
              请先点击「检查」并通过远程验证后再创建
            </q-tooltip>
          </q-btn>
        </q-card-actions>
      </q-card>
    </q-dialog>

    <ProviderTrendDialog v-model="trendDialogOpen" :row="trendRow" />
    </div>
  </q-page>
</template>

<script setup lang="ts">
import ProviderHAConfig from "../components/platform/ProviderHAConfig.vue";
import ProviderModelListHeader from "../components/platform/ProviderModelListHeader.vue";
import ProviderModelRow from "../components/platform/ProviderModelRow.vue";
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
  providerTypeFilter,
  categoryOptions,
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
  providerModelOptions,
  dialogTitle,
  dialogSubtitle,
  pricingWarningMessage,
  openCreate,
  openEdit,
  saveRow,
  applyProviderPreset,
  setCustomModelValue,
  inspectCurrentProviderModel,
  toggleApiKeyVisibility,
  toggleSecretKeyVisibility,
  toggleEnabled,
  confirmRemoveRow,
  openTrend,
  providerCodeRule,
  getCategories,
  metadataLabel
} = useResourceManagerPage();
</script>
