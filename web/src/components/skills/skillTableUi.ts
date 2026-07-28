import type { QTableColumn } from 'quasar';
import type { Skill, SkillInvocation } from '../../features/skills/types';
import {
  REGISTRY_COL_W,
  registryCol,
  registryColActions,
  registryColEnabled,
} from '../../features/ui/registryTableColumns';

/** SkillTable 列定义 */
export const SKILL_TABLE_COLUMNS: QTableColumn<Skill>[] = [
  registryCol<Skill>('name', '名称', 'name', 'left', REGISTRY_COL_W.nameWide),
  registryCol<Skill>('tags', '标签', 'tags', 'left', '16%'),
  registryCol<Skill>('origin', '来源', 'sync_origin', 'left', '96px'),
  registryCol<Skill>('disk', '磁盘', 'filesystem_missing', 'center', REGISTRY_COL_W.metric),
  registryCol<Skill>('status', '状态 / 版本', 'status', 'left', REGISTRY_COL_W.status),
  registryColEnabled<Skill>(),
  registryCol<Skill>('stats', '使用统计', 'invoke_count', 'left', '220px'),
  registryCol<Skill>('last', '最近调用', 'last_invoked_at', 'left', '150px'),
  registryColActions<Skill>(),
];

/** SkillRunsTable 列定义 */
export const SKILL_RUNS_TABLE_COLUMNS: QTableColumn<SkillInvocation>[] = [
  registryCol<SkillInvocation>('time', '时间 / 耗时', 'started_at', 'left', REGISTRY_COL_W.time),
  registryCol<SkillInvocation>('skill', 'Skill', 'skill_name', 'left', REGISTRY_COL_W.name),
  registryCol<SkillInvocation>('agent', 'Agent', 'agent_display_name', 'left', REGISTRY_COL_W.agent),
  registryCol<SkillInvocation>('status', '结果', 'status', 'left', REGISTRY_COL_W.status),
];

/** Skill 状态标签 */
export function skillStatusLabel(status: string) {
  return ({ draft: '草稿', published: '已发布', archived: '已归档' } as Record<string, string>)[status] ?? status;
}

/** Skill 状态颜色 */
export function skillStatusColor(status: string) {
  return status === 'published' ? 'positive' : status === 'draft' ? 'warning' : 'grey';
}

/** Skill 来源标签 */
export function skillOriginLabel(origin: string) {
  return ({ filesystem: '磁盘', import: 'ZIP', manual: '手动' } as Record<string, string>)[origin] ?? origin;
}

/** Skill 导入候选项状态颜色 */
export function skillCandidateStatusColor(status: string) {
  return status === 'pass' ? 'positive' : status === 'warn' ? 'warning' : 'negative';
}

/** Skill 导入候选项图标 */
export function skillCandidateIcon(status: string) {
  return status === 'pass' ? 'check_circle' : status === 'warn' ? 'merge_type' : 'error';
}
