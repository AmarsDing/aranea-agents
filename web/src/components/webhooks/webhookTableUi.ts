import {
  registryCol,
  registryColEnabled,
  registryColActions,
  REGISTRY_COL_W,
  withCardRole,
} from '../../features/ui/registryTableColumns';
import type { WebhookRow } from '../../features/webhooks/types';

export function createWebhookColumns(t: (key: string) => string) {
  return [
    registryCol<WebhookRow>('name', t('webhooksPage.colName'), 'name', 'left', REGISTRY_COL_W.nameWide, {
      cardRole: 'title',
    }),
    registryCol<WebhookRow>('events', t('webhooksPage.colEvents'), 'event_types_json', 'left', REGISTRY_COL_W.stats, {
      sortable: false,
    }),
    withCardRole(registryColEnabled<WebhookRow>(t('webhooksPage.colEnabled')), 'status'),
    registryColActions<WebhookRow>(),
  ];
}
