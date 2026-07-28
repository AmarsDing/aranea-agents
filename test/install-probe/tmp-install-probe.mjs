// WS 探针：验证 system_admin 在团队内能调用 cli_admin_skill_install_from_url
// 用法: node tmp-install-probe.mjs

// 登录获取 JWT（Set-Cookie: access_token），WS 通过 ?token= 携带
const loginResp = await fetch('http://127.0.0.1:8000/v1/admins/login', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ username: 'admin', password: 'changeme' }),
});
if (!loginResp.ok) {
  console.error('[probe] login failed:', loginResp.status, await loginResp.text());
  process.exit(1);
}
const setCookie = loginResp.headers.get('set-cookie') || '';
const m = setCookie.match(/access_token=([^;]+)/);
const token = m ? m[1] : '';
if (!token) {
  console.error('[probe] no access_token cookie:', setCookie.slice(0, 200));
  process.exit(1);
}
console.log('[probe] login ok, token len:', token.length);

// 创建精灵会话（WS 通道要求会话已存在）
const authH = { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` };
const csResp = await fetch('http://127.0.0.1:8000/v1/sessions', {
  method: 'POST',
  headers: authH,
  body: JSON.stringify({ title: 'install-probe', agent_id: 'agent___spirit__', dialog_mode: 'agent' }),
});
if (!csResp.ok) {
  console.error('[probe] create session failed:', csResp.status, await csResp.text());
  process.exit(1);
}
const csBody = await csResp.json();
const sessionId = csBody.id || csBody.ID;
if (!sessionId) {
  console.error('[probe] no session id in response:', JSON.stringify(csBody).slice(0, 300));
  process.exit(1);
}

const url = `ws://127.0.0.1:8000/v1/ws?session_id=${sessionId}&token=${encodeURIComponent(token)}`;
console.log('[probe] session:', sessionId);

const ws = new WebSocket(url);
const t0 = Date.now();
const log = (...a) => console.log(`[+${((Date.now() - t0) / 1000).toFixed(1)}s]`, ...a);

const prompt = `任务：安装 2 个 Skill 验证链路。所需信息已完整提供，不要触发澄清门，直接执行。

执行方式：调用 plan_and_execute 工具，agent_keys 参数必须显式指定为 ["__system_admin__"]（显式路由到系统管家执行安装），为下列每个 Skill 创建一个子任务（共 2 个子任务）。每个子任务由系统管家调用 cli_admin_skill_install_from_url 工具完成安装。

待安装清单（url 均为 https://github.com/anthropics/skills ，ref 留空使用默认分支，decision="skip"）：
1. subpath = "skills/pdf"
2. subpath = "skills/docx"

每个子任务完成后用 set_deliverable 提交 JSON：{"skill":"<subpath>","installed":true或false,"detail":"<安装工具返回摘要>"}。`;

ws.onopen = () => {
  log('[probe] ws open, sending install task');
  ws.send(JSON.stringify({
    direction: 'client_to_server',
    channel: 'chat',
    type: 'user_message',
    request_id: 'probe-' + Date.now(),
    payload: {
      session_id: sessionId,
      agent_key: '__spirit__',
      content: prompt,
      options: { dialog_mode: 'agent', provider: '', model: '', attachments: [] },
    },
  }));
};

let lastEvent = '';
ws.onmessage = (ev) => {
  let msg;
  try { msg = JSON.parse(ev.data); } catch { return; }
  if (msg.type === 'v2_event') {
    const kind = msg.kind;
    if (kind.startsWith('team.') || kind === 'turn.completed' || kind === 'task.completed' || kind === 'task.failed') {
      const p = msg.payload || {};
      const brief = JSON.stringify(p).slice(0, 220);
      if (brief !== lastEvent) {
        lastEvent = brief;
        log(`[v2] ${kind} ${brief}`);
      }
    }
  } else if (msg.type === 'assistant_message') {
    const text = (msg.payload && (msg.payload.content || msg.payload.text)) || '';
    log('[assistant]', String(text).slice(0, 400));
  } else if (msg.type && msg.type !== 'pong') {
    log(`[ws] type=${msg.type}`);
  }
};

ws.onerror = (e) => log('[probe] ws error:', e.message || e);
ws.onclose = (e) => log('[probe] ws close:', e.code, e.reason || '');

// 5 分钟后退出
setTimeout(() => {
  log('[probe] ===== TIMEOUT, exiting =====');
  process.exit(0);
}, 300_000);
