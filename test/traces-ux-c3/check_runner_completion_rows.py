"""T9 实测取证：monitor_traces name(domain) 分布 + runner.completion 行画像。"""
import psycopg2

conn = psycopg2.connect(
    host="127.0.0.1", port=5432, dbname="aranea",
    user="postgres", password="Hangshan@123",
)
cur = conn.cursor()

print("== name(domain) 分布 ==")
cur.execute(
    """
    SELECT name, COUNT(*) AS cnt,
           COUNT(*) FILTER (WHERE status='interrupted') AS interrupted,
           COUNT(*) FILTER (WHERE agent_id='') AS no_agent,
           COUNT(*) FILTER (WHERE session_id='') AS no_session,
           COUNT(*) FILTER (WHERE team_id<>'') AS has_team
    FROM monitor_traces WHERE deleted_at=''
    GROUP BY name ORDER BY cnt DESC
    """
)
for r in cur.fetchall():
    print(f"  name={r[0]!r:22} cnt={r[1]:<5} interrupted={r[2]:<4} no_agent={r[3]:<4} no_session={r[4]:<4} has_team={r[5]}")

print("\n== runner.completion 行样本 ==")
cur.execute(
    """
    SELECT id, status, agent_id, session_id, team_id, run_id, duration_ms, total_tokens, created_at
    FROM monitor_traces WHERE name='runner.completion' AND deleted_at=''
    ORDER BY created_at DESC LIMIT 8
    """
)
for r in cur.fetchall():
    print(f"  id={r[0][:12]} status={r[1]:<12} agent={r[2]!r} sess={r[3][:8]!r} team={r[4]!r} run={r[5][:8]!r} dur={r[6]} tok={r[7]} created={r[8]}")

print("\n== runner.completion 行的 session/agent 是否还能解析 ==")
cur.execute(
    """
    SELECT COUNT(*) AS total,
           COUNT(*) FILTER (WHERE s.id IS NOT NULL) AS session_alive,
           COUNT(*) FILTER (WHERE a.id IS NOT NULL) AS agent_alive
    FROM monitor_traces t
    LEFT JOIN sessions s ON s.id = t.session_id
    LEFT JOIN agents a ON a.id = t.agent_id
    WHERE t.name='runner.completion' AND t.deleted_at=''
    """
)
r = cur.fetchone()
print(f"  total={r[0]} session_alive={r[1]} agent_alive={r[2]}")

conn.close()
