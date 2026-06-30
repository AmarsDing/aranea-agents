var inputs = document.querySelectorAll('input');
var buttons = document.querySelectorAll('button');
var result = {
  inputCount: inputs.length,
  inputs: [],
  buttonCount: buttons.length,
  buttons: []
};
for (var i = 0; i < inputs.length; i++) {
  result.inputs.push({
    type: inputs[i].type,
    name: inputs[i].name,
    placeholder: inputs[i].placeholder,
    id: inputs[i].id,
    value: inputs[i].value
  });
}
for (var j = 0; j < buttons.length; j++) {
  result.buttons.push({
    text: buttons[j].textContent.trim(),
    type: buttons[j].type,
    id: buttons[j].id
  });
}
JSON.stringify(result);
