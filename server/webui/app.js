(function(){
  const $ = (id)=>document.getElementById(id);
  const log = (el, v)=>{ el.textContent = (typeof v==='string')? v : JSON.stringify(v,null,2); };

  // settings in localStorage
  const S = {
    baseUrl: $('baseUrl'),
    secret:  $('secret'),
    uuid:    $('uuid'),
    key:     $('key'),
  };
  function loadSettings(){
    try {
      const s = JSON.parse(localStorage.getItem('wisp-ui')||'{}');
      S.baseUrl.value = s.baseUrl || location.origin;
      S.secret.value  = s.secret  || '';
      S.uuid.value    = s.uuid    || '';
      S.key.value     = s.key     || '';
    } catch(_) {}
  }
  function saveSettings(){
    localStorage.setItem('wisp-ui', JSON.stringify({
      baseUrl: S.baseUrl.value.trim() || location.origin,
      secret:  S.secret.value.trim(),
      uuid:    S.uuid.value.trim(),
      key:     S.key.value.trim(),
    }));
  }
  $('save-settings').onclick = ()=>{ saveSettings(); alert('Saved'); };
  loadSettings();

  const headers = {'Content-Type':'application/json'};

  // ping
  $('btn-health').onclick = async()=>{
    try {
      const r = await fetch(S.baseUrl.value + '/healthz'); $('health-result').textContent = (r.ok?'OK '+r.status:'ERR '+r.status);
    } catch(e){ $('health-result').textContent='ERR'; }
  };
  $('btn-ready').onclick = async()=>{
    try {
      const r = await fetch(S.baseUrl.value + '/readyz'); $('ready-result').textContent = (r.ok?'OK '+r.status:'ERR '+r.status);
    } catch(e){ $('ready-result').textContent='ERR'; }
  };

  // device actions
  const dlog = $('device-log');
  $('btn-register').onclick = async()=>{
    saveSettings();
    const body = new URLSearchParams();
    body.set('secret', S.secret.value);
    body.set('name', $('dev-hostname').value || 'openwrt');
    body.set('backend', $('dev-backend').value || 'openwrt');
    body.set('mac_address', $('dev-mac').value || '');
    try {
      const r = await fetch(S.baseUrl.value + '/controller/register/', {method:'POST', body});
      const t = await r.text();
      log(dlog, t);

      // вытаскиваем uuid/key из ответа
      const uuid = /uuid:\s*([0-9a-f\-]{36})/i.exec(t)?.[1] || '';
      const key  = /key:\s*([0-9a-f]+)/i.exec(t)?.[1] || '';
      if (uuid) S.uuid.value = uuid;
      if (key)  S.key.value  = key;
      saveSettings();
    } catch(e){ log(dlog, String(e)); }
  };

  $('btn-checksum').onclick = async()=>{
    saveSettings();
    if(!S.uuid.value || !S.key.value){ return log(dlog, 'Set uuid/key first'); }
    const u = `${S.baseUrl.value}/controller/checksum/${S.uuid.value}/?key=${encodeURIComponent(S.key.value)}`;
    try { const r = await fetch(u); log(dlog, await r.text()); } catch(e){ log(dlog, String(e)); }
  };

  $('btn-download').onclick = async()=>{
    saveSettings();
    if(!S.uuid.value || !S.key.value){ return log(dlog, 'Set uuid/key first'); }
    const u = `${S.baseUrl.value}/controller/download-config/${S.uuid.value}/?key=${encodeURIComponent(S.key.value)}`;
    try {
      const r = await fetch(u);
      const b = await r.blob();
      const link = document.createElement('a');
      link.href = URL.createObjectURL(b);
      link.download = 'configuration.tar.gz';
      link.click();
      log(dlog, `Downloaded (${r.status})`);
    } catch(e){ log(dlog, String(e)); }
  };

  $('btn-debug').onclick = async()=>{
    saveSettings();
    if(!S.uuid.value || !S.key.value){ return log(dlog, 'Set uuid/key first'); }
    const u = `${S.baseUrl.value}/controller/debug-config/${S.uuid.value}/?key=${encodeURIComponent(S.key.value)}`;
    try { const r = await fetch(u); log(dlog, await r.json()); } catch(e){ log(dlog, String(e)); }
  };

  $('btn-report-ok').onclick = async()=>{
    saveSettings();
    if(!S.uuid.value || !S.key.value){ return log(dlog, 'Set uuid/key first'); }
    const body = new URLSearchParams();
    body.set('key', S.key.value);
    body.set('status', 'applied');
    try {
      const r = await fetch(`${S.baseUrl.value}/controller/report-status/${S.uuid.value}/`, {method:'POST', body});
      log(dlog, await r.text());
    } catch(e){ log(dlog, String(e)); }
  };

  // templates
  const tplTBody = document.querySelector('#tpl-table tbody');
  function rowTpl(t){
    const tr = document.createElement('tr');
    tr.innerHTML = `<td>${t.ID||t.id||''}</td>
      <td>${t.Name||t.name||''}</td>
      <td>${t.Type||t.type||'go'}</td>
      <td>${(t.Required??t.required)?'yes':'no'}</td>
      <td>${(t.Default??t.default)?'yes':'no'}</td>
      <td>${t.Path||t.path||''}</td>
      <td>
        <button data-act="del" data-id="${t.ID||t.id}">Del</button>
      </td>`;
    tr.addEventListener('click', async (ev)=>{
      const btn = ev.target.closest('button'); if(!btn) return;
      if(btn.dataset.act==='del'){
        const id = btn.dataset.id;
        const r = await fetch(`${S.baseUrl.value}/api/v1/templates/${id}`, {method:'DELETE'});
        if(r.ok){ $('btn-tpl-list').click(); }
      }
    });
    return tr;
  }
  $('btn-tpl-create').onclick = async()=>{
    const p = {
      name: $('tpl-name').value.trim(),
      path: $('tpl-path').value.trim(),
      type: $('tpl-type').value,
      body: $('tpl-body').value,
      required: $('tpl-required').checked,
      default: $('tpl-default').checked,
    };
    const r = await fetch(`${S.baseUrl.value}/api/v1/templates`, {method:'POST', headers, body: JSON.stringify(p)});
    const j = await r.json().catch(()=> ({}));
    alert(r.ok ? `created id=${j.ID||j.id}` : `error ${r.status}`);
  };
  $('btn-tpl-list').onclick = async()=>{
    const r = await fetch(`${S.baseUrl.value}/api/v1/templates`);
    const arr = await r.json().catch(()=>[]);
    tplTBody.innerHTML = '';
    arr.forEach(t => tplTBody.appendChild(rowTpl(t)));
  };

  // device vars / group vars
  const vLog = $('vars-log');
  $('btn-dev-var-upsert').onclick = async()=>{
    if(!S.uuid.value) return log(vLog,'Set uuid first');
    const p = {key:$('var-key').value.trim(), value:$('var-value').value.trim()};
    const r = await fetch(`${S.baseUrl.value}/api/v1/devices/${S.uuid.value}/vars`, {method:'POST', headers, body: JSON.stringify(p)});
    log(vLog, r.ok ? 'OK' : 'ERR '+r.status);
  };
  $('btn-dev-vars-list').onclick = async()=>{
    if(!S.uuid.value) return log(vLog,'Set uuid first');
    const r = await fetch(`${S.baseUrl.value}/api/v1/devices/${S.uuid.value}/vars`);
    log(vLog, await r.json().catch(()=>({})));
  };

  $('btn-group-create').onclick = async()=>{
    const p = {name:$('group-name').value.trim()};
    const r = await fetch(`${S.baseUrl.value}/api/v1/groups`, {method:'POST', headers, body: JSON.stringify(p)});
    const j = await r.json().catch(()=>({}));
    log(vLog, r.ok ? j : ('ERR '+r.status));
  };
  $('btn-group-list').onclick = async()=>{
    if(!S.uuid.value) return log(vLog,'Set uuid first');
    const r = await fetch(`${S.baseUrl.value}/api/v1/devices/${S.uuid.value}/groups`);
    log(vLog, await r.json().catch(()=>[]));
  };
  $('btn-join-group').onclick = async()=>{
    if(!S.uuid.value) return log(vLog,'Set uuid first');
    const gid = $('group-id').value.trim();
    const r = await fetch(`${S.baseUrl.value}/api/v1/devices/${S.uuid.value}/groups/${gid}`, {method:'POST', headers, body:'{}'});
    log(vLog, r.ok ? 'joined' : 'ERR '+r.status);
  };
  $('btn-group-var-upsert').onclick = async()=>{
    const gid = $('group-id').value.trim();
    const p = {key:$('gvar-key').value.trim(), value:$('gvar-value').value.trim()};
    const r = await fetch(`${S.baseUrl.value}/api/v1/groups/${gid}/vars`, {method:'POST', headers, body: JSON.stringify(p)});
    log(vLog, r.ok ? 'OK' : 'ERR '+r.status);
  };

  // group templates / blocks / resolved
  const aLog = $('assign-log');
  $('btn-assign-tpl-group').onclick = async()=>{
    const gid = $('agt-group-id').value.trim();
    const p = {
      template_id: parseInt($('agt-tpl-id').value,10)||0,
      order: parseInt($('agt-order').value,10)||100,
      enabled: $('agt-enabled').value === 'true'
    };
    const r = await fetch(`${S.baseUrl.value}/api/v1/groups/${gid}/templates`, {method:'POST', headers, body: JSON.stringify(p)});
    log(aLog, r.ok ? 'OK' : 'ERR '+r.status);
  };
  $('btn-block-tpl').onclick = async()=>{
    if(!S.uuid.value) return log(aLog,'Set uuid first');
    const tid = $('block-tpl-id').value.trim();
    const r = await fetch(`${S.baseUrl.value}/api/v1/devices/${S.uuid.value}/templates/${tid}/block`, {method:'POST'});
    log(aLog, r.ok ? 'blocked' : 'ERR '+r.status);
  };
  $('btn-unblock-tpl').onclick = async()=>{
    if(!S.uuid.value) return log(aLog,'Set uuid first');
    const tid = $('block-tpl-id').value.trim();
    const r = await fetch(`${S.baseUrl.value}/api/v1/devices/${S.uuid.value}/templates/${tid}/unblock`, {method:'POST'});
    log(aLog, r.ok ? 'unblocked' : 'ERR '+r.status);
  };
  $('btn-device-assign-tpl-list').onclick = async()=>{
    if(!S.uuid.value) return log(aLog,'Set uuid first');
    const r = await fetch(`${S.baseUrl.value}/api/v1/devices/${S.uuid.value}/templates`);
    log(aLog, await r.json().catch(()=>[]));
  };
  $('btn-resolved-templates').onclick = async()=>{
    if(!S.uuid.value) return log(aLog,'Set uuid first');
    const r = await fetch(`${S.baseUrl.value}/api/v1/devices/${S.uuid.value}/templates/resolved`);
    log(aLog, await r.json().catch(()=>[]));
  };

  // ipam
  const iLog = $('ipam-log');
  $('btn-group-prefix-add').onclick = async()=>{
    const gid = $('ipam-group-id').value.trim();
    const cidr = $('ipam-cidr').value.trim();
    const r = await fetch(`${S.baseUrl.value}/api/v1/groups/${gid}/prefixes`, {method:'POST', headers, body: JSON.stringify({cidr})});
    log(iLog, await r.json().catch(()=>({status:r.status})));
  };
  $('btn-alloc-ip').onclick = async()=>{
    if(!S.uuid.value) return log(iLog,'Set uuid first');
    const r = await fetch(`${S.baseUrl.value}/api/v1/ipam/assign/${S.uuid.value}`, {method:'POST'});
    log(iLog, await r.json().catch(()=>({status:r.status})));
  };

  // debug-config JSON preview
  const dbg = $('debug-log');
  $('btn-debug-build').onclick = async()=>{
    if(!S.uuid.value || !S.key.value) return log(dbg,'Set uuid/key first');
    const u = `${S.baseUrl.value}/controller/debug-config/${S.uuid.value}/?key=${encodeURIComponent(S.key.value)}`;
    try { const r = await fetch(u); log(dbg, await r.json()); } catch(e){ log(dbg, String(e)); }
  };

  // авто-пинг при загрузке
  $('btn-health').click();
})();
