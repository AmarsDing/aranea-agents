import fs from 'fs';

const data = JSON.parse(fs.readFileSync('tmp_current_activities.json', 'utf-8'));
const items = data.items || data;

console.log('total', items.length);

const kinds = {};
for (const it of items) {
  kinds[it.kind] = (kinds[it.kind] || 0) + 1;
}
console.log('kinds', kinds);

for (const k of ['team_stage', 'session', 'graph_stage', 'plan']) {
  console.log(`\n=== ${k} ===`);
  for (const it of items) {
    if (it.kind === k) {
      console.log(JSON.stringify({
        id: it.id,
        kind: it.kind,
        status: it.status,
        parent_activity_id: it.parent_activity_id,
        session_id: it.session_id,
        spirit_session_id: it.spirit_session_id,
        team_id: it.team_id,
        dag_node_id: it.dag_node_id,
        agent_key: it.agent_key,
        agent_name: it.agent_name,
        content: it.content,
        label: it.label,
        timestamp: it.timestamp,
        meta: it.meta,
      }));
    }
  }
}
