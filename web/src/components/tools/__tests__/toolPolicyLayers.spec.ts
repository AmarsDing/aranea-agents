import { describe, expect, it } from 'vitest';
import { toolPolicyLayers, type GuardPlugin } from '../toolPolicyLayers';
import type { Tool } from '../../../features/tools/types';

const t = (key: string) => key;
const tool = (over: Partial<Tool>): Tool =>
  ({ key: 'shell_exec', enabled: true, requires_confirmation: false, allowed_agent_count: 5, ...over }) as Tool;
const plugin = (
  enabled: boolean,
  config: Record<string, unknown>,
  defaults: Record<string, unknown> = {},
): GuardPlugin => ({
  enabled,
  config_json: JSON.stringify(config),
  default_config_json: JSON.stringify(defaults),
});
const none = { confirmationGuard: null, permissionGuard: null, paramRuleCount: 0 };

describe('toolPolicyLayers', () => {
  it('plain tool runs directly; every layer is listed', () => {
    const { layers, final } = toolPolicyLayers(tool({}), none, t);
    expect(layers.map((l) => l.key)).toEqual([
      'catalog',
      'permission',
      'agents',
      'flag',
      'guard',
      'params',
      'approval',
    ]);
    expect(final.label).toBe('cap.tools.policy.finalDirect');
  });

  it('disabled or globally denied or no agents → blocked, and blocking wins over confirmation', () => {
    expect(toolPolicyLayers(tool({ enabled: false, requires_confirmation: true }), none, t).final.label).toBe(
      'cap.tools.policy.finalDisabled',
    );
    const denied = toolPolicyLayers(
      tool({}),
      { ...none, permissionGuard: plugin(true, { deny_tools: ['shell_exec'] }) },
      t,
    );
    expect(denied.final).toMatchObject({ tone: 'alert', label: 'cap.tools.policy.finalDenied' });
    expect(toolPolicyLayers(tool({ allowed_agent_count: 0 }), none, t).final.label).toBe(
      'cap.tools.policy.finalNoAgent',
    );
  });

  it('deny list limited to some agents does not block everyone', () => {
    const r = toolPolicyLayers(
      tool({}),
      { ...none, permissionGuard: plugin(true, { deny_tools: ['shell_exec'], agent_allowlist: ['a1', 'a2'] }) },
      t,
    );
    expect(r.layers.find((l) => l.key === 'permission')?.tone).toBe('alert');
    expect(r.final.label).toBe('cap.tools.policy.finalDirect');
  });

  it('a disabled guard plugin does not count, even if its config lists the tool', () => {
    const r = toolPolicyLayers(
      tool({}),
      { ...none, confirmationGuard: plugin(false, { confirm_tools: ['shell_exec'] }) },
      t,
    );
    expect(r.final.label).toBe('cap.tools.policy.finalDirect');
  });

  it('confirmation from the tool flag or the guard list; patterns and param rules are conditional', () => {
    expect(toolPolicyLayers(tool({ requires_confirmation: true }), none, t).final.label).toBe(
      'cap.tools.policy.finalConfirm',
    );
    const listed = { ...none, confirmationGuard: plugin(true, { confirm_tools: ['shell_exec'] }) };
    expect(toolPolicyLayers(tool({}), listed, t).final.label).toBe('cap.tools.policy.finalConfirm');
    // 用户配置为空时用默认值里的模式
    const patterns = { ...none, confirmationGuard: plugin(true, {}, { confirm_patterns: ['rm -rf'] }) };
    expect(toolPolicyLayers(tool({}), patterns, t).final.label).toBe('cap.tools.policy.finalConditional');
    expect(toolPolicyLayers(tool({}), { ...none, paramRuleCount: 2 }, t).final.label).toBe(
      'cap.tools.policy.finalConditional',
    );
  });
});
