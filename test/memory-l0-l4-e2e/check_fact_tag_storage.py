# 确认 <fact> 标签残留在哪个存储：steps_v2 vs 其他
import psycopg2
conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()
SID = '685dfbcb-f7e0-40f7-9792-003bfe2405ca'

cur.execute("""SELECT id, kind, left(content, 400) FROM steps_v2
               WHERE session_id=%s AND kind='reply' ORDER BY started_at""", (SID,))
rows = cur.fetchall()
print(f'reply steps: {len(rows)}')
for r in rows:
    has_tag = '<fact' in (r[2] or '')
    print(f'  step={r[0][:8]} has_fact_tag={has_tag}')
    print(f'    content_tail=...{(r[2] or "")[-180:]}')

# v1 messages 表是否还存在/有数据
cur.execute("""SELECT count(*) FROM information_schema.tables WHERE table_name='messages'""")
print('messages table exists:', cur.fetchone())
conn.close()
