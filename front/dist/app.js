// codie-music-player frontend — vanilla JS, no build step.
const $ = (sel) => document.querySelector(sel);
const api = {
  async req(method, path, body) {
    const res = await fetch(path, {
      method,
      headers: body ? { "Content-Type": "application/json" } : undefined,
      body: body ? JSON.stringify(body) : undefined,
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok || data.success === false) {
      throw new Error(data.message || `HTTP ${res.status}`);
    }
    return data.data;
  },
  get: (p) => api.req("GET", p),
  post: (p, b) => api.req("POST", p, b || {}),
  del: (p) => api.req("DELETE", p),
};

const fmtTime = (ms) => {
  const s = Math.max(0, Math.round(ms / 1000));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
};

// ---- tabs ----
document.querySelectorAll("nav button").forEach((btn) => {
  btn.onclick = () => {
    document.querySelectorAll("nav button").forEach((b) => b.classList.remove("active"));
    document.querySelectorAll(".tab").forEach((t) => t.classList.remove("active"));
    btn.classList.add("active");
    $(`#tab-${btn.dataset.tab}`).classList.add("active");
  };
});

// ---- devices ----
let pairingDeviceId = null;
async function refreshDevices() {
  try {
    const devices = await api.get("/api/devices");
    const status = await api.get("/api/devices/status");
    $("#deviceList").innerHTML = (devices || [])
      .map(
        (d) => `<div class="card row">
          <div><b>${esc(d.name || d.addr)}</b><span class="hint"> ${d.addr} ${d.paired ? "✅ 已配对" : "⚠️ 未配对"} ${d.active ? "· 使用中" : ""}</span></div>
          <button data-del="${d.id}">删除</button>
        </div>`
      )
      .join("");
    $("#deviceList").querySelectorAll("[data-del]").forEach((b) => {
      b.onclick = async () => {
        await api.del(`/api/devices/${b.dataset.del}`);
        refreshDevices();
      };
    });
    const badge = $("#deviceBadge");
    if (status.connected) {
      badge.textContent = `设备在线 ${status.tf_loaded ? "· TF 卡就绪" : "· ⚠️ 无TF卡"}`;
      badge.classList.add("good");
      badge.classList.remove("bad");
    } else {
      badge.textContent = "未连接设备";
      badge.classList.add("bad");
      badge.classList.remove("good");
    }
    $("#deviceStatus").textContent = JSON.stringify(status, null, 2);
  } catch (e) {
    $("#deviceStatus").textContent = e.message;
  }
}
$("#pairStartBtn").onclick = async () => {
  const name = $("#deviceName").value.trim();
  const addr = $("#deviceAddr").value.trim();
  if (!addr) return alert("填写设备地址");
  try {
    const r = await api.post("/api/devices", { name, addr });
    pairingDeviceId = r.id;
    $("#pairArea").hidden = false;
    $("#pairCode").focus();
  } catch (e) {
    alert(e.message);
  }
};
$("#pairConfirmBtn").onclick = async () => {
  if (!pairingDeviceId) return;
  try {
    await api.post(`/api/devices/${pairingDeviceId}/confirm`, { code: $("#pairCode").value.trim() });
    $("#pairArea").hidden = true;
    pairingDeviceId = null;
    refreshDevices();
  } catch (e) {
    alert(e.message);
  }
};

// ---- plugins ----
async function refreshPlugins() {
  const data = await api.get("/api/plugins");
  const presets = data.presets || [];
  $("#presetList").innerHTML = presets
    .map(
      (p) => `<div class="card row">
        <div><b>${esc(p.name)}</b> <span class="hint">${p.repo}@${p.branch} ${p.note ? "· " + esc(p.note) : ""}</span></div>
        ${p.installed ? '<span class="hint">已安装</span>' : `<button class="primary" data-install="${p.id}">安装</button>`}
      </div>`
    )
    .join("");
  $("#presetList").querySelectorAll("[data-install]").forEach((b) => {
    b.onclick = async () => {
      b.disabled = true;
      b.textContent = "安装中…";
      try {
        await api.post("/api/plugins/install", { id: b.dataset.install });
        await refreshPlugins();
        await refreshSearchPlugins();
      } catch (e) {
        alert(e.message);
        b.disabled = false;
        b.textContent = "安装";
      }
    };
  });

  const plugins = data.plugins || [];
  $("#installedList").innerHTML = plugins.length
    ? plugins
        .map(
          (p) => `<div class="card row" data-plugin="${p.id}">
          <div>
            <b>${esc(p.name || p.id)}</b> <span class="hint">v${esc(p.version || "?")} · ${p.loaded ? "已加载" : "加载失败"} ${p.error ? "· " + esc(p.error) : ""}</span>
            <div class="account hint"></div>
          </div>
          <div>
            <button data-signin>登录</button>
            <button data-signout>登出</button>
            <button data-uninstall>卸载</button>
          </div>
        </div>`
        )
        .join("")
    : '<p class="hint">还没有安装插件，从上面装一个吧。</p>';
  for (const el of $("#installedList").querySelectorAll("[data-plugin]")) {
    const id = el.dataset.plugin;
    el.querySelector("[data-signin]").onclick = () => startSignIn(id, el);
    el.querySelector("[data-signout]").onclick = async () => {
      try { await api.post(`/api/plugins/${id}/signout`); refreshPluginAccounts(); } catch (e) { alert(e.message); }
    };
    el.querySelector("[data-uninstall]").onclick = async () => {
      if (!confirm(`卸载 ${id}?`)) return;
      await api.del(`/api/plugins/${id}`);
      refreshPlugins();
      refreshSearchPlugins();
    };
  }
  refreshPluginAccounts();
}
async function refreshPluginAccounts() {
  for (const el of $("#installedList").querySelectorAll("[data-plugin]")) {
    const id = el.dataset.plugin;
    const box = el.querySelector(".account");
    try {
      const r = await api.get(`/api/plugins/${id}/account`);
      if (!r.supported) { box.textContent = "（无账号概念）"; continue; }
      box.textContent = r.account ? `👤 ${r.account.name || r.account.id}` : r.error ? "" : "未登录";
    } catch (e) { box.textContent = ""; }
  }
}

// QR sign-in
let qrTimer = null;
function startSignIn(id, el) {
  $("#qrModal").hidden = false;
  $("#qrTitle").textContent = `扫码登录 · ${id}`;
  $("#qrText").textContent = "等待二维码…";
  $("#qrImg").src = "";
  api.post(`/api/plugins/${id}/signin`).catch((e) => ($("#qrText").textContent = e.message));
  clearInterval(qrTimer);
  qrTimer = setInterval(async () => {
    try {
      const p = await api.get(`/api/plugins/${id}/signin/prompt`);
      if (p && p.qr) {
        $("#qrImg").src = `/api/plugins/${id}/signin/qr.png?t=${p.at}`;
        $("#qrText").textContent = p.text || "扫码中…";
      }
      const acc = await api.get(`/api/plugins/${id}/account`);
      if (acc && acc.account) {
        $("#qrText").textContent = `✅ 登录成功：${acc.account.name || acc.account.id}`;
        clearInterval(qrTimer);
        setTimeout(() => { $("#qrModal").hidden = true; refreshPluginAccounts(); }, 1200);
      }
    } catch (e) { /* keep polling */ }
  }, 1500);
}
$("#qrClose").onclick = () => { clearInterval(qrTimer); $("#qrModal").hidden = true; };

// ---- search ----
async function refreshSearchPlugins() {
  const data = await api.get("/api/plugins");
  const sel = $("#searchPlugin");
  const current = sel.value;
  sel.innerHTML = (data.plugins || [])
    .filter((p) => p.loaded)
    .map((p) => `<option value="${p.id}">${esc(p.name || p.id)}</option>`)
    .join("");
  if (current) sel.value = current;
}
async function doSearch() {
  const q = $("#searchInput").value.trim();
  const plugin = $("#searchPlugin").value;
  if (!q || !plugin) return;
  $("#searchStatus").textContent = "搜索中…";
  $("#searchResults").innerHTML = "";
  try {
    const r = await api.get(`/api/plugins/${plugin}/search?q=${encodeURIComponent(q)}`);
    const tracks = r.tracks || [];
    $("#searchStatus").textContent = tracks.length ? `${tracks.length} 条结果` : "没有结果";
    $("#searchResults").innerHTML = tracks
      .map(
        (t, i) => `<li class="row">
          <div class="grow">
            <div><b>${esc(t.name)}</b></div>
            <div class="hint">${esc(t.artist)} ${t.album ? "· " + esc(t.album) : ""} · ${fmtTime(t.duration)}</div>
          </div>
          <button data-add="${i}" title="加入队列">＋</button>
          <button data-play="${i}" class="primary" title="立即播放">▶</button>
        </li>`
      )
      .join("");
    window.__searchTracks = tracks;
    $("#searchResults").querySelectorAll("[data-add]").forEach((b) => {
      b.onclick = () => enqueue([tracks[+b.dataset.add]]);
    });
    $("#searchResults").querySelectorAll("[data-play]").forEach((b) => {
      b.onclick = async () => {
        const t = tracks[+b.dataset.play];
        const added = await enqueue([t]);
        await refreshQueue();
        await playQueue(added - 1 >= 0 ? added - 1 : undefined);
      };
    });
  } catch (e) {
    $("#searchStatus").textContent = `搜索失败：${e.message}`;
  }
}
$("#searchBtn").onclick = doSearch;
$("#searchInput").addEventListener("keydown", (e) => e.key === "Enter" && doSearch());

async function enqueue(tracks) {
  try {
    const r = await api.post("/api/queue", { tracks });
    await refreshQueue();
    return r.added;
  } catch (e) {
    alert(e.message);
  }
}

// ---- queue & playback ----
const stateLabels = {
  pending: "待处理", resolving: "取流中", downloading: "下载中",
  uploading: "上传中", ready: "✅ 就绪", failed: "❌ 失败",
};
async function refreshQueue() {
  const r = await api.get("/api/queue");
  const items = r.items || [];
  $("#queueList").innerHTML = items.length
    ? items
        .map(
          (it, i) => `<li class="row ${i === r.current ? "current" : ""}">
          <div class="grow">
            <div>${i === r.current ? "🔊 " : ""}<b>${esc(it.track.name)}</b></div>
            <div class="hint">${esc(it.track.artist)} · ${stateLabels[it.state] || it.state}${it.error ? " · " + esc(it.error) : ""}</div>
          </div>
          <button data-qplay="${i}" class="primary">▶</button>
          <button data-qdel="${i}">✕</button>
        </li>`
        )
        .join("")
    : '<li class="hint">队列为空，去搜索加歌吧</li>';
  $("#queueList").querySelectorAll("[data-qplay]").forEach((b) => {
    b.onclick = () => playQueue(+b.dataset.qplay);
  });
  $("#queueList").querySelectorAll("[data-qdel]").forEach((b) => {
    b.onclick = async () => { await api.del(`/api/queue/${b.dataset.qdel}`); refreshQueue(); };
  });
}
async function playQueue(index) {
  try {
    const body = index !== undefined && index !== null ? { index } : {};
    await api.post("/api/queue/play", body);
    setTimeout(refreshPlayback, 1500);
  } catch (e) {
    alert(e.message);
  }
}
$("#playAllBtn").onclick = () => playQueue();
$("#clearQueueBtn").onclick = async () => { await api.post("/api/queue/clear"); refreshQueue(); };

async function refreshPlayback() {
  try {
    const pb = await api.get("/api/playback");
    const np = $("#nowPlaying");
    if (pb && pb.track_id) {
      np.classList.remove("empty");
      np.textContent = `${pb.is_playing ? "🔊" : "⏸"} ${pb.track_name || pb.track_id} · ${fmtTime(pb.position_ms)} / ${fmtTime(pb.duration_ms)} · ${pb.status}`;
    } else {
      np.classList.add("empty");
      np.textContent = pb && pb.error_code ? `⚠️ ${pb.error_code}` : "未在播放";
    }
    $("#volume").value = pb ? pb.volume_level : 0;
    refreshQueueCurrent(pb);
  } catch (e) { /* device offline */ }
}
function refreshQueueCurrent(pb) {
  if (!pb || !pb.track_id) return;
  const r0 = window.__lastQueue;
  if (r0) {
    // cheap highlight refresh without refetch: handled on next queue poll
  }
}
document.querySelectorAll(".controls button").forEach((b) => {
  b.onclick = async () => {
    try { await api.post("/api/playback/control", { action: b.dataset.action }); }
    catch (e) { alert(e.message); }
    setTimeout(refreshPlayback, 800);
  };
});
$("#volume").onchange = async (e) => {
  try { await api.post("/api/playback/control", { action: "SET_VOLUME", volume_level: +e.target.value }); }
  catch (err) { alert(err.message); }
};

// ---- boot ----
function esc(s) {
  return String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}
(async function boot() {
  refreshDevices();
  await refreshPlugins();
  await refreshSearchPlugins();
  refreshQueue();
  refreshPlayback();
  setInterval(refreshDevices, 15000);
  setInterval(refreshQueue, 4000);
  setInterval(refreshPlayback, 2000);
})();
