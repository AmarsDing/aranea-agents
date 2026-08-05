import psycopg2

conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()

cur.execute("""SELECT id, task_id, session_id, seq, status FROM turns_v2
               WHERE id='47f3d50e-361f-42d8-b237-13c7cfb01959'""")
print('turn 47f3d50e:', cur.fetchall())

cur.execute("""SELECT id, turn_id, kind, notice_type, status, seq FROM steps_v2
               WHERE turn_id='47f3d50e-361f-42d8-b237-13c7cfb01959' ORDER BY seq""")
print('steps:')
for r in cur.fetchall():
    print(' ', r)

# all turns for task 7fdb1f61
cur.execute("""SELECT id, seq, status, started_at FROM turns_v2
               WHERE task_id='7fdb1f61-8c69-4c97-ab34-a13a91e7658e' ORDER BY seq""")
print('turns of task 7fdb1f61:')
for r in cur.fetchall():
    print(' ', r)
conn.close()
