(function(){
  var el=document.querySelector('#q-app');
  if(!el||!el.__vue_app__)return 'no vue';
  var p=el.__vue_app__.config.globalProperties['$pinia'];
  if(!p)return 'no pinia';
  var ms=p._s.get('message');
  if(!ms)return 'no messageStore';
  var ss=p._s.get('session');
  if(!ss)return 'no sessionStore';
  var sid=ss.selectedSession?ss.selectedSession.id:'none';
  var msgs=ms.getMessages(sid);
  return JSON.stringify({sid:sid,msgCount:msgs.length,msgIds:msgs.slice(0,5).map(function(m){return m.id+':'+m.role+':'+m.status})});
})()