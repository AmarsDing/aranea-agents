// Create 3 test teams in the social-media domain:
//  A: standard (3 members)  B: overstaffed (6 members)  C: understaffed (1 member)
const BASE = 'http://127.0.0.1:18000';

const AGENTS = {
  strategist: 'f424afe504b2f9ac4b6d1393', // social_media_strategist__general
  xiaohongshu: '63c47d025f170e47e4afa612', // xiaohongshu_specialist__general
  weibo: '7f4a555f5c20499324dc7e42', // weibo_strategist__general
  zhihu: '7d6e2b46a891b0d80be0a781', // zhihu_strategist__general
  twitter: 'ba013586a59421e5c355644e', // twitter_engager__general
  reddit: '7ff370a982cabcc33c1fb3ac', // reddit_community_builder__general
};

function member(agentId, role, sort) {
  return { agent_id: agentId, enabled: true, name: '', role, sort_order: sort };
}

function defJson(members, description) {
  return JSON.stringify({
    max_concurrency: 1,
    members,
    mode: 'sequential',
    run_timeout_sec: 600,
    runtime_engine: 'graph',
    team_graph_runtime: true,
    timeout_seconds: 600,
    version: 2,
    description,
  });
}

const teams = [
  {
    team_key: 'test_social_standard',
    display_name: '社媒运营测试团队-标准',
    status: 'active',
    definition_json: defJson(
      [
        member(AGENTS.strategist, 'worker', 1),
        member(AGENTS.xiaohongshu, 'worker', 2),
        member(AGENTS.weibo, 'worker', 3),
      ],
      '社交媒体运营标准团队：整体策略制定 + 小红书运营方案 + 微博运营方案。'
    ),
  },
  {
    team_key: 'test_social_overstaffed',
    display_name: '社媒运营测试团队-超员',
    status: 'active',
    definition_json: defJson(
      [
        member(AGENTS.strategist, 'worker', 1),
        member(AGENTS.xiaohongshu, 'worker', 2),
        member(AGENTS.weibo, 'worker', 3),
        member(AGENTS.zhihu, 'worker', 4),
        member(AGENTS.twitter, 'worker', 5),
        member(AGENTS.reddit, 'worker', 6),
      ],
      '社交媒体运营超员团队：策略 + 小红书 + 微博 + 知乎 + Twitter + Reddit 全平台覆盖。'
    ),
  },
  {
    team_key: 'test_social_understaffed',
    display_name: '社媒运营测试团队-缺员',
    status: 'active',
    definition_json: defJson(
      [member(AGENTS.strategist, 'worker', 1)],
      '社交媒体运营缺员团队：仅整体策略制定，无平台执行人员。'
    ),
  },
];

(async () => {
  for (const t of teams) {
    const res = await fetch(`${BASE}/v1/teams`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(t),
    });
    const body = await res.json();
    if (res.status >= 400) {
      console.log(`FAIL ${t.team_key}:`, res.status, JSON.stringify(body));
    } else {
      console.log(`OK   ${t.team_key}: id=${body.id} status=${body.status}`);
    }
  }
})();
