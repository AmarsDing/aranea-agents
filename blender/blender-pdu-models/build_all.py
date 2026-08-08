# -*- coding: utf-8 -*-
"""build_all.py — 一键重建三台国产 PDU + 导出清单
PDU1 公牛 BULL GNE-1080A  : 计量型 8xGB10A + LCD + 过载保护开关 (10A/2500W)
PDU2 突破 TOP TZ-C032     : 智能监测 8xGB10A(每路LED) + LCD + RJ45 (32A/7360W)
PDU3 克莱沃 CLEVER MPDU Pro PFGA-134-0800 : 防脱插座(每路LED) + LCD + 32A液断 (32A)
幂等: 仅清理 PDU* 集合与展台对象, 不动其他会话的数据。
"""
import sys, importlib, os, bpy

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import pdu_kit as sk
importlib.reload(sk)
MM = sk.MM

OUT = sk.OUT_DIR
MY_COLLS = ('PDU1_BULL', 'PDU2_TOP', 'PDU3_PFGA')
MY_ROOTS = tuple('ROOT_' + n for n in MY_COLLS)   # 必须是 tuple: 生成器一次耗尽后 clean_mine 会漏删旧 ROOT
STAGE = ('PDUKey', 'PDUFill', 'PDURim', 'PDUAccent', 'PDUTop', 'PDUFront', 'PDUCam', 'PDUGround', 'PDUTarget')

FBX_NAMES = {
    'BULL': 'PDU1_BULL_GNE-1080A',
    'TOP':  'PDU2_TOP_TZ-C032',
    'PFGA': 'PDU3_CLEVER_PFGA-134-0800',
}

PURPOSES = ['机柜服务器 A 路', '机柜服务器 B 路', '核心交换机', '存储阵列',
            'KVM 管理终端', '备份电源输入', '预留扩容位', '维护调试插座']


def clean_mine():
    for n in list(MY_ROOTS) + list(STAGE):
        o = bpy.data.objects.get(n)
        if o:
            bpy.data.objects.remove(o)
    # 前缀清扫: 杀掉历史残留的 ROOT_PDU*.001 等重名副本
    for o in list(bpy.data.objects):
        if o.name.startswith('ROOT_PDU'):
            bpy.data.objects.remove(o)
    for n in MY_COLLS:
        c = bpy.data.collections.get(n)
        if c:
            for o in list(c.objects):
                bpy.data.objects.remove(o)
            bpy.data.collections.remove(c)


def _layout_outlets(coll, prefix, fy, M, x0, pitch, wide, states, anti_shed, led_default='green'):
    outlets = []
    for i in range(8):
        idx = i + 1
        st = states.get(idx, led_default)
        outlets.append(sk.outlet_gb10a(coll, prefix, idx, (x0 + i * pitch) * MM, fy, 0, M,
                                       led=st, anti_shed=anti_shed, wide=wide))
    return outlets


def build_pdu1(M):
    """公牛 GNE-1080A: 银灰铝壳 1U, LCD 计量 + 过载保护, 无分位灯(真实还原)。"""
    coll = bpy.data.collections.new('PDU1_BULL')
    bpy.context.scene.collection.children.link(coll)
    fy = sk.chassis(coll, 'BULL', 0.440, 0.0444, 0.0444, M, 'BULL', 'GNE-1080A  PDU  10A/2500W',
                    silver=True, rating='250V~ 10A MAX 2500W')
    sysleds = sk.sys_leds(coll, 'BULL', -206 * MM, fy, -15.5 * MM, M, ('PWR',))
    lcd = sk.lcd_module(coll, 'BULL', -172 * MM, fy, 0, M)
    outlets = _layout_outlets(coll, 'BULL', fy, M, -128, 41.5, 40, {}, anti_shed=False, led_default=None)
    sysleds.append(sk.rocker_breaker(coll, 'BULL', 196 * MM, fy, 1 * MM, M))
    sk.join_statics(coll, 'BULL', 'BULL_DETAIL')
    return dict(prefix='BULL', coll=coll.name, dims_mm=[440, 44.4, 44.4],
                model='BULL 公牛 GNE-1080A', role='机柜计量型 PDU (入门)',
                outlets=outlets, system_leds=sysleds, lcd=lcd,
                electrical={'input': '220V~/10A 国标三插', 'rated_a': 10, 'max_w': 2500,
                            'phase': '单相', 'metering': 'LCD 本地计量 (V/A/W/kWh)'})


