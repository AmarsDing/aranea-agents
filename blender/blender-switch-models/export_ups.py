# -*- coding: utf-8 -*-
"""export_ups.py — UPS 资产一键交付脚本
用法: blender -b --python export_ups.py   (headless 可跑, EEVEE 渲染)
流程: 重建三台 UPS -> 逐台归零位导出 FBX+GLB+manifest JSON -> 展台灯光 -> 渲染验收图 -> 保存 blend。
产物均在 models/ 目录; 只动 UPS* 集合, 不影响交换机资产。
"""
import sys, os, math, json, importlib, traceback

sys.path.insert(0, r'f:\aranea-agents\test\blender-switch-models')
import bpy
import switch_kit as sk
importlib.reload(sk)
import ups_kit as uk
importlib.reload(uk)
import build_ups
importlib.reload(build_ups)

OUT = sk.OUT_DIR
os.makedirs(OUT, exist_ok=True)
LOG = os.path.join(OUT, 'ups_export_log.txt')
_lines = []


def log(msg):
    print('[ups-export]', msg)
    _lines.append(str(msg))
    with open(LOG, 'w', encoding='utf-8') as f:
        f.write('\n'.join(_lines))


# ---------------------------------------------------------------- 展台

def _track(o, tgt):
    con = o.constraints.new('TRACK_TO')
    con.target = tgt
    con.track_axis = 'TRACK_NEGATIVE_Z'
    con.up_axis = 'UP_Y'


def _area(name, loc, energy, color, size, tgt):
    ld = bpy.data.lights.new(name, 'AREA')
    ld.energy = energy
    ld.color = color
    ld.shape = 'DISK'
    ld.size = size
    o = bpy.data.objects.new(name, ld)
    bpy.context.scene.collection.objects.link(o)
    o.location = loc
    _track(o, tgt)
    return o


def stage():
    for n in build_ups.STAGE:
        o = bpy.data.objects.get(n)
        if o:
            bpy.data.objects.remove(o)
    bpy.ops.mesh.primitive_plane_add(size=40, location=(0, 0, -0.001))
    g = bpy.context.active_object
    g.name = 'UPSGround'
    g.data.materials.append(sk._mat('M_UPS_Ground', (0.006, 0.008, 0.012), 0.25, 0.55))
    tgt = bpy.data.objects.new('UPSTarget', None)
    bpy.context.scene.collection.objects.link(tgt)
    tgt.location = (0.0, -0.05, 0.30)
    tgt.hide_render = True
    _area('UPSKey', (-1.35, -1.60, 2.10), 11, (1.00, 0.96, 0.90), 1.2, tgt)
    _area('UPSFill', (1.55, -1.10, 1.30), 7, (0.70, 0.83, 1.00), 1.2, tgt)
    _area('UPSRim', (0.30, 1.40, 1.60), 15, (0.30, 0.60, 1.00), 1.4, tgt)
    _area('UPSTop', (0.00, 0.10, 2.60), 5, (1.00, 1.00, 1.00), 1.2, tgt)
    _area('UPSFront', (0.02, -2.00, 0.60), 9, (0.86, 0.93, 1.00), 1.0, tgt)
    pl = bpy.data.lights.new('UPSAccent', 'POINT')
    pl.energy = 2.5
    pl.color = (0.10, 0.45, 1.00)
    po = bpy.data.objects.new('UPSAccent', pl)
    bpy.context.scene.collection.objects.link(po)
    po.location = (0.0, -0.70, 0.30)
    cd = bpy.data.cameras.new('UPSCam')
    cd.lens = 42
    cam = bpy.data.objects.new('UPSCam', cd)
    bpy.context.scene.collection.objects.link(cam)
    _track(cam, tgt)
    bpy.context.scene.camera = cam
    w = bpy.data.worlds.get('World') or bpy.data.worlds.new('World')
    bpy.context.scene.world = w
    w.use_nodes = True
    bg = w.node_tree.nodes.get('Background')
    bg.inputs[0].default_value = (0.004, 0.007, 0.014, 1.0)
    bg.inputs[1].default_value = 1.0
    return cam, tgt


