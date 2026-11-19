// 运行台的分屏模型（照 Orca：标签归属窗格，窗格可横竖嵌套切分；
// 每个 worktree 一套独立布局，切换 worktree 整体换布局）。纯函数，便于测试。

export type TabKind = 'terminal' | 'diff' | 'scm';

export interface Tab {
  id: string;
  kind: TabKind;
  /** terminal 标签对应的终端 id */
  ref?: string;
}

export interface Leaf {
  type: 'leaf';
  id: string;
  tabs: string[];
  active: string | null;
}

export interface Split {
  type: 'split';
  id: string;
  dir: 'row' | 'col';
  ratio: number;
  a: PaneNode;
  b: PaneNode;
}

export type PaneNode = Leaf | Split;

export interface Layout {
  root: PaneNode;
  tabs: Record<string, Tab>;
  focus: string;
}

let seq = 0;
const uid = (p: string) => `${p}${Date.now().toString(36)}${(seq++).toString(36)}`;

export function emptyLayout(): Layout {
  const leaf: Leaf = { type: 'leaf', id: uid('p'), tabs: [], active: null };
  return { root: leaf, tabs: {}, focus: leaf.id };
}

export function leaves(n: PaneNode): Leaf[] {
  return n.type === 'leaf' ? [n] : [...leaves(n.a), ...leaves(n.b)];
}

export function findLeaf(l: Layout, id: string): Leaf | undefined {
  return leaves(l.root).find((x) => x.id === id);
}

export function leafOfTab(l: Layout, tabId: string): Leaf | undefined {
  return leaves(l.root).find((x) => x.tabs.includes(tabId));
}

function focusedLeaf(l: Layout): Leaf {
  return findLeaf(l, l.focus) ?? leaves(l.root)[0]!;
}

/** 打开一个标签：已存在就激活，否则加到当前焦点窗格。 */
export function openTab(l: Layout, tab: Tab): Layout {
  const existing = Object.values(l.tabs).find((t) => t.kind === tab.kind && t.ref === tab.ref);
  if (existing) {
    const leaf = leafOfTab(l, existing.id);
    if (leaf) {
      leaf.active = existing.id;
      l.focus = leaf.id;
    }
    return l;
  }
  l.tabs[tab.id] = tab;
  const leaf = focusedLeaf(l);
  leaf.tabs.push(tab.id);
  leaf.active = tab.id;
  return l;
}

export function newTab(kind: TabKind, ref?: string): Tab {
  return ref === undefined ? { id: uid('t'), kind } : { id: uid('t'), kind, ref };
}

/** 关掉标签；窗格空了就并回兄弟。 */
export function closeTab(l: Layout, tabId: string): Layout {
  const leaf = leafOfTab(l, tabId);
  delete l.tabs[tabId];
  if (!leaf) return l;
  const i = leaf.tabs.indexOf(tabId);
  leaf.tabs.splice(i, 1);
  if (leaf.active === tabId) leaf.active = leaf.tabs[Math.max(0, i - 1)] ?? null;
  if (leaf.tabs.length === 0) collapse(l, leaf.id);
  return l;
}

function collapse(l: Layout, leafId: string) {
  if (l.root.type === 'leaf') return;
  const replace = (n: PaneNode): PaneNode => {
    if (n.type === 'leaf') return n;
    if (n.a.type === 'leaf' && n.a.id === leafId) return n.b;
    if (n.b.type === 'leaf' && n.b.id === leafId) return n.a;
    n.a = replace(n.a);
    n.b = replace(n.b);
    return n;
  };
  l.root = replace(l.root);
  if (!findLeaf(l, l.focus)) l.focus = leaves(l.root)[0]!.id;
}

export type DropZone = 'center' | 'right' | 'bottom' | 'left' | 'top';

/** 把标签拖到某窗格：中间 = 并入该窗格；边缘 = 在那一侧切出新窗格。 */
export function moveTab(l: Layout, tabId: string, targetLeafId: string, zone: DropZone): Layout {
  const from = leafOfTab(l, tabId);
  const target = findLeaf(l, targetLeafId);
  if (!from || !target) return l;
  if (zone === 'center') {
    if (from.id === target.id) return l;
    from.tabs.splice(from.tabs.indexOf(tabId), 1);
    if (from.active === tabId) from.active = from.tabs[0] ?? null;
    target.tabs.push(tabId);
    target.active = tabId;
    l.focus = target.id;
    if (from.tabs.length === 0) collapse(l, from.id);
    return l;
  }
  if (from.id === target.id && from.tabs.length === 1) return l; // 自己切自己没有意义
  from.tabs.splice(from.tabs.indexOf(tabId), 1);
  if (from.active === tabId) from.active = from.tabs[0] ?? null;
  const fresh: Leaf = { type: 'leaf', id: uid('p'), tabs: [tabId], active: tabId };
  const dir: Split['dir'] = zone === 'right' || zone === 'left' ? 'row' : 'col';
  const first = zone === 'left' || zone === 'top';
  const wrap = (n: PaneNode): PaneNode => {
    if (n.type === 'leaf') {
      if (n.id !== target.id) return n;
      return { type: 'split', id: uid('s'), dir, ratio: 0.5, a: first ? fresh : n, b: first ? n : fresh };
    }
    n.a = wrap(n.a);
    n.b = wrap(n.b);
    return n;
  };
  l.root = wrap(l.root);
  l.focus = fresh.id;
  if (from.tabs.length === 0) collapse(l, from.id);
  return l;
}

/** 让布局里的终端标签与实际终端一致：新终端补标签，已关闭的移除标签。 */
export function syncTerminals(l: Layout, terminalIds: string[]): boolean {
  let changed = false;
  const want = new Set(terminalIds);
  for (const t of Object.values(l.tabs)) {
    if (t.kind === 'terminal' && t.ref && !want.has(t.ref)) {
      closeTab(l, t.id);
      changed = true;
    }
  }
  const have = new Set(
    Object.values(l.tabs)
      .filter((t) => t.kind === 'terminal')
      .map((t) => t.ref),
  );
  for (const id of terminalIds) {
    if (!have.has(id)) {
      const tab = newTab('terminal', id);
      l.tabs[tab.id] = tab;
      const leaf = leaves(l.root)[0]!;
      leaf.tabs.push(tab.id);
      if (!leaf.active) leaf.active = tab.id;
      changed = true;
    }
  }
  return changed;
}

/** 校验从本地存储读回来的布局，坏了就重建。 */
export function sanitize(v: unknown): Layout {
  const l = v as Layout;
  if (!l || typeof l !== 'object' || !l.root || !l.tabs) return emptyLayout();
  const ok = leaves(l.root).every((x) => Array.isArray(x.tabs) && x.tabs.every((t) => l.tabs[t]));
  if (!ok) return emptyLayout();
  if (!findLeaf(l, l.focus)) l.focus = leaves(l.root)[0]!.id;
  return l;
}
