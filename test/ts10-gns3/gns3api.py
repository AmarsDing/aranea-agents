"""GNS3 2.2 REST API helper for TS-10 semi-physical network scenario.

Thin wrapper around the GNS3 v2 controller API (http://127.0.0.1:3080).
Auth: HTTP Basic (admin/123456) when server auth is enabled.
"""
import json
import time
import urllib.request
import urllib.error
import base64

BASE = "http://127.0.0.1:3080"
USER = "admin"
PASSWORD = "123456"


def _req(method, path, body=None, timeout=120):
    url = BASE + path
    data = None
    headers = {"Content-Type": "application/json"}
    if USER:
        token = base64.b64encode(f"{USER}:{PASSWORD}".encode()).decode()
        headers["Authorization"] = "Basic " + token
    if body is not None:
        data = json.dumps(body).encode()
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8", "replace")
            return json.loads(raw) if raw.strip() else {}
    except urllib.error.HTTPError as e:
        detail = e.read().decode("utf-8", "replace")
        raise RuntimeError(f"{method} {path} -> {e.code}: {detail}")


def version():
    return _req("GET", "/v2/version")


def compute_ids():
    return _req("GET", "/v2/computes")


def create_project(name):
    return _req("POST", "/v2/projects", {"name": name})


def find_project(name):
    for p in _req("GET", "/v2/projects"):
        if p.get("name") == name:
            return p
    return None


def open_project(pid):
    return _req("POST", f"/v2/projects/{pid}/open")


def list_nodes(pid):
    return _req("GET", f"/v2/projects/{pid}/nodes")


def list_links(pid):
    return _req("GET", f"/v2/projects/{pid}/links")


def create_node(pid, node_type, name, x=0, y=0, properties=None, compute_id="local", template_id=None):
    body = {
        "name": name,
        "node_type": node_type,
        "compute_id": compute_id,
        "x": x,
        "y": y,
    }
    if properties:
        body["properties"] = properties
    if template_id:
        body["template_id"] = template_id
    return _req("POST", f"/v2/projects/{pid}/nodes", body)


def create_node_from_template(pid, template_id, x=0, y=0, name=None):
    body = {"x": x, "y": y}
    if name:
        body["name"] = name
    return _req("POST", f"/v2/projects/{pid}/templates/{template_id}", body)


def create_template(name, node_type, properties, compute_id="local"):
    body = {
        "name": name,
        "node_type": node_type,
        "compute_id": compute_id,
        **properties,
    }
    return _req("POST", "/v2/templates", body)


def find_template(name):
    for t in _req("GET", "/v2/templates"):
        if t.get("name") == name:
            return t
    return None


def link(pid, a, b):
    """a/b: (node_id, adapter_number, port_number)"""
    body = {"nodes": [
        {"node_id": a[0], "adapter_number": a[1], "port_number": a[2]},
        {"node_id": b[0], "adapter_number": b[1], "port_number": b[2]},
    ]}
    return _req("POST", f"/v2/projects/{pid}/links", body)


def start_all(pid):
    return _req("POST", f"/v2/projects/{pid}/nodes/start", timeout=600)


def stop_all(pid):
    return _req("POST", f"/v2/projects/{pid}/nodes/stop", timeout=300)


def start_node(pid, nid):
    return _req("POST", f"/v2/projects/{pid}/nodes/{nid}/start", timeout=300)


def node(pid, nid):
    return _req("GET", f"/v2/projects/{pid}/nodes/{nid}")


def wait_started(pid, timeout=180):
    deadline = time.time() + timeout
    while time.time() < deadline:
        nodes = list_nodes(pid)
        if nodes and all(n.get("status") == "started" for n in nodes):
            return nodes
        time.sleep(3)
    return list_nodes(pid)


def get_settings():
    return _req("GET", "/v2/settings")


def set_settings(s):
    return _req("PUT", "/v2/settings", s)
