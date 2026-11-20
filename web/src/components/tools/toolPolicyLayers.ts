// 一个工具被调用前要过的各层规则，逐层给结论，再给最终结论。纯函数，单测见 __tests__/toolPolicyLayers.spec.ts。
// 层次与后端一致：目录启停 → 权限守卫插件的全局禁用名单 → 各 Agent 工具策略 → 工具自身「需人工确认」
// → 确认守卫插件的名单 / 参数模式 → 参数规则；会话 / Agent 级审批策略设了就优先于确认相关的几层。
import type { Tool } from '../../features/tools/types';
import type { CapTone } from '../capability/capTypes';

type T = (key: string, named?: Record<string, unknown>) => string;

export type GuardPlugin = { enabled: boolean; config_json: string; default_config_json: string };

export type ToolPolicyInput = {
  confirmationGuard: GuardPlugin | null;
  permissionGuard: GuardPlugin | null;
  /** null = 没查到 */
  paramRuleCount: number | null;
};

export type PolicyLayer = { key: string; label: string; verdict: string; tone: CapTone };
export type PolicyVerdict = { tone: CapTone; label: string };

function pluginConfig(p: GuardPlugin): Record<string, unknown> {
  const parse = (raw: string) => {
    try {
      const v = JSON.parse(raw || '{}');
      return v && typeof v === 'object' ? (v as Record<string, unknown>) : {};
    } catch {
      return {};
    }
  };
  // 与后端一致：默认值垫底，用户配置覆盖
  return { ...parse(p.default_config_json), ...parse(p.config_json) };
}

function strList(v: unknown): string[] {
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string' && x.trim() !== '') : [];
}

export function toolPolicyLayers(
  tool: Tool,
  input: ToolPolicyInput,
  t: T,
): { layers: PolicyLayer[]; final: PolicyVerdict } {
  const layers: PolicyLayer[] = [];
  let blocked: string | null = null;
  let confirm = false;
  let conditional = false;

  // 1 目录
  if (tool.enabled) {
    layers.push({ key: 'catalog', label: t('cap.tools.policy.catalog'), verdict: t('cap.common.enabled'), tone: 'ok' });
  } else {
    layers.push({
      key: 'catalog',
      label: t('cap.tools.policy.catalog'),
      verdict: t('cap.tools.policy.catalogOff'),
      tone: 'off',
    });
    blocked = t('cap.tools.policy.finalDisabled');
  }

  // 2 权限守卫：全局禁用名单
  const pg = input.permissionGuard;
  if (!pg || !pg.enabled) {
    layers.push({
      key: 'permission',
      label: t('cap.tools.policy.permission'),
      verdict: t('cap.tools.policy.pluginOff'),
      tone: 'off',
    });
  } else {
    const cfg = pluginConfig(pg);
    const denied = strList(cfg.deny_tools).includes(tool.key);
    const scoped = strList(cfg.agent_allowlist).length;
    if (denied) {
      layers.push({
        key: 'permission',
        label: t('cap.tools.policy.permission'),
        verdict: scoped ? t('cap.tools.policy.deniedFor', { n: scoped }) : t('cap.tools.policy.deniedAll'),
        tone: 'alert',
      });
      if (!scoped) blocked = blocked ?? t('cap.tools.policy.finalDenied');
    } else {
      layers.push({
        key: 'permission',
        label: t('cap.tools.policy.permission'),
        verdict: t('cap.tools.policy.notListed'),
        tone: 'ok',
      });
    }
  }

  // 3 各 Agent 的工具策略
  const n = tool.allowed_agent_count;
  layers.push({
    key: 'agents',
    label: t('cap.tools.policy.agents'),
    verdict: n === null || n === undefined ? '—' : t('cap.tools.policy.agentsN', { n }),
    tone: n === 0 ? 'watch' : n === null || n === undefined ? 'pending' : 'ok',
  });
  if (n === 0) blocked = blocked ?? t('cap.tools.policy.finalNoAgent');

  // 4 工具自身的确认标记
  layers.push({
    key: 'flag',
    label: t('cap.tools.policy.flag'),
    verdict: tool.requires_confirmation ? t('cap.tools.confirmAsk') : t('cap.tools.confirmDirect'),
    tone: tool.requires_confirmation ? 'watch' : 'ok',
  });
  if (tool.requires_confirmation) confirm = true;

  // 5 确认守卫插件：名单 / 参数模式
  const cg = input.confirmationGuard;
  if (!cg || !cg.enabled) {
    layers.push({
      key: 'guard',
      label: t('cap.tools.policy.guard'),
      verdict: t('cap.tools.policy.pluginOff'),
      tone: 'off',
    });
  } else {
    const cfg = pluginConfig(cg);
    const listed = strList(cfg.confirm_tools).includes(tool.key);
    const patterns = strList(cfg.confirm_patterns).length;
    if (listed) {
      layers.push({
        key: 'guard',
        label: t('cap.tools.policy.guard'),
        verdict: t('cap.tools.policy.guardListed'),
        tone: 'watch',
      });
      confirm = true;
    } else if (patterns) {
      layers.push({
        key: 'guard',
        label: t('cap.tools.policy.guard'),
        verdict: t('cap.tools.policy.guardPatterns', { n: patterns }),
        tone: 'watch',
      });
      conditional = true;
    } else {
      layers.push({
        key: 'guard',
        label: t('cap.tools.policy.guard'),
        verdict: t('cap.tools.policy.notListed'),
        tone: 'ok',
      });
    }
  }

  // 6 参数规则
  const rules = input.paramRuleCount;
  layers.push({
    key: 'params',
    label: t('cap.tools.policy.params'),
    verdict:
      rules === null ? '—' : rules ? t('cap.tools.policy.paramsN', { n: rules }) : t('cap.tools.policy.paramsNone'),
    tone: rules ? 'watch' : rules === null ? 'pending' : 'ok',
  });
  if (rules) conditional = true;

  // 7 会话 / Agent 级审批策略：按会话或 Agent 设，这里列不全，只说明它的位置
  layers.push({
    key: 'approval',
    label: t('cap.tools.policy.approval'),
    verdict: t('cap.tools.policy.approvalNote'),
    tone: 'pending',
  });

  let final: PolicyVerdict;
  if (blocked) final = { tone: 'alert', label: blocked };
  else if (confirm) final = { tone: 'watch', label: t('cap.tools.policy.finalConfirm') };
  else if (conditional) final = { tone: 'watch', label: t('cap.tools.policy.finalConditional') };
  else final = { tone: 'ok', label: t('cap.tools.policy.finalDirect') };
  return { layers, final };
}
