var inputs = document.querySelectorAll('input');
var usernameInput = inputs[0];
var passwordInput = inputs[1];

// Focus and set value using Vue-compatible method
var nativeInputSetter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set;
nativeInputSetter.call(usernameInput, 'admin');
usernameInput.dispatchEvent(new Event('input', { bubbles: true }));
usernameInput.dispatchEvent(new Event('change', { bubbles: true }));

nativeInputSetter.call(passwordInput, 'admin123');
passwordInput.dispatchEvent(new Event('input', { bubbles: true }));
passwordInput.dispatchEvent(new Event('change', { bubbles: true }));

// Click login button
var buttons = document.querySelectorAll('button');
for (var i = 0; i < buttons.length; i++) {
  if (buttons[i].textContent.trim() === '登录') {
    buttons[i].click();
    break;
  }
}
'login attempted: ' + usernameInput.value + '/' + passwordInput.value;
