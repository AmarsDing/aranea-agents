# Aranea Server 3D Model Generator v2 — realistic 1U/2U/4U rackmount servers
# Design refs: Dell PowerEdge R650 (1U), Dell PowerEdge R750 (2U), Supermicro 4029GP (4U GPU)
# Units: meters (1:1 real scale). Front = -Y, Up = +Z (Blender), FBX default axes for UE5.
# All hardware objects carry custom properties (hw_*) exported to UE5 via FBX custom props.
#
# Explosion design (graded tiers, small amplitudes, non-overlapping Z/X/Y bands):
#   cover > gpu > heatsink > fan > dimm  (vertical ladder, X-disjoint where Z overlaps)
#   drives slide -Y staggered; PSUs slide +Y staggered; bezel/grille -Y small; mobo -Z small.
import bpy
import math
import json
import os
from mathutils import Vector

U = 0.04445                      # 1 rack unit height (m)
OUT_DIR = r"f:\aranea-agents\test\server-3d"
WALL = 0.0012                    # chassis sheet metal thickness
CW = 0.430                       # chassis body width (fits between 19" rack ears)
EW = 0.4826                      # rack ear total width (19")

# ---------------------------------------------------------------- materials (PBR)
def _bsdf_input(bsdf, *names):
    for n in names:
        s = bsdf.inputs.get(n)
        if s is not None:
            return s
    return None

def mat(name, color, metallic=0.0, roughness=0.5, emission=None, estr=0.0):
    m = bpy.data.materials.get(name)
    if m is None:
        m = bpy.data.materials.new(name)
    m.use_nodes = True
    bsdf = m.node_tree.nodes.get("Principled BSDF")
    _bsdf_input(bsdf, "Base Color").default_value = (*color, 1.0)
    _bsdf_input(bsdf, "Metallic").default_value = metallic
    _bsdf_input(bsdf, "Roughness").default_value = roughness
    if emission is not None:
        e = _bsdf_input(bsdf, "Emission Color", "Emission")
        if e is not None:
            e.default_value = (*emission, 1.0)
        s = _bsdf_input(bsdf, "Emission Strength")
        if s is not None:
            s.default_value = estr
    return m

def build_materials():
    return {
        # powder-coated dark steel chassis (Dell/Supermicro style)
        "steel":   mat("MAT_Chassis_Steel",   (0.235, 0.245, 0.26), 0.85, 0.42),
        # front bezel / ear panels: matte black powder coat
        "bezel":   mat("MAT_Front_Bezel",     (0.035, 0.037, 0.042), 0.30, 0.48),
        # drive tray front: light silver-grey painted metal
        "tray":    mat("MAT_Tray_Silver",     (0.55, 0.56, 0.58), 0.75, 0.40),
        "pcb":     mat("MAT_PCB_Green",       (0.012, 0.075, 0.028), 0.15, 0.42),
        "black":   mat("MAT_Plastic_Black",   (0.014, 0.014, 0.016), 0.05, 0.52),
        "alu":     mat("MAT_Heatsink_Al",     (0.72, 0.73, 0.75), 1.0, 0.30),
        "copper":  mat("MAT_Heatsink_Cu",     (0.69, 0.33, 0.14), 1.0, 0.26),
        "chip":    mat("MAT_Chip_Black",      (0.010, 0.010, 0.012), 0.25, 0.38),
        "gold":    mat("MAT_Contact_Gold",    (0.80, 0.62, 0.18), 1.0, 0.22),
        "hdd":     mat("MAT_HDD_Top",         (0.60, 0.61, 0.63), 0.85, 0.38),
        "gpu":     mat("MAT_GPU_Shroud",      (0.045, 0.047, 0.055), 0.70, 0.32),
        "psu":     mat("MAT_PSU_Shell",       (0.30, 0.31, 0.33), 0.85, 0.42),
        "dimm":    mat("MAT_DIMM_PCB",        (0.012, 0.020, 0.045), 0.15, 0.42),
        "label":   mat("MAT_Label_White",     (0.88, 0.88, 0.85), 0.0, 0.55),
        "usb_blue": mat("MAT_USB_Blue",       (0.02, 0.10, 0.55), 0.1, 0.40),
        "vga_blue": mat("MAT_VGA_Blue",       (0.03, 0.08, 0.35), 0.2, 0.45),
        # --- status LED materials (UE5: swap via Dynamic Material Instance) ---
        "led_pwr":    mat("M_LED_Power",   (0.0, 0.4, 0.0), 0.0, 0.3, (0.0, 1.0, 0.1), 12.0),
        "led_health": mat("M_LED_Health",  (0.0, 0.4, 0.0), 0.0, 0.3, (0.0, 1.0, 0.1), 12.0),
        "led_alert":  mat("M_LED_Alert",   (0.4, 0.2, 0.0), 0.0, 0.3, (1.0, 0.55, 0.0), 12.0),
        "led_uid":    mat("M_LED_UID",     (0.0, 0.1, 0.4), 0.0, 0.3, (0.1, 0.4, 1.0), 12.0),
        "led_nic":    mat("M_LED_NIC",     (0.0, 0.4, 0.0), 0.0, 0.3, (0.0, 1.0, 0.1), 12.0),
        "led_disk":   mat("M_LED_Disk",    (0.0, 0.4, 0.0), 0.0, 0.3, (0.0, 1.0, 0.1), 12.0),
    }

