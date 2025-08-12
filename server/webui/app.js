(() => {
  const $  = (id) => document.getElementById(id);
  const logEl = $("log"), badge = $("healthBadge"), etagEl = $("etagVal");

  // ---------- state ----------
  const S = {
    get base() { return $("baseUrl").value.trim().replace(/\/+$/,''); },
    set base(v){ $("baseUrl").value=v; localStorage.setItem("owgo.base", v); },

    get uuid(){ return localStorage.getItem("owgo.uuid") || ""; },
    set uuid(v){ localStorage.setItem("owgo.uuid", v); $("devUuid").textContent=v||"—"; },

    get key(){ return localStorage.getItem("owgo.key") || ""; },
    set key(v){ localStorage.setItem("owgo.key", v); $("devKey").textContent=v||"—"; },

    get group(){ return localStorage.getItem("owgo.group") || ""; },
    set group(v){ localStorage.setItem("owgo.group", v); $("groupId").textContent=v||"—"; },

    get root(){ return localStorage.getItem("owgo.root") || ""; },
    set root(v){ localStorage.setItem("owgo.root", v); $("rootPid").textContent=v||"—"; },

    get gprefix(){ return localStorage.getItem("owgo.gprefix") || ""; },
    set gprefix(v){ localStorage.setItem("owgo.gprefix", v); $("groupPid").textContent=v||"—"; },

    get etag(){ return localStorage.getItem("owgo.etag") || ""; },
    set etag(v){ localStorage.setItem("owgo.etag", v); etagEl.textContent = v || "—"; },
  };
  S.base = localStorage.getItem("owgo.base") || location.origin;
  $("devUuid").textContent=S.uuid||"—"; $("devKey").textContent=S.key||"—";
  $("groupId").textContent=S.group||"—"; $("rootPid").textContent=S.root||"—";
  $("groupPid").textContent=S.gprefix||"—"; etagEl.textContent=S.etag||"—";

  // ---------- helpers ----------
  const toastBox = (() => {
    let el;
    const show = (msg,type="ok")=>{
      if(!el){ el=document.createElement("div"); el.className="toast"; document.body.appendChild(el); }
      el.className="toast show " + (type==="err"?"err":"ok");
      el.textContent = msg;
      setTimeout(()=>{ el.classList.remove("show"); }, 1800);
    };
    return { show };
  })();

  const log = (m, type="INFO") => {
    const line = `[${new Date().toISOString()}] ${type}: ${m}\n`;
    logEl.textContent += line; logEl.scrollTop = logEl.scrollHeight;
  };

  async function req(method, path, body, headers={}) {
    const url = S.base + path;
    const init = { method, headers: {...headers} };
    if (body && !(body instanceof FormData)) {
      init.headers["Content-Type"] = "application/json";
      init.body = JSON.stringify(body);
    } else if (body instanceof FormData) {
      // keep multipart/form-data
      init.body = body;
    }
    const res = await fetch(url, init);
    const ct = res.headers.get("Content-Type") || "";
    if (!res.ok) {
      let detail = res.statusText;
      if (ct.includes("application/problem+json") || ct.includes("application/json")) {
        const pj = await res.json().catch(()=>null);
        if (pj) detail = (pj.title || res.statusText) + (pj.detail? (": "+pj.detail) : "");
      } else {
        const t = await res.text().catch(()=> "");
        detail = t || res.statusText;
      }
      throw new Error(`${res.status} ${detail}`);
    }
    if (ct.includes("application/json")) return res.json();
    if (ct.includes("application/gzip")) return { blob: await res.blob(), res };
    return res.text();
  }

  const ok   = () => { badge.textContent = "ok"; badge.style.background = "#1d3d2b"; };
  const bad  = () => { badge.textContent = "down"; badge.style.background = "#42222a"; };

  // ---------- Health ----------
  $("btnHealth").addEventListener("click", async () => {
    try {
      await req("GET", "/healthz"); ok(); log("healthz OK");
      const rdy = await fetch(S.base + "/readyz"); log(`readyz ${rdy.status}`);
      toastBox.show("Health OK");
    } catch (e) { bad(); log("health error: "+e.message, "ERR"); toastBox.show(e.message,"err"); }
  });
  $("baseUrl").addEventListener("change", () => S.base = $("baseUrl").value);

  // ---------- Register ----------
  $("formRegister").addEventListener("submit", async (ev) => {
    ev.preventDefault();
    const fd = new FormData(ev.target);
    const body = new URLSearchParams();
    for (const [k,v] of fd.entries()) body.append(k, v);
    try {
      const res = await fetch(S.base + "/controller/register/", {
        method: "POST", headers: {"Content-Type":"application/x-www-form-urlencoded"}, body
      });
      const txt = await res.text();
      if (!res.ok) throw new Error(`${res.status} ${txt}`);
      const pick = (k) => (txt.match(new RegExp(`^${k}:\\s*(.+)$`,`m`))||["",""])[1].trim();
      const uuid = pick("uuid"); const key = pick("key"); const isNew = pick("is-new");
      S.uuid = uuid; S.key = key;
      $("devStatus").textContent = isNew==="1" ? "new" : "existing";
      log(`registered: uuid=${uuid}, key=${key}, is-new=${isNew}`);
      toastBox.show("Registered");
    } catch (e) { log("register error: " + e.message, "ERR"); toastBox.show(e.message,"err"); }
  });

  // ---------- Checksum ----------
  $("btnChecksum").addEventListener("click", async () => {
    if (!S.uuid || !S.key) return alert("Register first");
    try {
      const sum = await req("GET", `/controller/checksum/${S.uuid}/?key=${encodeURIComponent(S.key)}`);
      $("checksumVal").textContent = (sum||"").toString().trim();
      log("checksum ok");
      toastBox.show("Checksum OK");
    } catch (e) { log("checksum error: "+e.message, "ERR"); toastBox.show(e.message,"err"); }
  });

  // ---------- Download ----------
  $("btnDownload").addEventListener("click", async () => {
    if (!S.uuid || !S.key) return alert("Register first");
    try {
      const h = {}; if (S.etag) h["If-None-Match"] = S.etag;
      const { blob, res } = await req("GET", `/controller/download-config/${S.uuid}/?key=${encodeURIComponent(S.key)}`, null, h);
      if (res.status === 304) { $("dlHint").textContent="Not modified (304)"; log("download 304"); toastBox.show("Not modified"); return; }
      const et = res.headers.get("ETag"); if (et) S.etag = et;
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url; a.download = "configuration.tar.gz";
      document.body.appendChild(a); a.click(); a.remove();
      URL.revokeObjectURL(url);
      $("dlHint").textContent = "Downloaded";
      log("download ok"); toastBox.show("Downloaded");
    } catch (e) { log("download error: "+e.message, "ERR"); toastBox.show(e.message,"err"); }
  });

  // ---------- Debug-config ----------
  $("btnDebugCfg").addEventListener("click", async () => {
    if (!S.uuid || !S.key) return alert("Register first");
    try {
      const j = await req("GET", `/controller/debug-config/${S.uuid}/?key=${encodeURIComponent(S.key)}`);
      $("debugCfg").textContent = JSON.stringify(j, null, 2);
      log("debug-config shown"); toastBox.show("Debug OK");
    } catch (e) { log("debug-config error: "+e.message, "ERR"); toastBox.show(e.message,"err"); }
  });

  // ---------- Vars catalog ----------
  $("btnCatalog").addEventListener("click", async () => {
    try {
      const j = await req("GET", "/api/v1/vars/catalog");
      $("catalogOut").textContent = JSON.stringify(j, null, 2);
      toastBox.show("Catalog loaded");
    } catch (e) { log("catalog error: "+e.message, "ERR"); toastBox.show(e.message,"err"); }
  });

  // ---------- Templates ----------
  $("btnSampleSystem").addEventListener("click", () => {
    const f = $("formTemplate");
    f.name.value = "system-hostname";
    f.path.value = "etc/config/system";
    f.body.value =
`config system 'system'
  option hostname '{{{{ .vars.hostname }}}}'
  option timezone '{{{{ .vars.timezone }}}}'
`;
  });
  $("btnSampleNetwork").addEventListener("click", () => {
    const f = $("formTemplate");
    f.name.value = "network-lan";
    f.path.value = "etc/config/network";
    f.body.value =
`config interface 'wan'
  option ifname '{{{{ .vars.wan_iface }}}}'
  option proto '{{{{ .vars.wan_proto }}}}'
{{"{{"}}- if eq .vars.wan_proto "static" -{{"}}"}}
  option ipaddr '{{"{{"}} .vars.ipv4_address {{"}}"}}'
  option netmask '{{"{{"}} .vars.ipv4_netmask {{"}}"}}'
  option gateway '{{"{{"}} .vars.ipv4_gateway {{"}}"}}'
{{"{{"}}- end -{{"}}"}}
`;
  });
  $("btnSampleWireless").addEventListener("click", () => {
    const f = $("formTemplate");
    f.name.value = "wireless";
    f.path.value = "etc/config/wireless";
    f.body.value =
`config wifi-device 'radio0'
  option country '{{{{ .vars.wifi_country }}}}'
  option band '{{{{ .vars.wifi_band }}}}'

config wifi-iface 'default_radio0'
  option ssid '{{{{ .vars.wifi_ssid }}}}'
  option encryption '{{{{ .vars.wifi_encryption }}}}'
  option key '{{{{ .vars.wifi_psk }}}}'
`;
  });

  $("formTemplate").addEventListener("submit", async (ev) => {
    ev.preventDefault();
    const fd = new FormData(ev.target);
    const payload = Object.fromEntries(fd.entries());
    try {
      const t = await req("POST", "/api/v1/templates", payload);
      $("lastTplId").textContent = t.id || t.ID || "—";
      log(`template created id=${t.id||t.ID}`); toastBox.show("Template created");
    } catch (e) { log("create template error: " + e.message, "ERR"); toastBox.show(e.message,"err"); }
  });

  $("btnListTpl").addEventListener("click", async () => {
    try {
      const list = await req("GET", "/api/v1/templates");
      const tb = $("tplTable").querySelector("tbody");
      tb.innerHTML = "";
      for (const t of list) {
        const tr = document.createElement("tr");
        tr.innerHTML = `<td>${t.id ?? t.ID}</td><td>${t.name ?? t.Name}</td><td>${t.path ?? t.Path}</td>
          <td><a class="action" data-id="${t.id ?? t.ID}" data-act="del">delete</a></td>`;
        tb.appendChild(tr);
      }
      tb.onclick = async (e) => {
        const a = e.target.closest("a.action"); if (!a) return;
        const id = a.dataset.id;
        if (a.dataset.act === "del") {
          if (!confirm(`Delete template #${id}?`)) return;
          try { await req("DELETE", `/api/v1/templates/${id}`); toastBox.show("Deleted"); $("btnListTpl").click(); }
          catch(e){ toastBox.show(e.message,"err"); log("delete template error: "+e.message,"ERR"); }
        }
      };
      toastBox.show("Templates listed");
    } catch (e) { log("list templates error: "+e.message, "ERR"); toastBox.show(e.message,"err"); }
  });

  $("btnAssignTpl").addEventListener("click", async () => {
    if (!S.uuid) return alert("Register first");
    const ids = $("assignTplIds").value.split(",").map(s=>s.trim()).filter(Boolean);
    try {
      for (const id of ids) {
        await req("POST", `/api/v1/devices/${S.uuid}/templates/${id}`, { enabled: true });
      }
      const list = await req("GET", `/api/v1/devices/${S.uuid}/templates`);
      $("tplList").textContent = JSON.stringify(list, null, 2);
      toastBox.show("Assigned");
      log("templates assigned");
    } catch (e) { log("assign template error: " + e.message, "ERR"); toastBox.show(e.message,"err"); }
  });

  // ---------- Device vars ----------
  $("btnUpsertVars").addEventListener("click", async () => {
    if (!S.uuid) return alert("Register first");
    let obj;
    try { obj = JSON.parse($("varsJSON").value || "{}"); }
    catch(e){ return alert("Vars JSON invalid: " + e.message); }
    try {
      // bulk endpoint если есть:
      await req("POST", `/api/v1/devices/${S.uuid}/vars/bulk`, obj);
      log("device vars upserted (bulk)"); toastBox.show("Vars saved");
    } catch (e) {
      // если сервак ещё без bulk — fallback по одному ключу
      try {
        for (const [k,v] of Object.entries(obj)) {
          await req("POST", `/api/v1/devices/${S.uuid}/vars`, { key: k, value: String(v) });
        }
        log("device vars upserted"); toastBox.show("Vars saved (single)");
      } catch (e2) { log("vars upsert error: " + e2.message, "ERR"); toastBox.show(e2.message,"err"); }
    }
  });

  $("btnShowVars").addEventListener("click", async () => {
    if (!S.uuid) return alert("Register first");
    try {
      const m = await req("GET", `/api/v1/devices/${S.uuid}/vars`);
      $("varsOut").textContent = JSON.stringify(m, null, 2);
      log("device vars listed"); toastBox.show("Vars shown");
    } catch (e) { log("vars list error: " + e.message, "ERR"); toastBox.show(e.message,"err"); }
  });

  // ---------- Groups ----------
  $("formGroup").addEventListener("submit", async (ev) => {
    ev.preventDefault();
    const name = new FormData(ev.target).get("name");
    try {
      const g = await req("POST", "/api/v1/groups", { name });
      const id = g.id || g.ID;
      S.group = String(id);
      log(`group ready id=${id}`); toastBox.show("Group ready");
    } catch (e) { log("create group error: " + e.message, "ERR"); toastBox.show(e.message,"err"); }
  });

  $("btnJoinGroup").addEventListener("click", async () => {
    if (!S.uuid) return alert("Register first");
    if (!S.group || !/^\d+$/.test(S.group)) return alert("Create/select group first");
    try {
      await req("POST", `/api/v1/devices/${S.uuid}/groups/${S.group}`, {});
      log("device joined group"); toastBox.show("Joined");
    } catch (e) { log("join group error: " + e.message, "ERR"); toastBox.show(e.message,"err"); }
  });

  $("btnLeaveGroup").addEventListener("click", async () => {
    if (!S.uuid) return alert("Register first");
    if (!S.group || !/^\d+$/.test(S.group)) return alert("Create/select group first");
    try {
      await req("DELETE", `/api/v1/devices/${S.uuid}/groups/${S.group}`);
      log("device removed from group"); toastBox.show("Removed");
    } catch (e) { log("leave group error: " + e.message, "ERR"); toastBox.show(e.message,"err"); }
  });

  $("btnListGroups").addEventListener("click", async () => {
    if (!S.uuid) return alert("Register first");
    try {
      const gs = await req("GET", `/api/v1/devices/${S.uuid}/groups`);
      $("groupsOut").textContent = JSON.stringify(gs, null, 2);
      log("device groups listed"); toastBox.show("Groups listed");
    } catch (e) { log("list groups error: " + e.message, "ERR"); toastBox.show(e.message,"err"); }
  });

  $("btnUpsertGVars").addEventListener("click", async () => {
    if (!S.group) return alert("Create group first");
    let obj;
    try { obj = JSON.parse($("gvarsJSON").value || "{}"); }
    catch(e){ return alert("Group vars JSON invalid: " + e.message); }
    try {
      for (const [k,v] of Object.entries(obj)) {
        await req("POST", `/api/v1/groups/${S.group}/vars`, { key: k, value: String(v) });
      }
      log("group vars upserted"); toastBox.show("Group vars saved");
    } catch (e) { log("group vars error: " + e.message, "ERR"); toastBox.show(e.message,"err"); }
  });

  // ---------- IPAM ----------
  $("btnCreateRoot").addEventListener("click", async () => {
    const cidr = $("rootCIDR").value.trim();
    try {
      const p = await req("POST", "/api/v1/ipam/prefixes", { cidr, note: "root" });
      const id = p.id || p.ID; S.root = String(id);
      log(`root prefix id=${id} cidr=${p.cidr||p.CIDR}`); toastBox.show("Root prefix created");
    } catch (e) { log("create root prefix error: " + e.message, "ERR"); toastBox.show(e.message,"err"); }
  });

  $("btnAssignGroupPrefix").addEventListener("click", async () => {
    if (!S.group) return alert("Create group first");
    if (!S.root) return alert("Create root prefix first");
    const len = $("newPrefixLen").value.trim() || "24";
    try {
      const p = await req("POST", `/api/v1/ipam/assign/group/${S.group}?parent=${S.root}&len=${encodeURIComponent(len)}&note=ui`);
      const id = p.id || p.ID; S.gprefix = String(id);
      log(`group prefix id=${id} cidr=${p.cidr||p.CIDR}`); toastBox.show("Group /24 assigned");
    } catch (e) { log("assign group prefix error: " + e.message, "ERR"); toastBox.show(e.message,"err"); }
  });

  $("btnAssignIP").addEventListener("click", async () => {
    if (!S.uuid) return alert("Register first");
    if (!S.group) return alert("Create/select group first");
    try {
      const r = await req("POST", `/api/v1/ipam/assign/device/${S.uuid}?group=${S.group}`);
      const list = await req("GET", `/api/v1/ipam/devices/${S.uuid}/ips`);
      $("ipList").textContent = JSON.stringify(list, null, 2);
      log(`ip assigned: ${r.address || r.Address}`); toastBox.show("IP assigned");
    } catch (e) { log("assign ip error: " + e.message, "ERR"); toastBox.show(e.message,"err"); }
  });
})();
