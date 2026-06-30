// Parse activities.json and extract session + team_stage activities
const fs = require('fs');
const data = JSON.parse(fs.readFileSync('F:/aranea-agents/tmp_screenshots/activities.json', 'utf8'));

const activities = data.activities || data.items || data;

const sessionActivities = activities.filter(a => a.kind === 'session');
const teamStageActivities = activities.filter(a => a.kind === 'team_stage');

console.log('=== SESSION ACTIVITIES (kind=session) ===');
for (const a of sessionActivities) {
  console.log(JSON.stringify({
    id: a.id,
    status: a.status,
    agentKey: a.agent_key || a.agentKey,
    agentName: a.agent_name || a.agentName,
    teamId: a.team_id || a.teamId,
    stage: a.stage,
    parentActivityId: a.parent_activity_id || a.parentActivityId,
    timestamp: a.timestamp,
    metaChildSessionId: a.meta?.child_session_id
  }, null, 2));
}

console.log('\n=== TEAM_STAGE ACTIVITIES (kind=team_stage) ===');
for (const a of teamStageActivities) {
  console.log(JSON.stringify({
    id: a.id,
    status: a.status,
    teamId: a.team_id || a.teamId,
    teamName: a.meta?.team_name,
    stage: a.stage,
    progressPct: a.meta?.progress_pct,
    membersCount: a.meta?.members?.length,
    members: a.meta?.members?.map(m => ({ agent_key: m.agent_key, status: m.status, session_id: m.session_id })),
    hasTeamSummary: !!a.meta?.team_summary,
    teamSummaryMembers: a.meta?.team_summary?.members?.map(m => ({ agent_key: m.agent_key, status: m.status }))
  }, null, 2));
}

console.log('\n=== SUMMARY ===');
console.log('Total activities:', activities.length);
console.log('Session activities:', sessionActivities.length);
console.log('Team_stage activities:', teamStageActivities.length);
console.log('Session statuses:', sessionActivities.map(a => `${a.agent_key || a.agentKey}: ${a.status}`).join(', '));
