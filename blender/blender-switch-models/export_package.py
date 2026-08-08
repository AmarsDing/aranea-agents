# -*- coding: utf-8 -*-
"""export_package.py — 一键交付脚本（GUI 模式运行: blender --python export_package.py）
流程: 重建三台交换机 -> 展台灯光/相机 -> 逐台归零位导出 FBX+GLB+manifest JSON -> 渲染验收图 -> 保存 blend。
产物均在 models/ 目录。
"""
import sys, os, math, json, importlib, traceback

sys.path.insert(0, r'f:\aranea-agents\test\blender-switch-models')
import bpy
import switch_kit as sk
importlib.reload(sk)
import build_all
importlib.reload(build_all)

OUT = sk.OUT_DIR
os.makedirs(OUT, exist_ok=True)
LOG = os.path.join(OUT, 'export_log.txt')
_lines = []


def log(msg):
    print('[export]', msg)
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
    # 地面
    bpy.ops.mesh.primitive_plane_add(size=30, location=(0, 0, -0.026))
    g = bpy.context.active_object
    g.name = 'SWGround'
    gm = sk._mat('M_Ground', (0.010, 0.012, 0.018), 0.20, 0.60)
    g.data.materials.append(gm)
    # 注视点
    tgt = bpy.data.objects.new('SWTarget', None)
    bpy.context.scene.collection.objects.link(tgt)
    tgt.location = (0.02, 0.0, -0.01)
    tgt.hide_render = True
    # 灯光 (小瓦数: EEVEE 对此小尺度场景敏感, 实测梯度确定)
    _area('SWKey', (-0.95, -1.25, 1.45), 8, (1.00, 0.95, 0.90), 1.0, tgt)
    _area('SWFill', (1.15, -0.75, 0.85), 5, (0.72, 0.84, 1.00), 1.0, tgt)
    _area('SWRim', (0.20, 0.95, 1.05), 10, (0.32, 0.62, 1.00), 1.2, tgt)
    _area('SWTop', (0.00, 0.10, 1.85), 3, (1.00, 1.00, 1.00), 1.0, tgt)
    _area('SWFront', (0.02, -1.45, 0.38), 8, (0.88, 0.94, 1.00), 0.9, tgt)
    pl = bpy.data.lights.new('SWAccent', 'POINT')
    pl.energy = 3
    pl.color = (0.10, 0.45, 1.00)
    po = bpy.data.objects.new('SWAccent', pl)
    bpy.context.scene.collection.objects.link(po)
    po.location = (0.02, -0.42, 0.10)
    # 相机
    cd = bpy.data.cameras.new('SWCam')
    cd.lens = 50
    cam = bpy.data.objects.new('SWCam', cd)
    bpy.context.scene.collection.objects.link(cam)
    _track(cam, tgt)
    bpy.context.scene.camera = cam
    # 世界: 深蓝黑
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
    try:
        sc.eevee.use_raytracing = True
    except Exception:
        pass
    sc.render.image_settings.file_format = 'PNG'
    sc.render.resolution_percentage = 100
    # 辉光(LED 泛光) —— 科技感关键; Blender 5.x: compositing_node_group, Glare 属性为输入插口
    try:
        ng = bpy.data.node_groups.get('SWComp')
        if ng:
            bpy.data.node_groups.remove(ng)
        ng = bpy.data.node_groups.new('SWComp', 'CompositorNodeTree')
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

FBX_NAMES = {
    'S5735': 'SW1_Huawei_S5735-L24P4S-A1',
    'H3C52': 'SW2_H3C_S5130S-52S-EI',
    'CE6857': 'SW3_Huawei_CE6857-48S8CQ',
}

LED_SEMANTICS = {
    'green': 'Link Up (常亮) / 对应材质 M_LED_GREEN',
    'amber': 'Active 数据收发 (闪烁) / M_LED_AMBER',
    'red':   'Alarm 端口告警 / M_LED_RED',
    'blue':  'Uplink 上联指示 / M_LED_BLUE',
    'off':   'Link Down 无连接 / M_LED_OFF',
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
        'naming': {
            'static_mesh': spec['coll'][:-4] if False else prefix + '_DETAIL',
            'led_link': prefix + '_LED_P{nn}_LNK',
            'led_act': prefix + '_LED_P{nn}_ACT',
            'anchor': prefix + '_ANCHOR_P{nn}  (Empty, 端口 UI 挂点)',
            'sys_led': prefix + '_LED_SYS_{PWR|SYS|STK}',
        },
        'led_semantics': LED_SEMANTICS,
        'ports': spec['ports'],
        'system_leds': spec['system_leds'],
    }
    mp = os.path.join(OUT, prefix + '_manifest.json')
    sk.write_manifest(manifest, mp)
    log('manifest %s (%d ports)' % (os.path.basename(mp), len(spec['ports'])))


# ---------------------------------------------------------------- 主流程

def main():
    log('=== rebuild ===')
    for n in ('Cube', 'Light', 'Camera'):  # 启动默认物体, Cube 会包裹展台
        o = bpy.data.objects.get(n)
        if o:
            bpy.data.objects.remove(o)
    specs = build_all.main()
    log('built: %s' % [(s['prefix'], len(s['ports'])) for s in specs])

    log('=== export ===')
    for s in specs:
        export_one(s)

    log('=== stage & render ===')
    cam, tgt = stage()
    setup_render()
    bpy.context.view_layer.update()
    r = sk.ROOT
    shot(cam, tgt, (0.02, -1.90, 0.95), (0.02, -0.02, -0.03),
         os.path.join(r, 'showcase.png'), (1920, 1080), 35)
    shot(cam, tgt, (-0.84, -0.68, 0.30), (-0.53, -0.15, -0.015),
         os.path.join(r, 'preview_sw1.png'), (1600, 900), 40)
    shot(cam, tgt, (-0.30, -0.72, 0.30), (0.0, -0.15, -0.015),
         os.path.join(r, 'preview_sw2.png'), (1600, 900), 40)
    shot(cam, tgt, (0.88, -0.78, 0.32), (0.56, -0.15, -0.015),
         os.path.join(r, 'preview_sw3.png'), (1600, 900), 40)
    shot(cam, tgt, (-0.075, -0.42, 0.048), (-0.055, -0.212, 0.002),
         os.path.join(r, 'preview_jack.png'), (1600, 900), 70)

    blend = os.path.join(OUT, 'switch_showcase.blend')
    bpy.ops.wm.save_as_mainfile(filepath=blend)
    log('saved %s' % blend)
    log('=== DONE ===')


try:
    main()
except Exception:
    log('FATAL:\n' + traceback.format_exc())
    raise
