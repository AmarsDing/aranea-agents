import { REGISTRY_COL_W, registryCol } from '../../features/ui/registryTableColumns';

/** AgentSettings — Prompt 组装预览表 */
export const AGENT_PROMPT_ASSEMBLY_TABLE_COLUMNS = [
  registryCol('label', '区块', 'label', 'left', '20%'),
  registryCol('source', '来源', 'source', 'left', REGISTRY_COL_W.category),
  registryCol('est_tokens', '估算 tokens', 'est_tokens', 'right', REGISTRY_COL_W.metric),
];
