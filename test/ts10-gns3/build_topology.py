"""Build the TS-10 semi-physical enterprise network topology in GNS3 2.2.

Topology:
  PC1 (VPCS) ──┐
               ├── SW1 (builtin ethernet switch) ── edge-router (OpenWrt QEMU, eth0=LAN 192.168.10.1)
  PC2 (VPCS) ──┘                                        │
  MGMT (cloud -> GNS3-Loopback, host 192.168.10.254) ───┘ (host access to the emulated LAN)
  NAT1 (builtin NAT) ── edge-router eth1=WAN (DHCP, internet for opkg)

Usage: python build_topology.py
"""
import json
import os
import shutil
import sys
import time

import gns3api

PROJECT = "TS10-Enterprise-Network"
IMAGE_SRC = r"F:\aranea-agents\test\ts10-gns3\images\openwrt-25.12.5-x86-64-generic-ext4-combined.qcow2"
IMAGE_NAME = "openwrt-25.12.5-x86-64.qcow2"
GNS3_QEMU_IMAGE_DIR = r"F:\aranea-agents\test\ts10-gns3\gns3-images\QEMU"


def ensure_image():
    dst = os.path.join(GNS3_QEMU_IMAGE_DIR, IMAGE_NAME)
    if os.path.exists(dst):
        print("[image] already in GNS3 images dir:", dst)
        return
    os.makedirs(GNS3_QEMU_IMAGE_DIR, exist_ok=True)
    print("[image] copying", IMAGE_SRC, "->", dst)
    shutil.copyfile(IMAGE_SRC, dst)
    print("[image] done")


def main():
    print("[api] version:", gns3api.version())

    # 1. project
    proj = gns3api.find_project(PROJECT)
    if proj:
        print("[project] exists:", proj["project_id"])
    else:
        proj = gns3api.create_project(PROJECT)
        print("[project] created:", proj["project_id"])
    pid = proj["project_id"]
    gns3api.open_project(pid)

    # 2. image + qemu template
    ensure_image()
    tpl = gns3api.find_template("OpenWrt-25.12")
    if not tpl:
        tpl = gns3api.create_template("OpenWrt-25.12", "qemu", {
            "hda_disk_image": IMAGE_NAME,
            "ram": 256,
            "cpus": 1,
            "adapters": 2,
            "adapter_type": "e1000",
            "console_type": "telnet",
            "platform": "x86_64",
            "hda_disk_interface": "ide",
            "boot_priority": "c",
            "options": "-machine pc",
            "usage": "OpenWrt edge router for TS-10 semi-physical ops loop",
            "symbol": ":/symbols/router.svg",
            "category": "router",
            "template_type": "qemu",
            "first_port_name": "eth0",
            "port_name_format": "eth{0}",
        })
        print("[template] OpenWrt-25.12 created:", tpl["template_id"])
    else:
        print("[template] OpenWrt-25.12 exists:", tpl["template_id"])

    vpc_tpl = gns3api.find_template("VPCS-PC")
    if not vpc_tpl:
        vpc_tpl = gns3api.create_template("VPCS-PC", "vpcs", {
            "symbol": ":/symbols/computer.svg",
            "category": "guest",
            "template_type": "vpcs",
        })
        print("[template] VPCS-PC created:", vpc_tpl["template_id"])

    # L2 access switch = second OpenWrt (4 ports bridged as br-lan, mgmt 192.168.10.2)
    sw_tpl = gns3api.find_template("OpenWrt-L2Switch")
    if not sw_tpl:
        sw_tpl = gns3api.create_template("OpenWrt-L2Switch", "qemu", {
            "hda_disk_image": IMAGE_NAME,
            "ram": 128,
            "cpus": 1,
            "adapters": 4,
            "adapter_type": "e1000",
            "console_type": "telnet",
            "platform": "x86_64",
            "hda_disk_interface": "ide",
            "boot_priority": "c",
            "options": "-machine pc",
            "usage": "L2 access switch (OpenWrt br-lan bridge, SSH/SNMP manageable)",
            "symbol": ":/symbols/ethernet_switch.svg",
            "category": "switch",
            "template_type": "qemu",
            "first_port_name": "eth0",
            "port_name_format": "eth{0}",
        })
        print("[template] OpenWrt-L2Switch created:", sw_tpl["template_id"])

    # 3. nodes
    existing = {n["name"]: n for n in gns3api.list_nodes(pid)}

    def ensure_node(name, creator):
        if name in existing:
            print("[node] exists:", name)
            return existing[name]
        n = creator()
        print("[node] created:", name, n["node_id"])
        return n

    edge = ensure_node("edge-router", lambda: gns3api.create_node_from_template(
        pid, tpl["template_id"], x=-200, y=0, name="edge-router"))
    sw1 = ensure_node("SW1", lambda: gns3api.create_node_from_template(
        pid, sw_tpl["template_id"], x=0, y=0, name="SW1"))
    pc1 = ensure_node("PC1", lambda: gns3api.create_node_from_template(
        pid, vpc_tpl["template_id"], x=200, y=-120, name="PC1"))
    pc2 = ensure_node("PC2", lambda: gns3api.create_node_from_template(
        pid, vpc_tpl["template_id"], x=200, y=120, name="PC2"))
    wan = ensure_node("WAN", lambda: gns3api.create_node(
        pid, "cloud", "WAN", x=-420, y=-160, properties={
            "ports_mapping": [{"name": "eth0", "type": "ethernet", "port_number": 0, "interface": "以太网"}]
        }))
    mgmt = ensure_node("MGMT", lambda: gns3api.create_node(
        pid, "cloud", "MGMT", x=0, y=220, properties={
            "ports_mapping": [{"name": "eth0", "type": "ethernet", "port_number": 0, "interface": "GNS3-Loopback"}]
        }))

    # 4. links (idempotent: skip if node already linked)
    current_links = gns3api.list_links(pid)
    linked = set()
    for l in current_links:
        for nd in l.get("nodes", []):
            linked.add(nd["node_id"])

    def ensure_link(a, b):
        if a[0] in linked and b[0] in linked:
            print("[link] skip (nodes already linked)")
            return
        r = gns3api.link(pid, a, b)
        print("[link]", r.get("link_id"))

    # edge eth0 <-> SW1 port0 ; SW1 port1 <-> PC1 port0 ; SW1 port2 <-> PC2 port0
    # SW1 port3 <-> MGMT port0 ; edge eth1 <-> NAT port0
    ensure_link((edge["node_id"], 0, 0), (sw1["node_id"], 0, 0))
    ensure_link((sw1["node_id"], 1, 0), (pc1["node_id"], 0, 0))
    ensure_link((sw1["node_id"], 2, 0), (pc2["node_id"], 0, 0))
    ensure_link((sw1["node_id"], 3, 0), (mgmt["node_id"], 0, 0))
    ensure_link((edge["node_id"], 1, 0), (wan["node_id"], 0, 0))

    print("[done] project", PROJECT, "id", pid)
    with open(r"F:\aranea-agents\test\ts10-gns3\topology.json", "w", encoding="utf-8") as f:
        json.dump({"project_id": pid,
                   "nodes": gns3api.list_nodes(pid),
                   "links": gns3api.list_links(pid)}, f, indent=2, default=str)
    print("[saved] topology.json")


if __name__ == "__main__":
    sys.exit(main())
