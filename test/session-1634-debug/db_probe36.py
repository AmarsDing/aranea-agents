"""Probe 36: graph stage status + team stages + tasks for 16:34 session."""
import psycopg2, psycopg2.extras

conn = psycopg2.connect(host="127.0.0.1", port=5432, dbname="aranea",
                        user="postgres", password="Hangshan@123")
cur = conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor)

def cols(table):
    cur.execute("""SELECT column_name FROM information_schema.columns
                   WHERE table_name=%s ORDER BY ordinal_position""", (table,))
    return [r['column_name'] for r in cur.fetchall()]

print("graph_stages_v2 cols:", cols('graph_stages_v2'))
print("team_stages_v2 cols:", cols('team_stages_v2'))
print("tasks_v2 cols:", cols('tasks_v2'))
