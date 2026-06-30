const fs = require('fs');
const raw = fs.readFileSync('f:/aranea-agents/tmp_act.json', 'utf8');
const j = JSON.parse(raw);
const acts = j.activities || j.items || j;
if (!Array.isArray(acts)) {
  console.log('Not an array. Keys:', Object.keys(j));
  console.log('First 500 chars:', raw.substring(0, 500));
  process.exit(0);
}
const byKind = {};
for (const a of acts) {
  byKind[a.kind] = (byKind[a.kind] || 0) + 1;
}
console.log('total:', acts.length);
console.log('byKind:', JSON.stringify(byKind));

const ts = acts.filter(a => a.kind === 'team_stage');
console.log('\nteam_stage count:', ts.length);
for (const t of ts) {
  let m = {};
  try { m = t.metaJson ? JSON.parse(t.metaJson) : (t.meta || {}); } catch (e) {}
  console.log('  id:', t.id);
  console.log('  teamId:', t.teamId, 'status:', t.status, 'parent:', t.parentActivityId);
  console.log('  team_name:', m.team_name || m.teamName, 'task_summary:', m.task_summary);
  const members = m.members || (m.team_summary && m.team_summary.members) || [];
  console.log('  members count:', members.length);
  if (members[0]) console.log('  first member:', JSON.stringify(members[0]));
}

const ss = acts.filter(a => a.kind === 'session');
console.log('\nsession count:', ss.length);

const gs = acts.filter(a => a.kind === 'graph_stage');
console.log('\ngraph_stage count:', gs.length);
for (const g of gs) {
  let m = {};
  try { m = g.metaJson ? JSON.parse(g.metaJson) : (g.meta || {}); } catch (e) {}
  console.log('  id:', g.id, 'parent:', g.parentActivityId);
  console.log('  nodes:', JSON.stringify(m.nodes?.slice(0, 3)));
}

const plans = acts.filter(a => a.kind === 'plan');
console.log('\nplan count:', plans.length);
for (const p of plans) {
  let m = {};
  try { m = p.metaJson ? JSON.parse(p.metaJson) : (p.meta || {}); } catch (e) {}
  console.log('  id:', p.id, 'parent:', p.parentActivityId, 'steps count:', (m.steps || []).length);
  for (const s of (m.steps || []).slice(0, 5)) {
    console.log('    step:', s.id, s.label || s.name, 'status:', s.status, 'agentKey:', s.agent_key || s.agentKey);
  }
}