def build_pdu2(M):
    """突破 TOP TZ-C032: 黑色智能监测, 每路 LED + LCD + RJ45 网管, 32A 工业输入。"""
    coll = bpy.data.collections.new('PDU2_TOP')
    bpy.context.scene.collection.children.link(coll)
    fy = sk.chassis(coll, 'TOP', 0.440, 0.060, 0.0444, M, 'TOP', 'TZ-C032  SMART PDU  32A',
                    rating='250V~ 32A MAX 7360W')
    sysleds = sk.sys_leds(coll, 'TOP', -208 * MM, fy, -15.5 * MM, M, ('PWR', 'ALM', 'NET'),
                          states={'PWR': 'green', 'NET': 'blue'})
    lcd = sk.lcd_module(coll, 'TOP', -166 * MM, fy, 0, M)
    sk.rj45_net(coll, 'TOP', -132 * MM, fy, 9 * MM, M)
    outlets = _layout_outlets(coll, 'TOP', fy, M, -100, 41, 38,
                              {4: 'amber', 6: 'red', 7: 'off'}, anti_shed=False)
    sk.join_statics(coll, 'TOP', 'TOP_DETAIL')
    return dict(prefix='TOP', coll=coll.name, dims_mm=[440, 60, 44.4],
                model='TOP 突破 TZ-C032 智能PDU', role='智能监测型 PDU (网管)',
                outlets=outlets, system_leds=sysleds, lcd=lcd,
                electrical={'input': '220V~/32A IEC 60309 工业插头', 'rated_a': 32, 'max_w': 7360,
                            'phase': '单相', 'metering': '分位计量 + SNMP/Modbus 网管'})


def build_pdu3(M):
    """克莱沃 CLEVER MPDU Pro PFGA-134-0800: 数据中心管理型, 防脱插座 + 32A 液断。"""
    coll = bpy.data.collections.new('PDU3_PFGA')
    bpy.context.scene.collection.children.link(coll)
    fy = sk.chassis(coll, 'PFGA', 0.4824, 0.066, 0.0444, M, 'CLEVER', 'PFGA-134-0800  MPDU Pro  32A',
                    rating='250V~ 32A MAX 7360W')
    sysleds = sk.sys_leds(coll, 'PFGA', -228 * MM, fy, -15.5 * MM, M, ('PWR', 'NET'),
                          states={'PWR': 'green', 'NET': 'blue'})
    lcd = sk.lcd_module(coll, 'PFGA', -194 * MM, fy, 0, M)
    sk.rj45_net(coll, 'PFGA', -158 * MM, fy, 9 * MM, M)
    outlets = _layout_outlets(coll, 'PFGA', fy, M, -118, 42, 40,
                              {3: 'amber', 5: 'red', 8: 'off'}, anti_shed=True)
    sysleds.append(sk.hydraulic_breaker(coll, 'PFGA', 216 * MM, fy, 0, M, amps=32))
    sk.join_statics(coll, 'PFGA', 'PFGA_DETAIL')
    return dict(prefix='PFGA', coll=coll.name, dims_mm=[482.4, 66, 44.4],
                model='CLEVER 克莱沃 MPDU Pro PFGA-134-0800', role='数据中心管理型 PDU',
                outlets=outlets, system_leds=sysleds, lcd=lcd,
                electrical={'input': '220V~/32A IEC 60309 工业插头', 'rated_a': 32, 'max_w': 7360,
                            'phase': '单相', 'metering': '1% 计费级计量 + 四级告警阈值'})


def enrich(spec):
    rated = 10 if spec['prefix'] != 'PFGA' else 16
    for o in spec['outlets']:
        o['socket'] = 'GB 10A 新国标五孔'
        o['rated_a'] = rated
        o['purpose'] = PURPOSES[(o['index'] - 1) % len(PURPOSES)]
        if o['led'] is None:
            o.pop('led', None)
            o['led'] = None
    return spec


def main():
    clean_mine()
    M = sk.init_materials()
    specs = []
    for builder, xoff, yoff in ((build_pdu1, -0.55, -0.105),
                                (build_pdu2, 0.0, -0.085),
                                (build_pdu3, 0.57, 0.0)):
        s = enrich(builder(M))
        root = bpy.data.objects.new('ROOT_' + s['coll'], None)
        bpy.context.scene.collection.objects.link(root)
        root.location = (xoff, yoff, -0.004)
        coll = bpy.data.collections[s['coll']]
        for o in coll.objects:
            if o.parent is None:
                o.parent = root
        s['root'] = root      # 导出时直接引用, 避免按名查找拿到残留旧 ROOT
        specs.append(s)
    print('built:', [(s['prefix'], len(s['outlets'])) for s in specs])
    return specs


if __name__ == '__main__':
    specs = main()
