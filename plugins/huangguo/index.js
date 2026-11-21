// Huangguo, a source of short dramas, for personal watching. The catalog,
// the search and the streams come from the site's own API, which asks for
// nothing signed; a stream's picture and sound are one HLS playlist. The
// covers come encrypted under a key the site publishes for its own page,
// which is what decrypts them here. Nothing is kept or served of the site's:
// this reads it as its page does. For study only.

const BASE = "https://huangguoai.com";
// As the site's own page asks: a phone's browser.
const UA = "Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 Chrome/120 Mobile Safari/537.36";

// What the covers are encrypted under, as the site's crypto-image-protocol.js
// publishes to its page. A cover that starts as a picture is plain already.
const IMG_KEY = new TextEncoder().encode("f5d965df75336270");
const IMG_IV = new TextEncoder().encode("97b60394abc2fbe1");

const isPicture = (b) => (b[0] === 0xff && b[1] === 0xd8) || (b[0] === 0x89 && b[1] === 0x50);

// api asks the site, whose answers wrap what they carry in {status, data}.
async function api(path) {
  const r = await fetch(BASE + path, { headers: { "User-Agent": UA, Accept: "application/json" } });
  if (!r.ok) throw new Error("HTTP " + r.status);
  const j = await r.json();
  return j && j.data !== undefined ? j.data : j;
}

// dramasOf is the dramas of an answer, whatever shape it came in.
function dramasOf(d) {
  if (Array.isArray(d)) return d;
  for (const k of ["items", "list", "videos"]) {
    if (d && Array.isArray(d[k])) return d[k];
  }
  return [];
}

// pagesOf is how far a catalog answer says its pages go.
function pagesOf(d) {
  const pg = d && (d.page_info || d.pagination || d);
  return { page: +((pg && pg.page) || 1), pages: +((pg && pg.pages) || (pg && pg.total_pages) || 1) };
}

// cover is a picture the app can show for the site's cover at url, which it
// hands as bytes it encrypted, or as a plain picture already.
async function cover(url) {
  if (!url) return "";
  try {
    const r = await fetch(url, { headers: { "User-Agent": UA } });
    if (!r.ok) return "";
    const data = new Uint8Array(await r.arrayBuffer());
    let plain = data;
    if (!isPicture(data)) {
      if (data.length === 0 || data.length % 16 !== 0) return "";
      plain = new Uint8Array(host.crypto.aesCbcDecrypt(IMG_KEY, IMG_IV, data));
    }
    if (!isPicture(plain)) return "";
    let binary = "";
    for (let i = 0; i < plain.length; i += 0x8000) {
      binary += String.fromCharCode.apply(null, plain.subarray(i, i + 0x8000));
    }
    return "data:image/jpeg;base64," + btoa(binary);
  } catch (e) {
    return "";
  }
}

// itemOf is a card of the window's for a drama of the site's.
function itemOf(v) {
  return {
    id: String(v.id),
    title: v.title || "",
    cover: v.cover || "",
    subtitle: (v.episode_count || v.total_episodes ? (v.episode_count || v.total_episodes) + " · " : "") + ((v.author && v.author.name) || ""),
  };
}

// dramaOf is the page's for a drama of the site's, its episodes one to each.
function dramaOf(d) {
  const episodes = (d.episodes || []).map((ep) => ({
    id: String(ep.ep_num),
    title: (d.title || "") + " · " + ep.ep_num,
    duration: ep.duration ? +ep.duration * 1000 : 0,
  }));
  return {
    id: String(d.id),
    title: d.title || "",
    cover: d.cover || "",
    subtitle: ((d.author && d.author.name) || "") + " · " + (d.is_finished ? host.t("Completed") : host.t("Ongoing")),
    intro: d.description || d.synopsis || "",
    episodes,
  };
}

