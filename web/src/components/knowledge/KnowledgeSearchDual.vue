<template>
  <div class="knowledge-search-dual">
    <q-input
      :model-value="query"
      dense
      outlined
      rounded
      clearable
      :placeholder="t('knowledgePage.searchDualPlaceholder')"
      @update:model-value="onQueryInput"
      @keyup.enter="$emit('search')"
    >
      <template #prepend>
        <!-- G3-F2 搜索范围选择器：弹出迷你目录树（仅目录单选），选中后即时区前端过滤 + 语义区走 B7 -->
        <knowledge-scope-picker
          :scope-prefix="scopePrefix"
          :scope-nodes="scopeNodes"
          @update:scope-prefix="$emit('update:scope-prefix', $event)"
          @scope-lazy-load="$emit('scope-lazy-load', $event)"
        />
        <q-icon name="search" />
      </template>
      <template #append>
        <!-- P3-3 意图分流徽标：规则与后端 internal/knowledge/search_intent.go 共享定义 -->
        <q-chip v-if="query.trim()" dense size="sm" :color="intentColor" text-color="white">{{ intentLabel }}</q-chip>
      </template>
    </q-input>

    <q-card v-if="query.trim()" flat bordered class="knowledge-search-dual__panel">
      <!-- 即时区：纯前端毫秒过滤（fzf 式内存索引） -->
      <template v-if="showInstant">
        <div class="knowledge-search-dual__zone-header">
          <q-icon name="bolt" size="14px" />
          {{ t('knowledgePage.searchZoneInstant') }}
        </div>
        <q-list v-if="instantResults.length" dense>
          <q-item v-for="d in instantResults" :key="d.id" clickable @click="$emit('select-instant', d)">
            <q-item-section avatar><q-icon name="insert_drive_file" size="18px" /></q-item-section>
            <q-item-section>
              <q-item-label lines="1">{{ d.source }}</q-item-label>
              <q-item-label caption lines="1">{{ d.rel_path || d.summary }}</q-item-label>
            </q-item-section>
          </q-item>
        </q-list>
        <div v-else class="knowledge-search-dual__empty">{{ t('knowledgePage.searchInstantEmpty') }}</div>
      </template>

      <q-separator v-if="showInstant && showSemantic" />

      <!-- 语义区：回车走后端 Search（亚秒） -->
      <template v-if="showSemantic">
        <div class="knowledge-search-dual__zone-header">
          <q-icon name="psychology" size="14px" />
          {{ t('knowledgePage.searchZoneSemantic') }}
        </div>
        <q-linear-progress v-if="semanticLoading" indeterminate color="primary" />
        <!-- F4：错误内联展示（如 embedder 未配置），不弹红 toast -->
        <div v-else-if="semanticError" class="knowledge-search-dual__empty knowledge-search-dual__empty--error">
          <q-icon name="cloud_off" size="14px" class="q-mr-xs" />{{ semanticError }}
        </div>
        <template v-else-if="semanticRan">
          <q-list v-if="semanticResults.length" dense>
            <q-item v-for="c in semanticResults" :key="c.id" clickable @click="$emit('select-semantic', c)">
              <q-item-section>
                <q-item-label lines="1">
                  {{ docSourceMap[c.doc_id] || c.doc_id }}
                  <q-chip dense size="sm" outline class="q-ml-xs">{{ c.score.toFixed(3) }}</q-chip>
                </q-item-label>
                <q-item-label caption lines="2">{{ c.content }}</q-item-label>
              </q-item-section>
            </q-item>
          </q-list>
          <div v-else class="knowledge-search-dual__empty">{{ t('knowledgePage.searchSemanticEmpty') }}</div>
        </template>
        <div v-else class="knowledge-search-dual__hint">{{ t('knowledgePage.searchSemanticHint') }}</div>
      </template>
    </q-card>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { SearchIntent } from '../../features/knowledge/searchIntent';
import type { KnowledgeChunk, KnowledgeDocument } from '../../features/knowledge/types';
import type { VaultLazyLoadPayload, VaultQTreeNode } from '../../features/knowledge/useVaultExplorer';
import KnowledgeScopePicker from './KnowledgeScopePicker.vue';

const props = defineProps<{
  query: string;
  intent: SearchIntent;
  instantResults: KnowledgeDocument[];
  semanticResults: KnowledgeChunk[];
  semanticLoading: boolean;
  semanticRan: boolean;
  /** F4：语义区错误文案（空 = 无错误），内联展示。 */
  semanticError: string;
  showInstant: boolean;
  showSemantic: boolean;
  docSourceMap: Record<string, string>;
  /** G3-F2：搜索范围（vault 相对目录前缀；'' = 全库）。 */
  scopePrefix: string;
  /** G3-F2：范围迷你树根节点（当前 vault；目录经 lazy-load 仅目录）。 */
  scopeNodes: VaultQTreeNode[];
}>();

const emit = defineEmits<{
  'update:query': [value: string];
  search: [];
  'select-instant': [doc: KnowledgeDocument];
  'select-semantic': [chunk: KnowledgeChunk];
  /** G3-F2：范围变更（vault 根 = '' 全库；× 清除 = ''）。 */
  'update:scope-prefix': [prefix: string];
  'scope-lazy-load': [payload: VaultLazyLoadPayload];
}>();

const { t } = useI18n();

const intentColor = computed(() => {
  if (props.intent === 'instant') return 'primary';
  if (props.intent === 'semantic') return 'deep-purple';
  return 'grey';
});

const intentLabel = computed(() => {
  if (props.intent === 'instant') return t('knowledgePage.searchZoneInstant');
  if (props.intent === 'semantic') return t('knowledgePage.searchZoneSemantic');
  return t('knowledgePage.searchIntentAuto');
});

function onQueryInput(value: string | number | null) {
  emit('update:query', value == null ? '' : String(value));
}
</script>

<style lang="scss" scoped>
.knowledge-search-dual {
  position: relative;

  &__panel {
    position: absolute;
    top: calc(100% + 4px);
    left: 0;
    right: 0;
    z-index: 20;
    max-height: 420px;
    overflow-y: auto;
    border-radius: 10px;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.12);
  }

  &__zone-header {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 8px 12px 4px;
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.04em;
    color: var(--color-text-secondary);
    text-transform: uppercase;
  }

  &__empty,
  &__hint {
    padding: 8px 12px 12px;
    font-size: 12px;
    color: var(--color-text-secondary);
  }

  &__empty--error {
    display: flex;
    align-items: center;
    color: var(--q-negative);
  }
}
</style>
