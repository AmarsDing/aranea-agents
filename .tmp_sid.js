(function(){
  try {
    var app = document.querySelector('#q-app').__vue_app__;
    var pinia = app.config.globalProperties.$pinia;
    var chatSession = pinia._s.get('chatSession');
    var sid = chatSession && chatSession.selectedSession ? chatSession.selectedSession.id : null;
    return sid || 'NULL';
  } catch(e) { return 'ERR:' + e.message; }
})()
