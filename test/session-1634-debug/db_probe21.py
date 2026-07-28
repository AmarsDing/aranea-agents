"""Probe 21: plan steps detail (plan_id column)."""
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
            print({k: (str(v)[:1200] if v is not None else None) for k, v in r.items()})
        if not rows:
            print("(empty)")
    except Exception as e:
        conn.rollback()
        print("ERR:", e)

q("plan_steps for board", """
SELECT id, plan_id, task_id, label, description, depends_on, mapped_team_stage_id,
       status, error, agent_keys, deliverables, input_contract, result, seq, version
FROM plan_steps_v2 WHERE plan_id = 'pb_tp_63f7f8ac-651b-4eb2-ad46-a7d866bffaf8'
ORDER BY seq
""")

conn.close()
