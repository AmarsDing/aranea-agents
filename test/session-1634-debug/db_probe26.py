"""Probe 26: what actually happened inside the two failed team sessions."""
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
            print({k: (str(v)[:700] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

q("0. candidate tables", """
SELECT table_name FROM information_schema.tables
WHERE table_schema='public' AND (table_name LIKE '%message%' OR table_name LIKE '%team_run%' OR table_name LIKE '%chat%')
ORDER BY 1
""")

q("0b. team_runs_v2 columns", """
SELECT column_name FROM information_schema.columns
WHERE table_name='team_runs_v2' ORDER BY ordinal_position
""")

for t in ("chat_message", "chat_messages", "message", "messages"):
    q(f"table {t} exists?", """
    SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_name=%s
    """, (t,))

for sid, name in [("3c730e21-c061-44d6-a56f-99fc8c48f8c9", "pdf team"),
                  ("bdeda93d-f5e4-441c-8dd1-c747cd702cca", "docx team")]:
    q(f"chat_message of {name} session", """
    SELECT role, left(content_markdown, 500) AS content, status, error_message, created_at
    FROM chat_message WHERE session_id = %s ORDER BY created_at LIMIT 20
    """, (sid,))

q("team_runs_v2 for the two teams", """
SELECT id, task_id, status, left(COALESCE(error,''), 400) AS error, started_at, completed_at
FROM team_runs_v2
WHERE team_stage_id IN (
  SELECT id FROM team_stages_v2 WHERE team_id IN ('8455b32294f5a840220c0fdf', '3c1676eee233b3f049ce673a')
)
ORDER BY started_at
""")