# ---------------------------------------------------------------- primitives
def _coll(name):
    c = bpy.data.collections.get(name)
    if c is None:
        c = bpy.data.collections.new(name)
        bpy.context.scene.collection.children.link(c)
    return c

def _move(o, coll):
    for c in list(o.users_collection):
        c.objects.unlink(o)
    coll.objects.link(o)

def _finish(o, name, loc, matl, parent, coll, bevel, props):
    o.name = name
    if parent is not None:
        o.parent = parent
    o.location = Vector(loc)
    if matl is not None:
        o.data.materials.append(matl)
    if coll is not None:
        _move(o, coll)
    if bevel > 0.0:
        mod = o.modifiers.new("Bev", "BEVEL")
        mod.width = bevel
        mod.segments = 2
        try:
            bpy.context.view_layer.objects.active = o
            o.select_set(True)
            bpy.ops.object.modifier_apply(modifier="Bev")
        except Exception:
            pass
    if props:
        for k, v in props.items():
            o[k] = v
    return o

def box(name, dims, loc, matl=None, parent=None, coll=None, bevel=0.0, props=None):
    bpy.ops.mesh.primitive_cube_add(size=1.0, location=(0, 0, 0))
    o = bpy.context.active_object
    o.dimensions = dims
    bpy.ops.object.transform_apply(location=False, rotation=False, scale=True)
    return _finish(o, name, loc, matl, parent, coll, bevel, props)

def cyl(name, radius, depth, loc, matl=None, parent=None, coll=None, rot=(0, 0, 0), bevel=0.0, props=None, verts=24):
    bpy.ops.mesh.primitive_cylinder_add(radius=radius, depth=depth, vertices=verts, location=(0, 0, 0))
    o = bpy.context.active_object
    o.rotation_euler = rot
    bpy.ops.object.transform_apply(location=False, rotation=True, scale=True)
    return _finish(o, name, loc, matl, parent, coll, bevel, props)

def multi_box(name, boxes, loc, matl=None, parent=None, coll=None, props=None):
    """Many boxes merged into ONE mesh (cheap: fins, chips, grille bars, fan blades).
       boxes: (dx,dy,dz, cx,cy,cz) or (dx,dy,dz, cx,cy,cz, rot_y) — rot_y spins the
       box around the Y axis through (cx,cz), for fan blades."""
    verts, faces = [], []
    for entry in boxes:
        dx, dy, dz, cx, cy, cz = entry[:6]
        ry = entry[6] if len(entry) > 6 else 0.0
        hx, hy, hz = dx / 2.0, dy / 2.0, dz / 2.0
        cs, sn = math.cos(ry), math.sin(ry)
        base = [(-hx, -hy, -hz), (hx, -hy, -hz), (hx, hy, -hz), (-hx, hy, -hz),
                (-hx, -hy, hz), (hx, -hy, hz), (hx, hy, hz), (-hx, hy, hz)]
        b = len(verts)
        for (vx, vy, vz) in base:
            rx = vx * cs + vz * sn
            rz = -vx * sn + vz * cs
            verts.append((cx + rx, cy + vy, cz + rz))
        faces += [(b + 0, b + 1, b + 2, b + 3), (b + 4, b + 7, b + 6, b + 5),
                  (b + 0, b + 4, b + 5, b + 1), (b + 1, b + 5, b + 6, b + 2),
                  (b + 2, b + 6, b + 7, b + 3), (b + 4, b + 0, b + 3, b + 7)]
    mesh = bpy.data.meshes.new(name + "_mesh")
    mesh.from_pydata(verts, [], faces)
    mesh.update()
    o = bpy.data.objects.new(name, mesh)
    (coll or bpy.context.scene.collection).objects.link(o)
    return _finish(o, name, loc, matl, parent, None if coll else None, 0.0, props)

def grille(name, w, h, cell, bar, depth, loc, matl, parent, coll, props=None):
    """Perforated ventilation panel as ONE mesh: lattice of bars, cell-sized openings."""
    boxes = []
    nx = max(2, int(w / cell))
    ny = max(2, int(h / cell))
    # vertical bars
    for i in range(nx + 1):
        x = -w / 2 + i * (w / nx)
        boxes.append((bar, depth, h, x, 0, 0))
    # horizontal bars
    for j in range(ny + 1):
        z = -h / 2 + j * (h / ny)
        boxes.append((w, depth, bar, 0, 0, z))
    return multi_box(name, boxes, loc, matl, parent, coll, props)

