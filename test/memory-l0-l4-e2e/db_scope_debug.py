import psycopg2
conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()
# scopes of facts that WERE recalled
cur.execute("""
    SELECT id, scope_type, scope_id, user_id, agent_id, left(statement,60)
    FROM memory_facts
    WHERE id IN ('78cea0f1-d06f-44f4-8a37-1c7007e9d44a','ac53d99f-0da1-4209-8801-9d56e0abca33')
""")
print('recalled facts scope:')
for r in cur.fetchall(): print(' ', r)
# count facts in agent scope for spirit + user 1
cur.execute("""
    SELECT scope_type, scope_id, user_id, COUNT(*) FROM memory_facts
    WHERE status='active' AND deleted_at='' AND valid_until=''
    GROUP BY 1,2,3 ORDER BY 4 DESC
""")
print('all active fact scopes:')
for r in cur.fetchall(): print(' ', r)
# spirit agent memory settings
cur.execute("""
    SELECT l3_recall_scopes, l3_min_score_query, l3_min_score_passive, l2_recall_enabled, l3_recall_budget_tokens
    FROM agent_runtime_settings WHERE agent_id='agent___spirit__'
""")
print('spirit runtime settings:', cur.fetchall())
conn.close()
