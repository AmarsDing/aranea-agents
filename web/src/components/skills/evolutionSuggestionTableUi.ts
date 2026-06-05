import type { QTableColumn } from 'quasar';
import type { SkillEvolutionSuggestionMsg } from '../../services/kratos/skill_evolution_suggestion/v1/index';
import { REGISTRY_COL_W, registryCol, registryColActions } from '../../features/ui/registryTableColumns';

/** EvolutionSuggestionTable 列定义 */
export const EVOLUTION_SUGGESTION_TABLE_COLUMNS: QTableColumn<SkillEvolutionSuggestionMsg>[] = [
  registryCol<SkillEvolutionSuggestionMsg>('skillId', 'Skill ID', 'skillId', 'left', REGISTRY_COL_W.name),
  registryCol<SkillEvolutionSuggestionMsg>('type', '类型', 'type', 'left', REGISTRY_COL_W.category),
  registryCol<SkillEvolutionSuggestionMsg>('status', '状态', 'status', 'left', REGISTRY_COL_W.status),
  registryCol<SkillEvolutionSuggestionMsg>('triggerReason', '触发原因', 'triggerReason', 'left', REGISTRY_COL_W.desc),
  registryCol<SkillEvolutionSuggestionMsg>('sandboxPassed', '沙箱验证', 'sandboxPassed', 'center', REGISTRY_COL_W.metric),
  registryCol<SkillEvolutionSuggestionMsg>('createdAt', '创建时间', 'createdAt', 'left', REGISTRY_COL_W.timeWide),
  registryColActions<SkillEvolutionSuggestionMsg>(),
];
