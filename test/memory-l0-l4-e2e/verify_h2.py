import psycopg2, json, urllib.request

conn = psycopg2.connect("postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable")
cur = conn.cursor()

# setup: mark 1 spirit agent-scope fact + 2 skills user-scope facts as conflicting
cur.execute("UPDATE memory_facts SET conflict_count=1 WHERE agent_id='agent___spirit__' AND scope_type='agent' AND status='active' AND deleted_at='' AND id IN (SELECT id FROM memory_facts WHERE agent_id='agent___spirit__' LIMIT 1)")
print("spirit marked:", cur.rowcount)
cur.execute("UPDATE memory_facts SET conflict_count=1 WHERE agent_id='agent___skills__' AND scope_type='user' AND status='active' AND deleted_at=''")
print("skills marked:", cur.rowcount)
conn.commit()

def get(url):
    with urllib.request.urlopen(url, timeout=15) as r:
        return json.loads(r.read())

base = "http://localhost:8000/v1/memory/l3/facts/conflicts"

# H2-1: agent_id only (spirit) -> 1
j = get(f"{base}?agent_id=agent___spirit__")
print("H2-1 spirit agent_id total:", j.get("total"), "scopes:", [i.get("scope_type") for i in j.get("items", [])])

# H2-2: agent_id only (skills) -> 2, both user scope (old scope-only query would miss)
j = get(f"{base}?agent_id=agent___skills__")
print("H2-2 skills agent_id total:", j.get("total"), "scopes:", [i.get("scope_type") for i in j.get("items", [])])

# H2-3: back-compat scope query
j = get(f"{base}?scope_type=user&scope_id=default_user")
print("H2-3 scope query total:", j.get("total"))

# H2-4: no filter -> all conflicting (spirit 1 + skills 2 = 3)
j = get(base)
print("H2-4 no filter total:", j.get("total"))

# cleanup
cur.execute("UPDATE memory_facts SET conflict_count=0 WHERE conflict_count=1")
conn.commit()
print("cleanup done")
conn.close()
