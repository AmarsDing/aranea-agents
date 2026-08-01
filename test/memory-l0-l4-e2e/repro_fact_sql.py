import psycopg2

conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()

# 1. memory_facts 列清单（含 version?）
cur.execute("""SELECT column_name, data_type FROM information_schema.columns
               WHERE table_name='memory_facts' ORDER BY ordinal_position""")
cols = [r[0] for r in cur.fetchall()]
print('memory_facts cols:', cols)
print('has version col:', 'version' in cols)

# 2. 唯一索引
cur.execute("""SELECT indexname, indexdef FROM pg_indexes WHERE tablename='memory_facts'""")
for r in cur.fetchall():
    print(r[0], '::', r[1][:200])

# 3. 直接重现最小 SQL（一个显式 version 引用歧义场景）
cur.execute("SAVEPOINT s1")
try:
    cur.execute("""
        INSERT INTO memory_facts (id, scope_type, scope_id, workspace_id, user_id, team_id, agent_id,
            statement, statement_normalized, fingerprint, fact_kind, tags_json,
            confidence, importance, version, status, pii_flag, redacted_statement,
            quality_score, metadata_json, created_at, updated_at, archived_at, deleted_at,
            valid_from, valid_until, links, keywords, tags)
        VALUES ('repro-1','agent','repro_agent','','u','','repro_agent',
            'repro statement','repro statement','fp-repro-1','fact','[]',
            0.9,0.8,1,'active',0,'',
            0.5,'{}','2026-07-30T00:00:00Z','2026-07-30T00:00:00Z','','',
            '2026-07-30T00:00:00Z','','[]','[]','[]')
        ON CONFLICT(scope_type, scope_id, fingerprint) DO UPDATE SET
            statement = excluded.statement,
            version = memory_facts.version + 1,
            updated_at = excluded.updated_at
    """)
    print('repro insert OK')
except Exception as e:
    print('repro ERROR:', e)
finally:
    cur.execute("ROLLBACK TO SAVEPOINT s1")

conn.close()
