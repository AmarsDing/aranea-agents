import { describe, expect, it } from 'vitest';
import { formatCount, formatRate, rateTone, riskTone } from '../capTypes';
import { toolFacts, toolRelation, toolStatus, toolTags } from '../../tools/toolCapCard';
import { skillStatus, skillTags } from '../../skills/skillCapCard';
import { pluginScopeText, pluginStatus } from '../../plugins/pluginCapCard';
import type { Tool } from '../../../features/tools/types';
import type { Skill } from '../../../features/skills/types';
import type { Plugin } from '../../../features/plugins/types';

// 文案不重要，回传 key 便于断言走的是哪条分支
const t = (key: string, named?: Record<string, unknown>) => (named ? `${key}:${JSON.stringify(named)}` : key);

const tool = (over: Partial<Tool>): Tool =>
  ({
    key: 'k',
    category: 'web',
    source: 'builtin',
    risk_level: 'low',
    enabled: true,
    readonly: false,
    requires_confirmation: false,
    runtime_status: 'available',
    invoke_count: 0,
    invoke_count_24h: 0,
    success_count: 0,
    agent_override_count: 0,
    ...over,
  }) as Tool;

describe('capTypes', () => {
  it('formats counts and rates', () => {
    expect(formatCount(999)).toBe('999');
    expect(formatCount(1234)).toBe('1.2k');
    expect(formatCount(undefined)).toBe('—');
    expect(formatRate(null)).toBe('—');
    expect(formatRate(0.9876)).toBe('98.8%');
  });
  it('maps risk and rate to tones', () => {
    expect(riskTone('critical')).toBe('alert');
    expect(riskTone('medium')).toBe('watch');
    expect(riskTone('low')).toBe('ok');
    expect(rateTone(null)).toBe('pending');
    expect(rateTone(0.5)).toBe('alert');
    expect(rateTone(0.9)).toBe('watch');
    expect(rateTone(0.99)).toBe('ok');
  });
});

describe('toolCapCard', () => {
  it('status: disabled wins, then config, then runtime, then last failure', () => {
    expect(toolStatus(tool({ enabled: false, runtime_status: 'needs_config' }), t).tone).toBe('off');
    expect(toolStatus(tool({ runtime_status: 'needs_config' }), t).tone).toBe('watch');
    expect(toolStatus(tool({ runtime_status: 'registered_only' }), t).tone).toBe('pending');
    expect(toolStatus(tool({ last_status: 'blocked' }), t).tone).toBe('alert');
    // 随会话装配的工具照常算可用
    expect(toolStatus(tool({ runtime_status: 'session_bound' }), t).tone).toBe('ok');
  });
  it('facts: never called → third fact is the confirmation mode, not empty usage', () => {
    const facts = toolFacts(tool({ risk_level: 'high', requires_confirmation: true, allowed_agent_count: 12 }), t);
    expect(facts[0]).toMatchObject({ tone: 'alert' });
    expect(facts[1]).toMatchObject({ label: 'cap.tools.agentsCanUse', value: '12' });
    expect(facts[2]).toMatchObject({ label: 'cap.tools.confirmMode', value: 'cap.tools.confirmAsk', tone: 'watch' });
    expect(toolFacts(tool({}), t)[2]).toMatchObject({ value: 'cap.tools.confirmDirect' });
  });
  it('facts: with calls → success rate; unknown agent count shows a dash, enabled-but-unused-by-anyone is flagged', () => {
    const facts = toolFacts(tool({ invoke_count: 10, success_count: 7 }), t);
    expect(facts[1]).toMatchObject({ value: '—' });
    expect(facts[2]).toMatchObject({ value: '70%', tone: 'alert' });
    expect(toolFacts(tool({ allowed_agent_count: 0 }), t)[1]).toMatchObject({ value: '0', tone: 'watch' });
  });
  it('relation: usage when called, otherwise overrides or never-called', () => {
    expect(toolRelation(tool({ invoke_count: 3, invoke_count_24h: 1 }), t)).toContain('cap.tools.relationUsage');
    expect(toolRelation(tool({ agent_override_count: 2 }), t)).toContain('cap.tools.relationOverrides');
    expect(toolRelation(tool({}), t)).toBe('cap.tools.relationNeverCalled');
  });
  it('tags: source only when not builtin; confirm tag only once there are calls', () => {
    const fresh = toolTags(tool({ source: 'mcp', requires_confirmation: true, readonly: true }), t).map((x) => x.label);
    expect(fresh).toEqual(['web', 'mcp', 'cap.tools.tagReadonly']);
    const used = toolTags(tool({ requires_confirmation: true, invoke_count: 1 }), t).map((x) => x.label);
    expect(used).toEqual(['web', 'cap.tools.tagConfirm']);
  });
});

describe('skillCapCard', () => {
  const skill = (over: Partial<Skill>): Skill => ({ status: 'published', enabled: true, tags: [], ...over }) as Skill;
  it('status precedence: missing files > archived > draft > enabled/disabled', () => {
    expect(skillStatus(skill({ filesystem_missing: true, status: 'draft' }), t).tone).toBe('alert');
    expect(skillStatus(skill({ status: 'archived' }), t).tone).toBe('off');
    expect(skillStatus(skill({ status: 'draft' }), t).tone).toBe('pending');
    expect(skillStatus(skill({}), t).tone).toBe('ok');
    expect(skillStatus(skill({ enabled: false }), t).tone).toBe('off');
  });
  it('tags: dimension prefix stripped, at most three', () => {
    const tags = skillTags(
      skill({
        tags: [{ name: 'domain:电商' }, { name: 'plain' }, { name: 'a:b' }, { name: 'dropped' }] as Skill['tags'],
      }),
    ).map((x) => x.label);
    expect(tags).toEqual(['电商', 'plain', 'b']);
  });
});

describe('pluginCapCard', () => {
  const plugin = (over: Partial<Plugin>): Plugin => ({ enabled: true, scope: 'global', ...over }) as Plugin;
  it('scope text follows backend PluginMatchesScope: empty/global = all, else agent id', () => {
    const name = (id: string) => (id === 'a1' ? '客服' : id);
    expect(pluginScopeText(plugin({ scope: '' }), t, name)).toBe('cap.plugins.scopeGlobal');
    expect(pluginScopeText(plugin({ scope: 'GLOBAL' }), t, name)).toBe('cap.plugins.scopeGlobal');
    expect(pluginScopeText(plugin({ scope: 'a1' }), t, name)).toBe('cap.plugins.scopeAgent:{"agent":"客服"}');
  });
  it('status: disabled, last error, ok', () => {
    expect(pluginStatus(plugin({ enabled: false }), t).tone).toBe('off');
    expect(pluginStatus(plugin({ last_status: 'ERROR: boom' }), t).tone).toBe('alert');
    expect(pluginStatus(plugin({}), t).tone).toBe('ok');
  });
});
