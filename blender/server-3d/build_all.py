# Builds all three servers (1U/2U/4U). Run inside Blender via exec().
# Layouts follow real machines:
#   1U ~ Dell PowerEdge R650 4xLFF  (left ear: health bar/UID/alert; right ear: power/USB/VGA)
#   2U ~ Dell PowerEdge R750 8xSFF  (portrait trays left, vent grille right)
#   4U ~ Supermicro 4029GP          (bottom SFF row + full-width top vent grille, 8 fans, 4 PSU)
exec(open(r'f:\aranea-agents\test\server-3d\server_lib.py').read())

def build_all(M=None):
    if M is None:
        M = build_materials()
    scn = bpy.context.scene
    scn.frame_start = 1
    scn.frame_end = 100
    scn.unit_settings.system = 'METRIC'

    cfg_1u = {
        'name': '1U Compute Server', 'model': 'Aranea R110 (1U)',
        'spec': '2P Xeon, 8x DDR5, 4x LFF, 1x LP GPU, 2x 800W',
        'cpu_model': 'Intel Xeon Silver 4510', 'cpu_spec': '12C/24T, 2.4GHz, 150W',
        'cpu_count': 2, 'hs_h': 0.026, 'dimm_per_cpu': 4,
        'mobo': (0.30, 0.24, -0.01), 'mobo_model': 'Aranea MB-1U-2P', 'mobo_spec': '2P LGA4677, C741, 8x DDR5',
        'bays': {'count': 4, 'w': 0.098, 'd': 0.147, 'h': 0.026, 'form': 'LFF',
                 'x0': -0.199, 'z0': 0.0087},
        'ctrl_left': True, 'ctrl_right': {'vga': True},
        'fan_y': -0.148,
        'fans': [(0.04, x, 0.022) for x in (-0.17, -0.102, -0.034, 0.034, 0.102, 0.17)],
        'gpus': [{'x': 0.128, 'y': 0.05, 'z': 0.005, 'l': 0.169, 'w': 0.069, 'h': 0.018,
                  'model': 'NVIDIA L4 24GB', 'spec': '24GB GDDR6, 72W, low-profile single-slot'}],
        'psu': {'count': 2, 'w': 0.075, 'd': 0.185, 'h': 0.039, 'watt': 800, 'xoff': 0.034},
        'rear_io': {'x0': -0.208, 'z0': 0.006},
        'rear_pcie': [(0.178, 0.009, 0.068, 0.013), (0.178, 0.027, 0.068, 0.013)],
    }
    cfg_2u = {
        'name': '2U Virtualization Server', 'model': 'Aranea R220 (2U)',
        'spec': '2P Xeon, 16x DDR5, 8x SFF, 2x GPU, 2x 1200W',
        'cpu_model': 'Intel Xeon Gold 6448Y', 'cpu_spec': '32C/64T, 2.1GHz, 225W',
        'cpu_count': 2, 'hs_h': 0.045, 'dimm_per_cpu': 8,
        'mobo': (0.33, 0.28, 0.005), 'mobo_model': 'Aranea MB-2U-2P', 'mobo_spec': '2P LGA4677, C741, 16x DDR5, 5x PCIe5',
        'bays': {'count': 8, 'w': 0.030, 'd': 0.11, 'h': 0.0725, 'form': 'SFF',
                 'x0': -0.202, 'z0': 0.007},
        'grille_zones': [(0.062, 0.005, 0.144, 0.073)],
        'ctrl_left': True, 'ctrl_right': {'usb2': True},
        'fans': [(0.06, x, 0.045) for x in (-0.17, -0.102, -0.034, 0.034, 0.102, 0.17)],
        'gpus': [
            {'x': -0.105, 'y': 0.0, 'z': 0.058, 'l': 0.267, 'w': 0.095, 'h': 0.024,
             'model': 'NVIDIA L40S 48GB', 'spec': '48GB GDDR6, 350W, dual-slot FHFL'},
            {'x': 0.105, 'y': 0.0, 'z': 0.058, 'l': 0.267, 'w': 0.095, 'h': 0.024,
             'model': 'NVIDIA L40S 48GB', 'spec': '48GB GDDR6, 350W, dual-slot FHFL'}],
        'psu': {'count': 2, 'w': 0.0735, 'd': 0.20, 'h': 0.039, 'watt': 1200, 'xoff': 0.0325},
        'rear_io': {'x0': -0.208, 'z0': 0.006},
        'rear_pcie': [(0.175, z, 0.075, 0.013) for z in (0.010, 0.030, 0.050, 0.070)],
    }
    cfg_4u = {
        'name': '4U AI Training Server', 'model': 'Aranea R540 (4U)',
        'spec': '2P Xeon, 16x DDR5, 10x SFF, 4x GPU, 4x 2000W',
        'cpu_model': 'Intel Xeon Platinum 8480+', 'cpu_spec': '56C/112T, 2.0GHz, 350W',
        'cpu_count': 2, 'hs_h': 0.10, 'dimm_per_cpu': 8,
        'mobo': (0.33, 0.33, 0.0), 'mobo_model': 'Aranea MB-4U-2P', 'mobo_spec': '2P LGA4677, C741, 16x DDR5, 8x PCIe5 x16',
        'bays': {'count': 10, 'w': 0.030, 'd': 0.11, 'h': 0.0725, 'form': 'SFF',
                 'x0': -0.198, 'z0': 0.008},
        'grille_zones': [(-0.202, 0.082, 0.404, 0.094)],
        'grille_above': True,
        'ctrl_left': True, 'ctrl_right': {'vga': True, 'usb2': True, 'anchor': 'bottom'},
        'fan_y': -0.225,
        'fans': [(0.08, x, z) for z in (0.043, 0.127) for x in (-0.165, -0.055, 0.055, 0.165)],
        'gpus': [
            {'x': -0.1425, 'y': -0.01, 'z': 0.125, 'l': 0.30, 'w': 0.09, 'h': 0.045,
             'model': 'NVIDIA H100 80GB PCIe', 'spec': '80GB HBM3, 350W, NVLink'},
            {'x': -0.0475, 'y': -0.01, 'z': 0.125, 'l': 0.30, 'w': 0.09, 'h': 0.045,
             'model': 'NVIDIA H100 80GB PCIe', 'spec': '80GB HBM3, 350W, NVLink'},
            {'x': 0.0475, 'y': -0.01, 'z': 0.125, 'l': 0.30, 'w': 0.09, 'h': 0.045,
             'model': 'NVIDIA H100 80GB PCIe', 'spec': '80GB HBM3, 350W, NVLink'},
            {'x': 0.1425, 'y': -0.01, 'z': 0.125, 'l': 0.30, 'w': 0.09, 'h': 0.045,
             'model': 'NVIDIA H100 80GB PCIe', 'spec': '80GB HBM3, 350W, NVLink'}],
        'psu': {'count': 4, 'w': 0.0735, 'd': 0.20, 'h': 0.039, 'watt': 2000},
        'rear_io': {'x0': -0.205, 'z0': 0.055},
        'rear_pcie': [(0.055, z, 0.10, 0.014) for z in (0.060, 0.080, 0.100)] +
                     [(0.165, z, 0.095, 0.014) for z in (0.060, 0.080, 0.100)],
    }
    r1 = build_server('1U', 1, 0.66, -1.1, 0.0, -0.33, cfg_1u, M)
    r2 = build_server('2U', 2, 0.70, 0.0, 0.0, -0.35, cfg_2u, M)
    r4 = build_server('4U', 4, 0.75, 1.15, 0.0, -0.375, cfg_4u, M)
    return r1, r2, r4