# ---------------------------------------------------------------- animation
def add_explode(o, vec):
    """Record explode offset as custom props (UE5 procedural explode) + bake keyframes."""
    o["explode_dx"], o["explode_dy"], o["explode_dz"] = float(vec[0]), float(vec[1]), float(vec[2])
    home = o.location.copy()
    o.keyframe_insert("location", frame=1)
    o.location = home + Vector(vec)
    o.keyframe_insert("location", frame=100)
    o.location = home

# ---------------------------------------------------------------- LED helper
def add_led(prefix, led_id, led_type, matl, loc, parent, coll, desc, bind, r=0.0022):
    o = cyl("%s_LED_%s" % (prefix, led_id), r, 0.003, loc, matl, parent, coll,
            rot=(math.radians(90), 0, 0), props={
                "hw_category": "LED",
                "led_type": led_type,
                "led_desc": desc,
                "data_bind": bind,
            })
    return o

# ---------------------------------------------------------------- cleanup
def delete_server(prefix):
    for o in list(bpy.data.objects):
        if o.name.startswith(prefix):
            bpy.data.objects.remove(o, do_unlink=True)
    for m in list(bpy.data.meshes):
        if m.name.startswith(prefix) and m.users == 0:
            bpy.data.meshes.remove(m)
    c = bpy.data.collections.get(prefix)
    if c is not None:
        bpy.data.collections.remove(c)

# ---------------------------------------------------------------- fan (hot-swap module)
def build_fan(prefix, idx, size, loc, parent, coll, M):
    t = size * 0.32
    frame = box("%s_FAN_%d" % (prefix, idx), (size, t, size), loc, M["black"], parent, coll,
                bevel=0.0015, props={"hw_category": "FAN", "hw_name": "Cooling Fan %d" % idx,
                                     "hw_model": "Delta FFB series (dual-rotor)",
                                     "hw_spec": "%dmm PWM, hot-swap" % int(size * 1000)})
    # hub + 7 swept blades merged into one mesh (blades rotated tangentially)
    r = size * 0.40
    blades = []
    for b in range(7):
        a = b * (2 * math.pi / 7.0)
        bx, bz = math.cos(a) * r * 0.45, math.sin(a) * r * 0.45
        blades.append((r * 0.95, 0.0012, size * 0.16, bx, 0.0, bz, -a + 0.35))
    multi_box("%s_FAN_%d_blades" % (prefix, idx), blades, (0, -t * 0.05, 0), M["black"], frame, coll)
    cyl("%s_FAN_%d_hub" % (prefix, idx), size * 0.17, t * 0.85,
        (0, -t * 0.05, 0), M["black"], frame, coll, rot=(math.radians(90), 0, 0), verts=20)
    return frame

# ---------------------------------------------------------------- CPU + fin-stack heatsink
def build_cpu(prefix, idx, loc, hs_h, M, parent, coll, cpu_model, cpu_spec):
    x, y, z = loc
    box("%s_CPU_%d_socket" % (prefix, idx), (0.062, 0.062, 0.003), (x, y, z + 0.0015),
        M["black"], parent, coll, bevel=0.0005,
        props={"hw_category": "CPU_SOCKET", "hw_name": "CPU %d Socket" % idx})
    box("%s_CPU_%d" % (prefix, idx), (0.052, 0.052, 0.0022), (x, y, z + 0.004),
        M["gold"], parent, coll, bevel=0.0004,
        props={"hw_category": "CPU", "hw_name": "CPU %d" % idx,
               "hw_model": cpu_model, "hw_spec": cpu_spec})
    # heatsink: copper base + aluminium fin stack (fins merged into ONE mesh)
    hs_w = 0.090
    base_z = z + 0.006
    hs = box("%s_CPU_%d_Heatsink" % (prefix, idx), (hs_w, hs_w, 0.0035), (x, y, base_z + 0.0018),
             M["copper"], parent, coll, bevel=0.0006,
             props={"hw_category": "HEATSINK", "hw_name": "CPU %d Heatsink" % idx,
                    "hw_model": "Passive coldplate, front-to-back airflow",
                    "hw_spec": "%dmm Al fin stack + Cu base" % int(hs_h * 1000)})
    fin_h = hs_h - 0.004
    nfin = max(8, int(hs_w / 0.0036))
    fins = []
    for i in range(nfin):
        fx = -hs_w / 2 + (i + 0.5) * (hs_w / nfin)
        fins.append((0.0008, hs_w, fin_h, fx, 0, 0.0018 + fin_h / 2.0))
    multi_box("%s_CPU_%d_HS_fins" % (prefix, idx), fins, (0, 0, 0), M["alu"], hs, coll)
    return hs

