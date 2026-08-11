<template>
  <q-dialog :model-value="open" persistent @update:model-value="$emit('update:open', !!$event)">
    <q-card class="memory-fact-edit-dialog">
      <q-card-section>
        <div class="text-h6">
          {{ mode === 'refine' ? t('memory.factEdit.refineTitle') : t('memory.factEdit.createTitle') }}
        </div>
        <div class="text-caption text-grey-7">
          {{ mode === 'refine' ? t('memory.factEdit.refineCaption') : t('memory.factEdit.createCaption') }}
        </div>
      </q-card-section>
      <q-card-section class="q-pt-none column q-gutter-md">
        <q-input
          v-model="statement"
          outlined
          autogrow
          :label="t('memory.factEdit.statementLabel')"
          :hint="t('memory.factEdit.statementHint')"
          :rules="[(v: string) => !!v.trim() || t('memory.factEdit.statementRequired')]"
        />
        <q-input
          v-model="detailsMarkdown"
          outlined
          autogrow
          type="textarea"
          :label="t('memory.factEdit.detailsLabel')"
          :hint="t('memory.factEdit.detailsHint')"
        />
        <div class="row q-gutter-md">
          <q-select
            v-model="factKind"
            class="col"
            outlined
            dense
            emit-value
            map-options
            options-dense
            :label="t('memory.factEdit.kindLabel')"
            :options="kindOptions"
          />
          <q-select
            v-model="tags"
            class="col"
            outlined
            dense
            multiple
            use-chips
            use-input
            new-value-mode="add-unique"
            hide-dropdown-icon
            :label="t('memory.factEdit.tagsLabel')"
            :hint="t('memory.factEdit.tagsHint')"
          />
        </div>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn v-close-popup flat no-caps :label="t('memory.factEdit.cancel')" :disable="saving" />
        <q-btn
          color="primary"
          unelevated
          no-caps
          :label="mode === 'refine' ? t('memory.factEdit.submitRefine') : t('memory.factEdit.submitCreate')"
          :disable="!statement.trim()"
          :loading="saving"
          @click="onSubmit"
        />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import type { MemoryFact } from '../../features/memory/types';

// 事实编辑/新建对话框：纯展示组件，submit 由 Page composable 调 Store action。
const props = defineProps<{
  open: boolean;
  mode: 'refine' | 'create';
  fact: MemoryFact | null;
  saving: boolean;
}>();

const emit = defineEmits<{
  'update:open': [value: boolean];
  submit: [payload: { statement: string; details_markdown: string; fact_kind: string; tags: string[] }];
}>();

const { t } = useI18n();

const statement = ref('');
const detailsMarkdown = ref('');
const factKind = ref('fact');
const tags = ref<string[]>([]);

// 与后端持久化白名单对齐（biz/memory_write_pipeline.go factKindWhitelist）
// + 通用 fact；rule/experience 等遗留值仅作展示回退，不再提供新增入口。
const kindOptions = computed(() =>
  ['fact', 'preference', 'profile', 'goal', 'constraint', 'decision', 'relationship'].map((value) => ({
    label: t(`memory.factEdit.kind.${value}`),
    value,
  })),
);

watch(
  () => props.open,
  (open) => {
    if (!open) return;
    statement.value = props.mode === 'refine' ? (props.fact?.statement ?? '') : '';
    detailsMarkdown.value = props.mode === 'refine' ? (props.fact?.details_markdown ?? '') : '';
    factKind.value = props.mode === 'refine' ? props.fact?.fact_kind || 'fact' : 'fact';
    tags.value = props.mode === 'refine' ? parseTags(props.fact?.tags_json) : [];
  },
);

function parseTags(raw?: string): string[] {
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw) as unknown;
    return Array.isArray(parsed) ? parsed.filter((x): x is string => typeof x === 'string') : [];
  } catch {
    return [];
  }
}

function onSubmit() {
  emit('submit', {
    statement: statement.value.trim(),
    details_markdown: detailsMarkdown.value,
    fact_kind: factKind.value,
    tags: tags.value,
  });
}
</script>

<style lang="scss" scoped>
.memory-fact-edit-dialog {
  min-width: 480px;
  max-width: 600px;
}
</style>
