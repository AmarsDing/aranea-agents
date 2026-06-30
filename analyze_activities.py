import json
from collections import Counter

with open('tmp_current_activities.json', 'r', encoding='utf-8') as f:
    data = json.load(f)

items = data.get('items', data)
print('total', len(items))

kinds = Counter(it.get('kind') for it in items)
print('kinds', dict(kinds))

for k in ['team_stage', 'session', 'graph_stage', 'plan']:
    print(f'\n=== {k} ===')
    for it in items:
        if it.get('kind') == k:
            print(json.dumps({
                'id': it.get('id'),
                'kind': it.get('kind'),
                'status': it.get('status'),
                'parent_activity_id': it.get('parent_activity_id'),
                'session_id': it.get('session_id'),
                'spirit_session_id': it.get('spirit_session_id'),
                'team_id': it.get('team_id'),
                'dag_node_id': it.get('dag_node_id'),
                'agent_key': it.get('agent_key'),
                'agent_name': it.get('agent_name'),
                'content': it.get('content'),
                'label': it.get('label'),
                'timestamp': it.get('timestamp'),
                'meta': it.get('meta'),
            }, ensure_ascii=False, indent=None))
