"""Aranea platform API client for TS-10 orchestration (Spirit session lifecycle)."""
import json
import time
import urllib.request
import urllib.error

PLATFORM = "http://localhost:8000"
TOKEN_PATH = r"F:\aranea-agents\test\it-ops-closed-loop\.token"


def _token():
    with open(TOKEN_PATH, encoding="utf-8") as f:
        return f.read().strip()


def api(method, path, body=None, timeout=120):
    req = urllib.request.Request(
        PLATFORM + path,
        data=json.dumps(body).encode() if body is not None else None,
        headers={"Authorization": "Bearer " + _token(), "Content-Type": "application/json"},
        method=method,
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            raw = r.read().decode("utf-8", "replace")
            return json.loads(raw) if raw.strip() else {}
    except urllib.error.HTTPError as e:
        raise RuntimeError(f"{method} {path} -> {e.code}: {e.read().decode('utf-8','replace')[:500]}")


def create_spirit_session(title):
    s = api("POST", "/v1/sessions", {"agent_id": "agent___spirit__", "title": title})
    return s["id"]


def submit_message(session_id, content):
    return api("POST", "/v1/chat/messages/submit", {"session_id": session_id, "content": content})


def run_status(session_id):
    return api("GET", f"/v1/chat/run-status?session_id={session_id}")


def answer_clarification(session_id, step_id, answers):
    return api("POST", f"/v1/chat/clarifications/{step_id}",
               {"session_id": session_id, "step_id": step_id, "answers": answers})


def confirm_plan(session_id, plan_id):
    return api("POST", f"/v1/chat/plans/{plan_id}/confirm", {"session_id": session_id})


def list_plans(session_id):
    return api("GET", f"/v1/chat/plans?session_id={session_id}")


def respond_tool_confirm(session_id, tool_call_id, decision="allow"):
    for path, body in [
        (f"/v1/chat/tool-confirms/{tool_call_id}", {"session_id": session_id, "decision": decision}),
        ("/v1/chat/tool-confirm", {"session_id": session_id, "tool_call_id": tool_call_id, "decision": decision}),
    ]:
        try:
            return api("POST", path, body)
        except RuntimeError:
            continue
    raise RuntimeError("tool confirm endpoint not found")


def activities(session_id):
    return api("GET", f"/v1/activities?session_id={session_id}&limit=500")


def deliverables(session_id):
    return api("GET", f"/v1/deliverables?session_id={session_id}&limit=200")


def wait_terminal(session_id, on_event=None, timeout_min=45):
    """Drive a Spirit session to terminal state, auto-answering gates.

    on_event(kind, payload) called on: clarification, plan_gate, tool_confirm, status.
    Returns final run-status dict.
    """
    deadline = time.time() + timeout_min * 60
    last = ""
    while time.time() < deadline:
        time.sleep(8)
        try:
            rs = run_status(session_id)
        except Exception as e:  # noqa: BLE001
            if on_event:
                on_event("poll_error", str(e))
            continue
        status = rs.get("status", "")
        if status != last:
            last = status
            if on_event:
                on_event("status", rs)
        if status in ("completed", "failed", "cancelled", "succeeded"):
            return rs
        if status == "awaiting_user":
            kind = rs.get("awaitKind") or ""
            if kind == "clarification":
                if on_event:
                    on_event("clarification", rs)
                # auto-answer with first options
                answers = [{"selected": ["使用默认推荐方案"], "other": "按平台推荐执行，全部授权"}]
                try:
                    answer_clarification(session_id, rs.get("awaitToolCallId"), answers)
                except Exception as e:  # noqa: BLE001
                    if on_event:
                        on_event("clarify_error", str(e))
            elif "plan" in kind:
                plans = list_plans(session_id)
                items = plans.get("items") or []
                if items:
                    pid = items[0].get("planId") or items[0].get("plan_id") or items[0].get("id")
                    if on_event:
                        on_event("plan_gate", {"plan_id": pid})
                    try:
                        confirm_plan(session_id, pid)
                    except Exception as e:  # noqa: BLE001
                        if on_event:
                            on_event("confirm_error", str(e))
            elif kind == "tool_confirm":
                if on_event:
                    on_event("tool_confirm", rs)
                try:
                    respond_tool_confirm(session_id, rs.get("awaitToolCallId"), "allow")
                except Exception as e:  # noqa: BLE001
                    if on_event:
                        on_event("tool_confirm_error", str(e))
    raise TimeoutError(f"session {session_id} not terminal within {timeout_min}min (last={last})")
