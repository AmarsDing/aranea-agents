// WS 探针：验证澄清提交后 step.updated 是否经 WS 送达
// 用法: node scripts/tmp-ws-clarify-probe.mjs
const sessionId = crypto.randomUUID();
const url = `ws://127.0.0.1:8000/v1/ws?session_id=${sessionId}`;
console.log('[probe] session:', sessionId);

const ws = new WebSocket(url);
let clarifyStepId = null;
let submitted = false;
let gotStepUpdated = false;
const t0 = Date.now();
const log = (...a) => console.log(`[+${((Date.now() - t0) / 1000).toFixed(1)}s]`, ...a);

ws.onopen = () => {
  log('[probe] ws open, sending user_message');
  ws.send(JSON.stringify({
    direction: 'client_to_server',
    channel: 'chat',
    type: 'user_message',
    request_id: 'probe-' + Date.now(),
    payload: {
      session_id: sessionId,
      agent_key: '__spirit__',
      content: '帮我做个方案',
      options: { dialog_mode: 'agent', provider: '', model: '', attachments: [] },
    },
  }));
};

ws.onmessage = async (ev) => {
  let msg;
  try { msg = JSON.parse(ev.data); } catch { return; }
  if (msg.type === 'v2_event') {
    const kind = msg.kind;
    const p = msg.payload || {};
    if (kind.startsWith('step.') || kind.startsWith('task.') || kind.startsWith('turn.') || kind.startsWith('system.')) {
      const extra = p.Step ? ` stepId=${p.Step.ID} kind=${p.Step.Kind} status=${p.Step.Status} ver=${p.Step.Version}` : '';
      log(`[v2] ${kind}${extra}`);
    }
    if (kind === 'step.created' && p.Step && p.Step.Kind === 'clarify' && !submitted) {
      clarifyStepId = p.Step.ID;
      submitted = true;
      log('[probe] clarify step detected, submitting answers via REST:', clarifyStepId);
      try {
        const resp = await fetch(`http://127.0.0.1:8000/v1/chat/clarifications/${clarifyStepId}`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            session_id: sessionId,
            step_id: clarifyStepId,
            answers: [{ selected: [], other: 'probe answer' }, { selected: [], other: '' }],
          }),
        });
        const body = await resp.text();
        log('[probe] REST submit status:', resp.status, body.slice(0, 200));
      } catch (e) {
        log('[probe] REST submit error:', e.message);
      }
    }
    if (kind === 'step.updated' && p.Step && p.Step.ID === clarifyStepId) {
      gotStepUpdated = true;
      log('[probe] *** step.updated for clarify step RECEIVED, status =', p.Step.Status, '***');
    }
  } else if (msg.type && msg.type !== 'pong') {
    log(`[ws] type=${msg.type} channel=${msg.channel || ''}`);
  }
};

ws.onerror = (e) => log('[probe] ws error:', e.message || e);
ws.onclose = (e) => log('[probe] ws close:', e.code, e.reason || '');

// 90 秒后总结退出
setTimeout(() => {
  log('[probe] ===== SUMMARY =====');
  log('[probe] clarifyStepId:', clarifyStepId);
  log('[probe] REST submitted:', submitted);
  log('[probe] step.updated received via WS:', gotStepUpdated);
  process.exit(gotStepUpdated ? 0 : 2);
}, 90_000);
