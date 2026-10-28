import { registryCol, registryColActions } from './registryTableColumns';

export interface ThemePreviewRow {
  id: string;
  name: string;
  description: string;
  state: string;
  color: string;
  updated: string;
}

export function createThemePreviewUi(t: (key: string) => string) {
  const columns = [
    { ...registryCol('name', t('uiSystem.preview.name'), 'name', 'left', '42%'), sortable: true },
    { ...registryCol('state', t('uiSystem.preview.state'), 'state', 'left', '120px'), sortable: true },
    registryCol('updated', t('uiSystem.preview.updated'), 'updated', 'left', '180px'),
    registryColActions('96px'),
  ];

  const rows: ThemePreviewRow[] = [
    {
      id: 'design',
      name: t('uiSystem.preview.designName'),
      description: t('uiSystem.preview.designNote'),
      state: t('uiSystem.preview.available'),
      color: 'positive',
      updated: '2026-10-26 09:40',
    },
    {
      id: 'review',
      name: t('uiSystem.preview.reviewName'),
      description: t('uiSystem.preview.reviewNote'),
      state: t('uiSystem.preview.pending'),
      color: 'warning',
      updated: '2026-10-26 09:25',
    },
    {
      id: 'sync',
      name: t('uiSystem.preview.syncName'),
      description: t('uiSystem.preview.syncNote'),
      state: t('uiSystem.preview.failed'),
      color: 'negative',
      updated: '2026-10-26 09:10',
    },
  ];
  return { columns, rows };
}
