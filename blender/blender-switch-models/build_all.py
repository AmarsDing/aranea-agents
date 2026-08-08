# -*- coding: utf-8 -*-
"""build_all.py — 一键重建三台交换机 + 展台场景 + 导出 FBX/manifest
在 Blender 中通过 exec(compile(open(...).read(), ...)) 或 MCP execute_code 调用。
幂等: 仅清理 SW* 集合与 ROOT_SW* / 展台灯光, 不动其他会话的集合。
"""
import sys, importlib, bpy
sys.path.insert(0, r'f:\aranea-agents\test\blender-switch-models')
import switch_kit as sk
importlib.reload(sk)
MM = sk.MM
from mathutils import Vector

OUT = sk.OUT_DIR
MY_COLLS = ('SW1_S5735', 'SW2_H3C52', 'SW3_CE6857')
MY_ROOTS = ('ROOT_SW1_S5735', 'ROOT_SW2_H3C52', 'ROOT_SW3_CE6857')
STAGE = ('SWKey', 'SWFill', 'SWRim', 'SWAccent', 'SWTop', 'SWFront', 'SWCam', 'SWGround', 'SWTarget')


def clean_mine():
    for n in MY_ROOTS + STAGE:
        o = bpy.data.objects.get(n)
        if o:
            bpy.data.objects.remove(o)
    for n in MY_COLLS:
        c = bpy.data.collections.get(n)
        if c:
            for o in list(c.objects):
                bpy.data.objects.remove(o)
            bpy.data.collections.remove(c)


def build_sw1(M):
    coll = bpy.data.collections.new('SW1_S5735')
    bpy.context.scene.collection.children.link(coll)
    fy = sk.chassis(coll, 'S5735', 0.442, 0.220, 0.0436, M, 'HUAWEI', 'S5735-L24P4S-A1')
    sysleds = sk.sys_leds(coll, 'S5735', -215*MM, fy, -15.5*MM, M, ('PWR','SYS','STK'))
    ports = []
    states = {7:'red', 11:'off', 12:'off'}
    for c in range(12):
        for r in range(2):
            idx = c*2 + r + 1
            ports.append(sk.rj45(coll, 'S5735', idx, -175*MM + c*16.4*MM, fy,
                                 (8.2 if r == 0 else -8.2)*MM, M,
                                 led_mode=states.get(idx, 'green')))
    ports.append(sk.rj45(coll, 'S5735', 90, 90*MM, fy, 0, M, leds=False))
    ports.append(sk.rj45(coll, 'S5735', 91, 106.5*MM, fy, 0, M, led_mode='green'))
    sk.usb(coll, 'S5735', 124*MM, fy, 0, M)
    for i in range(4):
        idx = 25 + i
        ports.append(sk.sfp_cage(coll, 'S5735', idx, (151.75 + i*15.5)*MM, fy, 0, M,
                                 kind='SFP', led_mode=('off' if idx == 27 else 'green')))
    sk.join_statics(coll, 'S5735', 'S5735_DETAIL')
    ifaces = ([('GigabitEthernet0/0/%d' % i, 'RJ45_1G_POE', 1000) for i in range(1, 25)] +
              [('GigabitEthernet0/0/%d' % i, 'SFP_1G', 1000) for i in range(25, 29)] +
              [('MGMT0/0/0', 'RJ45_1G_MGMT', 1000)])
    return coll, ports, sysleds, ifaces, [442, 220, 43.6], 'Huawei CloudEngine S5735-L24P4S-A1', '接入层 PoE 交换机'


def build_sw2(M):
    coll = bpy.data.collections.new('SW2_H3C52')
    bpy.context.scene.collection.children.link(coll)
    fy = sk.chassis(coll, 'H3C52', 0.440, 0.260, 0.0436, M, 'H3C', 'S5130S-52S-EI')
    sysleds = sk.sys_leds(coll, 'H3C52', -213.5*MM, fy, -15.5*MM, M, ('PWR','SYS'))
    ports = []
    states = {5:'red', 19:'off', 20:'off'}
    for c in range(24):
        for r in range(2):
            idx = c*2 + r + 1
            ports.append(sk.rj45(coll, 'H3C52', idx, -202*MM + c*15.5*MM, fy,
                                 (8.2 if r == 0 else -8.2)*MM, M,
                                 led_mode=states.get(idx, 'green')))
    for i in range(4):
        idx = 49 + i
        ports.append(sk.sfp_cage(coll, 'H3C52', idx, (163.55 + i*15.3)*MM, fy, 0, M,
                                 kind='SFP+', led_mode=('off' if idx == 52 else 'green')))
    sk.join_statics(coll, 'H3C52', 'H3C52_DETAIL')
    ifaces = ([('GigabitEthernet1/0/%d' % i, 'RJ45_1G', 1000) for i in range(1, 49)] +
              [('Ten-GigabitEthernet1/0/%d' % i, 'SFP+_10G', 10000) for i in range(49, 53)])
    return coll, ports, sysleds, ifaces, [440, 260, 43.6], 'H3C S5130S-52S-EI', '高密度千兆接入交换机'


