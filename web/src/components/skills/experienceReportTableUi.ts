import type { QTableColumn } from 'quasar';
import type { ExperienceReport } from '../../services/kratos/skill_intelligence/v1/index';
import { REGISTRY_COL_W, registryCol } from '../../features/ui/registryTableColumns';

/** ExperienceReportTable 列定义 */
export const EXPERIENCE_REPORT_TABLE_COLUMNS: QTableColumn<ExperienceReport>[] = [
  registryCol<ExperienceReport>('skillId', 'Skill ID', 'skillId', 'left', REGISTRY_COL_W.name),
  registryCol<ExperienceReport>('result', '结果', 'isSuccess', 'center', REGISTRY_COL_W.status),
  registryCol<ExperienceReport>('score', '评分', 'score', 'center', REGISTRY_COL_W.metric),
  registryCol<ExperienceReport>('failureTags', '失败标签', 'failureTags', 'left', REGISTRY_COL_W.category),
  registryCol<ExperienceReport>('flowSummary', '流程摘要', 'flowSummary', 'left', REGISTRY_COL_W.desc),
  registryCol<ExperienceReport>('createdAt', '创建时间', 'createdAt', 'left', REGISTRY_COL_W.timeWide),
];
