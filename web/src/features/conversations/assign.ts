// 全局交办面板的入口：哪里都可以打开同一个面板（指令条、任务页、看板节点…），造的是同一种任务。
// 设计：docs/aranea-agents/plan/2026-11-21-任务入口与执行统筹方案.md §4
import type { PlatformResourceTreeNode as OrgNode } from '../platform/types';

export const OPEN_ASSIGN_EVENT = 'aranea:open-assign';

export type AssignPick = 'spirit' | 'agent' | 'flow' | 'tool' | 'run';

export interface AssignRequest {
  /** 预填「要办什么」 */
  text?: string;
  files?: File[];
  to?: AssignPick;
  /** 预选的执行者：`agent:<id>` / `team:<id>` / 工具键 */
  prefer?: string;
}

export function openAssign(detail: AssignRequest = {}) {
  window.dispatchEvent(new CustomEvent<AssignRequest>(OPEN_ASSIGN_EVENT, { detail }));
}

// ——— 最近交给过谁（记在本机，全局面板没有任务列表可推） ———
const RECENT_KEY = 'aranea.assign.recent';
const RECENT_MAX = 8;

export function readRecent(): string[] {
  try {
    const v: unknown = JSON.parse(localStorage.getItem(RECENT_KEY) ?? '[]');
    return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string') : [];
  } catch {
    return [];
  }
}

export function pushRecent(executor: string): string[] {
  const next = [executor, ...readRecent().filter((x) => x !== executor)].slice(0, RECENT_MAX);
  try {
    localStorage.setItem(RECENT_KEY, JSON.stringify(next));
  } catch {
    /* 只是记住上次的选择 */
  }
  return next;
}

// ——— 按组织找人：公司 › 部门 ———
export interface DeptOption {
  /** 部门节点 id */
  value: string;
  /** 「公司 › 部门」；只有一家公司时只写部门 */
  label: string;
  /** 这个部门及其下属岗位的节点 id（Agent 挂在岗位或部门节点上） */
  nodes: Set<string>;
}

export function deptOptions(tree: readonly OrgNode[]): DeptOption[] {
  const companies = tree.filter((n) => n.level === 'company');
  const out: DeptOption[] = [];
  for (const co of companies) {
    for (const dept of co.children ?? []) {
      if (dept.level !== 'department') continue;
      const nodes = new Set<string>();
      const walk = (n: OrgNode) => {
        nodes.add(n.id);
        for (const c of n.children ?? []) walk(c);
      };
      walk(dept);
      out.push({ value: dept.id, label: companies.length > 1 ? `${co.name} › ${dept.name}` : dept.name, nodes });
    }
  }
  return out;
}

// ——— 工作目录：资料里的本机资料库 ———
/** 下拉里「资料里还没登记成工作目录的资料库」的取值前缀 */
export const LIB_PREFIX = 'lib:';

const normPath = (p: string) =>
  p
    .replace(/[\\/]+$/, '')
    .replace(/\//g, '\\')
    .toLowerCase();
/** 同一个本机目录（不分大小写、斜杠方向、末尾斜杠） */
export const samePath = (a: string, b: string) => Boolean(a) && normPath(a) === normPath(b);

// ——— 流程（Graph）的输入：必填的状态字段要在交办时填上 ———
/** 运行时自己维护的状态键，以及由交接单填的 input，不让用户再填 */
const RUNTIME_KEYS = new Set(['messages', 'user_input', 'input', 'last_response', 'node_responses', 'metadata']);

export function flowInputs(fields: readonly { name: string; type: string; required: boolean }[]): string[] {
  return fields.filter((f) => f.required && f.type === 'string' && !RUNTIME_KEYS.has(f.name)).map((f) => f.name);
}
