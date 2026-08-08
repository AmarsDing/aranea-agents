# -*- coding: utf-8 -*-
"""export_room.py — 机房环境一键交付
用法: blender -b --python export_room.py
产物: models/ROOM_ENV.fbx|glb + room_manifest.json + preview_room.png + room_showcase.blend
"""
import sys, os, json, math, traceback
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import bpy
import room_kit

OUT = room_kit.OUT_DIR
os.makedirs(OUT, exist_ok=True)
LOG = os.path.join(OUT, 'room_export_log.txt')
_lines = []


def log(msg):
    print('[room]', msg)
    _lines.append(str(msg))
    with open(LOG, 'w', encoding='utf-8') as f:
        f.write('\n'.join(_lines))


def clean():
    for o in list(bpy.data.objects):
        bpy.data.objects.remove(o, do_unlink=True)


def export(root):
    coll = bpy.data.collections['ROOM']
    bpy.ops.object.select_all(action='DESELECT')
    for o in coll.all_objects:
        o.select_set(True)
    bpy.context.view_layer.objects.active = root
    base = os.path.join(OUT, 'ROOM_ENV')
    bpy.ops.export_scene.fbx(
        filepath=base + '.fbx', use_selection=True,
        object_types={'MESH', 'EMPTY'}, use_mesh_modifiers=True,
        add_leaf_bones=False, use_custom_props=True)
    log('exported ROOM_ENV.fbx')
    try:
        bpy.ops.export_scene.gltf(filepath=base + '.glb', export_format='GLB', use_selection=True)
        log('exported ROOM_ENV.glb')
    except Exception as e:
        log('glb export failed: %s' % e)


def manifest():
    m = {
        'model': 'Aranea Data Center Room Shell',
        'role': '机房环境 (地板/墙/吊顶/灯盘/通道标识)',
        'prefix': 'ROOM',
        'fbx': 'ROOM_ENV.fbx',
        'glb': 'ROOM_ENV.glb',
        'dims_m': {'width': room_kit.ROOM_W, 'depth': room_kit.ROOM_D,
                   'ceiling_h': room_kit.ROOM_H, 'floor_void_m': room_kit.FLOOR_VOID},
        'tile_grid_mm': 600,
        'units': 'meters in Blender; FBX -> UE5 cm (1:1)',
        'coordinate': 'front(cold aisle)=-Y, up=+Z, 地板完成面 z=0',
        'naming': {
            'root': 'ROOM_ROOT',
            'rack_anchor': 'ROOM_ANCHOR_RACK_{id}  (Empty, 与 datacenter_layout.json racks[].position 对齐)',
            'light_panel': 'ROOM_LightPanel_{r}_{c}  (自发光, UE5 可替换为灯光)',
        },
        'layout_ref': '../datacenter_layout.json',
        'rack_anchors': [
            {'rack_id': 'A01', 'pos_m': [0.0, 0.0, 0.0]},
            {'rack_id': 'A02', 'pos_m': [0.7, 0.0, 0.0]},
        ],
    }
    with open(os.path.join(OUT, 'room_manifest.json'), 'w', encoding='utf-8') as f:
        json.dump(m, f, ensure_ascii=False, indent=1)
    log('manifest room_manifest.json')


def stage_and_render():
    scn = bpy.context.scene
    try:
        scn.render.engine = 'BLENDER_EEVEE_NEXT'
    except Exception:
        try:
            scn.render.engine = 'BLENDER_EEVEE'
        except Exception:
            pass
    scn.render.resolution_x = 1600
    scn.render.resolution_y = 900
    scn.render.image_settings.file_format = 'PNG'
    w = bpy.data.worlds.get('World') or bpy.data.worlds.new('World')
    scn.world = w
    w.use_nodes = True
    bg = w.node_tree.nodes.get('Background')
    bg.inputs[0].default_value = (0.02, 0.03, 0.05, 1.0)
    # 相机: 冷通道入口俯瞰
    cd = bpy.data.cameras.new('RoomCam')
    cd.lens = 24
    cam = bpy.data.objects.new('RoomCam', cd)
    scn.collection.objects.link(cam)
    scn.camera = cam
    from mathutils import Vector
    cam.location = (4.5, -4.5, 3.2)
    cam.rotation_euler = (Vector((0, 0, 0.6)) - cam.location).to_track_quat('-Z', 'Y').to_euler()
    # 顶部区域光补光
    ld = bpy.data.lights.new('RoomKey', 'AREA')
    ld.energy = 800
    ld.shape = 'RECTANGLE'
    ld.size = 4.0
    ld.size_y = 4.0
    lo = bpy.data.objects.new('RoomKey', ld)
    scn.collection.objects.link(lo)
    lo.location = (0, 0, 2.9)
    scn.render.filepath = os.path.join(OUT, 'preview_room.png')
    bpy.ops.render.render(write_still=True)
    log('rendered preview_room.png')


def main():
    log('=== room export ===')
    clean()
    root = room_kit.build_room()
    log('built room (%d objects)' % len(bpy.data.collections['ROOM'].all_objects))
    export(root)
    manifest()
    stage_and_render()
    bpy.ops.wm.save_as_mainfile(filepath=os.path.join(OUT, 'room_showcase.blend'))
    log('=== DONE ===')


try:
    main()
except Exception:
    log('FATAL:\n' + traceback.format_exc())
    raise
