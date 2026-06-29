(function(){
  var textarea = document.querySelector('textarea');
  if (!textarea) return JSON.stringify({error: 'no textarea found'});
  var msg = 'F:\\3D-Wind-Field-master，给我分析一下这个项目，分派两个team进行，一个负责代码分析，一个负责模拟数据分析，完成后给我汇总一下。';
  var nativeInputValueSetter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, 'value').set;
  nativeInputValueSetter.call(textarea, msg);
  textarea.dispatchEvent(new Event('input', { bubbles: true }));
  textarea.dispatchEvent(new Event('change', { bubbles: true }));
  return JSON.stringify({ok: true, value: textarea.value, len: textarea.value.length});
})()
