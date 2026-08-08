# -*- coding: utf-8 -*-
"""export_rack.py — 42U 机柜一键交付脚本
用法: blender -b server_rack_42u.blend --python export_rack.py
流程: 注入 U 位锚点(Empty) + 根节点契约属性 -> 归零导出 FBX+GLB -> rack_manifest.json -> 前/后渲染验收图。
产物均在 models/ 目录。
坐标约定: front=-Y, up=+Z, 单位米; U1 底 z=0.115, U 高 44.45mm, 42U 顶 z=1.982。
"""
import bpy, os, json, math, traceback
from mathutils import Vector

ROOT_DIR = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.join(ROOT_DIR, 'models')
os.makedirs(OUT, exist_ok=True)
LOG = os.path.join(OUT, 'rack_export_log.txt')
_lines = []

U_HEIGHT = 0.04445
U_COUNT = 42
RAIL_Z0 = 0.115            # U1 底
RAIL_FRONT_Y = -0.45       # 前导轨安装面
RAIL_X = 0.2326            # 导轨中心 |x|
RACK_W, RACK_D, RACK_H = 0.60, 1.20, 1.982

STAGE_OBJECTS = ('Ground', 'Camera', 'KeyLight', 'FillLight', 'RimLight',
                 'InteriorLight', 'InteriorLight2')


def log(msg):
    print('[rack]', msg)
    _lines.append(str(msg))
    with open(LOG, 'w', encoding='utf-8') as f:
        f.write('\n'.join(_lines))


def u_bottom_z(u):
    return RAIL_Z0 + (u - 1) * U_HEIGHT


# ---------------------------------------------------------------- 锚点与契约属性

def inject_anchors(root):
    """每 U 一个 Empty 锚点 (前导轨平面, 左右各一可选)。已存在则跳过。"""
    coll = root.users_collection[0] if root.users_collection else bpy.context.scene.collection
    made = 0
    for u in range(1, U_COUNT + 1):
        name = 'RACK_ANCHOR_U%02d' % u
        if bpy.data.objects.get(name):
            continue
        o = bpy.data.objects.new(name, None)
        o.empty_display_size = 0.01
        o.parent = root
        o.location = (0.0, RAIL_FRONT_Y, u_bottom_z(u) + U_HEIGHT / 2)
        o['hw_category'] = 'ANCHOR'
        o['u_position'] = u
        o['data_bind'] = 'rack.units[%d]' % (u - 1)
        coll.objects.link(o)
        made += 1
    log('anchors injected: %d (total %d)' % (made, U_COUNT))


def inject_props(root):
    root['hw_category'] = 'RACK'
    root['hw_name'] = '42U Server Rack Cabinet'
    root['hw_model'] = 'Aranea RK-42U-6012'
    root['hw_spec'] = '600x1200mm, 42U, front glass door, rear split steel door, 2x vertical PDU'
    root['rack_u_count'] = U_COUNT
    root['rack_u_height_mm'] = 44.45
    root['rack_u1_bottom_m'] = RAIL_Z0
    root['rack_front'] = '-Y'
    root['data_bind'] = 'rack'
    # PDU 契约
    for name, side in (('PDU_L', 'left'), ('PDU_R', 'right')):
        o = bpy.data.objects.get(name)
        if o:
            o['hw_category'] = 'PDU'
            o['hw_name'] = 'Vertical PDU (%s)' % side
            o['data_bind'] = 'rack.pdu.%s' % side
    log('root/pdu props injected')


# ---------------------------------------------------------------- 导出

def export_fbx_glb(root):
    saved = tuple(root.location)
    root.location = (0.0, 0.0, 0.0)
    bpy.context.view_layer.update()
    try:
        bpy.ops.object.select_all(action='DESELECT')
        stack = [root]
        while stack:
            o = stack.pop()
            o.select_set(True)
            stack.extend(o.children)
        bpy.context.view_layer.objects.active = root
        base = os.path.join(OUT, 'RACK_42U')
        bpy.ops.export_scene.fbx(
            filepath=base + '.fbx', use_selection=True,
            object_types={'MESH', 'EMPTY'}, use_mesh_modifiers=True,
            add_leaf_bones=False, use_custom_props=True)
        log('exported RACK_42U.fbx')
        try:
            bpy.ops.export_scene.gltf(
                filepath=base + '.glb', export_format='GLB', use_selection=True)
            log('exported RACK_42U.glb')
        except Exception as e:
            log('glb export failed: %s' % e)
    finally:
        root.location = saved
        bpy.context.view_layer.update()


