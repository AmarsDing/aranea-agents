import fs from 'fs';
const files=[
  'components/a2a/A2AAuditPanel.vue',
  'components/a2a/A2ADiscoverPanel.vue',
  'components/a2a/A2AGatewayPanel.vue',
  'components/agents/AgentsListSection.vue',
  'components/evaluation/EvaluationResultsDialog.vue',
  'components/hooks/HooksTable.vue',
  'components/knowledge/KnowledgeDocumentsPanel.vue',
  'components/monitor/AuditTable.vue',
  'components/monitor/RealtimeEvents.vue',
  'components/monitor/TraceList.vue',
  'components/sessions/SessionsTableSection.vue',
  'components/tools/ToolDetailDrawer.vue',
  'features/channels/ChannelDeliveriesPanel.vue',
  'features/channels/ChannelTurnJobsPanel.vue',
  'features/memory/MemoryCascadePanel.vue',
  'features/memory/MemoryKnowledgePanel.vue',
  'features/memory/MemorySnapshotDrawer.vue'
];
for(const p of files){
  let s=fs.readFileSync('src/'+p,'utf8');
  const m=s.match(/<template>[\s\S]*?<\/template>(?=\s*<script)/);
  if(!m){console.log('NO TEMPLATE',p); continue;}
  const t0=m[0];
  let t=t0;
  t=t.replace(/(#[^\s=]+)="props"/g,'$1="slotProps"');
  t=t.replace(/:props="props"/g,':props="slotProps"');
  t=t.replace(/props\./g,'slotProps.');
  if(t!==t0){
    s=s.replace(t0,t);
    fs.writeFileSync('src/'+p,s);
    console.log('FIXED',p);
  } else {
    console.log('NO CHANGE',p);
  }
}
