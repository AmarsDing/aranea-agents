"""Probe 22: member sessions created after 17:05 restart — task_id filled?"""
import psycopg2, psycopg2.extras

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)

def q(label, sql, args=()):
    print(f"\n===== {label} =====")
    try:
        cur.execute(sql, args)
        rows = cur.fetchall()
        for r in rows:
            print({k: (str(v)[:300] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

q("member_sessions_v2 started after 17:00 today", """
SELECT id, task_id, session_id, agent_key, status, error, version, started_at, finished_at
FROM member_sessions_v2
WHERE started_at >= '2026-07-28 17:00:00+08'
ORDER BY started_at DESC LIMIT 20
""")

q("all member_sessions_v2 task_id fill stats", """
SELECT (task_id = '') AS empty_task_id, count(*)
FROM member_sessions_v2 GROUP BY 1
""")

q("graph node / stage status update times", """
SELECT id, status FROM graph_nodes_v2
WHERE graph_stage_id = '86677865-60ce-5ed6-b9e0-40c281f7a160'
""")

conn.close()
