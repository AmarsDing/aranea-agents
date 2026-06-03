import { computed, ref, watch, type Ref } from 'vue';
import type { PlatformResourceTreeNode } from './types';
import {
  collectExpandedIdsForFilter,
  filterTaxonomyTree,
  findTaxonomyPath,
  formatTaxonomyPath,
  levelLabel,
  toQTreeNodes,
  type TaxonomyLevel,
  type TaxonomyQTreeNode,
} from './taxonomyTreeUtils';

type UseTaxonomyTreeFieldOptions = {
  modelValue: Ref<string | null>;
  tree: Ref<PlatformResourceTreeNode[]>;
  selectableLevel: Ref<TaxonomyLevel | 'any'>;
  onUpdate: (value: string | null) => void;
};

/** 业务分类树形下拉：菜单状态、搜索过滤、展开与选中逻辑 */
export function useTaxonomyTreeField(opts: UseTaxonomyTreeFieldOptions) {
  const menuOpen = ref(false);
  const menuKeyword = ref('');
  const expanded = ref<string[]>([]);

  const industryTree = computed(() => opts.tree.value.filter((node) => node.level === 'industry'));

  const filteredTree = computed(() => filterTaxonomyTree(industryTree.value, menuKeyword.value));

  const menuNodes = computed(() =>
    toQTreeNodes(filteredTree.value, {
      selectableLevel: opts.selectableLevel.value,
      enabledOnly: true,
    }),
  );

  const displayLabel = computed(() => {
    if (!opts.modelValue.value) return '';
    const path = findTaxonomyPath(opts.tree.value, opts.modelValue.value);
    return path.length ? formatTaxonomyPath(path) : '';
  });

  watch([filteredTree, menuKeyword], () => {
    const fromSearch = collectExpandedIdsForFilter(filteredTree.value, menuKeyword.value);
    expanded.value = fromSearch.length ? fromSearch : filteredTree.value.map((node) => node.id);
  });

  watch(menuOpen, (open) => {
    if (!open) {
      menuKeyword.value = '';
      return;
    }
    expanded.value = filteredTree.value.map((node) => node.id);
  });

  function onPick(node: TaxonomyQTreeNode) {
    if (opts.selectableLevel.value !== 'any' && !node.selectable) return;
    opts.onUpdate(node.id);
    menuOpen.value = false;
  }

  function clearSelection() {
    opts.onUpdate(null);
  }

  function onExpandedUpdate(value: readonly string[]) {
    expanded.value = [...value];
  }

  return {
    menuOpen,
    menuKeyword,
    expanded,
    menuNodes,
    displayLabel,
    levelLabel,
    onPick,
    clearSelection,
    onExpandedUpdate,
  };
}

// TECH-DEBT: legacy alias, remove after all consumers migrated
export type UseCategoryTreeFieldOptions = UseTaxonomyTreeFieldOptions;
export const useCategoryTreeField = useTaxonomyTreeField;
