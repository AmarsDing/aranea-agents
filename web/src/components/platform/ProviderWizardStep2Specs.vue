<template>
  <div class="provider-wizard-panel">
    <h3 class="provider-step-heading">模型规格</h3>
    <p class="provider-step-hint">能力分类、上下文窗口、评级与价格快照。</p>
    <div class="app-form-field-grid app-form-field-grid--2col">
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
      <div class="app-grid-span-full">
        <div class="row items-center justify-between">
          <span class="text-caption text-grey-7">模型评级（1-100，越高表示认为模型越强）</span>
          <span class="text-caption text-weight-medium">{{ providerForm.model_rating ?? 0 }}</span>
        </div>
        <q-slider v-model="providerForm.model_rating" class="provider-rating-slider" :min="1" :max="100" />
      </div>
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
      <div v-else class="text-caption text-grey-7 app-grid-span-full q-mb-sm">
        从目录选择模型后自动填充；自定义模型可留空。
      </div>

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
      <q-input
        v-model.number="providerForm.input_price_usd_per_1m"
        dense
        outlined
        type="number"
        min="0"
        step="0.0001"
        label="输入价格"
      />
      <q-input
        v-model.number="providerForm.output_price_usd_per_1m"
        dense
        outlined
        type="number"
        min="0"
        step="0.0001"
        label="输出价格"
      />
      <q-input
        v-model.number="providerForm.cache_read_usd_per_1m"
        dense
        outlined
        type="number"
        min="0"
        step="0.0001"
        label="缓存读取价格"
      />
      <q-input
        v-model.number="providerForm.cache_write_usd_per_1m"
        dense
        outlined
        type="number"
        min="0"
        step="0.0001"
        label="缓存写入价格"
      />
      <q-input
        v-model.number="providerForm.reasoning_price_usd_per_1m"
        dense
        outlined
        type="number"
        min="0"
        step="0.0001"
        label="推理 Token 价格"
      />
      <q-input
        v-model.number="providerForm.embedding_price_usd_per_1m"
        dense
        outlined
        type="number"
        min="0"
        step="0.0001"
        label="Embedding 价格"
      />
      <q-input v-model.number="providerForm.sort_order" dense outlined type="number" label="排序" />
      <q-input
        v-model="providerForm.description"
        class="app-grid-span-full"
        dense
        outlined
        autogrow
        type="textarea"
        label="描述"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
const providerForm = defineModel<ProviderForm>('providerForm', { required: true });
import type { ProviderForm, ModelCategory } from '../../features/platform/types';

defineProps<{
  providerAddMode: 'catalog' | 'custom';
  categoryOptions: ModelCategory[];
  catalogPricingMissing: boolean;
  showPricingWarning: boolean;
  pricingWarningMessage: () => string;
}>();
</script>
