// 技能 → 构件卡（CapCard）的取值。纯函数，单测见 __tests__/skillCapCard.spec.ts。
import type { Skill } from '../../features/skills/types';
import { formatCount, formatRate, rateTone, type CapFact, type CapStatus, type CapTag } from '../capability/capTypes';
import { skillOriginLabel } from './skillTableUi';

type T = (key: string, named?: Record<string, unknown>) => string;

/** 磁盘缺失最要紧；其次按生命周期：归档 → 草稿 → 已发布（启用 / 停用） */
export function skillStatus(skill: Skill, t: T): CapStatus {
  if (skill.filesystem_missing) return { tone: 'alert', label: t('cap.skills.status.missing') };
  if (skill.status === 'archived') return { tone: 'off', label: t('cap.skills.status.archived') };
  if (skill.status !== 'published') return { tone: 'pending', label: t('cap.skills.status.draft') };
  return skill.enabled
    ? { tone: 'ok', label: t('cap.skills.status.on') }
    : { tone: 'off', label: t('cap.skills.status.off') };
}

/** 版本、能看见它的 Agent 数；第三格有调用时是成功率，没有时是 7 天使用次数 */
export function skillFacts(skill: Skill, t: T): CapFact[] {
  const visible = skill.visible_agent_count;
  const facts: CapFact[] = [
    { label: t('cap.skills.version'), value: skill.current_version?.version ?? '—' },
    {
      label: t('cap.skills.visibleAgents'),
      value: visible === null || visible === undefined ? '—' : visible,
      hint: t('cap.skills.visibleAgentsHint'),
    },
  ];
  if (skill.invoke_count > 0) {
    const rate = skill.success_count / skill.invoke_count;
    facts.push({ label: t('cap.common.successRate'), value: formatRate(rate), tone: rateTone(rate) });
  } else {
    facts.push({ label: t('cap.skills.usage7d'), value: formatCount(skill.usage_count_7d) });
  }
  return facts;
}

/** 最多 3 个标签（`维度:值` 只显示值），再加来源 */
export function skillTags(skill: Skill): CapTag[] {
  const tags: CapTag[] = (skill.tags ?? []).slice(0, 3).map((tag) => {
    const idx = tag.name.indexOf(':');
    return { label: idx > 0 ? tag.name.slice(idx + 1) || tag.name : tag.name };
  });
  if (skill.sync_origin) tags.push({ label: skillOriginLabel(skill.sync_origin), tone: 'pending' });
  return tags;
}

export function skillRelation(skill: Skill, t: T, relative: (iso: string) => string): string {
  if (!skill.last_invoked_at) return t('cap.skills.relationNever');
  return t('cap.skills.relationLast', {
    agent: skill.last_agent_display_name || t('skillsPage.unknownAgent'),
    when: relative(skill.last_invoked_at),
  });
}
