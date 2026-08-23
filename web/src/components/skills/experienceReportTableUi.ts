import type { QTableColumn } from 'quasar';
import type { ExperienceReportView } from '../../features/skills/types';
import { REGISTRY_COL_W, registryCol } from '../../features/ui/registryTableColumns';

/** ExperienceReportTable 列定义 */
export const EXPERIENCE_REPORT_TABLE_COLUMNS: QTableColumn<ExperienceReportView>[] = [
  registryCol<ExperienceReportView>('_expand', '', 'id', 'center', '40px', { sortable: false }),
  registryCol<ExperienceReportView>('skillName', 'Skill 名称', 'skillName', 'left', REGISTRY_COL_W.name),
  registryCol<ExperienceReportView>('result', '结果', 'isSuccess', 'center', REGISTRY_COL_W.status),
  registryCol<ExperienceReportView>('score', '评分', 'score', 'center', REGISTRY_COL_W.metric),
  registryCol<ExperienceReportView>('failureTags', '失败标签', 'failureTags', 'left', REGISTRY_COL_W.category),
  registryCol<ExperienceReportView>('flowSummary', '流程摘要', 'flowSummary', 'left', REGISTRY_COL_W.desc),
  registryCol<ExperienceReportView>('createdAt', '创建时间', 'createdAt', 'left', REGISTRY_COL_W.timeWide),
];
