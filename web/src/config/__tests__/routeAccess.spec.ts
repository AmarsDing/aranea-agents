import { describe, expect, it } from 'vitest';
import { evaluateRouteAccess } from '../routeAccess';

const ALL = ['/agents', '/tools', '/sessions'];

describe('evaluateRouteAccess', () => {
  it('放行白名单未获取的降级场景', () => {
    expect(evaluateRouteAccess({ path: '/agents', visiblePaths: null, permissions: [] })).toEqual({
      allowed: true,
      reason: 'degraded',
    });
  });

  it('放行不在导航目录中的路由', () => {
    expect(evaluateRouteAccess({ path: '/unknown-detail/1', visiblePaths: ALL, permissions: [] }).allowed).toBe(true);
  });

  it('按最长前缀判定详情页权限', () => {
    expect(evaluateRouteAccess({ path: '/agents/a-1/settings', visiblePaths: ALL, permissions: [] })).toEqual({
      allowed: true,
      reason: 'visible',
    });
  });

  it('派生入口跟随父入口：/tools 可见则 /tools/runs 可见', () => {
    expect(evaluateRouteAccess({ path: '/tools/runs', visiblePaths: ALL, permissions: [] }).allowed).toBe(true);
  });

  it('父入口不可见时拦截派生入口并给出所需入口', () => {
    const decision = evaluateRouteAccess({ path: '/tools/runs', visiblePaths: ['/agents'], permissions: [] });
    expect(decision).toEqual({ allowed: false, reason: 'menu', requiredPath: '/tools' });
  });

  it('白名单不含目标入口时拦截', () => {
    expect(evaluateRouteAccess({ path: '/sessions', visiblePaths: ['/agents'], permissions: [] })).toEqual({
      allowed: false,
      reason: 'menu',
      requiredPath: '/sessions',
    });
  });

  it('禁止页面与首页始终放行', () => {
    expect(evaluateRouteAccess({ path: '/403', visiblePaths: [], permissions: [] }).allowed).toBe(true);
    expect(evaluateRouteAccess({ path: '/overview', visiblePaths: [], permissions: [] }).allowed).toBe(true);
  });

  it('空菜单下发按降级放行，superadmin 通配符放行', () => {
    expect(evaluateRouteAccess({ path: '/sessions', visiblePaths: [], permissions: [] })).toEqual({
      allowed: true,
      reason: 'degraded',
    });
    expect(evaluateRouteAccess({ path: '/sessions', visiblePaths: ['/agents'], permissions: ['*'] })).toEqual({
      allowed: true,
      reason: 'always',
    });
  });

  it('meta.permission 缺失时拦截，通配符与命中时放行', () => {
    const base = { path: '/agents', visiblePaths: ALL, permission: 'agent.write' };
    expect(evaluateRouteAccess({ ...base, permissions: ['agent.read'] })).toEqual({
      allowed: false,
      reason: 'permission',
      requiredPermission: 'agent.write',
    });
    expect(evaluateRouteAccess({ ...base, permissions: ['agent.write'] }).allowed).toBe(true);
    expect(evaluateRouteAccess({ ...base, permissions: ['*'] }).allowed).toBe(true);
    expect(evaluateRouteAccess({ ...base, permissions: [] }).allowed).toBe(true);
  });
});
