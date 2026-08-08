// M74 V2-T4: client_tool.invoke dispatch + register_capabilities uplink tests.
import { describe, it, expect, vi } from 'vitest';
import { createWsTransport } from '../ws-transport';

class MockWS {
  onopen: (() => void) | null = null;
  onmessage: ((ev: { data: string }) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: ((e: unknown) => void) | null = null;
  readyState = 1;
  sent: string[] = [];
  send(data: string) {
    this.sent.push(data);
  }
  close() {}
}

function makeTransport(mock: MockWS, extra?: Record<string, unknown>) {
  return createWsTransport({
    sessionId: 's1',
    url: 'ws://localhost',
    socketFactory: () => mock as unknown as WebSocket,
    ...extra,
  });
}

describe('ws-transport client_tool', () => {
  it('dispatches client_tool.invoke to onClientToolInvoke', () => {
    const mock = new MockWS();
    const onClientToolInvoke = vi.fn();
    const transport = makeTransport(mock, { onClientToolInvoke });
    transport.connect();

    mock.onmessage!({
      data: JSON.stringify({
        direction: 'server_to_client',
        channel: 'system',
        type: 'client_tool.invoke',
        payload: {
          invocation_id: 'inv-1',
          session_id: 's1',
          tool: 'client_open_app',
          args: { target: 'wechat' },
        },
      }),
    });

    expect(onClientToolInvoke).toHaveBeenCalledTimes(1);
    expect(onClientToolInvoke.mock.calls[0][0]).toEqual({
      invocation_id: 'inv-1',
      session_id: 's1',
      tool: 'client_open_app',
      args: { target: 'wechat' },
    });
    transport.disconnect();
  });

  it('ignores client_tool.invoke without payload and does not crash', () => {
    const mock = new MockWS();
    const onClientToolInvoke = vi.fn();
    const transport = makeTransport(mock, { onClientToolInvoke });
    transport.connect();

    mock.onmessage!({
      data: JSON.stringify({
        direction: 'server_to_client',
        channel: 'system',
        type: 'client_tool.invoke',
      }),
    });
    expect(onClientToolInvoke).not.toHaveBeenCalled();
    transport.disconnect();
  });

  it('does not dispatch invoke frames to unrelated callbacks', () => {
    const mock = new MockWS();
    const onV2Event = vi.fn();
    const onMonitorEvent = vi.fn();
    const onClientToolInvoke = vi.fn();
    const transport = makeTransport(mock, { onV2Event, onMonitorEvent, onClientToolInvoke });
    transport.connect();

    mock.onmessage!({
      data: JSON.stringify({
        direction: 'server_to_client',
        channel: 'system',
        type: 'client_tool.invoke',
        payload: { invocation_id: 'i', session_id: 's1', tool: 'client_open_url' },
      }),
    });
    expect(onV2Event).not.toHaveBeenCalled();
    expect(onMonitorEvent).not.toHaveBeenCalled();
    expect(onClientToolInvoke).toHaveBeenCalledTimes(1);
    transport.disconnect();
  });

  it('registerCapabilities sends the system register_capabilities frame', () => {
    const mock = new MockWS();
    const transport = makeTransport(mock);
    transport.connect();
    mock.onopen!();
    mock.sent.length = 0;

    transport.registerCapabilities(['desktop_companion']);

    expect(mock.sent).toHaveLength(1);
    const frame = JSON.parse(mock.sent[0]);
    expect(frame).toEqual({
      direction: 'client_to_server',
      channel: 'system',
      type: 'register_capabilities',
      payload: { capabilities: ['desktop_companion'] },
    });
    transport.disconnect();
  });

  it('client_tool.result uplink uses the business queue contract', () => {
    const mock = new MockWS();
    const transport = makeTransport(mock);
    transport.connect();
    mock.onopen!();
    mock.sent.length = 0;

    transport.send({
      direction: 'client_to_server',
      channel: 'system',
      type: 'client_tool.result',
      payload: { invocation_id: 'inv-1', ok: true, output: 'launched' },
    });

    expect(mock.sent).toHaveLength(1);
    const frame = JSON.parse(mock.sent[0]);
    expect(frame.type).toBe('client_tool.result');
    expect(frame.payload.ok).toBe(true);
    transport.disconnect();
  });
});
