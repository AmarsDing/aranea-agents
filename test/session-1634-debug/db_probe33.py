"""Probe 33: graph_nodes_v2 status via graph_stage_id."""
import psycopg2, psycopg2.extras

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)

cur.execute("""
SELECT id, graph_stage_id, dag_node_id, team_stage_id, label, status
FROM graph_nodes_v2 WHERE graph_stage_id = '86677865-60ce-5ed6-b9e0-40c281f7a160'
""")
for r in cur.fetchall():
    print({k: (str(v)[:200] if v is not None else None) for k, v in r.items()})
