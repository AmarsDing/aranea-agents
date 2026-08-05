import psycopg2

SID = '48f311e3-ab69-4823-ba35-fd63767471bd'
conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()

cur.execute("""SELECT table_name FROM information_schema.tables
               WHERE table_schema='public' AND (table_name LIKE '%turn%' OR table_name LIKE '%task%')""")
print('turn/task tables:', cur.fetchall())

for t in ('turns', 'turns_v2', 'tasks_v2'):
    try:
        cur.execute(f"SELECT count(*) FROM {t} WHERE session_id=%s", (SID,))
        print(t, 'rows:', cur.fetchone())
    except Exception as e:
        conn.rollback()
        print(t, 'ERR', e)
conn.close()
