// Fetch activities from API and analyze the order/structure
const http = require('http');

const SESSION_ID = 'a7ed74d1-bc98-4f48-ade4-67115b94f45f';

function fetchActivities() {
  return new Promise((resolve, reject) => {
    const options = {
      hostname: 'localhost',
      port: 9001,
      path: `/v1/sessions/${SESSION_ID}/activities?limit=200`,
      method: 'GET',
      headers: { 'Accept': 'application/json' },
    };
    const req = http.request(options, (res) => {
      let data = '';
      res.on('data', (c) => (data += c));
      res.on('end', () => {
        try {
          resolve(JSON.parse(data));
        } catch (e) {
          reject(e);
        }
      });
    });
    req.on('error', reject);
    req.end();
  });
}

(async () => {
  try {
    const result = await fetchActivities();
    const activities = result.activities || result.items || result.data || result;
    if (!Array.isArray(activities)) {
      console.log('Response type:', typeof activities);
      console.log('Keys:', Object.keys(result).slice(0, 10));
      console.log('Preview:', JSON.stringify(result).slice(0, 500));
      return;
    }
    console.log(`Total activities: ${activities.length}\n`);
    
    // Show root-level activities (parentId is null or matches root)
    const roots = activities.filter(a => !a.parentId || !a.parentActivityId);
    console.log(`Root activities: ${roots.length}`);
    
    // Show all activities with their kind, parentId, timestamp
    console.log('\n=== All activities (kind | id | parentId | timestamp | metaJson preview) ===');
    activities.forEach((a, i) => {
      const kind = a.kind || a.type || '?';
      const id = (a.id || '').slice(0, 12);
      const pid = (a.parentId || a.parentActivityId || '').slice(0, 12) || 'ROOT';
      const ts = a.timestamp || a.createdAt || a.created_at || '?';
      const meta = a.metaJson ? JSON.parse(a.metaJson) : (a.meta || {});
      const isFinal = meta.is_final ? '[FINAL]' : '';
      const title = (a.title || a.content || '').slice(0, 40);
      console.log(`${String(i).padStart(3)} | ${kind.padEnd(12)} | ${id.padEnd(12)} | parent=${pid.padEnd(12)} | ${ts} | ${isFinal} ${title}`);
    });
    
    // Build tree and show structure
    console.log('\n=== Tree structure ===');
    const byId = new Map();
    activities.forEach(a => byId.set(a.id, { ...a, children: [] }));
    const treeRoots = [];
    activities.forEach(a => {
      const pid = a.parentId || a.parentActivityId;
      if (pid && byId.has(pid)) {
        byId.get(pid).children.push(byId.get(a.id));
      } else {
        treeRoots.push(byId.get(a.id));
      }
    });
    
    function printTree(node, depth = 0) {
      const indent = '  '.repeat(depth);
      const kind = node.kind || '?';
      const id = (node.id || '').slice(0, 8);
      const ts = node.timestamp || node.createdAt || '?';
      const meta = node.metaJson ? JSON.parse(node.metaJson) : (node.meta || {});
      const isFinal = meta.is_final ? ' [FINAL]' : '';
      const title = (node.title || node.content || '').slice(0, 50);
      console.log(`${indent}[${kind}] ${id} ${ts}${isFinal} ${title}`);
      (node.children || []).forEach(c => printTree(c, depth + 1));
    }
    treeRoots.forEach(r => printTree(r));
    
  } catch (e) {
    console.error('Error:', e.message);
  }
})();
