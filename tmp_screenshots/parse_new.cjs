// Parse activities_new.json to inspect team_stage and session data
const fs = require('fs');
const data = JSON.parse(fs.readFileSync('tmp_screenshots/activities_new.json', 'utf8'));
const activities = data.activities || data.items || data || [];

console.log('=== TOTAL ACTIVITIES ===', activities.length);

// Group by kind
const byKind = {};
for (const a of activities) {
  const kind = a.kind || 'unknown';
  if (!byKind[kind]) byKind[kind] = [];
  byKind[kind].push(a);
}
console.log('\n=== BY KIND ===');
for (const [kind, arr] of Object.entries(byKind)) {
  console.log(`${kind}: ${arr.length}`);
}

// Show all team_stage activities with FULL meta
console.log('\n=== TEAM_STAGE ACTIVITIES (FULL META) ===');
for (const a of byKind.team_stage || []) {
  console.log(JSON.stringify({
    id: a.id,
    status: a.status,
    stage: a.stage,
    teamId: a.teamId,
    agentKey: a.agentKey,
    meta: a.meta,
  }, null, 2));
}

// Show all session activities with FULL meta
console.log('\n=== SESSION ACTIVITIES (FULL DATA) ===');
for (const a of byKind.session || []) {
  console.log(JSON.stringify({
    id: a.id,
    status: a.status,
    stage: a.stage,
    teamId: a.teamId,
    agentKey: a.agentKey,
    agentName: a.agentName,
    parentActivityId: a.parentActivityId,
    meta: a.meta,
  }, null, 2));
}