# ---------------------------------------------------------------- GPU
def build_gpu(prefix, idx, loc, length, width, height, M, parent, coll, model, spec):
    x, y, z = loc
    card = box("%s_GPU_%d" % (prefix, idx), (width, length, height), (x, y, z + height / 2.0),
               M["gpu"], parent, coll, bevel=0.0015,
               props={"hw_category": "GPU", "hw_name": "GPU %d" % idx,
                      "hw_model": model, "hw_spec": spec})
    # backplate
    box("%s_GPU_%d_backplate" % (prefix, idx), (width - 0.004, length - 0.01, 0.0012),
        (0, 0, -height / 2 - 0.0008), M["black"], card, coll, bevel=0.0008)
    # rear I/O bracket
    box("%s_GPU_%d_bracket" % (prefix, idx), (width, 0.0012, height + 0.012),
        (0, length / 2 + 0.0006, 0.004), M["steel"], card, coll)
    # PCIe gold fingers
    box("%s_GPU_%d_conn" % (prefix, idx), (width * 0.6, 0.05, 0.004),
        (0, -length / 2 + 0.03, -height / 2 - 0.002), M["gold"], card, coll)
    # 8-pin power connector
    box("%s_GPU_%d_pwr" % (prefix, idx), (0.021, 0.012, 0.008),
        (width / 2 - 0.015, -length / 2 + 0.05, height / 2 + 0.002), M["black"], card, coll)
    # blower fan(s)
    nf = 2 if length > 0.22 else 1
    for f in range(nf):
        fy = (-length / 2 + 0.06 + f * (length - 0.12) / max(nf - 1, 1)) if nf > 1 else 0.0
        cyl("%s_GPU_%d_fan%d" % (prefix, idx, f), min(0.032, width * 0.4), height * 0.4,
            (0, fy, height * 0.3 + 0.002), M["black"], card, coll, verts=20)
    return card

# ---------------------------------------------------------------- DIMM (PCB + chips + contacts, one mesh for chips)
def build_dimm(prefix, idx, loc, M, parent, coll):
    body = box("%s_DIMM_%d" % (prefix, idx), (0.0035, 0.127, 0.0313), loc,
               M["dimm"], parent, coll, bevel=0.0004,
               props={"hw_category": "MEMORY", "hw_name": "DIMM %d" % idx,
                      "hw_model": "DDR5 RDIMM 64GB", "hw_spec": "64GB DDR5-5600 ECC Registered"})
    # 8 DRAM chips on each face + gold contact edge, merged single mesh per face
    chips = []
    for s in (-1, 1):
        for i in range(8):
            cy = -0.055 + i * 0.0157
            chips.append((0.0006, 0.011, 0.009, s * 0.00205, cy, 0.004))
    chips.append((0.0008, 0.118, 0.003, 0, 0, -0.0145))  # contact edge
    m = multi_box("%s_DIMM_%d_chips" % (prefix, idx), chips, (0, 0, 0), M["chip"], body, coll)
    return body

# ---------------------------------------------------------------- drive tray (SFF portrait / LFF landscape)
def build_hdd(prefix, idx, loc, w, d, h, M, parent, coll, led_mat, form):
    """Tray with front plate, ejector handle, latch button, 2 LEDs, label.
       loc = bottom-center of tray cavity; face is proud of chassis front."""
    x, y, z = loc
    portrait = h > w
    tray = box("%s_HDD_%d" % (prefix, idx), (w, d, h), (x, y, z + h / 2.0),
               M["hdd"], parent, coll, bevel=0.0008,
               props={"hw_category": "STORAGE", "hw_name": "Drive Bay %d" % idx,
                      "hw_model": ("3.5in SAS HDD 8TB" if form == "LFF" else "2.5in NVMe SSD 3.84TB"),
                      "hw_spec": ("8TB 7.2K SAS 12Gb/s" if form == "LFF" else "3.84TB NVMe Gen4 U.2")})
    fy = -d / 2 - 0.0005  # local front face
    # front plate (slightly proud, silver)
    box("%s_HDD_%d_face" % (prefix, idx), (w + 0.001, 0.0016, h + 0.001),
        (0, fy - 0.0008, 0), M["tray"], tray, coll, bevel=0.0006)
    if portrait:
        # vertical ejector handle on right edge
        box("%s_HDD_%d_handle" % (prefix, idx), (0.006, 0.004, h * 0.62),
            (w / 2 - 0.006, fy - 0.003, 0), M["black"], tray, coll, bevel=0.0015)
        # latch button top-right
        cyl("%s_HDD_%d_latch" % (prefix, idx), 0.0028, 0.002,
            (w / 2 - 0.006, fy - 0.004, h / 2 - 0.008), M["tray"], tray, coll,
            rot=(math.radians(90), 0, 0), verts=16)
        # LEDs: activity (top-left), status (below)
        add_led(prefix, "HDD%d" % idx, "disk", led_mat,
                (x - w / 2 + 0.004, y + fy - 0.003, z + h - 0.006), parent, coll,
                "Drive %d activity" % idx, "server.disks[%d].activity" % idx, r=0.0015)
        add_led(prefix, "HDD%d_st" % idx, "disk_status", M["led_alert"],
                (x - w / 2 + 0.004, y + fy - 0.003, z + h - 0.013), parent, coll,
                "Drive %d status" % idx, "server.disks[%d].status" % idx, r=0.0015)
    else:
        # horizontal handle on right
        box("%s_HDD_%d_handle" % (prefix, idx), (w * 0.30, 0.004, 0.008),
            (w / 2 - w * 0.17, fy - 0.003, 0), M["black"], tray, coll, bevel=0.0018)
        cyl("%s_HDD_%d_latch" % (prefix, idx), 0.0032, 0.002,
            (w / 2 - 0.012, fy - 0.004, 0), M["tray"], tray, coll,
            rot=(math.radians(90), 0, 0), verts=16)
        add_led(prefix, "HDD%d" % idx, "disk", led_mat,
                (x - w / 2 + 0.006, y + fy - 0.003, z + h - 0.0065), parent, coll,
                "Drive %d activity" % idx, "server.disks[%d].activity" % idx, r=0.0015)
        add_led(prefix, "HDD%d_st" % idx, "disk_status", M["led_alert"],
                (x - w / 2 + 0.006, y + fy - 0.003, z + 0.005), parent, coll,
                "Drive %d status" % idx, "server.disks[%d].status" % idx, r=0.0015)
        # label sticker
        box("%s_HDD_%d_label" % (prefix, idx), (w * 0.35, 0.0004, h * 0.5),
            (-w * 0.12, fy - 0.0012, 0), M["label"], tray, coll)
    return tray

