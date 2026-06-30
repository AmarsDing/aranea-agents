// Inspect current team_stage and session activity data from the API
const fs = require('fs');

const SPIRIT_SESSION_ID = 'a7ed74d1-bc98-4f48-ade4-67115b94f45f';
const API_URL = `http://localhost:9001/api/v1/sessions/${SPIRIT_SESSION_ID}/activities?limit=200`;

(async () => {
  try {
    const resp = await fetch(API_URL);
    if (!resp.ok) {
      console.log('API error:', resp.status, await resp.text());
      return;
    }
    const data = await resp.json();
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

    // Show plan activities
    console.log('\n=== PLAN ACTIVITIES ===');
    for (const a of byKind.plan || []) {
      const steps = a.meta?.steps || [];
      console.log(JSON.stringify({
        id: a.id,
        status: a.status,
        turnId: a.turnId,
        stepsCount: steps.length,
        steps: steps.map(s => ({ id: s.id, title: s.title, status: s.status, dag_node_id: s.dag_node_id })),
      }, null, 2));
    }

    // Show graph_stage activities
    console.log('\n=== GRAPH_STAGE ACTIVITIES ===');
    for (const a of byKind.graph_stage || []) {
      const nodes = a.meta?.nodes || [];
      console.log(JSON.stringify({
        id: a.id,
        status: a.status,
        nodesCount: nodes.length,
        nodes: nodes.map(n => ({ id: n.id, label: n.label, status: n.status, dependsOn: n.dependsOn })),
      }, null, 2));
    }
  } catch (e) {
    console.error('Error:', e.message);
  }
})();
