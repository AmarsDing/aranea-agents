<template>
  <q-dialog v-model="open" persistent>
    <q-card class="taxonomy-dialog app-dialog-card app-dialog-card--md app-glass-dialog">
      <q-card-section class="app-glass-dialog__head taxonomy-dialog__head row items-start justify-between no-wrap">
        <div class="min-width-0">
          <div class="app-glass-dialog__title taxonomy-dialog__title">
            {{ editingId ? '编辑组织节点' : `新增${levelLabel(form.level)}` }}
          </div>
          <div class="app-glass-dialog__subtitle taxonomy-dialog__subtitle">固定三层结构：公司 → 部门 → 职位</div>
        </div>
        <q-btn v-close-popup flat round dense icon="close" />
      </q-card-section>
      <q-separator />
      <q-card-section class="app-glass-dialog__body taxonomy-dialog__body">
        <div class="app-form-field-grid app-form-field-grid--2col taxonomy-dialog__form">
          <q-input
            v-model.trim="form.name"
            class="taxonomy-control"
            dense
            outlined
            label="名称 *"
            maxlength="120"
            counter
          />
          <q-input
            v-model.number="form.sort_order"
            class="taxonomy-control"
            dense
            outlined
            type="number"
            min="0"
            label="排序"
          />
        </div>

        <div v-if="editingId" class="taxonomy-dialog__key-hint q-mt-sm">
          <span class="text-caption text-grey-7">Key: </span>
          <code class="app-mono text-caption">{{ form.key }}</code>
        </div>

        <div class="taxonomy-dialog__meta">
          <div class="taxonomy-meta-item">
            <span class="taxonomy-meta-item__label">层级</span>
            <span class="taxonomy-meta-item__value">{{ levelLabel(form.level) }}</span>
          </div>
          <div class="taxonomy-meta-item">
            <span class="taxonomy-meta-item__label">父级</span>
            <span class="taxonomy-meta-item__value ellipsis">{{ parentName }}</span>
          </div>
        </div>

        <div class="taxonomy-dialog__desc">
          <div class="taxonomy-dialog__desc-label">{{ currentDescLabel }}</div>
          <q-input
            v-model="form.description"
            class="taxonomy-control taxonomy-dialog__desc-input"
            dense
            outlined
            type="textarea"
            :rows="4"
            :placeholder="currentDescPlaceholder"
          />
          <div class="row justify-end q-mt-xs">
            <AiRefineButton
              :scope="levelScope(currentLevelNum)"
              :resource-id="editingId || undefined"
              :text="form.description ?? ''"
              :refine-fn="refinePromptField"
              flat
              size="sm"
              label="AI 优化描述"
              @apply="onApplyRefinedDescription"
              @error="(msg: string) => $emit('refine-error', msg)"
            />
          </div>
        </div>

        <div class="taxonomy-dialog__enabled row items-center q-mt-md">
          <q-toggle v-model="form.enabled" color="primary" label="启用" />
          <span class="taxonomy-dialog__enabled-hint text-caption text-grey-7 q-ml-sm">
            停用后 Agent / Team 筛选与分组仍会保留数据，但默认不再出现在选择器中。
          </span>
        </div>
      </q-card-section>
      <q-card-actions align="right" class="app-actions-bar app-glass-dialog__actions taxonomy-dialog__actions">
        <q-btn v-close-popup flat rounded no-caps label="取消" />
        <q-btn color="primary" rounded unelevated no-caps label="保存" :loading="saving" @click="$emit('submit')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import AiRefineButton from './AIRefineButton.vue';
import { refinePromptField } from '../../features/agents/aiRefine';
import { levelLabel } from '../../features/platform/taxonomyTreeUtils';
import {
  descriptionLabel,
  descriptionPlaceholder,
  levelScope,
  parseLevelNumber,
} from '../../features/platform/taxonomyLabels';
import type { PlatformResourceInput } from '../../features/platform/types';
import type { TaxonomyLevel } from '../../features/platform/taxonomyTreeUtils';

const props = defineProps<{
  modelValue: boolean;
  editingId: string;
  form: PlatformResourceInput & { level: TaxonomyLevel };
  parentName: string;
  saving: boolean;
}>();

const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  submit: [];
  'refine-error': [message: string];
}>();

const open = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
});

const currentLevelNum = computed(() => parseLevelNumber(props.form.level));
const currentDescLabel = computed(() => descriptionLabel(currentLevelNum.value));
const currentDescPlaceholder = computed(() => descriptionPlaceholder(currentLevelNum.value));

function onApplyRefinedDescription(v: string) {
  props.form.description = v;
}
</script>
