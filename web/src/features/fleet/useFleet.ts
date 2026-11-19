// 运行台状态中心（单例）：数据轮询、当前 worktree、每个 worktree 的分屏布局、通知。
import { computed, reactive } from 'vue';
import { Notify } from 'quasar';
import * as api from './api';
import type { AgentPreset, AgentUsage, FleetEvent, Project, Terminal, Workspace } from './api';
import { emptyLayout, newTab, openTab, sanitize, syncTerminals, type Layout } from './layout';

export type FleetView = 'workspace' | 'feed' | 'board' | 'runs' | 'automations';

const LAYOUT_KEY = 'fleet:layouts:v1';
const CURRENT_KEY = 'fleet:current:v1';
const NOTIFY_KINDS = new Set([
  'agent_done',
  'needs_input',
  'worker_done',
  'question',
  'worktree_failed',
  'automation',
  'exited', // agent 启动失败或自己退出（用户主动关闭的不会产生这类事件）
]);

function readJSON<T>(key: string, fallback: T): T {
  try {
    const raw = localStorage.getItem(key);
    return raw ? (JSON.parse(raw) as T) : fallback;
  } catch {
    return fallback;
  }
}

function writeJSON(key: string, v: unknown) {
  try {
    localStorage.setItem(key, JSON.stringify(v));
  } catch {
    /* 隐私模式等存不了就算了，布局只是便利项 */
  }
}

const state = reactive({
  presets: [] as AgentPreset[],
  projects: [] as Project[],
  workspaces: [] as Workspace[],
  terminals: [] as Terminal[],
  events: [] as FleetEvent[],
  unread: 0,
  usage: [] as AgentUsage[],
  view: 'workspace' as FleetView,
  currentWs: readJSON<string>(CURRENT_KEY, ''),
  layouts: {} as Record<string, Layout>,
  loaded: false,
  error: '',
  jumpOpen: false,
  createOpen: false,
  createProjectId: '',
  lastEventId: '',
});

{
  const stored = readJSON<Record<string, unknown>>(LAYOUT_KEY, {});
  for (const [k, v] of Object.entries(stored)) state.layouts[k] = sanitize(v);
}

let saveTimer: ReturnType<typeof setTimeout> | null = null;
function saveLayouts() {
  if (saveTimer) clearTimeout(saveTimer);
  saveTimer = setTimeout(() => writeJSON(LAYOUT_KEY, state.layouts), 300);
}

// ---------- 刷新 ----------

async function refreshProjects() {
  const [presets, projects] = await Promise.all([api.listPresets(), api.listProjects()]);
  state.presets = presets;
  state.projects = projects;
}

async function refreshWorkspaces() {
  state.workspaces = await api.listWorkspaces();
  if (!state.workspaces.some((w) => w.id === state.currentWs)) {
    state.currentWs = state.workspaces[0]?.id ?? '';
  }
}

async function refreshTerminals() {
  state.terminals = await api.listTerminals();
  let changed = false;
  for (const [wsId, layout] of Object.entries(state.layouts)) {
    const ids = state.terminals.filter((t) => t.workspaceId === wsId).map((t) => t.id);
    if (syncTerminals(layout, ids)) changed = true;
  }
  if (changed) saveLayouts();
}

async function refreshEvents() {
  const res = await api.listEvents(150);
  const prevTop = state.lastEventId;
  state.events = res.items;
  state.unread = res.unread;
  const top = res.items[0]?.id ?? '';
  if (prevTop && top && top !== prevTop) {
    const fresh = res.items.filter((e) => e.id > prevTop && !e.read && NOTIFY_KINDS.has(e.kind));
    fresh.slice(0, 3).forEach(notifyEvent);
  }
  state.lastEventId = top || prevTop;
}

async function refreshUsage() {
  state.usage = await api.getUsage();
}

function notifyEvent(e: FleetEvent) {
  const urgent = ['needs_input', 'question', 'worktree_failed', 'exited'].includes(e.kind);
  Notify.create({
    type: urgent ? 'warning' : 'info',
    message: e.title,
    caption: e.body ? e.body.split('\n').slice(-1)[0] : undefined,
    timeout: urgent ? 8000 : 4000,
    actions: e.workspaceId ? [{ label: '查看', color: 'white', handler: () => jumpToEvent(e) }] : [],
  });
  if (typeof Notification !== 'undefined' && Notification.permission === 'granted' && document.hidden) {
    const n = new Notification(e.title, { body: e.body.slice(0, 160), tag: e.id });
    n.onclick = () => {
      window.focus();
      jumpToEvent(e);
    };
  }
}

async function refreshAll() {
  try {
    await refreshProjects();
    await Promise.all([refreshWorkspaces(), refreshTerminals(), refreshEvents()]);
    state.error = '';
  } catch (e) {
    state.error = api.errText(e);
  } finally {
    state.loaded = true;
  }
  void refreshUsage().catch(() => undefined);
}

// ---------- 轮询（页面挂载期间） ----------

let users = 0;
const timers: ReturnType<typeof setInterval>[] = [];

