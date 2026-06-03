import type { PlatformResourceTreeNode } from './types';

export type TaxonomyLevel = 'industry' | 'department' | 'position';

export type TaxonomyQTreeNode = {
  id: string;
  label: string;
  icon: string;
  caption?: string;
  level: TaxonomyLevel;
  selectable: boolean;
  node: PlatformResourceTreeNode;
  children?: TaxonomyQTreeNode[];
};

const LEVEL_ICONS: Record<TaxonomyLevel, string> = {
  industry: 'domain',
  department: 'lan',
  position: 'badge',
};

const LEVEL_LABELS: Record<TaxonomyLevel, string> = {
  industry: '行业',
  department: '部门',
  position: '职位',
};

export function levelLabel(level: string) {
  return LEVEL_LABELS[level as TaxonomyLevel] ?? '分类';
}

export function parseIsSystem(node: PlatformResourceTreeNode) {
  if (node.is_system) return true;
  try {
    return Boolean(JSON.parse(node.metadata_json || '{}').is_system);
  } catch {
    return false;
  }
}

export function trimmedDesc(raw?: string | null) {
  return (raw ?? '').trim();
}

export function flattenTaxonomyTree(nodes: PlatformResourceTreeNode[]): PlatformResourceTreeNode[] {
  return nodes.flatMap((node) => [node, ...flattenTaxonomyTree(node.children ?? [])]);
}

export function findTaxonomyNode(tree: PlatformResourceTreeNode[], id: string): PlatformResourceTreeNode | null {
  if (!id) return null;
  for (const node of tree) {
    if (node.id === id) return node;
    const found = findTaxonomyNode(node.children ?? [], id);
    if (found) return found;
  }
  return null;
}

export function findTaxonomyPath(tree: PlatformResourceTreeNode[], id: string): PlatformResourceTreeNode[] {
  if (!id) return [];
  for (const node of tree) {
    if (node.id === id) return [node];
    const childPath = findTaxonomyPath(node.children ?? [], id);
    if (childPath.length) return [node, ...childPath];
  }
  return [];
}

export function formatTaxonomyPath(path: PlatformResourceTreeNode[]) {
  return path.map((node) => node.name).join(' / ');
}

export function collectPositionIds(node: PlatformResourceTreeNode): Set<string> {
  const ids = new Set<string>();
  if (node.level === 'position') {
    ids.add(node.id);
    return ids;
  }
  for (const child of node.children ?? []) {
    for (const id of collectPositionIds(child)) {
      ids.add(id);
    }
  }
  return ids;
}

export function flattenTaxonomyPositions(
  nodes: PlatformResourceTreeNode[],
  prefix = '',
): Array<{ label: string; value: string }> {
  return nodes.flatMap((node) => {
    const label = prefix ? `${prefix} / ${node.name}` : node.name;
    if (node.level === 'position') {
      return [{ label, value: node.id }];
    }
    return flattenTaxonomyPositions(node.children ?? [], label);
  });
}

function nodeMatchesKeyword(node: PlatformResourceTreeNode, keyword: string) {
  const q = keyword.trim().toLowerCase();
  if (!q) return false;
  return [node.name, node.description, node.key].some((value) => (value || '').toLowerCase().includes(q));
}

export { nodeMatchesKeyword };

export function departmentPositions(department: PlatformResourceTreeNode) {
  return (department.children ?? []).filter((node) => node.level === 'position');
}

export function filterTaxonomyTree(
  nodes: PlatformResourceTreeNode[],
  keyword: string,
  onlyCustom = false,
): PlatformResourceTreeNode[] {
  const q = keyword.trim().toLowerCase();
  return nodes
    .map((node) => ({
      ...node,
      children: filterTaxonomyTree(node.children ?? [], keyword, onlyCustom),
    }))
    .filter((node) => {
      const matchKeyword = !q || nodeMatchesKeyword(node, q) || (node.children?.length ?? 0) > 0;
      const matchCustom = !onlyCustom || !parseIsSystem(node) || (node.children?.length ?? 0) > 0;
      return matchKeyword && matchCustom;
    });
}

