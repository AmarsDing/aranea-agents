// 实证探针：让 ops_fault_diagnosis 自己使用 alibabacloud-find-skills 搜索并安装技能
// 用法: node ./test/install-probe/tmp-ops-probe.mjs
// 使用 Node 22+ 内置全局 WebSocket

const loginResp = await fetch('http://127.0.0.1:8000/v1/admins/login', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ username: 'admin', password: 'changeme' }),
});
const m = (loginResp.headers.get('set-cookie') || '').match(/access_token=([^;]+)/);
const token = m ? m[1] : '';
if (!token) { console.error('login failed'); process.exit(1); }

const authH = { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` };
// sessions 的 agent_id 需要 agents.id（非 agent_key）
const AGENT_ID = 'b642b0d388e44d7c92667f49'; // ops_fault_diagnosis
const csResp = await fetch('http://127.0.0.1:8000/v1/sessions', {
  method: 'POST',
  headers: authH,
  body: JSON.stringify({ title: 'ops-self-install-probe', agent_id: AGENT_ID, dialog_mode: 'agent' }),
});
const cs = await csResp.json();
const sessionId = cs.id || cs.session_id || (cs.data && (cs.data.session_id || cs.data.id));
if (!sessionId) { console.error('[probe] no session id:', JSON.stringify(cs).slice(0, 300)); process.exit(1); }
console.log('[probe] session:', sessionId);

const prompt = `你是故障诊断 Agent。系统已为你安装并启用技能 alibabacloud-find-skills（阿里云技能市场搜索）。

任务：
1. 使用 alibabacloud-find-skills 技能中描述的方式，搜索阿里云技能市场中与"ECS 故障诊断"相关的技能（提示：可用 web_fetch 访问 https://agentexplorer.aliyuncs.com/openapi/for-agent/skills?keyword=ECS故障诊断&searchMode=semantic&maxResults=5 ，需带 header: User-Agent: AlibabaCloud-Agent-Skills/alibabacloud-find-skills 和 x-acs-version: 2026-03-17）。
2. 找到最匹配的技能后，尝试将它安装到本平台（你可以尝试任何你拥有的安装途径）。
3. 汇报：搜索到了什么、选择了哪个、安装是否成功、如果失败说明失败原因。`;

const url = `ws://127.0.0.1:8000/v1/ws?session_id=${sessionId}&token=${encodeURIComponent(token)}`;
const ws = new WebSocket(url);
const t0 = Date.now();
const log = (...a) => console.log(`[+${((Date.now() - t0) / 1000).toFixed(1)}s]`, ...a);
let done = false;
const finish = (code) => { if (!done) { done = true; process.exit(code); } };

ws.onopen = () => {
  log('[probe] ws open');
  ws.send(JSON.stringify({
    direction: 'client_to_server', channel: 'chat', type: 'user_message',
    request_id: 'probe-' + Date.now(),
    payload: { session_id: sessionId, agent_key: '', content: prompt, options: { dialog_mode: 'agent', provider: '', model: '', attachments: [] } },
  }));
};

let replyBuf = '';
ws.onmessage = (ev) => {
  let msg; try { msg = JSON.parse(ev.data); } catch { return; }
  if (msg.type === 'assistant_message') {
    const text = (msg.payload && (msg.payload.content || msg.payload.text)) || '';
    if (text) { replyBuf += text + '\n'; log('[assistant]', String(text).slice(0, 500)); }
  } else if (msg.type === 'v2_event') {
    const kind = msg.kind;
    if (kind === 'turn.completed' || kind === 'task.completed' || kind === 'task.failed') {
      log(`[probe] ${kind}, finishing in 3s`);
      setTimeout(() => finish(kind === 'task.failed' ? 2 : 0), 3000);
    } else if (kind && (kind.includes('tool') || kind.includes('step') || kind.includes('failed') || kind.includes('error'))) {
      log(`[v2] ${kind}`, JSON.stringify(msg.payload || {}).slice(0, 800));
    }
  }
};
ws.onerror = (e) => log('[probe] ws error:', e.message || e);
ws.onclose = (e) => { log('[probe] ws close:', e.code); finish(3); };
setTimeout(() => { log('[probe] TIMEOUT 150s'); finish(4); }, 150_000);
