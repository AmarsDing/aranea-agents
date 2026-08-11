<template>
  <div class="kb-panel-list">
    <template v-if="sections.some((s) => s.items.length)">
      <div v-for="s in sections" :key="s.type">
        <template v-if="s.items.length">
          <div class="kb-panel-group__head kb-panel-group__head--static">
            <q-icon :name="s.icon" size="14px" />
            <span>{{ s.label }}</span>
            <span class="kb-panel-group__count">{{ s.items.length }}</span>
          </div>
          <button
            v-for="(l, i) in s.items"
            :key="i"
            type="button"
            class="kb-panel-item"
            :title="l.target_rel_path"
            @click="l.target_doc_id && $emit('open-doc-id', l.target_doc_id)"
          >
            <span class="ellipsis">{{ l.target_source || l.target_rel_path }}</span>
          </button>
        </template>
      </div>
    </template>
    <div v-else class="kb-panel-empty">{{ t('knowledgePage.workbench.panels.noOutlinks') }}</div>
  </div>
</template>

<script setup lang="ts">
// 出链面板（SP2 §SP2-8）：显式/实体/语义三区分组。
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import type { KnowledgeLink } from '../../../features/knowledge/types';

const props = defineProps<{
  links: KnowledgeLink[];
}>();

defineEmits<{
  'open-doc-id': [docId: string];
}>();

const { t } = useI18n();

// ListDocumentLinks 双向返回（direction=out/in，入链 target 字段装对端）；
// 出链面板只显示出向，且按目标文档去重（同源多次提及只列一次，Obsidian 语义）。
function outOnly(list: KnowledgeLink[], linkType: string): KnowledgeLink[] {
  const seen = new Set<string>();
  const out: KnowledgeLink[] = [];
  for (const l of list) {
    if (l.link_type !== linkType || l.direction === 'in') continue;
    const key = l.target_doc_id || l.target_rel_path || l.target_source;
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(l);
  }
  return out;
}

const sections = computed(() => [
  {
    type: 'explicit',
    icon: 'link',
    label: t('knowledgePage.workbench.panels.linkTypeExplicit'),
    items: outOnly(props.links, 'explicit'),
  },
  {
    type: 'entity',
    icon: 'sell',
    label: t('knowledgePage.workbench.panels.linkTypeEntity'),
    items: outOnly(props.links, 'entity'),
  },
  {
    type: 'semantic',
    icon: 'psychology',
    label: t('knowledgePage.workbench.panels.linkTypeSemantic'),
    items: outOnly(props.links, 'semantic'),
  },
]);
</script>

<style lang="sass" scoped>
@use './panel-shared'
</style>
