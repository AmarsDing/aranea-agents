import {
  registryCol,
  registryColEnabled,
  registryColActions,
  REGISTRY_COL_W,
} from '../../features/ui/registryTableColumns';
import type { WebhookRow } from '../../features/webhooks/types';

export function createWebhookColumns(t: (key: string) => string) {
  return [
    registryCol<WebhookRow>('name', t('webhooksPage.colName'), 'name', 'left', REGISTRY_COL_W.nameWide),
    registryCol<WebhookRow>('events', t('webhooksPage.colEvents'), 'event_types_json', 'left', REGISTRY_COL_W.stats, {
      sortable: false,
    }),
    registryColEnabled<WebhookRow>(t('webhooksPage.colEnabled')),
    registryColActions<WebhookRow>(),
  ];
}
