"""Dump playwright MCP server config_json to learn the config schema."""
import psycopg2

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor()
cur.execute("SELECT config_json, metadata_json FROM mcp_server WHERE server_key='playwright'")
for r in cur.fetchall():
    print("config_json:", r[0])
    print("metadata_json:", r[1])
conn.close()