def build_sw3(M):
    coll = bpy.data.collections.new('SW3_CE6857')
    bpy.context.scene.collection.children.link(coll)
    fy = sk.chassis(coll, 'CE6857', 0.442, 0.420, 0.0436, M, 'HUAWEI', 'CE6857-48S8CQ')
    sysleds = sk.sys_leds(coll, 'CE6857', -215*MM, fy, -15.5*MM, M, ('PWR','SYS','STK'))
    ports = []
    states = {8:'red', 13:'off', 14:'off'}
    for c in range(24):
        for r in range(2):
            idx = c*2 + r + 1
            ports.append(sk.sfp_cage(coll, 'CE6857', idx, -203*MM + c*14.6*MM, fy,
                                     (5.3 if r == 0 else -5.3)*MM, M,
                                     kind='SFP28', led_mode=states.get(idx, 'green')))
    for c in range(4):
        for r in range(2):
            idx = 49 + c*2 + r
            ports.append(sk.sfp_cage(coll, 'CE6857', idx, (150 + c*19.2)*MM, fy,
                                     (5.5 if r == 0 else -5.5)*MM, M,
                                     kind='QSFP28', led_mode=('off' if idx == 54 else 'green')))
    sk.join_statics(coll, 'CE6857', 'CE6857_DETAIL')
    ifaces = ([('25GE1/0/%d' % i, 'SFP28_25G', 25000) for i in range(1, 49)] +
              [('100GE1/0/%d' % i, 'QSFP28_100G', 100000) for i in range(49, 57)])
    return coll, ports, sysleds, ifaces, [442, 420, 43.6], 'Huawei CloudEngine 6857-48S8CQ', '数据中心 TOR 交换机'


PURPOSES = ['办公网接入', 'AP接入', 'IP电话', '打印/外设', '视频监控', '服务器接入', '物联网关', '备用']


def enrich(ports, ifaces):
    out = []
    data = [p for p in ports if p['type'] != 'CONSOLE']
    for p, (ifname, ptype, speed) in zip(sorted(data, key=lambda x: x['index']), ifaces):
        is_uplink = speed >= 10000 or p['index'] >= 25 and p['type'] in ('SFP', 'SFP28', 'QSFP28', 'SFP+')
        p = dict(p)
        p['ifname'] = ifname
        p['port_type'] = ptype
        p['speed_mbps'] = speed
        p['poe'] = 'POE' in ptype
        p['purpose'] = ('上联-汇聚/核心' if is_uplink else PURPOSES[(p['index'] - 1) % len(PURPOSES)])
        p['vlan'] = ('Trunk: 1,10,20,30,100' if is_uplink else 'Access: %d' % (10 + (p['index'] % 4) * 10))
        if 'MGMT' in ptype:
            p['purpose'] = '带外管理网口'
            p['vlan'] = 'Mgmt: 100'
        out.append(p)
    for p in ports:
        if p['type'] == 'CONSOLE':
            q = dict(p)
            q.update({'ifname': 'Console', 'port_type': 'CONSOLE', 'speed_mbps': 0,
                      'poe': False, 'purpose': '带外管理串口', 'vlan': '-'})
            out.append(q)
    return sorted(out, key=lambda x: x['index'])


def main():
    clean_mine()
    M = sk.init_materials()
    specs = []
    roots = {}
    for builder, name, xoff, yoff in ((build_sw1, 'S5735', -0.53, -0.100),
                                      (build_sw2, 'H3C52', 0.0, -0.080),
                                      (build_sw3, 'CE6857', 0.56, 0.0)):
        coll, ports, sysleds, ifaces, dims, model, role = builder(M)
        root = bpy.data.objects.new('ROOT_' + coll.name, None)
        bpy.context.scene.collection.objects.link(root)
        root.location = (xoff, yoff, -0.004)
        for o in coll.objects:
            if o.parent is None:
                o.parent = root
        roots[name] = root
        specs.append({'prefix': name, 'model': model, 'role': role, 'coll': coll.name,
                      'dims_mm': dims, 'ports': enrich(ports, ifaces), 'system_leds': sysleds})
    print('built:', [(s['prefix'], len(s['ports'])) for s in specs])
    return specs


if __name__ == '__main__' or True:
    specs = main()
