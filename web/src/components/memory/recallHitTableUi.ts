import { REGISTRY_COL_W, registryCol, type RegistryTableColumn } from "../../features/ui/registryTableColumns";
import type { MemoryRecallHit } from "../../features/memory/types";

export const recallHitColumns: RegistryTableColumn<MemoryRecallHit>[] = [
  registryCol("id", "ID", "id", "left", REGISTRY_COL_W.nameWide),
  registryCol("total", "Total", (row) => row.scores.total, "right", REGISTRY_COL_W.metric),
  registryCol("cross_encoder", "CE", (row) => row.scores.cross_encoder, "right", REGISTRY_COL_W.metric)
];
