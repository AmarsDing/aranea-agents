import type { L1Field } from './types';

export type L1FieldTreeNode = {
  id: string;
  label: string;
  path: string;
  preview?: string;
  tokens?: number;
  source?: string;
  revision?: number;
  pinned?: boolean;
  children?: L1FieldTreeNode[];
};

/** Groups L1 fields by dotted `field_path` into a QTree-ready forest. */
export function buildL1FieldTree(fields: L1Field[] | null | undefined): L1FieldTreeNode[] {
  const roots: L1FieldTreeNode[] = [];
  const byPath = new Map<string, L1FieldTreeNode>();

  const sorted = [...(fields ?? [])].sort((a, b) => a.field_path.localeCompare(b.field_path));
  for (const field of sorted) {
    const path = (field.field_path || '').trim();
    if (!path) continue;
    const parts = path.split('.').filter(Boolean);
    let prefix = '';
    for (let i = 0; i < parts.length; i++) {
      const label = parts[i];
      prefix = prefix ? `${prefix}.${label}` : label;
      let node = byPath.get(prefix);
      if (!node) {
        node = { id: prefix, label, path: prefix, children: [] };
        byPath.set(prefix, node);
        if (i === 0) {
          roots.push(node);
        } else {
          const parent = byPath.get(parts.slice(0, i).join('.'));
          parent?.children?.push(node);
        }
      }
      if (i === parts.length - 1) {
        node.id = field.id || prefix;
        node.preview = field.preview || field.value_text || '';
        node.tokens = Number(field.token_estimate) || 0;
        node.source = field.source || '';
        node.revision = Number(field.revision) || 0;
        node.pinned = Boolean(field.pin_to_prompt);
        if (!node.children?.length) delete node.children;
      }
    }
  }
  pruneEmptyChildren(roots);
  return roots;
}

function pruneEmptyChildren(nodes: L1FieldTreeNode[]) {
  for (const node of nodes) {
    if (node.children?.length) pruneEmptyChildren(node.children);
    if (!node.children?.length) delete node.children;
  }
}

export function taskBudgetRatio(used: number, budget: number): number {
  if (!budget) return 0;
  const ratio = used / budget;
  if (!Number.isFinite(ratio) || ratio < 0) return 0;
  return Math.min(1, ratio);
}
