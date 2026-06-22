import fs from 'fs';

const edits = [
  {
    file: 'src/components/agents/AgentHooksPanel.vue',
    old: '      <!-- eslint-disable-next-line vue/no-v-html -- trusted i18n HTML hint -->\n      <!-- eslint-disable-next-line vue/no-v-html -- trusted i18n HTML hint -->\n      <p class="agent-hooks-panel__hint q-ma-none" v-html="t(\'hooksPage.agentPanel.hint\')" />',
    new: '      <!-- eslint-disable-next-line vue/no-v-html -- trusted i18n HTML hint -->\n      <p class="agent-hooks-panel__hint q-ma-none" v-html="t(\'hooksPage.agentPanel.hint\')" />'
  },
  {
    file: 'src/components/chat/ChatReasoningDrawer.vue',
    old: '        <div\n          class="chat-reasoning-drawer__content chat-message-prose"\n          :class="{ \'chat-message-content--dark\': isDark }"\n          :style="contentStyle"\n          <!-- eslint-disable-next-line vue/no-v-html -- sanitized markdown HTML -->\n          <!-- eslint-disable-next-line vue/no-v-html -- sanitized markdown HTML -->\n          v-html="renderedHtml"\n        />',
    new: '        <!-- eslint-disable-next-line vue/no-v-html -- sanitized markdown HTML -->\n        <div\n          class="chat-reasoning-drawer__content chat-message-prose"\n          :class="{ \'chat-message-content--dark\': isDark }"\n          :style="contentStyle"\n          v-html="renderedHtml"\n        />'
  },
  {
    file: 'src/components/common/CodeEditor.vue',
    old: '        <pre class="code-editor__highlight" aria-hidden="true"><code v-html="highlightedCode"></code></pre>',
    new: '        <!-- eslint-disable-next-line vue/no-v-html -- syntax highlighted code HTML -->\n        <pre class="code-editor__highlight" aria-hidden="true"><code v-html="highlightedCode"></code></pre>'
  }
];

for (const e of edits) {
  let s = fs.readFileSync(e.file, 'utf8');
  if (!s.includes(e.old)) {
    console.log('NOT FOUND', e.file);
    continue;
  }
  s = s.replace(e.old, e.new);
  fs.writeFileSync(e.file, s);
  console.log('FIXED', e.file);
}
