-- MCP Broker ad-hoc HTTP platform toggle (system_settings singleton).
ALTER TABLE system_settings ADD COLUMN mcp_allow_adhoc_http INTEGER NOT NULL DEFAULT 0;