# ---------------------------------------------------------------- PSU (CRPS, rear-visible)
def build_psu(prefix, idx, loc, w, d, h, M, parent, coll, watt):
    x, y, z = loc
    p = box("%s_PSU_%d" % (prefix, idx), (w, d, h), (x, y, z + h / 2.0),
            M["psu"], parent, coll, bevel=0.001,
            props={"hw_category": "PSU", "hw_name": "PSU %d" % idx,
                   "hw_model": "CRPS %dW Titanium" % watt,
                   "hw_spec": "%dW 80+ Titanium, hot-swap redundant" % watt})
    ry = d / 2  # local rear face
    # rear face plate
    box("%s_PSU_%d_rear" % (prefix, idx), (w - 0.002, 0.0012, h - 0.002),
        (0, ry + 0.0006, 0), M["black"], p, coll, bevel=0.0005)
    # extraction handle (horizontal bar across rear)
    box("%s_PSU_%d_handle" % (prefix, idx), (w * 0.42, 0.006, 0.009),
        (-w * 0.16, ry + 0.004, 0), M["black"], p, coll, bevel=0.002)
    # AC inlet (IEC C14/C20)
    box("%s_PSU_%d_inlet" % (prefix, idx), (0.021, 0.004, 0.016),
        (w / 2 - 0.017, ry + 0.002, -h * 0.18), M["chip"], p, coll, bevel=0.001)
    # fan grille: dark disc + hub
    cyl("%s_PSU_%d_fan" % (prefix, idx), min(w, h) * 0.30, 0.002,
        (w * 0.12, ry + 0.0015, h * 0.14), M["chip"], p, coll,
        rot=(math.radians(90), 0, 0), verts=20)
    add_led(prefix, "PSU%d" % idx, "psu", M["led_pwr"],
            (x - w / 2 + 0.007, y + ry + 0.0025, z + h - 0.007), parent, coll,
            "PSU %d status" % idx, "server.psus[%d].status" % idx, r=0.0018)
    return p

# ---------------------------------------------------------------- rear IO cluster
def build_rear_io(prefix, x0, z0, M, root, coll, rear_y):
    """IO shield on rear panel: 2xRJ45(+LEDs), 2xUSB3, VGA, iDRAC port."""
    y = rear_y + 0.001
    box(prefix + "_IO_shield", (0.155, 0.001, 0.030), (x0 + 0.0775, y, z0 + 0.015),
        M["tray"], root, coll, bevel=0.0005)
    for i in range(2):  # RJ45 stacked pair
        box("%s_IO_RJ45_%d" % (prefix, i), (0.016, 0.010, 0.013),
            (x0 + 0.012, y + 0.004, z0 + 0.008 + i * 0.015), M["chip"], root, coll, bevel=0.0005)
        add_led(prefix, "NIC%d" % (i + 1), "nic", M["led_nic"],
                (x0 + 0.012, y + 0.010, z0 + 0.013 + i * 0.015), root, coll,
                "NIC%d link/activity" % (i + 1), "server.nics[%d].activity" % i, r=0.0012)
    for i in range(2):  # USB 3.0 pair
        box("%s_IO_USB_%d" % (prefix, i), (0.0145, 0.009, 0.006),
            (x0 + 0.040, y + 0.003, z0 + 0.006 + i * 0.009), M["usb_blue"], root, coll, bevel=0.0004)
    # VGA
    box(prefix + "_IO_VGA", (0.016, 0.008, 0.008), (x0 + 0.070, y + 0.003, z0 + 0.010),
        M["vga_blue"], root, coll, bevel=0.001)
    # iDRAC dedicated mgmt port
    box(prefix + "_IO_iDRAC", (0.016, 0.010, 0.013), (x0 + 0.100, y + 0.004, z0 + 0.008),
        M["chip"], root, coll, bevel=0.0005,
        props={"hw_category": "PORT", "hw_name": "iDRAC Management Port",
               "hw_model": "RJ45 1GbE", "hw_spec": "dedicated BMC"})
    # serial COM
    box(prefix + "_IO_COM", (0.013, 0.007, 0.007), (x0 + 0.128, y + 0.003, z0 + 0.009),
        M["black"], root, coll, bevel=0.0008)

