# -*- coding: utf-8 -*-
"""build_ups.py — 一键重建三台国产主流 UPS + 演示状态
幂等: 仅清理 UPS* 集合 / ROOT_UPS* / 展台 UPS 灯光, 不动交换机资产与其他会话内容。
面板布局按真机规格书/产品图还原:
  UPS1 山特 C6KS      塔式 240x500x460  — LCD + 5状态灯 + 负载/电量段显 + 电源键
  UPS2 华为 UPS2000-A-10KTTL 机架2U 430x585x86 — LCD + 按键组 + 右列状态灯/段显
  UPS3 科士达 YDC9110H 塔式带脚轮 250x590x655 — LCD + 5状态灯 + 段显 + 功能键
状态灯统一契约: {P}_LED_{MAINS|BYPASS|INVERT|BATTERY|FAULT} + {P}_LED_LOAD_1..5 + {P}_LED_BATT_1..5
"""
import sys, importlib, bpy
sys.path.insert(0, r'f:\aranea-agents\test\blender-switch-models')
import switch_kit as sk
importlib.reload(sk)
import ups_kit as uk
importlib.reload(uk)

MM = sk.MM
anchor = sk.anchor
MY_COLLS = ('UPS1_C6KS', 'UPS2_HU10K', 'UPS3_YDC10H')
MY_ROOTS = tuple('ROOT_' + c for c in MY_COLLS)
STAGE = ('UPSKey', 'UPSFill', 'UPSRim', 'UPSAccent', 'UPSTop', 'UPSFront',
         'UPSCam', 'UPSGround', 'UPSTarget')


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


def _new_coll(name):
    coll = bpy.data.collections.new(name)
    bpy.context.scene.collection.children.link(coll)
    return coll


# ---------------------------------------------------------------- UPS1 山特 C6KS

def build_ups1(M):
    P = 'C6KS'
    coll = _new_coll('UPS1_C6KS')
    fy, ry = uk.tower_body(coll, P, 240, 500, 452, M,
                           'SANTAK', 'CASTLE C6KS', 'ON-LINE 6KVA / 5400W', base_h=8.0)
    uk.feet(coll, P, 240, 500, M)
    # 前面板: LCD + 5 状态灯 + 负载/电量段显 + 电源键 (演示态: 市电正常, 负载60%, 电池满充)
    scr = uk.lcd(coll, P, 0, fy, 372 * MM, 76 * MM, 42 * MM, M)
    leds = []
    labels = (('MAINS', 'green'), ('BYPASS', 'off'), ('INVERT', 'green'),
              ('BATTERY', 'off'), ('FAULT', 'off'))
    for i, (lb, c) in enumerate(labels):
        leds.append(uk.status_led(coll, P, lb, (-88 + i * 44) * MM, fy, 326 * MM, M, c))
    leds += uk.seg_bar(coll, P, 'LOAD', -52 * MM, fy, 288 * MM, M, lit=3, lit_color='green',
                       title='LOAD')
    leds += uk.seg_bar(coll, P, 'BATT', 52 * MM, fy, 288 * MM, M, lit=5, lit_color='green',
                       title='BATT')
    uk.power_button(coll, P, 0, fy, 244 * MM, M, r=8.0, label='ON/OFF')
    # 后面板
    uk.slot_cover(coll, P, 'INTSLOT', -52 * MM, ry, 408 * MM, 100, 40, M, 'INT SLOT')
    uk.comm_db9(coll, P, 'RS232', 28 * MM, ry, 412 * MM, M)
    uk.comm_usb(coll, P, 'USB', 58 * MM, ry, 412 * MM, M)
    uk.comm_rj45(coll, P, 'EPO', 90 * MM, ry, 412 * MM, M)
    brk = uk.breaker(coll, P, -80 * MM, ry, 336 * MM, M, 'INPUT')
    tin = uk.terminal_cover(coll, P, 'TERM_IN', -6 * MM, ry, 336 * MM, 92, 44, M, 'AC INPUT')
    tout = uk.terminal_cover(coll, P, 'TERM_OUT', 82 * MM, ry, 336 * MM, 64, 44, M, 'AC OUTPUT')
    s1 = uk.cn_socket(coll, P, 1, -36 * MM, ry, 250 * MM, M)
    s2 = uk.cn_socket(coll, P, 2, 36 * MM, ry, 250 * MM, M)
    bat = uk.batt_terminals(coll, P, 92 * MM, ry, 250 * MM, M)
    uk.fan(coll, P, 1, -40 * MM, ry, 118 * MM, M)
    uk.fan(coll, P, 2, 40 * MM, ry, 118 * MM, M)
    uk.gnd_post(coll, P, -95 * MM, ry, 60 * MM, M)
    uk.text(coll, P + '_RLBL', 'SANTAK CASTLE SERIES', (30 * MM, ry + 2.4 * MM, 40 * MM),
            2.4, M['silk_dim'], face='rear')
    sk.join_statics(coll, P, P + '_DETAIL')
    # UI 挂点
    a_panel = anchor(coll, P + '_ANCHOR_PANEL', (0, fy - 120 * MM, 500 * MM))
    a_in = anchor(coll, P + '_ANCHOR_INPUT', (-6 * MM, ry + 55 * MM, 336 * MM))
    a_out = anchor(coll, P + '_ANCHOR_OUTPUT', (36 * MM, ry + 55 * MM, 250 * MM))
    a_bat = anchor(coll, P + '_ANCHOR_BATT', (92 * MM, ry + 45 * MM, 250 * MM))
    spec = {
        'prefix': P, 'coll': coll.name, 'dims_mm': [240, 500, 460],
        'model': 'SANTAK CASTLE C6KS', 'role': '塔式在线双变换 UPS',
        'rated': {'kva': 6, 'kw': 5.4, 'input_v': '120-275VAC', 'output_v': '220VAC±2%',
                  'batt_vdc': 192, 'freq_hz': '50/60', 'input_a_max': 32, 'output_a_max': 27.3},
        'leds': leds, 'screen': scr,
        'io': {'input': {'anchor': a_in.name, 'desc': '市电输入: 空开+端子排', 'pos_mm': [-6, 255, 336]},
               'output': {'anchor': a_out.name, 'desc': '输出: 国标10A插座x2 + 端子排',
                          'pos_mm': [36, 255, 250], 'sockets': 2},
               'battery': {'anchor': a_bat.name, 'desc': '外接电池组 192VDC(16节x12V)',
                           'pos_mm': [92, 255, 250]},
               'comms': ['RS232', 'USB', 'EPO', 'INT SLOT(SNMP/干接点)']},
    }
    return coll, spec