def setup_render():
    sc = bpy.context.scene
    try:
        sc.render.engine = 'BLENDER_EEVEE'
    except Exception:
        sc.render.engine = 'BLENDER_EEVEE_NEXT'
    try:
        sc.eevee.taa_render_samples = 64
    except Exception:
        pass
    try:
        sc.eevee.use_raytracing = True
    except Exception:
        pass
    sc.render.image_settings.file_format = 'PNG'
    sc.render.resolution_percentage = 100
    try:
        ng = bpy.data.node_groups.get('UPSComp')
        if ng:
            bpy.data.node_groups.remove(ng)
        ng = bpy.data.node_groups.new('UPSComp', 'CompositorNodeTree')
        ng.interface.new_socket(name='Image', in_out='INPUT', socket_type='NodeSocketColor')
        ng.interface.new_socket(name='Image', in_out='OUTPUT', socket_type='NodeSocketColor')
        ng.nodes.new('NodeGroupInput')
        outp = ng.nodes.new('NodeGroupOutput')
        rl = ng.nodes.new('CompositorNodeRLayers')
        gl = ng.nodes.new('CompositorNodeGlare')
        gl.inputs['Type'].default_value = 'Fog Glow'
        gl.inputs['Quality'].default_value = 'High'
        gl.inputs['Threshold'].default_value = 0.8
        gl.inputs['Size'].default_value = 7
        ng.links.new(rl.outputs['Image'], gl.inputs['Image'])
        ng.links.new(gl.outputs['Image'], outp.inputs['Image'])
        sc.compositing_node_group = ng
    except Exception as e:
        log('compositor glare skipped: %s' % e)


def shot(cam, tgt, cam_loc, tgt_loc, path, res, lens=42):
    cam.location = cam_loc
    tgt.location = tgt_loc
    cam.data.lens = lens
    sc = bpy.context.scene
    sc.render.resolution_x, sc.render.resolution_y = res
    sc.render.filepath = path
    bpy.context.view_layer.update()
    bpy.ops.render.render(write_still=True)
    log('rendered %s' % os.path.basename(path))


# ---------------------------------------------------------------- 导出

FBX_NAMES = {
    'C6KS':   'UPS1_SANTAK_C6KS',
    'HU10K':  'UPS2_HUAWEI_UPS2000-A-10KTTL',
    'YDC10H': 'UPS3_KSTAR_YDC9110H',
}

LED_SEMANTICS = {
    'green': '正常/在线 (市电OK, 逆变工作) / M_LED_GREEN',
    'amber': '电池放电 / 旁路供电 / 中级告警 / M_LED_AMBER',
    'red':   '故障 / 过载 / 市电异常 / M_LED_RED',
    'off':   '未激活 / M_LED_OFF',
    'seg_load': '负载段: <=60%绿, 60-80%琥珀, >80%红 (输出电流水平)',
    'seg_batt': '电量段: >50%绿, 20-50%琥珀, <20%红(快闪)',
}

TELEMETRY_EXAMPLE = {
    'device': 'C6KS',
    'ts': 1786200000,
    'mode': 'normal | battery | bypass | eco | standby | fault',
    'input':  {'voltage_v': 220.4, 'current_a': 12.6, 'freq_hz': 50.0, 'state': 'ok'},
    'output': {'voltage_v': 220.1, 'current_a': 11.8, 'freq_hz': 50.0, 'load_pct': 58, 'state': 'on'},
    'battery': {'voltage_v': 192.0, 'soc_pct': 86, 'current_a': -4.2,
                'charging': True, 'runtime_min': 42},
    'temp_c': 33.5,
    'alarms': [],
}


