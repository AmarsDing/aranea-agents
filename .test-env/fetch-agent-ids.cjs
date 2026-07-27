// Fetch all agents (paginated) and print IDs for target agentKeys.
const BASE = 'http://127.0.0.1:18000';

async function fetchAll() {
  const all = [];
  let page = 1;
  const pageSize = 100;
  for (;;) {
    const res = await fetch(`${BASE}/v1/agents?page=${page}&pageSize=${pageSize}`);
    const data = await res.json();
    const items = data.items || [];
    all.push(...items);
    if (all.length >= (data.total || items.length) || items.length === 0) break;
    page++;
  }
  return all;
}

(async () => {
  const items = await fetchAll();
  console.log('total agents:', items.length);
  const keys = [
    'social_media_strategist__general',
    'xiaohongshu_specialist__general',
    'weibo_strategist__general',
    'zhihu_strategist__general',
    'twitter_engager__general',
    'reddit_community_builder__general',
    'wechat_mini_program_developer__general',
    'tiktok_strategist__general',
  ];
  for (const k of keys) {
    const a = items.find(x => x.agentKey === k);
    console.log(k, '=>', a ? `${a.id} | ${a.displayName} | ${a.status}` : 'NOT_FOUND');
  }
  require('fs').writeFileSync('.test-env/agents-full.json', JSON.stringify({ items }, null, 1));
})();
