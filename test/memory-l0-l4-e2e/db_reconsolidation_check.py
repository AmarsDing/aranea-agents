import psycopg2
import sys

conn = psycopg2.connect(host='127.0.0.1', dbname='aranea', user='postgres', password='Hangshan@123')
cur = conn.cursor()

label = sys.argv[1] if len(sys.argv) > 1 else 'snapshot'
print(f'=== {label} ===')
cur.execute("""
    SELECT id, name, entity_type, confidence, activation, use_count, activation_updated_at
    FROM memory_entities
    WHERE status = 'active'
    ORDER BY use_count DESC, updated_at DESC
    LIMIT 15
""")
print(f'{"name":<16} {"type":<12} {"conf":>5} {"act":>5} {"use":>4}  activation_updated_at')
for r in cur.fetchall():
    print(f'{str(r[1])[:15]:<16} {r[2]:<12} {r[3]:>5} {r[4]:>5} {r[5]:>4}  {r[6]}')
conn.close()
