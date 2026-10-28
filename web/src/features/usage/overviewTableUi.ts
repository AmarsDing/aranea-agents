import { registryCol, REGISTRY_COL_W } from '../ui/registryTableColumns';

export function buildOverviewAgentColumns(t: (key: string) => string) {
  return [
    registryCol('agent', t('overviewPage.v2.orgColAgent'), 'name', 'left', REGISTRY_COL_W.nameWide),
    registryCol('calls', t('overviewPage.v2.orgColCalls'), 'calls', 'right', REGISTRY_COL_W.metric),
    registryCol('rate', t('overviewPage.v2.orgColRate'), 'successRate', 'right', REGISTRY_COL_W.status),
  ];
}
