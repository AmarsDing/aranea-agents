const inputs = document.querySelectorAll('input');
const textInput = Array.from(inputs).find(i => i.type === 'text');
const pwdInput = Array.from(inputs).find(i => i.type === 'password');
const setVal = (el, val) => {
  const setter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set;
  setter.call(el, val);
  el.dispatchEvent(new Event('input', { bubbles: true }));
  el.dispatchEvent(new Event('change', { bubbles: true }));
};
setVal(textInput, 'admin');
setVal(pwdInput, 'changeme');
const btn = Array.from(document.querySelectorAll('button')).find(b => b.textContent.trim() === '登录');
btn.click();
'done'