def build_pcie_slots(prefix, slots, M, root, coll, rear_y):
    """Rear expansion slot openings; slots = list of (cx, cz, w, h) on the rear plane."""
    for i, (sx, sz, sw, sh) in enumerate(slots):
        box("%s_PCIe_%d" % (prefix, i), (sw, 0.0012, sh),
            (sx, rear_y + 0.0006, sz), M["tray"], root, coll, bevel=0.0004,
            props={"hw_category": "PCIE_SLOT", "hw_name": "PCIe Slot %d" % (i + 1),
                   "hw_model": "PCIe 5.0 x16 (riser)", "hw_spec": "expansion via riser cage"})

# ---------------------------------------------------------------- control panel details
def _front_power_btn(prefix, x, z, y, M, root, coll):
    cyl(prefix + "_BTN_Power", 0.005, 0.003, (x, y, z), M["black"], root, coll,
        rot=(math.radians(90), 0, 0),
        props={"hw_category": "BUTTON", "hw_name": "Power Button"})
    add_led(prefix, "Power", "power", M["led_pwr"], (x, y - 0.001, z + 0.009), root, coll,
            "Power status", "server.power", r=0.002)

def _front_usb(prefix, idx, x, z, y, M, root, coll):
    box("%s_FUSB_%d" % (prefix, idx), (0.0145, 0.004, 0.006), (x, y, z),
        M["usb_blue"], root, coll, bevel=0.0004,
        props={"hw_category": "PORT", "hw_name": "Front USB %d" % (idx + 1),
               "hw_model": "USB 3.0 Type-A", "hw_spec": "5Gb/s"})

def _front_vga(prefix, x, z, y, M, root, coll):
    box(prefix + "_FVGA", (0.016, 0.004, 0.008), (x, y, z), M["vga_blue"], root, coll,
        bevel=0.001, props={"hw_category": "PORT", "hw_name": "Front VGA",
                            "hw_model": "DE-15", "hw_spec": "BMC console video"})