export function collectExpandedIdsForFilter(nodes: PlatformResourceTreeNode[], keyword: string): string[] {
  const q = keyword.trim().toLowerCase();
  if (!q) return [];
  const ids = new Set<string>();

  function walk(node: PlatformResourceTreeNode, ancestors: string[]): boolean {
    const selfMatch = nodeMatchesKeyword(node, q);
    const childMatch = (node.children ?? []).some((child) => walk(child, [...ancestors, node.id]));
    if (selfMatch || childMatch) {
      ids.add(node.id);
      for (const id of ancestors) ids.add(id);
      return true;
    }
    return false;
  }

  for (const node of nodes) walk(node, []);
  return Array.from(ids);
}

export function taxonomyTreeStats(tree: PlatformResourceTreeNode[]) {
  const rows = flattenTaxonomyTree(tree);
  return {
    industries: rows.filter((row) => row.level === 'industry').length,
    departments: rows.filter((row) => row.level === 'department').length,
    positions: rows.filter((row) => row.level === 'position').length,
  };
}

export function toQTreeNodes(
  nodes: PlatformResourceTreeNode[],
  opts?: { selectableLevel?: TaxonomyLevel | 'any'; enabledOnly?: boolean },
): TaxonomyQTreeNode[] {
  const selectableLevel = opts?.selectableLevel ?? 'position';
  const enabledOnly = opts?.enabledOnly ?? false;

  return nodes
    .filter((node) => !enabledOnly || node.enabled)
    .map((node) => {
      const level = node.level as TaxonomyLevel;
      const children = toQTreeNodes(node.children ?? [], opts);
      const selectable =
        selectableLevel === 'any'
          ? !enabledOnly || node.enabled
          : level === selectableLevel && (!enabledOnly || node.enabled);
      return {
        id: node.id,
        label: node.name,
        icon: LEVEL_ICONS[level] ?? 'folder',
        caption: trimmedDesc(node.description) || undefined,
        level,
        selectable,
        node,
        children: children.length ? children : undefined,
      };
    });
}

export function inferCascadeFromPosition(
  tree: PlatformResourceTreeNode[],
  positionId: string,
): { industryId: string | null; departmentId: string | null } {
  const path = findTaxonomyPath(tree, positionId);
  return {
    industryId: path.find((node) => node.level === 'industry')?.id ?? null,
    departmentId: path.find((node) => node.level === 'department')?.id ?? null,
  };
}

export function patchTaxonomyTreeNode(
  tree: PlatformResourceTreeNode[],
  id: string,
  patch: Partial<PlatformResourceTreeNode>,
): PlatformResourceTreeNode[] {
  const [next, changed] = patchTaxonomyTreeNodeInner(tree, id, patch);
  return changed ? next : tree;
}

function patchTaxonomyTreeNodeInner(
  tree: PlatformResourceTreeNode[],
  id: string,
  patch: Partial<PlatformResourceTreeNode>,
): [PlatformResourceTreeNode[], boolean] {
  let changed = false;
  const next = tree.map((node) => {
    if (node.id === id) {
      changed = true;
      return { ...node, ...patch };
    }
    if (!node.children?.length) return node;
    const [children, childChanged] = patchTaxonomyTreeNodeInner(node.children, id, patch);
    if (childChanged) {
      changed = true;
      return { ...node, children };
    }
    return node;
  });
  return [next, changed];
}

export function collectDefaultExpandedIds(tree: PlatformResourceTreeNode[]) {
  const ids = new Set<string>();
  for (const industry of tree) {
    ids.add(industry.id);
    for (const department of industry.children ?? []) {
      if (department.level === 'department') ids.add(department.id);
    }
  }
  return ids;
}

// --- Legacy aliases (categoryTreeUtils compatibility) ---
// TECH-DEBT: remove after all consumers migrated to Taxonomy* names
export type CategoryLevel = TaxonomyLevel;
export type CategoryQTreeNode = TaxonomyQTreeNode;
export const flattenCategoryTree = flattenTaxonomyTree;
export const findCategoryNode = findTaxonomyNode;
export const findCategoryPath = findTaxonomyPath;
export const formatCategoryPath = formatTaxonomyPath;
export const flattenCategoryPositions = flattenTaxonomyPositions;
export const filterCategoryTree = filterTaxonomyTree;
export const categoryTreeStats = taxonomyTreeStats;
export const patchCategoryTreeNode = patchTaxonomyTreeNode;
