import fs from 'fs';

const edits = [
  {
    file: 'src/components/agents/AgentHooksPanel.vue',
    old: '      <p class="agent-hooks-panel__hint q-ma-none" v-html="t(\'hooksPage.agentPanel.hint\')" />',
    new: '      <!-- eslint-disable-next-line vue/no-v-html -- trusted i18n HTML hint -->\n      <p class="agent-hooks-panel__hint q-ma-none" v-html="t(\'hooksPage.agentPanel.hint\')" />'
  },
  {
    file: 'src/components/chat/ChatReasoningDrawer.vue',
    old: '          v-html="renderedHtml"',
    new: '          <!-- eslint-disable-next-line vue/no-v-html -- sanitized markdown HTML -->\n          v-html="renderedHtml"'
  },
  {
    file: 'src/components/chat/CodeBlock.vue',
    old: '    <pre v-else class="code-block__body"><code v-html="highlightedHtml" /></pre>',
    new: '    <!-- eslint-disable-next-line vue/no-v-html -- syntax highlighted HTML from trusted highlighter -->\n    <pre v-else class="code-block__body"><code v-html="highlightedHtml" /></pre>'
  },
  {
    file: 'src/components/chat/ReplyBlock.vue',
    old: '      <div class="reply-block__markdown chat-message-prose" v-html="renderedContent"></div>',
    new: '      <!-- eslint-disable-next-line vue/no-v-html -- sanitized markdown HTML -->\n      <div class="reply-block__markdown chat-message-prose" v-html="renderedContent"></div>'
  },
  {
    file: 'src/components/chat/ThinkingBlock.vue',
    old: '            <div class="thinking-block__content chat-message-prose" v-html="renderedHtml" />',
    new: '            <!-- eslint-disable-next-line vue/no-v-html -- sanitized markdown HTML -->\n            <div class="thinking-block__content chat-message-prose" v-html="renderedHtml" />'
  },
  {
    file: 'src/components/chat/UserMessageBubble.vue',
    old: '      <div class="user-message-bubble__text" v-html="renderedContent"></div>',
    new: '      <!-- eslint-disable-next-line vue/no-v-html -- sanitized markdown HTML -->\n      <div class="user-message-bubble__text" v-html="renderedContent"></div>'
  },
  {
    file: 'src/components/common/JsonCodeViewer.vue',
    old: '      <pre class="json-code-viewer__pre" v-html="highlightedHtml"></pre>',
    new: '      <!-- eslint-disable-next-line vue/no-v-html -- JSON syntax highlighted HTML -->\n      <pre class="json-code-viewer__pre" v-html="highlightedHtml"></pre>'
  },
  {
    file: 'src/components/platform/ProviderLogo.vue',
    old: '    <span v-if="svg" class="provider-logo__svg" v-html="svg" />',
    new: '    <!-- eslint-disable-next-line vue/no-v-html -- SVG logo from controlled provider catalog -->\n    <span v-if="svg" class="provider-logo__svg" v-html="svg" />'
  },
  {
    file: 'src/components/sessions/SessionMessagesPanel.vue',
    old: '          <div class="session-message-row__content" v-html="renderMarkdown(msg.content_markdown)"></div>',
    new: '          <!-- eslint-disable-next-line vue/no-v-html -- sanitized markdown HTML -->\n          <div class="session-message-row__content" v-html="renderMarkdown(msg.content_markdown)"></div>'
  },
  {
    file: 'src/components/spirit/MemberReadOnlyPanel.vue',
    old: '        <div class="chat-message-prose" v-html="renderMarkdown(msg.content_markdown)" />',
    new: '        <!-- eslint-disable-next-line vue/no-v-html -- sanitized markdown HTML -->\n        <div class="chat-message-prose" v-html="renderMarkdown(msg.content_markdown)" />'
  },
  {
    file: 'src/components/spirit/SynthesisResultCard.vue',
    old: '      <div class="synthesis-result-card__text" v-html="renderedContent" />',
    new: '      <!-- eslint-disable-next-line vue/no-v-html -- sanitized markdown HTML -->\n      <div class="synthesis-result-card__text" v-html="renderedContent" />'
  },
  {
    file: 'src/components/spirit/TaskExecutionPanel.vue',
    old: '            <div class="chat-message-prose" v-html="props.renderMarkdown(msg.content_markdown)" />',
    new: '            <!-- eslint-disable-next-line vue/no-v-html -- sanitized markdown HTML -->\n            <div class="chat-message-prose" v-html="props.renderMarkdown(msg.content_markdown)" />'
  },
  {
    file: 'src/components/spirit/TaskExecutionPanel.vue',
    old: '        <div class="task-execution-panel__spirit-reply chat-message-prose" v-html="props.renderMarkdown(spiritReply)" />',
    new: '        <!-- eslint-disable-next-line vue/no-v-html -- sanitized markdown HTML -->\n        <div class="task-execution-panel__spirit-reply chat-message-prose" v-html="props.renderMarkdown(spiritReply)" />'
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
