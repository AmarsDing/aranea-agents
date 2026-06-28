(function() {
  const textareas = document.querySelectorAll('textarea.q-field__native');
  const input = textareas[0];
  if (!input) {
    return JSON.stringify({error: 'no input found'});
  }
  const setter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value').set;
  setter.call(input, '你好，请简单回复一句话确认收到');
  input.dispatchEvent(new Event('input', { bubbles: true }));
  input.dispatchEvent(new Event('change', { bubbles: true }));
  // Find send button by send icon
  const buttons = document.querySelectorAll('button');
  const sendBtn = Array.from(buttons).find(b => {
    const icon = b.querySelector('i.material-icons');
    return icon && icon.textContent.trim() === 'send' && !b.disabled;
  });
  if (sendBtn) {
    sendBtn.click();
    return JSON.stringify({sent: 'clicked_send_btn'});
  }
  // Fallback: Enter key
  input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', code: 'Enter', keyCode: 13, bubbles: true }));
  input.dispatchEvent(new KeyboardEvent('keypress', { key: 'Enter', code: 'Enter', keyCode: 13, bubbles: true }));
  input.dispatchEvent(new KeyboardEvent('keyup', { key: 'Enter', code: 'Enter', keyCode: 13, bubbles: true }));
  return JSON.stringify({sent: 'enter_key_fallback'});
})();
