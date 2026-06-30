// Parse activities.json and show full meta for team_stage + session activities
const fs = require('fs');
const data = JSON.parse(fs.readFileSync('F:/aranea-agents/tmp_screenshots/activities.json', 'utf8'));

const activities = data.activities || data.items || data;

console.log('=== TEAM_STAGE FULL META ===');
for (const a of activities.filter(x => x.kind === 'team_stage')) {
  console.log(JSON.stringify({
    id: a.id,
    status: a.status,
    stage: a.stage,
    teamId: a.team_id,
    meta: a.meta
  }, null, 2));
}

console.log('\n=== SESSION FULL DATA ===');
for (const a of activities.filter(x => x.kind === 'session')) {
  console.log(JSON.stringify({
    id: a.id,
    status: a.status,
    stage: a.stage,
    agentKey: a.agent_key,
    agentName: a.agent_name,
    teamId: a.team_id,
    parentActivityId: a.parent_activity_id,
    timestamp: a.timestamp,
    meta: a.meta
  }, null, 2));
}
