var inputs = document.querySelectorAll('input');
inputs[0].value = 'admin';
inputs[0].dispatchEvent(new Event('input', {bubbles: true}));
inputs[1].value = 'admin123';
inputs[1].dispatchEvent(new Event('input', {bubbles: true}));
var buttons = document.querySelectorAll('button');
for (var i = 0; i < buttons.length; i++) {
  if (buttons[i].textContent.trim() === '登录') {
    buttons[i].click();
    break;
  }
}
'done';
