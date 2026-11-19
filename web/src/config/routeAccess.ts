import { findNavTrail, navLeafVisible } from './navView';

/**
 * 路由级访问判定（展示层）。
 *
 * 契约：
 * - 后端仍对每个 RPC 硬判权限，这里只决定「是否渲染页面」；
 * - 菜单白名单未获取/获取失败（visiblePaths === null）时放行，避免接口抖动把整站锁死；
 * - 不在导航目录中的路由（详情页、移动端、伴侣页等）放行，由页面自身与后端接口兜底；
 * - 派生入口（5 类运行记录页）跟随父入口的可见性；
 * - meta.permission 提供细粒度权限码校验，对应后端 403 响应中的 required_permission，
 *   后端尚未下发时该字段为空，判定自动跳过。
 */

/** 与菜单权限无关、必须可直达的页面（避免把用户锁死在 403）。 */
const ALWAYS_ALLOWED = new Set(['/', '/workbench', '/overview', '/403', '/login', '/force-reset-password', '/mobile/server-setup']);

export type RouteAccessInput = {
  path: string;
  /** route.meta.permission：细粒度权限码 */
  permission?: string;
  /** 菜单可见路径白名单；null = 未获取/获取失败 */
  visiblePaths: string[] | null;
  /** 生效权限码；superadmin 为 ['*'] */
  permissions: string[];
};

export type RouteAccessDecision =
  | { allowed: true; reason: 'always' | 'degraded' | 'no-menu-entry' | 'visible' }
  | { allowed: false; reason: 'menu'; requiredPath: string }
  | { allowed: false; reason: 'permission'; requiredPermission: string };

export function evaluateRouteAccess(input: RouteAccessInput): RouteAccessDecision {
  const path = input.path || '/';
  if (ALWAYS_ALLOWED.has(path)) return { allowed: true, reason: 'always' };

  const permissions = input.permissions ?? [];
  const wildcard = permissions.includes('*');
  // superadmin（['*']）与空菜单下发都按放行处理：空菜单几乎必然是配置问题，
  // 而接口仍会被后端 403 拒绝，宁可让页面可见也不要整站锁死。
  if (wildcard) return { allowed: true, reason: 'always' };
  if (input.permission && permissions.length > 0 && !permissions.includes(input.permission)) {
    return { allowed: false, reason: 'permission', requiredPermission: input.permission };
  }

  if (input.visiblePaths === null || input.visiblePaths.length === 0) {
    return { allowed: true, reason: 'degraded' };
  }

  const trail = findNavTrail(path);
  if (!trail) return { allowed: true, reason: 'no-menu-entry' };

  const requiredPath = trail.item.derived && trail.item.derivedFrom ? trail.item.derivedFrom : trail.item.to;
  const visible = navLeafVisible(trail.item, (p) => input.visiblePaths!.includes(p));
  return visible ? { allowed: true, reason: 'visible' } : { allowed: false, reason: 'menu', requiredPath };
}
