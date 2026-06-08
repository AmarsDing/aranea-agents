import type { QTableColumn } from 'quasar';
import type { Agent } from '../../features/agents/types';
import type { AgentToolOverrideRow } from '../../features/agents/useAgentToolOverrides';
import { REGISTRY_COL_W, registryCol, registryColActions } from '../../features/ui/registryTableColumns';
import { deriveMemoryToolMode } from './agentUi';

/** AgentsListSection 列定义（taxonomyLabel 由 store computed 注入 field） */
export function buildAgentTableColumns(
  taxonomyLabel: (id: string) => string,
  formatContext: (value?: number) => string,
): QTableColumn<Agent>[] {
  return [
    registryCol<Agent>('name', '名称', 'display_name', 'left', REGISTRY_COL_W.name),
    registryCol<Agent>('status', '状态', 'status', 'left', REGISTRY_COL_W.status),
    registryCol<Agent>('model', '模型', (row) => `${row.provider} / ${row.model}`, 'left', REGISTRY_COL_W.stats),
    registryCol<Agent>(
      'category',
      '业务分类',
      (row) => taxonomyLabel(row.taxonomy_position_id),
      'left',
      REGISTRY_COL_W.category,
    ),
    registryCol<Agent>('ctx', '上下文', (row) => formatContext(row.context_window), 'left', REGISTRY_COL_W.status),
    registryCol<Agent>(
      'memory_mode',
      '记忆模式',
      (row) => deriveMemoryToolMode(row.settings?.tools_deny_json),
      'left',
      REGISTRY_COL_W.category,
    ),
    registryColActions<Agent>(),
  ];
}

/** AgentToolOverridesPanel 列定义 */
export const AGENT_TOOL_OVERRIDE_TABLE_COLUMNS: QTableColumn<AgentToolOverrideRow>[] = [
  registryCol<AgentToolOverrideRow>('tool_key', '工具 Key', 'tool_key', 'left', '12%'),
  registryCol<AgentToolOverrideRow>('display_name', '名称', 'display_name', 'left', REGISTRY_COL_W.name),
  registryCol<AgentToolOverrideRow>('effective_state', '生效', 'effective_state', 'left', REGISTRY_COL_W.status),
  registryCol<AgentToolOverrideRow>(
    'requires_confirmation',
    '确认',
    'effective_requires_confirmation',
    'center',
    REGISTRY_COL_W.narrow,
  ),
  registryCol<AgentToolOverrideRow>('override', '覆盖', 'override', 'left', REGISTRY_COL_W.category),
  registryColActions<AgentToolOverrideRow>(REGISTRY_COL_W.actions, '', (row) => row.tool_key),
];

/** AgentSettings — Prompt 组装预览表 */
export const AGENT_PROMPT_ASSEMBLY_TABLE_COLUMNS = [
  registryCol('label', '区块', 'label', 'left', '20%'),
  registryCol('source', '来源', 'source', 'left', REGISTRY_COL_W.category),
  registryCol('est_tokens', '估算 tokens', 'est_tokens', 'right', REGISTRY_COL_W.metric),
];
