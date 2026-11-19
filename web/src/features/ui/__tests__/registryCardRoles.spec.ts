import { describe, expect, it } from 'vitest';
import type { ComposerTranslation } from 'vue-i18n';
import {
  channelDeliveriesColumns,
  channelTableColumns,
  channelTurnJobsColumns,
} from '../../../components/channels/channelUi';
import {
  createHookDeliveryTableColumns,
  createHooksAgentTableColumns,
  createHooksTableColumns,
} from '../../../components/hooks/hookTableUi';
import { MCP_SERVER_TABLE_COLUMNS } from '../../../components/mcp/mcpServerTableUi';
import {
  PLATFORM_RESOURCE_TABLE_COLUMNS,
  PROVIDER_MODEL_TABLE_COLUMNS,
} from '../../../components/platform/providerModelUi';
import { createPluginTableColumns } from '../../../components/plugins/pluginUi';
import { sessionsTableColumns } from '../../../components/sessions/sessionUi';
import { EVOLUTION_SUGGESTION_TABLE_COLUMNS } from '../../../components/skills/evolutionSuggestionTableUi';
import { createSkillRunsTableColumns, SKILL_TABLE_COLUMNS } from '../../../components/skills/skillTableUi';
import { toolAuditsTableColumns, toolRunsTableColumns, toolTableColumns } from '../../../components/tools/toolUi';
import { createWebhookColumns } from '../../../components/webhooks/webhookTableUi';
import {
  A2A_AUDIT_TABLE_COLUMNS,
  A2A_CARD_TABLE_COLUMNS,
  A2A_GATEWAY_TABLE_COLUMNS,
  A2A_REMOTE_TABLE_COLUMNS,
} from '../../a2a/a2aTableUi';
import { federationDirectoryColumns, federationOrgColumns } from '../../a2a/federationUi';
import { createArtifactColumns } from '../../artifact/artifactTableUi';
import { CRON_RUN_TABLE_COLUMNS, CRON_TASK_TABLE_COLUMNS } from '../../cron/cronTableUi';
import { createPluginRunsTableColumns } from '../../plugins/pluginRunsTableUi';
import { defaultViewMode, splitColumns, type RegistryColumnLike } from '../registryView';

const t = ((key: string) => key) as unknown as ComposerTranslation & ((key: string) => string);

type Cols = readonly RegistryColumnLike[];
/** 各页列定义带着自己的行类型；这里只关心列名与卡片角色，统一按通用列看待。 */
const asCols = (columns: readonly object[]): Cols => columns as unknown as Cols;

/** 「一行一个对象」的登记表：标了卡片角色，默认卡片视图。 */
const OBJECT_REGISTRIES: Record<string, readonly object[]> = {
  plugins: createPluginTableColumns(t),
  tools: toolTableColumns(),
  skills: SKILL_TABLE_COLUMNS,
  mcp: MCP_SERVER_TABLE_COLUMNS,
  channels: channelTableColumns(t),
  hooks: createHooksTableColumns(t),
  hooksAgent: createHooksAgentTableColumns(t),
  webhooks: createWebhookColumns(t),
  providerModels: PROVIDER_MODEL_TABLE_COLUMNS,
  platformResources: PLATFORM_RESOURCE_TABLE_COLUMNS,
  sessions: sessionsTableColumns,
  cron: CRON_TASK_TABLE_COLUMNS,
  a2aCards: A2A_CARD_TABLE_COLUMNS,
  a2aRemote: A2A_REMOTE_TABLE_COLUMNS,
  a2aGateway: A2A_GATEWAY_TABLE_COLUMNS,
  federationOrgs: federationOrgColumns(t),
  federationDirectory: federationDirectoryColumns(t),
  artifacts: createArtifactColumns(t),
  evolutionSuggestions: EVOLUTION_SUGGESTION_TABLE_COLUMNS,
};

/** 流水类（运行记录、审计、投递、作业）：不标角色，默认表格，便于逐行扫读。 */
const LOG_TABLES: Record<string, readonly object[]> = {
  toolRuns: toolRunsTableColumns(),
  toolAudits: toolAuditsTableColumns(),
  skillRuns: createSkillRunsTableColumns(t),
  pluginRuns: createPluginRunsTableColumns(t),
  hookDeliveries: createHookDeliveryTableColumns(t),
  cronRuns: CRON_RUN_TABLE_COLUMNS,
  channelTurnJobs: channelTurnJobsColumns(t),
  channelDeliveries: channelDeliveriesColumns(t),
  a2aAudit: A2A_AUDIT_TABLE_COLUMNS,
};

describe('登记页卡片角色', () => {
  it.each(Object.entries(OBJECT_REGISTRIES))('%s：一个标题，状态与指标各最多一个，默认卡片', (_name, raw) => {
    const cols = asCols(raw);
    const roles = cols.map((c) => c.cardRole).filter(Boolean);
    expect(roles.filter((r) => r === 'title')).toHaveLength(1);
    expect(roles.filter((r) => r === 'status').length).toBeLessThanOrEqual(1);
    expect(roles.filter((r) => r === 'metric').length).toBeLessThanOrEqual(1);
    const layout = splitColumns(cols);
    expect(layout.title?.cardRole).toBe('title');
    // 卡片不能只剩一个标题：至少还有状态、指标、元信息或正文之一
    expect(Boolean(layout.status || layout.metric || layout.meta.length || layout.body.length)).toBe(true);
    expect(defaultViewMode(cols)).toBe('cards');
  });

  it.each(Object.entries(LOG_TABLES))('%s：流水类不标角色，默认表格', (_name, raw) => {
    const cols = asCols(raw);
    expect(cols.some((c) => c.cardRole)).toBe(false);
    expect(defaultViewMode(cols)).toBe('table');
  });

  it('标题都落在表示「这是谁」的列上', () => {
    const titles = Object.fromEntries(
      Object.entries(OBJECT_REGISTRIES).map(([name, raw]) => [
        name,
        asCols(raw).find((c) => c.cardRole === 'title')?.name,
      ]),
    );
    expect(titles).toMatchObject({
      plugins: 'name',
      tools: 'name',
      skills: 'name',
      sessions: 'session',
      providerModels: 'model',
      a2aRemote: 'display_name',
      evolutionSuggestions: 'targetId',
      federationDirectory: 'agent',
    });
  });
});
