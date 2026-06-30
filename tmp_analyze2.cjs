const fs = require('fs');
const raw = fs.readFileSync('f:/aranea-agents/tmp_member_act.json', 'utf8');
const j = JSON.parse(raw);
const acts = j.items || j.activities || j;
if (!Array.isArray(acts)) {
  console.log('Keys:', Object.keys(j));
  process.exit(0);
}
console.log('member session activities total:', acts.length);
const byKind = {};
for (const a of acts) byKind[a.kind] = (byKind[a.kind] || 0) + 1;
console.log('byKind:', JSON.stringify(byKind));

// Check if any have spiritSessionId pointing to the spirit session
const spirit = 'a64f168a-e162-4f84-a3c4-b75fa33da05b';
const withSpirit = acts.filter(a => a.spiritSessionId === spirit);
console.log('with spiritSessionId:', withSpirit.length);

// Show session activities
const sessions = acts.filter(a => a.kind === 'session');
console.log('session count:', sessions.length);
for (const s of sessions) {
  console.log('  id:', s.id, 'status:', s.status, 'agentKey:', s.agentKey, 'parent:', s.parentActivityId);
}