# ---------------------------------------------------------------- UPS2 华为 UPS2000-A-10KTTL

def build_ups2(M):
    P = 'HU10K'
    coll = _new_coll('UPS2_HU10K')
    fy, ry = uk.rack_body(coll, P, 430, 585, 86, M, 'HUAWEI', 'UPS2000-A-10KTTL')
    # 前面板: LCD + 4 按键 + 右区状态灯行 + 段显 (演示态: 电池放电, 市电异常红灯)
    scr = uk.lcd(coll, P, -20 * MM, fy, 50 * MM, 64 * MM, 34 * MM, M)
    uk.panel_button(coll, P, 'ON', 34 * MM, fy, 58 * MM, M, label='ON')
    uk.panel_button(coll, P, 'OFF', 34 * MM, fy, 45 * MM, M, label='OFF')
    uk.panel_button(coll, P, 'UP', 52 * MM, fy, 58 * MM, M, label='^')
    uk.panel_button(coll, P, 'DN', 52 * MM, fy, 45 * MM, M, label='v')
    leds = []
    labels = (('MAINS', 'red'), ('BYPASS', 'off'), ('INVERT', 'green'),
              ('BATTERY', 'amber'), ('FAULT', 'off'))
    for i, (lb, c) in enumerate(labels):
        leds.append(uk.status_led(coll, P, lb, (96 + i * 27) * MM, fy, 16 * MM, M, c))
    leds += uk.seg_bar(coll, P, 'LOAD', 118 * MM, fy, 40 * MM, M, lit=3, lit_color='green',
                       title='LOAD')
    leds += uk.seg_bar(coll, P, 'BATT', 182 * MM, fy, 40 * MM, M, lit=2, lit_color='amber',
                       title='BATT')
    # 后面板
    brk = uk.breaker(coll, P, -180 * MM, ry, 54 * MM, M, 'INPUT')
    tin = uk.terminal_cover(coll, P, 'TERM_IN', -102 * MM, ry, 52 * MM, 86, 38, M, 'AC INPUT')
    tout = uk.terminal_cover(coll, P, 'TERM_OUT', -2 * MM, ry, 52 * MM, 74, 38, M, 'AC OUTPUT')
    bat = uk.batt_terminals(coll, P, 76 * MM, ry, 52 * MM, M)
    uk.fan(coll, P, 1, 136 * MM, ry, 50 * MM, M, r=26.0)
    uk.fan(coll, P, 2, 186 * MM, ry, 50 * MM, M, r=26.0)
    uk.comm_usb(coll, P, 'USB', -190 * MM, ry, 18 * MM, M)
    uk.comm_rj45(coll, P, 'RS485', -160 * MM, ry, 18 * MM, M, 'RS485')
    uk.comm_rj45(coll, P, 'EPO', -130 * MM, ry, 18 * MM, M, 'EPO')
    uk.comm_rj45(coll, P, 'PAR', -100 * MM, ry, 18 * MM, M, 'PAR')
    uk.comm_db9(coll, P, 'MBS', -62 * MM, ry, 18 * MM, M, 'MBS')
    uk.gnd_post(coll, P, 40 * MM, ry, 18 * MM, M)
    uk.text(coll, P + '_RLBL', 'HUAWEI UPS2000-A', (90 * MM, ry + 2.4 * MM, 16 * MM),
            2.4, M['silk_dim'], face='rear')
    sk.join_statics(coll, P, P + '_DETAIL')
    a_panel = anchor(coll, P + '_ANCHOR_PANEL', (0, fy - 110 * MM, 130 * MM))
    a_in = anchor(coll, P + '_ANCHOR_INPUT', (-102 * MM, ry + 50 * MM, 52 * MM))
    a_out = anchor(coll, P + '_ANCHOR_OUTPUT', (-2 * MM, ry + 50 * MM, 52 * MM))
    a_bat = anchor(coll, P + '_ANCHOR_BATT', (76 * MM, ry + 42 * MM, 52 * MM))
    spec = {
        'prefix': P, 'coll': coll.name, 'dims_mm': [430, 585, 86],
        'model': 'HUAWEI UPS2000-A-10KTTL', 'role': '机架式(2U)在线双变换 UPS',
        'rated': {'kva': 10, 'kw': 9, 'input_v': '80-280VAC', 'output_v': '220/230/240VAC±1%',
                  'batt_vdc': 240, 'freq_hz': '50/60', 'input_a_max': 50, 'output_a_max': 45.5},
        'leds': leds, 'screen': scr,
        'io': {'input': {'anchor': a_in.name, 'desc': '市电输入: 空开+端子排', 'pos_mm': [-102, 296, 52]},
               'output': {'anchor': a_out.name, 'desc': '输出: 端子排', 'pos_mm': [-2, 296, 52], 'sockets': 0},
               'battery': {'anchor': a_bat.name, 'desc': '外接电池组 240VDC(20节x12V)',
                           'pos_mm': [76, 296, 52]},
               'comms': ['USB', 'RS485', 'EPO', 'PAR 并机', 'MBS 维修旁路']},
    }
    return coll, spec


