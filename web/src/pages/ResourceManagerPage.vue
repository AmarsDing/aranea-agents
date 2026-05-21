<template>
  <q-page :class="['q-pa-md resource-manager-page', { 'is-dark': isDark }]">
    <q-card v-if="isProviderResource" flat bordered class="provider-card">
      <q-card-section class="provider-header row items-center q-col-gutter-md">
        <div class="col-12 col-md">
          <div class="text-h5 text-weight-bold">Provider</div>
          <div class="text-body2 text-grey-7">管理 LLM Provider</div>
        </div>
        <div class="col-12 col-md-3">
          <q-input v-model="keyword" dense outlined clearable debounce="200" placeholder="搜索Provider...">
            <template #prepend><q-icon name="search" /></template>
          </q-input>
        </div>
        <div class="col-12 col-md-3">
          <q-select
            v-model="providerTypeFilter"
            dense
            outlined
            multiple
            use-chips
            emit-value
            map-options
            label="Provider 类型"
            :options="providerTypeFilterOptions"
          />
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
        <q-card-section v-if="isProviderResource">
          <q-stepper v-model="providerStep" color="primary" animated flat bordered>
            <q-step :name="1" title="连接与身份" icon="link" :done="providerStep > 1">
              <div class="row q-col-gutter-md q-pt-sm">
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
          <q-select
            v-if="providerForm.provider_type === 'openai'"
            v-model="providerForm.variant"
            class="col-12 col-md-6"
            dense
            outlined
            emit-value
            map-options
            label="Variant"
            :options="variantOptions"
          />
          <template v-if="currentAuthType === 'secret_id_key'">
            <q-input v-model="providerForm.secret_id" class="col-12 col-md-6" dense outlined label="Secret ID" />
            <q-input
              v-model="providerForm.secret_key"
              class="col-12 col-md-6"
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
            class="col-12 col-md-6"
            dense
            outlined
            label="AWS Region"
            placeholder="us-east-1"
          />
              </div>
            </q-step>

            <q-step :name="2" title="模型规格" icon="tune" :done="providerStep > 2">
              <div class="row q-col-gutter-md q-pt-sm">
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
          <q-banner v-if="showPricingWarning" dense rounded class="bg-orange-1 text-orange-10 col-12 q-mb-sm">
            {{ pricingWarningMessage() }}
          </q-banner>
          <q-input v-model.number="providerForm.input_price_micro_usd_per_1k" class="col-12 col-md-3" dense outlined type="number" min="0" label="输入价格" />
          <q-input v-model.number="providerForm.output_price_micro_usd_per_1k" class="col-12 col-md-3" dense outlined type="number" min="0" label="输出价格" />
          <q-input v-model.number="providerForm.cached_input_price_micro_usd_per_1k" class="col-12 col-md-3" dense outlined type="number" min="0" label="缓存输入价格" />
          <q-input v-model.number="providerForm.reasoning_price_micro_usd_per_1k" class="col-12 col-md-3" dense outlined type="number" min="0" label="推理 Token 价格" />
          <q-input v-model.number="providerForm.sort_order" class="col-12 col-md-6" dense outlined type="number" label="排序" />
          <q-input v-model="providerForm.description" class="col-12" dense outlined autogrow type="textarea" label="描述" />
              </div>
            </q-step>

            <q-step :name="3" title="高可用" icon="swap_horiz" :done="providerStep > 3">
              <ProviderHAConfig
                v-model="providerHAForm"
                class="q-pt-sm"
                :provider-type-options="providerTypeFilterOptions"
              />
            </q-step>

            <q-step :name="4" title="高级选项" icon="settings">
              <div class="row q-col-gutter-md q-pt-sm">
                <q-toggle v-model="providerForm.enable_token_tailoring" class="col-12 col-md-6" label="Token Tailoring" />
                <q-toggle
                  v-if="providerForm.provider_type === 'openai'"
                  v-model="providerForm.optimize_for_cache"
                  class="col-12 col-md-6"
                  label="Prompt Cache 优化"
                />
                <q-toggle
                  v-if="providerForm.provider_type === 'openai' && providerForm.variant === 'deepseek'"
                  v-model="providerForm.reasoning_backfill"
                  class="col-12 col-md-6"
                  label="Reasoning 回填"
                />
                <q-toggle
                  v-if="['openai', 'anthropic'].includes(providerForm.provider_type)"
                  v-model="providerForm.show_tool_call_delta"
                  class="col-12 col-md-6"
                  label="Tool Call Delta"
                />
                <q-input
                  v-model.number="providerForm.rate_limit_rpm"
                  class="col-12 col-md-6"
                  dense
                  outlined
                  type="number"
                  min="0"
                  label="速率限制 (RPM)"
                />
                <q-input
                  v-if="providerForm.provider_type === 'ollama'"
                  v-model.number="providerForm.keep_alive_minutes"
                  class="col-12 col-md-6"
                  dense
                  outlined
                  type="number"
                  min="0"
                  label="Keep Alive (分钟)"
                />
              </div>
            </q-step>

            <template #navigation>
              <q-stepper-navigation>
                <q-btn v-if="providerStep > 1" flat label="上一步" @click="providerStep -= 1" />
                <q-btn v-if="providerStep < 4" color="primary" label="下一步" @click="providerStep += 1" />
              </q-stepper-navigation>
            </template>
          </q-stepper>
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
import { useResourceManagerPage } from "../features/platform/useResourceManagerPage";

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
