# -*- coding: utf-8 -*-
"""export_package.py — 一键交付 (headless: blender -b --python export_package.py)
流程: 重建三台 PDU -> 展台灯光/相机 -> 逐台归零位导出 FBX+GLB+manifest -> 渲染验收图 -> 保存 blend。
产物均在 models/ 目录。只触碰 PDU* 数据, 兼容并行会话。
"""
import sys, os, math, json, importlib, traceback

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import bpy
import pdu_kit as sk
importlib.reload(sk)
import build_all
importlib.reload(build_all)

OUT = sk.OUT_DIR
os.makedirs(OUT, exist_ok=True)
LOG = os.path.join(OUT, 'export_log.txt')
_lines = []


def log(msg):
    print('[export]', msg, flush=True)
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
    for n in build_all.STAGE:
        o = bpy.data.objects.get(n)
        if o:
            bpy.data.objects.remove(o)
    bpy.ops.mesh.primitive_plane_add(size=30, location=(0, 0, -0.026))
    g = bpy.context.active_object
    g.name = 'PDUGround'
    g.data.materials.append(sk._mat('M_Ground', (0.010, 0.012, 0.018), 0.20, 0.60))
    tgt = bpy.data.objects.new('PDUTarget', None)
    bpy.context.scene.collection.objects.link(tgt)
    tgt.location = (0.02, 0.0, -0.01)
    tgt.hide_render = True
    _area('PDUKey', (-0.95, -1.25, 1.45), 9, (1.00, 0.95, 0.90), 1.0, tgt)
    _area('PDUFill', (1.15, -0.75, 0.85), 6, (0.72, 0.84, 1.00), 1.0, tgt)
    _area('PDURim', (0.20, 0.95, 1.05), 11, (0.32, 0.62, 1.00), 1.2, tgt)
    _area('PDUTop', (0.00, 0.10, 1.85), 4, (1.00, 1.00, 1.00), 1.0, tgt)
    _area('PDUFront', (0.02, -1.45, 0.38), 9, (0.88, 0.94, 1.00), 0.9, tgt)
    pl = bpy.data.lights.new('PDUAccent', 'POINT')
    pl.energy = 3
    pl.color = (0.10, 0.45, 1.00)
    po = bpy.data.objects.new('PDUAccent', pl)
    bpy.context.scene.collection.objects.link(po)
    po.location = (0.02, -0.42, 0.10)
    cd = bpy.data.cameras.new('PDUCam')
    cd.lens = 50
    cam = bpy.data.objects.new('PDUCam', cd)
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


# ---------------------------------------------------------------- 渲染

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
    sc.render.image_settings.file_format = 'PNG'
    sc.render.resolution_percentage = 100
    try:  # 辉光 (LED/LCD 泛光)
        ng = bpy.data.node_groups.get('PDUComp')
        if ng:
            bpy.data.node_groups.remove(ng)
        ng = bpy.data.node_groups.new('PDUComp', 'CompositorNodeTree')
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


def shot(cam, tgt, cam_loc, tgt_loc, path, res, lens=50):
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

LED_SEMANTICS = {
    'green': '正常供电 (负载<60%) / M_LED_GREEN',
    'amber': '高负载 (60%~85%) / M_LED_AMBER',
    'red':   '告警/过载 (>85% 或越限) / M_LED_RED',
    'off':   '无输出 (继电器断开/断路器跳闸) / M_LED_OFF',
    'sys':   'PWR=电源(绿) ALM=告警(红) NET=通讯(蓝)',
}


def export_one(spec):
    prefix = spec['prefix']
    coll = bpy.data.collections[spec['coll']]
    root = spec.get('root') or bpy.data.objects.get('ROOT_' + spec['coll'])
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
        base = os.path.join(OUT, build_all.FBX_NAMES[prefix])
        bpy.ops.export_scene.fbx(
            filepath=base + '.fbx', use_selection=True,
            object_types={'MESH', 'EMPTY'}, use_mesh_modifiers=True,
            add_leaf_bones=False, use_custom_props=True)
        log('exported %s.fbx' % build_all.FBX_NAMES[prefix])
        try:
            bpy.ops.export_scene.gltf(filepath=base + '.glb', export_format='GLB', use_selection=True)
            log('exported %s.glb' % build_all.FBX_NAMES[prefix])
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
        'fbx': build_all.FBX_NAMES[prefix] + '.fbx',
        'glb': build_all.FBX_NAMES[prefix] + '.glb',
        'dims_mm': spec['dims_mm'],
        'units': 'mm (pos_mm) / FBX 导出为 cm, UE5 默认 1:1',
        'electrical': spec['electrical'],
        'lcd': spec['lcd'],
        'naming': {
            'static_mesh': prefix + '_DETAIL',
            'outlet_led': prefix + '_LED_P{nn}  (部分型号无分位灯则 outlets[].led=null)',
            'sys_led': prefix + '_LED_SYS_{PWR|ALM|NET}',
            'lcd_screen': prefix + '_LCD  (独立 Mesh, UE5 挂 RenderTarget/WidgetComponent)',
            'anchor_outlet': prefix + '_ANCHOR_P{nn}  (Empty, 插座 UI 挂点)',
            'anchor_lcd': prefix + '_ANCHOR_LCD  (Empty, 电力详情 UI 挂点)',
        },
        'led_semantics': LED_SEMANTICS,
        'outlets': spec['outlets'],
        'system_leds': spec['system_leds'],
    }
    mp = os.path.join(OUT, prefix + '_manifest.json')
    sk.write_manifest(manifest, mp)
    log('manifest %s (%d outlets)' % (os.path.basename(mp), len(spec['outlets'])))


# ---------------------------------------------------------------- 主流程

def main():
    log('=== rebuild ===')
    for n in ('Cube', 'Light', 'Camera'):
        o = bpy.data.objects.get(n)
        if o:
            bpy.data.objects.remove(o)
    specs = build_all.main()
    log('built: %s' % [(s['prefix'], len(s['outlets'])) for s in specs])

    log('=== export ===')
    for s in specs:
        export_one(s)

    log('=== stage & render ===')
    cam, tgt = stage()
    setup_render()
    bpy.context.view_layer.update()
    r = sk.ROOT
    shot(cam, tgt, (0.02, -1.85, 0.90), (0.02, -0.02, -0.03),
         os.path.join(r, 'showcase.png'), (1920, 1080), 36)
    shot(cam, tgt, (-0.86, -0.62, 0.28), (-0.55, -0.12, -0.015),
         os.path.join(r, 'preview_pdu1.png'), (1600, 900), 42)
    shot(cam, tgt, (-0.30, -0.66, 0.28), (0.0, -0.10, -0.015),
         os.path.join(r, 'preview_pdu2.png'), (1600, 900), 42)
    shot(cam, tgt, (0.92, -0.72, 0.30), (0.57, -0.02, -0.015),
         os.path.join(r, 'preview_pdu3.png'), (1600, 900), 42)
    shot(cam, tgt, (-0.10, -0.30, 0.075), (-0.172, -0.118, 0.0),
         os.path.join(r, 'preview_lcd.png'), (1600, 900), 60)
    shot(cam, tgt, (0.135, -0.34, 0.075), (0.10, -0.10, 0.0),
         os.path.join(r, 'preview_outlet.png'), (1600, 900), 60)

    blend = os.path.join(OUT, 'pdu_showcase.blend')
    bpy.ops.wm.save_as_mainfile(filepath=blend)
    log('saved %s' % blend)
    log('=== DONE ===')


try:
    main()
except Exception:
    log('FATAL:\n' + traceback.format_exc())
    raise
