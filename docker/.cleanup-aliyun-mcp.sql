BEGIN;
SELECT COUNT(*) AS to_delete FROM mcp_server
 WHERE deleted_at='' AND server_key IN ('alibaba-cloud-ops','aliyun-observability-sls','alibabacloud-rds-openapi');
UPDATE mcp_server
   SET deleted_at=to_char(now() AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),
       updated_at=to_char(now() AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
 WHERE deleted_at='' AND server_key IN ('alibaba-cloud-ops','aliyun-observability-sls','alibabacloud-rds-openapi');
SELECT server_key, status, enabled, deleted_at FROM mcp_server
 WHERE server_key IN ('alibaba-cloud-ops','aliyun-observability-sls','alibabacloud-rds-openapi');
COMMIT;
