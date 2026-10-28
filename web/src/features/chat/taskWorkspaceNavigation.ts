import type { LocationQuery, LocationQueryRaw } from 'vue-router';

export type WorkspacePanel = 'chat' | 'activity' | 'artifacts';
export interface TaskWorkspaceLocation {
  sessionId: string;
  taskId?: string;
  stepId?: string;
  panel?: WorkspacePanel;
  artifactId?: string;
  version?: number;
}
const targetKeys = ['task', 'step', 'panel', 'artifact', 'version'] as const;
const identifier = (value: unknown): value is string => typeof value === 'string' && /^[\w.@:-]{1,256}$/.test(value);

/** URL values describe a read target; they are never an approval or a send command. */
export function parseWorkspaceLocation(query: LocationQuery): TaskWorkspaceLocation | null {
  const present = (key: string) => query[key] !== undefined;
  if (!present('session') && !targetKeys.some(present)) return null;
  if (!identifier(query.session)) throw new Error('invalid-location');
  for (const key of ['task', 'step', 'artifact', 'agent', 'team'] as const) {
    if (present(key) && !identifier(query[key])) throw new Error('invalid-location');
  }
  if (present('agent') && present('team')) throw new Error('invalid-location');
  if (present('step') && !present('task')) throw new Error('invalid-location');
  if (
    present('version') &&
    (!present('artifact') || typeof query.version !== 'string' || !/^(0|[1-9]\d*)$/.test(query.version))
  )
    throw new Error('invalid-location');
  const version = present('version') ? Number(query.version) : undefined;
  if (version !== undefined && !Number.isSafeInteger(version)) throw new Error('invalid-location');
  if (present('panel') && !['chat', 'activity', 'artifacts'].includes(String(query.panel)))
    throw new Error('invalid-location');
  if (Array.isArray(query.panel)) throw new Error('invalid-location');
  return {
    sessionId: query.session,
    taskId: query.task as string | undefined,
    stepId: query.step as string | undefined,
    panel: query.panel as WorkspacePanel | undefined,
    artifactId: query.artifact as string | undefined,
    version,
  };
}

export function workspaceLocationQuery(target: TaskWorkspaceLocation): LocationQuery {
  const query: LocationQuery = { session: target.sessionId };
  if (target.taskId) query.task = target.taskId;
  if (target.stepId) query.step = target.stepId;
  if (target.panel) query.panel = target.panel;
  if (target.artifactId) query.artifact = target.artifactId;
  if (target.version !== undefined) query.version = String(target.version);
  return query;
}

export function isChatSurface(name: unknown) {
  return ['chat', 'task-workspace', 'mobile-chat', 'mobile-task-workspace'].includes(String(name));
}
export function workspaceRouteName(path: string) {
  return path.startsWith('/mobile/') ? 'mobile-task-workspace' : 'task-workspace';
}
/** Keep a target only while the selected session still matches it. */
export function sessionNavigationQuery(
  query: LocationQuery,
  sessionId: string,
  owner: { agent?: string; team?: string },
): LocationQueryRaw {
  const result: LocationQueryRaw = { session: sessionId, ...owner };
  if (query.session === sessionId) for (const key of targetKeys) if (query[key] !== undefined) result[key] = query[key];
  return result;
}

export function chatBreakpointTarget(path: string, query: LocationQuery, narrow: boolean) {
  const mobile = path === '/mobile' || path.startsWith('/mobile/');
  if (narrow && !mobile) {
    if (path === '/chat') return { path: '/mobile/chat', query };
    if (path === '/task-workspace') return { path: '/mobile/task-workspace', query };
    return { path: '/mobile/sessions' };
  }
  if (!narrow && mobile) {
    if (path === '/mobile/chat') return { path: '/chat', query };
    if (path === '/mobile/task-workspace') return { path: '/task-workspace', query };
    return { path: '/overview' };
  }
  return null;
}

/** Allow only local chat surfaces, and discard unrelated query data from return links. */
export function safeWorkspaceReturn(value: unknown): string | null {
  if (
    typeof value !== 'string' ||
    value.length > 4000 ||
    !value.startsWith('/') ||
    value.startsWith('//') ||
    value.includes('\\') ||
    Array.from(value).some((character) => character.charCodeAt(0) < 32 || character.charCodeAt(0) === 127)
  )
    return null;
  const url = new URL(value, 'https://aranea.invalid');
  if (
    url.origin !== 'https://aranea.invalid' ||
    !['/chat', '/task-workspace', '/mobile/chat', '/mobile/task-workspace'].includes(url.pathname)
  )
    return null;
  const query: LocationQuery = {};
  let duplicate = false;
  url.searchParams.forEach((val, key) => {
    if (query[key] !== undefined) duplicate = true;
    query[key] = val;
  });
  if (duplicate) return null;
  try {
    parseWorkspaceLocation(query);
  } catch {
    return null;
  }
  const allowed = new URLSearchParams();
  for (const key of ['session', 'agent', 'team', ...targetKeys]) {
    const val = query[key];
    if (typeof val === 'string' && (key === 'panel' || key === 'version' || identifier(val))) allowed.set(key, val);
  }
  return `${url.pathname}${allowed.size ? `?${allowed}` : ''}`;
}