def export_one(spec):
    prefix = spec['prefix']
    coll = bpy.data.collections[spec['coll']]
    root = bpy.data.objects.get('ROOT_' + spec['coll'])
    saved = tuple(root.location) if root else None
    if root:
        root.location = (0.0, 0.0, 0.0)
        bpy.context.view_layer.update()
    try:
        bpy.ops.object.select_all(action='DESELECT')
        for o in coll.objects:
            o.select_set(True)
        if coll.objects:
            bpy.context.view_layer.objects.active = coll.objects[0]
        base = os.path.join(OUT, FBX_NAMES[prefix])
        bpy.ops.export_scene.fbx(
            filepath=base + '.fbx', use_selection=True,
            object_types={'MESH', 'EMPTY'}, use_mesh_modifiers=True,
            add_leaf_bones=False, use_custom_props=True)
        log('exported %s.fbx' % FBX_NAMES[prefix])
        try:
            bpy.ops.export_scene.gltf(
                filepath=base + '.glb', export_format='GLB', use_selection=True)
            log('exported %s.glb' % FBX_NAMES[prefix])
        except Exception as e:
            log('glb export failed (%s): %s' % (prefix, e))
    finally:
        if root and saved:
            root.location = saved
            bpy.context.view_layer.update()
    manifest = {
        'model': spec['model'],
        'role': spec['role'],
        'prefix': prefix,
        'fbx': FBX_NAMES[prefix] + '.fbx',
        'glb': FBX_NAMES[prefix] + '.glb',
        'dims_mm': spec['dims_mm'],
        'units': 'mm (pos_mm) / FBX 导出为 cm, UE5 默认 1:1',
        'rated': spec['rated'],
        'naming': {
            'static_mesh': prefix + '_DETAIL',
            'screen': prefix + '_SCREEN  (LCD, 独立 Mesh, 替换为动态 UI)',
            'status_led': prefix + '_LED_{MAINS|BYPASS|INVERT|BATTERY|FAULT}',
            'load_seg': prefix + '_LED_LOAD_{1..5}  (输出负载/输出电流水平)',
            'batt_seg': prefix + '_LED_BATT_{1..5}',
            'anchor': prefix + '_ANCHOR_{PANEL|SCREEN|INPUT|OUTPUT|BATT}  (Empty, UI 挂点)',
        },
        'led_semantics': LED_SEMANTICS,
        'leds': spec['leds'],
        'screen': spec['screen'],
        'io': spec['io'],
        'telemetry': TELEMETRY_EXAMPLE,
    }
    mp = os.path.join(OUT, prefix + '_manifest.json')
    sk.write_manifest(manifest, mp)
    log('manifest %s (%d leds)' % (os.path.basename(mp), len(spec['leds'])))


# ---------------------------------------------------------------- 主流程

def main():
    log('=== rebuild ===')
    for n in ('Cube', 'Light', 'Camera'):
        o = bpy.data.objects.get(n)
        if o:
            bpy.data.objects.remove(o)
    specs = build_ups.main()
    log('built: %s' % [(s['prefix'], len(s['leds'])) for s in specs])

    log('=== export ===')
    for s in specs:
        export_one(s)

    log('=== stage & render ===')
    cam, tgt = stage()
    setup_render()
    bpy.context.view_layer.update()
    r = sk.ROOT
    shot(cam, tgt, (0.05, -2.20, 0.82), (0.02, 0.0, 0.24),
         os.path.join(r, 'ups_showcase.png'), (1920, 1080), 42)
    shot(cam, tgt, (-0.86, -0.95, 0.62), (-0.50, 0.06, 0.24),
         os.path.join(r, 'preview_ups1.png'), (1600, 900), 45)
    shot(cam, tgt, (0.02, -1.45, 0.17), (0.0, -0.42, 0.05),
         os.path.join(r, 'preview_ups2.png'), (1600, 900), 55)
    shot(cam, tgt, (0.95, -1.10, 0.66), (0.52, 0.10, 0.33),
         os.path.join(r, 'preview_ups3.png'), (1600, 900), 45)
    shot(cam, tgt, (-0.50, -0.78, 0.40), (-0.50, -0.19, 0.335),
         os.path.join(r, 'preview_ups_lcd.png'), (1600, 900), 55)
    # 后视: 华为后面板 IO 区
    shot(cam, tgt, (0.30, 0.75, 0.30), (0.0, -0.10, 0.05),
         os.path.join(r, 'preview_ups_rear.png'), (1600, 900), 50)

    blend = os.path.join(OUT, 'ups_showcase.blend')
    bpy.ops.wm.save_as_mainfile(filepath=blend)
    log('saved %s' % blend)
    log('=== DONE ===')


try:
    main()
except Exception:
    log('FATAL:\n' + traceback.format_exc())
    raise