# ---------------------------------------------------------------- UPS3 科士达 YDC9110H

def build_ups3(M):
    P = 'YDC10H'
    coll = _new_coll('UPS3_YDC10H')
    fy, ry = uk.tower_body(coll, P, 250, 590, 585, M,
                           'KSTAR', 'YDC9110H', 'ON-LINE 10KVA / 8KW', base_h=70.0)
    uk.wheels(coll, P, 250, 590, M, h=70.0)
    # 前面板 (演示态: 过载告警转旁路, FAULT 红 + BYPASS 琥珀 + 负载满段红)
    scr = uk.lcd(coll, P, 0, fy, 566 * MM, 84 * MM, 46 * MM, M)
    leds = []
    labels = (('MAINS', 'green'), ('BYPASS', 'amber'), ('INVERT', 'off'),
              ('BATTERY', 'off'), ('FAULT', 'red'))
    for i, (lb, c) in enumerate(labels):
        leds.append(uk.status_led(coll, P, lb, (-88 + i * 44) * MM, fy, 518 * MM, M, c))
    leds += uk.seg_bar(coll, P, 'LOAD', -52 * MM, fy, 478 * MM, M, lit=5, lit_color='red',
                       title='LOAD')
    leds += uk.seg_bar(coll, P, 'BATT', 52 * MM, fy, 478 * MM, M, lit=4, lit_color='green',
                       title='BATT')
    uk.panel_button(coll, P, 'ON', -36 * MM, fy, 440 * MM, M, w=14, h=6, label='ON')
    uk.panel_button(coll, P, 'OFF', 0, fy, 440 * MM, M, w=14, h=6, label='OFF')
    uk.panel_button(coll, P, 'FN', 36 * MM, fy, 440 * MM, M, w=14, h=6, label='ENTER')
    # 后面板
    uk.slot_cover(coll, P, 'SNMP', -46 * MM, ry, 608 * MM, 96, 40, M, 'SNMP')
    uk.comm_db9(coll, P, 'RS232', 42 * MM, ry, 612 * MM, M)
    uk.comm_usb(coll, P, 'USB', 74 * MM, ry, 612 * MM, M)
    uk.comm_rj45(coll, P, 'EPO', 104 * MM, ry, 612 * MM, M)
    brk = uk.breaker(coll, P, -78 * MM, ry, 520 * MM, M, 'INPUT')
    tin = uk.terminal_cover(coll, P, 'TERM_IN', 8 * MM, ry, 520 * MM, 92, 44, M, 'AC INPUT')
    tout = uk.terminal_cover(coll, P, 'TERM_OUT', -40 * MM, ry, 438 * MM, 86, 44, M, 'AC OUTPUT')
    bat = uk.batt_terminals(coll, P, 62 * MM, ry, 438 * MM, M)
    s1 = uk.cn_socket(coll, P, 1, -36 * MM, ry, 330 * MM, M)
    s2 = uk.cn_socket(coll, P, 2, 36 * MM, ry, 330 * MM, M)
    uk.fan(coll, P, 1, -42 * MM, ry, 190 * MM, M, r=40.0)
    uk.fan(coll, P, 2, 42 * MM, ry, 190 * MM, M, r=40.0)
    uk.gnd_post(coll, P, -95 * MM, ry, 110 * MM, M)
    uk.text(coll, P + '_RLBL', 'KSTAR YDC SERIES', (28 * MM, ry + 2.4 * MM, 100 * MM),
            2.4, M['silk_dim'], face='rear')
    sk.join_statics(coll, P, P + '_DETAIL')
    a_panel = anchor(coll, P + '_ANCHOR_PANEL', (0, fy - 130 * MM, 690 * MM))
    a_in = anchor(coll, P + '_ANCHOR_INPUT', (8 * MM, ry + 55 * MM, 520 * MM))
    a_out = anchor(coll, P + '_ANCHOR_OUTPUT', (0, ry + 55 * MM, 330 * MM))
    a_bat = anchor(coll, P + '_ANCHOR_BATT', (62 * MM, ry + 45 * MM, 438 * MM))
    spec = {
        'prefix': P, 'coll': coll.name, 'dims_mm': [250, 590, 655],
        'model': 'KSTAR YDC9110H', 'role': '塔式在线双变换 UPS(带脚轮)',
        'rated': {'kva': 10, 'kw': 8, 'input_v': '120-276VAC', 'output_v': '220VAC±2%',
                  'batt_vdc': 192, 'freq_hz': '50/60', 'input_a_max': 50, 'output_a_max': 45.5},
        'leds': leds, 'screen': scr,
        'io': {'input': {'anchor': a_in.name, 'desc': '市电输入: 空开+端子排', 'pos_mm': [8, 297, 520]},
               'output': {'anchor': a_out.name, 'desc': '输出: 国标10A插座x2 + 端子排',
                          'pos_mm': [0, 297, 330], 'sockets': 2},
               'battery': {'anchor': a_bat.name, 'desc': '外接电池组 192VDC(16-20节可选)',
                           'pos_mm': [62, 297, 438]},
               'comms': ['RS232', 'USB', 'EPO', 'SNMP 插槽']},
    }
    return coll, spec


# ---------------------------------------------------------------- 主流程

def main():
    clean_mine()
    M = uk.init_materials()
    specs = []
    for builder, xoff, yoff in ((build_ups1, -0.50, 0.06),
                                (build_ups2, 0.0, -0.42),
                                (build_ups3, 0.52, 0.10)):
        coll, spec = builder(M)
        root = bpy.data.objects.new('ROOT_' + coll.name, None)
        bpy.context.scene.collection.objects.link(root)
        root.location = (xoff, yoff, 0.0)
        for o in coll.objects:
            if o.parent is None:
                o.parent = root
        specs.append(spec)
    print('built:', [(s['prefix'], len(s['leds'])) for s in specs])
    return specs


if __name__ == '__main__' or True:
    specs = main()
