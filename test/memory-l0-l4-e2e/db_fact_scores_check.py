import psycopg2
conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()
cur.execute("""SELECT id, importance, confidence, recalled_count, injected_count, last_used_at
               FROM memory_facts WHERE id IN ('fts-smoke-fact-0001','78cea0f1-d06f-44f4-8a37-1c7007e9d44a','ac53d99f-0da1-4209-8801-9d56e0abca33')""")
for r in cur.fetchall():
    print(r)
conn.close()
