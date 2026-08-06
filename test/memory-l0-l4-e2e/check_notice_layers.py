import psycopg2, json
conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()
cur.execute("""SELECT started_at, content FROM steps_v2
               WHERE kind='notice' AND notice_type='memory_recalled'
               ORDER BY started_at DESC LIMIT 8""")
for ts, content in cur.fetchall():
    try:
        hits = json.loads(content).get('hits', [])
        layers = [h.get('layer') for h in hits]
        print(ts.strftime('%m-%d %H:%M:%S'), layers)
    except Exception:
        print(ts, 'parse-fail')
conn.close()