// videoHome is the window's first page: the catalog, the ranks and the
// categories, each with more behind it.
export async function videoHome() {
  const rows = [];
  const [videos, cats, rank] = await Promise.all([
    api("/api/videos?page=1").catch(() => null),
    api("/api/site/categories/video").catch(() => null),
    api("/api/ranks/hot").catch(() => null),
  ]);
  if (rank) {
    const hot = dramasOf(rank).slice(0, 10).map(itemOf);
    for (const item of hot) item.cover = await cover(item.cover);
    if (hot.length) rows.push({ title: host.t("Hot"), items: hot });
  }
  if (videos) {
    const fresh = dramasOf(videos).slice(0, 30).map(itemOf);
    for (const item of fresh) item.cover = await cover(item.cover);
    const pg = pagesOf(videos);
    rows.push({ title: host.t("All"), items: fresh, morePage: pg.page < pg.pages ? "videos:2" : "" });
  }
  if (cats) {
    const all = dramasOf(cats).filter((c) => c.id !== undefined && (c.name || c.title));
    // A category's page is the catalog through its filter.
    for (const c of all.slice(0, 20)) {
      rows.push({ title: c.name || c.title, morePage: "cate:" + c.id });
    }
  }
  return { title: "Huangguo", rows: rows.filter((r) => r.items === undefined || r.items.length || r.morePage) };
}

// videoBrowse is a page a row's More opened: videos:N is the catalog at page
// N, cate:id:N is it through a category's filter at page N.
export async function videoBrowse({ page }) {
  const parts = page.split(":");
  let path = "/api/videos?page=1";
  let title = host.t("All");
  let next = "";
  if (parts[0] === "videos") {
    const n = +parts[1] || 1;
    path = "/api/videos?page=" + n;
    title = host.t("All") + " · " + n;
    next = "videos:" + (n + 1);
  } else if (parts[0] === "cate") {
    const id = parts[1];
    const n = +(parts[2] || 1);
    path = "/api/videos?page=" + n + "&cate_id=" + encodeURIComponent(id);
    title = host.t("Category");
    next = "cate:" + id + ":" + (n + 1);
  } else {
    throw new Error("no such page");
  }
  const d = await api(path);
  const items = dramasOf(d).map(itemOf);
  for (const item of items) item.cover = await cover(item.cover);
  const pg = pagesOf(d);
  return { title, rows: [{ title: "", items, morePage: pg.page < pg.pages ? next : "" }] };
}

// videoSearch is what the site's search says of what's asked.
export async function videoSearch({ query, offset }) {
  const d = await api("/api/search?q=" + encodeURIComponent(query) + "&page=" + (Math.floor(offset / 20) + 1));
  const items = dramasOf(d).map(itemOf);
  for (const item of items) item.cover = await cover(item.cover);
  return { items, more: items.length >= 20 };
}

// videoItem is a drama and its episodes, as the site's detail page reads.
export async function videoItem({ id }) {
  const d = await api("/api/videos/" + encodeURIComponent(id));
  const drama = dramaOf(d);
  drama.cover = await cover(drama.cover);
  if (!drama.episodes.length) {
    const count = +d.episode_count || +d.total_episodes || 0;
    for (let n = 1; n <= count; n++) {
      drama.episodes.push({ id: String(n), title: (drama.title || "") + " · " + n });
    }
  }
  return drama;
}

// videoPlay is what an episode plays through: the site answers an HLS
// playlist, its address carrying the signature it needs. Some dramas give
// nothing for an episode, and say so.
export async function videoPlay({ id, episode }) {
  const r = await fetch(`${BASE}/api/videos/${encodeURIComponent(id)}/episodes/${encodeURIComponent(episode)}/playback`, {
    method: "POST",
    headers: { "User-Agent": UA, "Content-Type": "application/json" },
    body: "{}",
  });
  if (!r.ok) throw fail("unavailable", "HTTP " + r.status);
  const j = await r.json();
  const d = j && j.data !== undefined ? j.data : j;
  const st = (d && d.streams) || {};
  const url = st.main || st.hevc || st.preview;
  if (!url) throw fail("unavailable", (j && j.msg) || "");
  return { url, type: "hls" };
}

// An error with a code tells the app what to do about it.
function fail(code, message) {
  const e = new Error(message);
  e.code = code;
  return e;
}
