"""List current MCP servers in the platform DB."""
import json

import psycopg2

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor()
cur.execute("SELECT server_key, name, status, enabled, workspace_id FROM mcp_server WHERE deleted_at='' OR deleted_at IS NULL ORDER BY sort_order")
rows = cur.fetchall()
print(f"total: {len(rows)}")
for r in rows:
    print(json.dumps({"server_key": r[0], "name": r[1], "status": r[2], "enabled": r[3], "workspace_id": r[4]}, ensure_ascii=False))
conn.close()
