/**
 * Template C — Registry 表格列宽提示（与 .app-registry-table table-layout: fixed 配合）
 */
export const registryCol = {
  name: { style: "width: 148px; min-width: 120px" },
  desc: { style: "width: 28%; max-width: 320px" },
  chips: { style: "width: 152px" },
  callbacks: { style: "width: 176px" },
  toggle: { style: "width: 72px" },
  scope: { style: "width: 88px" },
  sort: { style: "width: 96px" },
  actions: { style: "width: 120px" },
  status: { style: "width: 96px" },
  time: { style: "width: 148px" },
  stats: { style: "width: 128px" },
  mime: { style: "width: 120px" },
  size: { style: "width: 88px" },
  agent: { style: "width: 120px" },
  session: { style: "width: 120px" },
  duration: { style: "width: 88px" },
  error: { style: "width: 20%; max-width: 360px" },
  trigger: { style: "width: 96px" },
  version: { style: "width: 72px" },
  plugin: { style: "width: 140px" },
  phase: { style: "width: 120px" }
} as const;
