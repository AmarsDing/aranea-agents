// 批量安装探针：通过精灵管家 plan_and_execute → system_admin → cli_admin_skill_install_from_url
// 用法: node tmp-install-batch.mjs <batch-spec.json>
// batch-spec.json 格式: { "repo": "https://github.com/anthropics/skills", "label": "anthropics",
//   "groups": [["skills/pdf","skills/docx"], ["skills/xlsx"]] }
// 每个 group 是一个子任务（一个 system_admin 团队顺序安装组内全部 skill）。

import { readFileSync } from 'node:fs';

const specFile = process.argv[2];
if (!specFile) {
  console.error('usage: node tmp-install-batch.mjs <batch-spec.json>');
  process.exit(1);
}
const spec = JSON.parse(readFileSync(specFile, 'utf8'));
const totalSkills = spec.groups.reduce((n, g) => n + g.length, 0);
console.log(`[batch] repo=${spec.repo} groups=${spec.groups.length} skills=${totalSkills}`);

// 登录获取 JWT（Set-Cookie: access_token），WS 通过 ?token= 携带
const loginResp = await fetch('http://127.0.0.1:8000/v1/admins/login', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ username: 'admin', password: 'changeme' }),
});
if (!loginResp.ok) {
  console.error('[batch] login failed:', loginResp.status, await loginResp.text());
  process.exit(1);
}
const setCookie = loginResp.headers.get('set-cookie') || '';
const m = setCookie.match(/access_token=([^;]+)/);
const token = m ? m[1] : '';
if (!token) {
  console.error('[batch] no access_token cookie');
  process.exit(1);
}
console.log('[batch] login ok');

const authH = { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` };
const csResp = await fetch('http://127.0.0.1:8000/v1/sessions', {
  method: 'POST',
  headers: authH,
  body: JSON.stringify({ title: `install-batch-${spec.label}`, agent_id: 'agent___spirit__', dialog_mode: 'agent' }),
});
if (!csResp.ok) {
  console.error('[batch] create session failed:', csResp.status, await csResp.text());
  process.exit(1);
}
const csBody = await csResp.json();
const sessionId = csBody.id || csBody.ID;
console.log('[batch] session:', sessionId);

// 构造提示词：每个 group 一个子任务
const subtaskLines = spec.groups.map((g, i) => {
  const items = g.map((s, j) => `    ${j + 1}) subpath = "${s}"`).join('\n');
  return `  子任务 ${i + 1}（安装 ${g.length} 个）：\n${items}`;
}).join('\n');

const prompt = `任务：批量安装 ${totalSkills} 个 Skill（批次 ${spec.label}）。所需信息已完整提供，不要触发澄清门，直接执行。

执行方式：调用 plan_and_execute 工具，agent_keys 参数必须显式指定为 ["__system_admin__"]（显式路由到系统管家执行安装）。创建 ${spec.groups.length} 个子任务，每个子任务由系统管家逐一调用 cli_admin_skill_install_from_url 工具完成安装（url 均为 ${spec.repo} ，ref 留空使用默认分支，decision="skip"）。

子任务划分（必须严格按此执行，每个子任务内的每个 subpath 都必须实际调用一次安装工具，不得遗漏、不得跳过）：
${subtaskLines}

每个子任务安装完全部 subpath 后，用 set_deliverable 提交 JSON：{"installed":["<成功的subpath>"],"failed":["<失败的subpath及原因>"]}。`;

const url = `ws://127.0.0.1:8000/v1/ws?session_id=${sessionId}&token=${encodeURIComponent(token)}`;
const ws = new WebSocket(url);
const t0 = Date.now();
const log = (...a) => console.log(`[+${((Date.now() - t0) / 1000).toFixed(1)}s]`, ...a);

let done = false;
const finish = (code) => { if (!done) { done = true; process.exit(code); } };

ws.onopen = () => {
  log('[batch] ws open, sending batch task');
  ws.send(JSON.stringify({
    direction: 'client_to_server',
    channel: 'chat',
    type: 'user_message',
    request_id: 'batch-' + Date.now(),
    payload: {
      session_id: sessionId,
      agent_key: '__spirit__',
      content: prompt,
      options: { dialog_mode: 'agent', provider: '', model: '', attachments: [] },
    },
  }));
};

const teamState = new Map(); // teamID -> last status
let lastEvent = '';
ws.onmessage = (ev) => {
  let msg;
  try { msg = JSON.parse(ev.data); } catch { return; }
  if (msg.type === 'v2_event') {
    const kind = msg.kind;
    const p = msg.payload || {};
    if (kind.startsWith('team.') || kind === 'task.completed' || kind === 'task.failed' || kind === 'turn.completed') {
      const brief = JSON.stringify(p).slice(0, 200);
      if (brief !== lastEvent) {
        lastEvent = brief;
        log(`[v2] ${kind} ${brief}`);
      }
      if (kind === 'team.run_status' && p.team_id) {
        teamState.set(p.team_id, p.status);
      }
      if (kind === 'task.completed' || kind === 'task.failed') {
        log(`[batch] ===== task ${kind}, exiting in 5s =====`);
        setTimeout(() => finish(kind === 'task.completed' ? 0 : 2), 5000);
      }
    }
  } else if (msg.type === 'assistant_message') {
    const text = (msg.payload && (msg.payload.content || msg.payload.text)) || '';
    log('[assistant]', String(text).slice(0, 300));
  }
};

ws.onerror = (e) => log('[batch] ws error:', e.message || e);
ws.onclose = (e) => { log('[batch] ws close:', e.code, e.reason || ''); finish(3); };

// 20 分钟硬超时
setTimeout(() => {
  log('[batch] ===== TIMEOUT 20min =====');
  log('[batch] team states:', JSON.stringify([...teamState.entries()]));
  finish(4);
}, 1200_000);
