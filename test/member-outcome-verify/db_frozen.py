"""Confirm ID-derivation collision: standalone team's v2 IDs are frozen at first run's terminal."""
import psycopg2

conn = psycopg2.connect(
    host="127.0.0.1", port=5432, dbname="aranea",
    user="postgres", password="Hangshan@123",
)
cur = conn.cursor()
TID = "9755a723338ec796b02c36c9"

print("=== teams.status ===")
cur.execute("SELECT id, display_name, status, auto_created FROM teams WHERE id = %s", (TID,))
for r in cur.fetchall():
    print(r)

print("\n=== team_stages_v2 for team ===")
cur.execute("""
    SELECT id, status, stage, version, task_id, session_id, started_at
    FROM team_stages_v2 WHERE team_id = %s
""", (TID,))
for r in cur.fetchall():
    print(r)

print("\n=== team_runs_v2 for those stages ===")
cur.execute("""
    SELECT tr.id, tr.team_stage_id, tr.status, tr.version, tr.task_id, tr.started_at, tr.completed_at
    FROM team_runs_v2 tr
    JOIN team_stages_v2 ts ON ts.id = tr.team_stage_id
    WHERE ts.team_id = %s
""", (TID,))
for r in cur.fetchall():
    print(r)

print("\n=== member_sessions_v2 for those runs ===")
cur.execute("""
    SELECT ms.id, ms.agent_key, ms.status, ms.version, ms.started_at, ms.finished_at
    FROM member_sessions_v2 ms
    JOIN team_runs_v2 tr ON tr.id = ms.team_run_id
    JOIN team_stages_v2 ts ON ts.id = tr.team_stage_id
    WHERE ts.team_id = %s
    ORDER BY ms.agent_key
""", (TID,))
for r in cur.fetchall():
    print(r)

print("\n=== sessions of team: how many turns (user messages) each ===")
cur.execute("""
    SELECT id, session_type, status, member_agent_key, created_at
    FROM sessions WHERE team_id = %s AND deleted_at = ''
    ORDER BY created_at
""", (TID,))
for r in cur.fetchall():
    print(r)

cur.close()
conn.close()
