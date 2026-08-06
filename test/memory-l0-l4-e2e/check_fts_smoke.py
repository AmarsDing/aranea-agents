"""P2-3 FTS smoke test (end-to-end via chat API).

Flow:
  setup  — create a spirit chat session + insert a fact whose distinctive
           content is an alphanumeric code (vector-weak, FTS-strong) with NO
           embedding, so only the FTS channel can surface it.
  send   — send a chat message containing the exact code.
  check  — verify the fact appears in the memory_recalled notice and its
           recalled_count incremented.
"""
import psycopg2, sys, time, json, urllib.request, urllib.error

DSN = dict(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
BASE = 'http://localhost:8000'
AGENT_ID = 'agent___spirit__'
SESSION_USER_ID = '1'  # auth middleware injects admin user 1 (CHAT_NATIVE_FORBIDDEN otherwise)
FACT_USER_ID = 'default_user'  # memory recall runtime user = TRPCUserKey → default_user
CODE = 'FALCON-7X-2024'
FACT_ID = 'fts-smoke-fact-0001'
SESSION_FILE = 'fts_smoke_session.txt'

def db():
    return psycopg2.connect(**DSN)

def req(method, path, body=None, timeout=300):
    data = json.dumps(body).encode() if body is not None else None
    r = urllib.request.Request(BASE + path, data=data, method=method,
                               headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(r, timeout=timeout) as resp:
            return resp.status, json.loads(resp.read().decode() or "{}")
    except urllib.error.HTTPError as e:
        return e.code, {"raw": e.read().decode()[:500]}
    except Exception as e:
        return -1, {"error": str(e)}

def setup():
    # 1. create chat session
    s, d = req("POST", "/v1/sessions", {
        "agent_id": AGENT_ID, "title": "fts-smoke", "user_id": SESSION_USER_ID})
    sid = d.get("id") or d.get("session", {}).get("id")
    print('create session:', s, sid, str(d)[:200])
    if not sid:
        sys.exit(1)
    with open(SESSION_FILE, 'w') as f:
        f.write(sid)

    # 2. insert FTS-strong fact (no embedding → vector channel cannot find it)
    conn = db()
    cur = conn.cursor()
    now = time.strftime('%Y-%m-%dT%H:%M:%S')
    stmt = f'用户的内部系统登录账号是 {CODE}，用于访问 Falcon 控制台'
    cur.execute("DELETE FROM memory_facts WHERE id=%s OR fingerprint='fts-smoke-fp-0001'", (FACT_ID,))
    cur.execute("""
        INSERT INTO memory_facts (
            id, scope_type, scope_id, workspace_id, user_id, team_id, agent_id,
            statement, statement_normalized, fingerprint, details_markdown,
            fact_kind, tags_json,
            confidence, importance, use_count, hit_count,
            positive_feedback_count, negative_feedback_count, conflict_count,
            source_kind, source_episode_id, source_session_id, source_message_id, source_external,
            version, status, superseded_by,
            embedding_status, embedding_model, embedding_dim, embedding_blob, embedding_norm,
            pii_flag, redacted_statement,
            ttl_days, decay_factor, next_decay_at, last_used_at, expires_at,
            quality_score, pii_types, metadata_json, created_at, updated_at,
            archived_at, deleted_at,
            valid_from, valid_until, links, keywords, tags,
            context_note
        ) VALUES (
            %s, 'agent', %s, '', %s, '', %s,
            %s, %s, %s, '',
            'profile', '[]',
            0.9, 0.8, 0, 0,
            0, 0, 0,
            'manual_smoke', '', %s, '', '',
            1, 'active', '',
            'skipped', '', 0, NULL, 0,
            0, '',
            0, 1.0, '', '', '',
            0.8, '[]', '{}', %s, %s,
            '', '',
            %s, '', '[]', '[]', '[]',
            ''
        )
        ON CONFLICT (scope_type, scope_id, fingerprint) DO UPDATE SET
            statement = EXCLUDED.statement, updated_at = EXCLUDED.updated_at
    """, (FACT_ID, AGENT_ID, FACT_USER_ID, AGENT_ID, stmt, stmt.lower(), 'fts-smoke-fp-0001',
          sid, now, now, now))
    conn.commit()

    # 3. verify FTS channel sees it (same query shape as searchL3FTS)
    cur.execute("""
        SELECT id FROM memory_facts
        WHERE status='active' AND deleted_at='' AND valid_until=''
          AND to_tsvector('simple', statement || ' ' || COALESCE(details_markdown,''))
              @@ plainto_tsquery('simple', %s)
          AND scope_type='agent' AND scope_id=%s AND user_id=%s
        ORDER BY ts_rank(to_tsvector('simple', statement || ' ' || COALESCE(details_markdown,'')),
                         plainto_tsquery('simple', %s)) DESC
        LIMIT 30
    """, (CODE, AGENT_ID, FACT_USER_ID, CODE))
    hits = [r[0] for r in cur.fetchall()]
    print('FTS channel hits:', hits)
    print('FTS_OK' if FACT_ID in hits else 'FTS_MISS')
    print('session_id:', sid)
    conn.close()

def send():
    with open(SESSION_FILE) as f:
        sid = f.read().strip()
    s, d = req("POST", "/v1/chat/messages",
               {"session_id": sid, "content": f"我的内部系统登录账号是什么？提示：代号 {CODE}"})
    print(f"status={s}")
    print(f"resp[:400]={str(d)[:400]}")
    am = d.get("agent_message") or {}
    txt = am.get("content") or d.get("content") or ""
    print(f"reply[:400]={txt[:400]}")

def clarify():
    with open(SESSION_FILE) as f:
        sid = f.read().strip()
    conn = db()
    cur = conn.cursor()
    cur.execute("""SELECT id FROM steps_v2 WHERE session_id=%s AND kind='clarify'
                   AND status='awaiting_input' ORDER BY started_at DESC LIMIT 1""", (sid,))
    row = cur.fetchone()
    conn.close()
    if not row:
        print('no awaiting clarify step')
        return
    step_id = row[0]
    s, d = req("POST", f"/v1/chat/clarifications/{step_id}",
               {"session_id": sid, "step_id": step_id,
                "answers": [{"selected": ["账号本身"]}]})
    print(f"clarify status={s} resp[:300]={str(d)[:300]}")

def check():
    conn = db()
    cur = conn.cursor()
    cur.execute("""
        SELECT turn_id, left(content, 800), started_at
        FROM steps_v2 WHERE kind='notice' AND notice_type='memory_recalled'
        ORDER BY started_at DESC LIMIT 3
    """)
    found = False
    for turn_id, content, started in cur.fetchall():
        print('---', started, turn_id)
        print((content or '')[:500])
        if FACT_ID in (content or '') or CODE in (content or ''):
            found = True
    cur.execute("SELECT recalled_count, last_used_at FROM memory_facts WHERE id=%s", (FACT_ID,))
    print('fact counters:', cur.fetchone())
    print('RECALL_OK' if found else 'RECALL_MISS')
    conn.close()

if __name__ == '__main__':
    {'setup': setup, 'send': send, 'clarify': clarify, 'check': check}[sys.argv[1]]()
