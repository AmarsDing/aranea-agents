import psycopg2

conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()
cur.execute("""SELECT column_name FROM information_schema.columns
               WHERE table_name='agent_runtime_settings'
               AND (column_name LIKE '%l4%' OR column_name LIKE '%memory%' OR column_name LIKE '%l3%')
               ORDER BY column_name""")
cols = [r[0] for r in cur.fetchall()]
print('columns:', cols)
if cols:
    cur.execute(f"SELECT {', '.join(cols)} FROM agent_runtime_settings WHERE agent_id='f2e5a24ab0756d6413d6a1a3'")
    row = cur.fetchone()
    if row:
        for c, v in zip(cols, row):
            print(f'  {c} = {v}')
    else:
        print('no row for agent')
conn.close()