def write_manifest():
    anchors = []
    for u in range(1, U_COUNT + 1):
        anchors.append({
            'u': u,
            'anchor': 'RACK_ANCHOR_U%02d' % u,
            'z_bottom_m': round(u_bottom_z(u), 5),
            'z_center_m': round(u_bottom_z(u) + U_HEIGHT / 2, 5),
            'front_y_m': RAIL_FRONT_Y,
        })
    manifest = {
        'model': 'Aranea RK-42U-6012',
        'role': '42U 服务器机柜',
        'prefix': 'RACK',
        'fbx': 'RACK_42U.fbx',
        'glb': 'RACK_42U.glb',
        'dims_m': {'width': RACK_W, 'depth': RACK_D, 'height': RACK_H},
        'units': 'meters in Blender; FBX exported -> UE5 centimeters (1:1 real size)',
        'coordinate': 'front=-Y, up=+Z; U1 在最底部',
        'rack_geometry': {
            'u_count': U_COUNT,
            'u_height_m': U_HEIGHT,
            'u1_bottom_z_m': RAIL_Z0,
            'rail_front_y_m': RAIL_FRONT_Y,
            'rail_rear_y_m': 0.45,
            'rail_x_m': [-RAIL_X, RAIL_X],
            'mount_width_m': 0.4826,
            'formula': 'u_bottom_z(u) = %.3f + (u-1)*%.5f; 设备前面板贴 y=%.2f' % (RAIL_Z0, U_HEIGHT, RAIL_FRONT_Y),
        },
        'naming': {
            'root': 'RACK_42U_ROOT',
            'u_anchor': 'RACK_ANCHOR_U{nn}  (Empty, U位装配/UI 挂点)',
            'pdu': 'PDU_L / PDU_R (含 PDU_Outlets 插座组)',
        },
        'mount_contract': {
            'server': '设备根节点 local: 前面 y=-D/2, 底 z=0; 装配时 root.z = u_bottom_z, root.y = rail_front_y + D/2',
            'switch_1u': '同上, 交换机 dims_mm 见各 *_manifest.json',
        },
        'pdu': {
            'left': {'object': 'PDU_L', 'data_bind': 'rack.pdu.left'},
            'right': {'object': 'PDU_R', 'data_bind': 'rack.pdu.right'},
        },
        'u_anchors': anchors,
    }
    mp = os.path.join(OUT, 'rack_manifest.json')
    with open(mp, 'w', encoding='utf-8') as f:
        json.dump(manifest, f, ensure_ascii=False, indent=1)
    log('manifest rack_manifest.json (%d U anchors)' % len(anchors))


# ---------------------------------------------------------------- 渲染验收

def render_previews():
    scn = bpy.context.scene
    cam = bpy.data.objects.get('Camera')
    if not cam:
        log('no camera, skip renders')
        return
    scn.camera = cam
    try:
        scn.render.engine = 'BLENDER_EEVEE_NEXT'
    except Exception:
        try:
            scn.render.engine = 'BLENDER_EEVEE'
        except Exception:
            pass
    scn.render.resolution_x = 1280
    scn.render.resolution_y = 720
    scn.render.image_settings.file_format = 'PNG'
    target = Vector((0.0, 0.0, 1.0))

    def shot(loc, path):
        cam.location = loc
        cam.rotation_euler = (target - Vector(loc)).to_track_quat('-Z', 'Y').to_euler()
        scn.render.filepath = path
        bpy.ops.render.render(write_still=True)
        log('rendered %s' % os.path.basename(path))

    try:
        shot((2.6, -3.2, 1.9), os.path.join(OUT, 'preview_rack_front.png'))
        shot((-2.6, 3.2, 1.9), os.path.join(OUT, 'preview_rack_rear.png'))
    except Exception as e:
        log('render skipped: %s' % e)


def main():
    log('=== rack export ===')
    root = bpy.data.objects.get('RACK_42U_ROOT')
    if not root:
        raise RuntimeError('RACK_42U_ROOT not found in blend')
    inject_anchors(root)
    inject_props(root)
    export_fbx_glb(root)
    write_manifest()
    render_previews()
    bpy.ops.wm.save_as_mainfile(filepath=os.path.join(ROOT_DIR, 'server_rack_42u.blend'))
    log('blend saved (anchors injected)')
    log('=== DONE ===')


try:
    main()
except Exception:
    log('FATAL:\n' + traceback.format_exc())
    raise