# ---------------------------------------------------------------- server builder
def build_server(tag, u_count, depth, x_base, z_base, front_y, cfg, M, rails=False):
    """cfg: see build_all.py. layout keys: bays{x0,z0,count,w,d,h,form},
       grille_zones[(x0,z0,w,h)], ctrl_left, ctrl_right{vga,usb2}, rear_pcie{count,fh,x0,z0}"""
    prefix = "SRV_%s" % tag
    delete_server(prefix)
    coll = _coll(prefix)
    H = u_count * U - 0.001
    D = depth
    cy = front_y + D / 2.0          # chassis center y (world)
    root = bpy.data.objects.new(prefix + "_ROOT", None)
    coll.objects.link(root)
    root.location = (x_base, cy, z_base)
    root["hw_category"] = "SERVER"
    root["hw_name"] = cfg["name"]
    root["hw_model"] = cfg["model"]
    root["hw_spec"] = cfg["spec"]
    root["server_u"] = u_count

    # explosion amplitudes per U-class (graded, small, non-overlapping)
    EX_COVER = 0.10 + 0.035 * u_count          # 0.135 / 0.17 / 0.24
    EX_HS = 0.070 + 0.004 * u_count            # heatsinks
    EX_GPU = 0.085 + 0.020 * u_count           # gpus
    EX_FAN = 0.038 + 0.004 * u_count           # fans
    EX_DIMM = 0.040 + 0.003 * u_count          # dimms
    EX_DRIVE = 0.040                            # +0.010 stagger per bay
    EX_PSU = 0.050                              # +0.012 stagger per psu
    EX_MOBO = 0.0                               # mobo is the anchor plane — must not sink through floor

    # ---- chassis shell ----
    box(prefix + "_Floor", (CW - 0.004, D - 0.004, WALL), (0, 0, WALL / 2), M["steel"], root, coll)
    box(prefix + "_Wall_L", (WALL, D - 0.004, H), (-(CW / 2 - WALL / 2), 0, H / 2), M["steel"], root, coll)
    box(prefix + "_Wall_R", (WALL, D - 0.004, H), ((CW / 2 - WALL / 2), 0, H / 2), M["steel"], root, coll)
    box(prefix + "_RearPanel", (CW - 0.004, WALL, H), (0, D / 2 - WALL / 2, H / 2), M["steel"], root, coll)
    cover = box(prefix + "_TopCover", (CW - 0.002, D - 0.002, WALL), (0, 0, H - WALL / 2),
                M["steel"], root, coll,
                props={"hw_category": "COVER", "hw_name": "Top Cover", "hw_model": "-", "hw_spec": "tool-less latch"})
    add_explode(cover, (0, 0, EX_COVER))
    # top cover pressed vent slots (front intake zone), merged one mesh
    slots = []
    for r in range(3):
        for c in range(14):
            slots.append((0.018, 0.0035, 0.0004, -0.150 + c * 0.023, -D / 2 + 0.05 + r * 0.008, WALL / 2 + 0.0002))
    multi_box(prefix + "_CoverVents", slots, (0, 0, 0), M["black"], cover, coll)

    fy = -D / 2  # front plane (local)
    ear_w = (EW - CW) / 2  # 0.0263
    # ---- rack ears with screw holes ----
    for s in (-1, 1):
        ex = s * (CW / 2 + ear_w / 2)
        box("%s_Ear_%s" % (prefix, "L" if s < 0 else "R"), (ear_w, 0.005, H),
            (ex, fy - 0.0025, H / 2), M["bezel"], root, coll, bevel=0.0008)
        for zi in (0.25, 0.75):
            cyl("%s_EarHole_%s_%d" % (prefix, "L" if s < 0 else "R", int(zi * 100)),
                0.0035, 0.006, (ex, fy - 0.0025, H * zi), M["chip"], root, coll,
                rot=(math.radians(90), 0, 0), verts=16)

    # ---- front face zones (per layout cfg) ----
    bezel_parts = []
    bay = cfg["bays"]
    bw, bd, bh = bay["w"], bay["d"], bay["h"]
    bx0, bz0 = bay["x0"], bay["z0"]
    n = bay["count"]
    gap = 0.002
    bays_right = bx0 + n * bw + (n - 1) * gap
    # bezel filler panels: left of bays and right of bays (below/above grille zones)
    left_w = bx0 - (-CW / 2)
    right_x0 = bays_right
    right_w = CW / 2 - right_x0
    bz_top = bz0 + bh
    if left_w > 0.004:
        b = box(prefix + "_Bezel_L", (left_w, 0.006, bh), (-CW / 2 + left_w / 2, fy - 0.003, bz0 + bh / 2),
                M["bezel"], root, coll, bevel=0.0006)
        bezel_parts.append(b)
    if right_w > 0.004:
        b = box(prefix + "_Bezel_R", (right_w, 0.006, bh), (right_x0 + right_w / 2, fy - 0.003, bz0 + bh / 2),
                M["bezel"], root, coll, bevel=0.0006)
        bezel_parts.append(b)
    # strip above bays up to H (if any)
    if H - bz_top > 0.004 and not cfg.get("grille_above"):
        b = box(prefix + "_Bezel_T", (CW, 0.006, H - bz_top), (0, fy - 0.003, (bz_top + H) / 2),
                M["bezel"], root, coll, bevel=0.0006)
        bezel_parts.append(b)
    # ventilation grille zones (single mesh each)
    grilles = []
    for gi, (gx0, gz0, gw, gh) in enumerate(cfg.get("grille_zones", [])):
        g = grille("%s_Grille_%d" % (prefix, gi), gw, gh, 0.0035, 0.0008, 0.0015,
                   (gx0 + gw / 2, fy - 0.001, gz0 + gh / 2), M["bezel"], root, coll,
                   props={"hw_category": "VENT", "hw_name": "Ventilation Grille %d" % gi})
        grilles.append(g)
    # explode: bezel+grille move forward together (small)
    for b in bezel_parts + grilles:
        add_explode(b, (0, -0.055, 0))

    # ---- control panels on rack ears (Dell R650/R750 style) ----
    ear_cx = CW / 2 + ear_w / 2   # 0.22815
    led_y = fy - 0.0075
    if cfg.get("ctrl_left"):
        lx = -ear_cx
        # health LED bar (vertical light pipe)
        box(prefix + "_HealthBar", (0.004, 0.0025, min(0.055, H * 0.7)),
            (lx, fy - 0.006, H / 2), M["led_health"], root, coll, bevel=0.001,
            props={"hw_category": "LED", "led_type": "health",
                   "led_desc": "System health bar (chassis health + system ID)",
                   "data_bind": "server.health"})
        add_led(prefix, "UID", "uid", M["led_uid"], (lx, led_y, H - 0.010), root, coll,
                "Unit ID beacon", "server.uid", r=0.002)
        add_led(prefix, "Alert", "alert", M["led_alert"], (lx, led_y, 0.010), root, coll,
                "Fault alert", "server.alert", r=0.002)
    cr = cfg.get("ctrl_right")
    if cr:
        rx = ear_cx
        if cr.get("anchor") == "bottom":
            z_pwr, z_usb = bz_top - 0.010, bz_top - 0.021
        else:
            z_pwr, z_usb = H - 0.010, H - 0.021
        _front_power_btn(prefix, rx, z_pwr, led_y, M, root, coll)
        _front_usb(prefix, 0, rx, z_usb, led_y + 0.001, M, root, coll)
        if cr.get("usb2"):
            _front_usb(prefix, 1, rx, z_usb - 0.011, led_y + 0.001, M, root, coll)
        if cr.get("vga"):
            _front_vga(prefix, rx, 0.011, led_y + 0.001, M, root, coll)

    # ---- rack rails (only when mounted in a rack) ----
    if rails:
        for s in (-1, 1):
            box("%s_Rail_%s" % (prefix, "L" if s < 0 else "R"), (0.02, 1.0, 0.006),
                (s * 0.225, 0.57 - cy, -0.004), M["steel"], root, coll, bevel=0.0008)

    z0 = WALL  # interior floor
    # ---- drive bays (front) ----
    for i in range(n):
        bx = bx0 + bw / 2 + i * (bw + gap)
        by = fy - 0.004 + bd / 2      # tray face proud of bezel plane
        t = build_hdd(prefix, i, (bx, by, bz0), bw, bd, bh, M, root, coll, M["led_disk"], bay["form"])
        add_explode(t, (0, -(EX_DRIVE + 0.010 * i), 0))

    # ---- fan wall ----
    yfan = cfg.get("fan_y", fy + bay["d"] + 0.045)
    for fi, (size, fx, fz) in enumerate(cfg["fans"]):
        f = build_fan(prefix, fi, size, (fx, yfan, fz), root, coll, M)
        add_explode(f, (0, 0, EX_FAN + 0.005 * (fi % 3)))

    # ---- motherboard ----
    mw, md, mcy = cfg["mobo"]
    mobo = box(prefix + "_Motherboard", (mw, md, 0.0016), (0, mcy, z0 + 0.006),
               M["pcb"], root, coll, bevel=0.0005,
               props={"hw_category": "MOTHERBOARD", "hw_name": "Motherboard",
                      "hw_model": cfg["mobo_model"], "hw_spec": cfg["mobo_spec"]})
    add_explode(mobo, (0, 0, EX_MOBO))
    mz = z0 + 0.0068
    # chipset + vrm heatsinks
    box(prefix + "_Chipset_HS", (0.04, 0.04, 0.008), (0.10, mcy + md / 2 - 0.05, mz + 0.004), M["alu"], root, coll)
    box(prefix + "_VRM_HS_L", (0.015, 0.06, 0.012), (-0.09, mcy, mz + 0.006), M["alu"], root, coll)
    box(prefix + "_VRM_HS_R", (0.015, 0.06, 0.012), (0.09, mcy, mz + 0.006), M["alu"], root, coll)

    # ---- rear IO cluster + PCIe slots ----
    rear_y = D / 2 - WALL
    io = cfg.get("rear_io")
    if io:
        build_rear_io(prefix, io["x0"], io["z0"], M, root, coll, rear_y)
    if cfg.get("rear_pcie"):
        build_pcie_slots(prefix, cfg["rear_pcie"], M, root, coll, rear_y)

    # ---- CPUs (inline along Y) + DIMM banks ----
    cc = cfg["cpu_count"]
    for c in range(cc):
        cyy = mcy + (c - (cc - 1) / 2.0) * 0.11
        hs = build_cpu(prefix, c, (0.0, cyy, mz), cfg["hs_h"], M, root, coll,
                       cfg["cpu_model"], cfg["cpu_spec"])
        add_explode(hs, (0, 0, EX_HS + 0.012 * c))
        per_side = cfg["dimm_per_cpu"] // 2
        for side in (-1, 1):
            for j in range(per_side):
                idx = c * cfg["dimm_per_cpu"] + (0 if side < 0 else per_side) + j
                dx = 0.055 + j * 0.009
                d = build_dimm(prefix, idx, (side * dx, cyy, mz + 0.0313 / 2), M, root, coll)
                add_explode(d, (0, 0, EX_DIMM + 0.004 * j + (0.004 if side > 0 else 0)))

    # ---- GPUs ----
    for gi, g in enumerate(cfg["gpus"]):
        card = build_gpu(prefix, gi, (g["x"], g["y"], g["z"]), g["l"], g["w"], g["h"],
                         M, root, coll, g["model"], g["spec"])
        add_explode(card, (0, 0, EX_GPU + 0.015 * gi))

    # ---- PSUs (rear) ----
    p = cfg["psu"]
    pw, pd, ph = p["w"], p["d"], p["h"]
    pgap = 0.008
    total = p["count"] * pw + (p["count"] - 1) * pgap
    for i in range(p["count"]):
        px = -total / 2 + pw / 2 + i * (pw + pgap) + p.get("xoff", 0.0)
        py = D / 2 + 0.002 - pd / 2   # rear face proud of rear panel (like real CRPS cutout)
        psu = build_psu(prefix, i, (px, py, z0 + 0.001), pw, pd, ph, M, root, coll, p["watt"])
        add_explode(psu, (0, EX_PSU + 0.012 * i, 0))

    return root
