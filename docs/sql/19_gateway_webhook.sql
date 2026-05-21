-- Gateway outbound webhooks (run.completed / run.failed / run.cancelled)
CREATE TABLE IF NOT EXISTS gateway_webhooks (
    id               TEXT PRIMARY KEY,
    name             TEXT NOT NULL,
    url              TEXT NOT NULL,
    event_types_json TEXT NOT NULL DEFAULT '[]',
    secret           TEXT NOT NULL DEFAULT '',
    headers_json     TEXT NOT NULL DEFAULT '{}',
    enabled          INTEGER NOT NULL DEFAULT 1,
    created_at       TEXT NOT NULL DEFAULT '',
    updated_at       TEXT NOT NULL DEFAULT ''
);
