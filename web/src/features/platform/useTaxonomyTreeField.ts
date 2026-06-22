import { computed, ref, watch } from 'vue';
import type { PlatformResourceTreeNode } from './types';
import {
  type TaxonomyLevel,
  type TaxonomyQTreeNode,
  collectDefaultExpandedIds,
  collectExpandedIdsForFilter,
  filterTaxonomyTree,
  findTaxonomyNode,
  levelLabel,
  toQTreeNodes,
} from './taxonomyTreeUtils';

export function useTaxonomyTreeField(opts: {
  modelValue: { value: string | null };
  tree: { value: PlatformResourceTreeNode[] };
  selectableLevel: { value: TaxonomyLevel | 'any' };
  onUpdate: (value: string | null) => void;
}) {
  const menuOpen = ref(false);
  const menuKeyword = ref('');
  const expanded = ref<string[]>([]);

  // Initialize expanded with default IDs when tree loads
  watch(
    () => opts.tree.value,
    (tree) => {
      if (Array.isArray(tree) && tree.length > 0 && expanded.value.length === 0) {
        expanded.value = Array.from(collectDefaultExpandedIds(tree));
      }
    },
    { immediate: true },
  );

  const menuNodes = computed<TaxonomyQTreeNode[]>(() => {
    const keyword = menuKeyword.value;
    const tree = opts.tree.value;
    if (!Array.isArray(tree)) return [];
    const filtered = filterTaxonomyTree(tree, keyword);
    return toQTreeNodes(filtered, {
      selectableLevel: opts.selectableLevel.value,
    });
  });

  const displayLabel = computed(() => {
    const id = opts.modelValue.value;
    if (!id) return '';
    const tree = opts.tree.value;
    if (!Array.isArray(tree)) return '';
    const node = findTaxonomyNode(tree, id);
    return node?.name || '';
  });

  function onPick(node: TaxonomyQTreeNode) {
    const canSelect = opts.selectableLevel.value === 'any' || node.level === opts.selectableLevel.value;
    if (!canSelect) return;
    opts.onUpdate(node.id);
    menuOpen.value = false;
    menuKeyword.value = '';
  }

  function clearSelection() {
    opts.onUpdate(null);
  }

  function onExpandedUpdate(ids: readonly string[]) {
    expanded.value = [...ids];
  }

  // Expand matching nodes when keyword changes
  watch(menuKeyword, (kw) => {
    if (kw.trim() && Array.isArray(opts.tree.value)) {
      const filterIds = collectExpandedIdsForFilter(opts.tree.value, kw);
      const existing = new Set(expanded.value);
      for (const id of filterIds) existing.add(id);
      expanded.value = Array.from(existing);
    }
  });

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
