import psycopg2
conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()
cur.execute("SELECT recalled_count, injected_count, cited_count, use_count, last_used_at FROM memory_facts WHERE id='fts-smoke-fact-0001'")
print('fts fact counters (recalled, injected, cited, use, last_used):', cur.fetchone())
cur.execute("SELECT id, recalled_count, injected_count FROM memory_facts WHERE id IN ('78cea0f1-d06f-44f4-8a37-1c7007e9d44a','ac53d99f-0da1-4209-8801-9d56e0abca33')")
print('other recalled facts:', cur.fetchall())
conn.close()
