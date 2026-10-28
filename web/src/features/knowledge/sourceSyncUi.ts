import type { SourceConnector, SourceCredentialView, SourceSyncItem, SourceSyncRun } from './types';

export type SourceConnectorForm = {
  kind: string;
  name: string;
  root: string;
  credential_key: string;
  query: string;
  interval_seconds: number;
  enabled: boolean;
  max_items_per_run: number;
  max_deletes_per_run: number;
  max_fetch_bytes_per_run: number;
};

export type SourceCredentialForm = { key: string; secret: string };

export interface SourceSyncPanelView {
  connectors: SourceConnector[];
  loading: boolean;
  loaded: boolean;
  loadError: string;
  actionError: string;
  busy: Record<string, string>;
  runsByConn: Record<string, SourceSyncRun[]>;
  missingByConn: Record<string, SourceSyncItem[]>;
  credsByConn: Record<string, SourceCredentialView[]>;
  detailLoading: Record<string, boolean>;
  detailErrors: Record<string, string>;
  credForm: Record<string, SourceCredentialForm>;
  dialogOpen: boolean;
  editing: SourceConnector | null;
  form: SourceConnectorForm;
}

export const emptySourceConnectorForm = (): SourceConnectorForm => ({
  kind: 'shared_dir', name: '', root: '', credential_key: 'default', query: '',
  interval_seconds: 900, enabled: true,
  max_items_per_run: 500, max_deletes_per_run: 200, max_fetch_bytes_per_run: 512 * 1024 * 1024,
});