function startPolling() {
  users++;
  if (users > 1) return;
  void refreshAll();
  const safe = (fn: () => Promise<unknown>) => () => void fn().catch(() => undefined);
  timers.push(setInterval(safe(refreshTerminals), 2000));
  timers.push(setInterval(safe(refreshEvents), 3000));
  timers.push(setInterval(safe(refreshWorkspaces), 5000));
  timers.push(setInterval(safe(refreshUsage), 60000));
}

function stopPolling() {
  users = Math.max(0, users - 1);
  if (users > 0) return;
  timers.splice(0).forEach(clearInterval);
}

// ---------- 导航与布局 ----------

function layoutFor(wsId: string): Layout {
  let l = state.layouts[wsId];
  if (!l) {
    l = emptyLayout();
    state.layouts[wsId] = l;
  }
  const ids = state.terminals.filter((t) => t.workspaceId === wsId).map((t) => t.id);
  if (syncTerminals(l, ids)) saveLayouts();
  return l;
}

function selectWorkspace(wsId: string) {
  state.currentWs = wsId;
  state.view = 'workspace';
  writeJSON(CURRENT_KEY, wsId);
  layoutFor(wsId);
}

function focusTerminal(terminalId: string) {
  const t = state.terminals.find((x) => x.id === terminalId);
  if (!t) return;
  selectWorkspace(t.workspaceId);
  openTab(layoutFor(t.workspaceId), newTab('terminal', t.id));
  saveLayouts();
}

function openPanel(kind: 'diff' | 'scm', wsId = state.currentWs) {
  if (!wsId) return;
  selectWorkspace(wsId);
  openTab(layoutFor(wsId), newTab(kind, kind));
  saveLayouts();
}

function jumpToEvent(e: FleetEvent) {
  if (e.terminalId && state.terminals.some((t) => t.id === e.terminalId)) focusTerminal(e.terminalId);
  else if (e.workspaceId && state.workspaces.some((w) => w.id === e.workspaceId)) selectWorkspace(e.workspaceId);
  if (!e.read) {
    void api
      .markEventsRead([e.id])
      .then(refreshEvents)
      .catch(() => undefined);
  }
}

async function newTerminal(wsId: string, agent: string, opts: { fullAccess?: boolean; prompt?: string } = {}) {
  try {
    const t = await api.createTerminal({
      workspaceId: wsId,
      agent,
      fullAccess: opts.fullAccess ?? false,
      prompt: opts.prompt ?? '',
    });
    state.terminals = [...state.terminals, t];
    focusTerminal(t.id);
    return t;
  } catch (e) {
    Notify.create({ type: 'negative', message: api.errText(e) });
    return null;
  }
}

function requestDesktopNotifications() {
  if (typeof Notification !== 'undefined' && Notification.permission === 'default') {
    void Notification.requestPermission();
  }
}

// ---------- 派生 ----------

const currentWorkspace = computed(() => state.workspaces.find((w) => w.id === state.currentWs) ?? null);
const currentProject = computed(() => state.projects.find((p) => p.id === currentWorkspace.value?.projectId) ?? null);
const availableAgents = computed(() => state.presets.filter((p) => p.available && p.id !== 'shell'));

/** worktree 的聚合状态：有人等你 > 有人在干活 > 有完成 > 空闲。 */
function workspaceStatus(wsId: string): string {
  const ts = state.terminals.filter((t) => t.workspaceId === wsId);
  if (ts.some((t) => t.status === 'needs_input')) return 'needs_input';
  if (ts.some((t) => t.status === 'working' || t.status === 'starting')) return 'working';
  if (ts.some((t) => t.status === 'failed')) return 'failed';
  if (ts.some((t) => t.status === 'done')) return 'done';
  if (ts.some((t) => t.status === 'idle')) return 'idle';
  return '';
}

function agentLabel(id: string): string {
  return state.presets.find((p) => p.id === id)?.label ?? (id === 'shell' ? '终端' : id);
}

export function useFleet() {
  return {
    state,
    currentWorkspace,
    currentProject,
    availableAgents,
    refreshAll,
    refreshWorkspaces,
    refreshTerminals,
    refreshEvents,
    refreshUsage,
    startPolling,
    stopPolling,
    layoutFor,
    saveLayouts,
    selectWorkspace,
    focusTerminal,
    openPanel,
    jumpToEvent,
    newTerminal,
    workspaceStatus,
    agentLabel,
    requestDesktopNotifications,
  };
}

/** 相对时间：刚刚 / 3 分 / 2 时 / 5 天。 */
export function ago(iso: string): string {
  if (!iso) return '';
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return '';
  const sec = Math.max(0, (Date.now() - t) / 1000);
  if (sec < 45) return '刚刚';
  if (sec < 3600) return `${Math.round(sec / 60)} 分`;
  if (sec < 86400) return `${Math.round(sec / 3600)} 时`;
  return `${Math.round(sec / 86400)} 天`;
}

export const STATUS_TEXT: Record<string, string> = {
  starting: '启动中',
  working: '工作中',
  needs_input: '等你',
  done: '完成',
  idle: '空闲',
  exited: '已退出',
  failed: '异常退出',
  shell: '终端',
  creating: '创建中',
};
