// A video source that does everything a video source can, with made-up dramas
// and no network. A video plugin exports videoSearch, videoItem and videoPlay,
// and may export videoHome and videoBrowse besides; one whose plugin.json says
// "kind": "video", as this one does, is a source of videos alone, and stays out
// of the sources of music. Functions take one object of parameters, and may be
// async.

const dramas = [1, 2, 3].map((n) => ({
  id: `d${n}`,
  title: `Drama ${n}`,
  cover: "",
  subtitle: "3 episodes",
}));

const episodesOf = (id) => [1, 2, 3].map((n) => ({ id: `${id}-e${n}`, title: `Episode ${n}` }));

// An error with one of these codes tells the app what to do about it: signIn,
// setup, unavailable or unsupported. Any other shows its message.
function fail(code, message) {
  const e = new Error(message);
  e.code = code;
  return e;
}

// What the window shows first.
export async function videoHome() {
  return { title: "Demo Video", rows: [{ title: "Now on", items: dramas }] };
}

// Another page of the source's own, which a row's morePage opens.
export async function videoBrowse({ page }) {
  if (page !== "all") throw new Error("no such page");
  return { title: "Every drama", rows: [{ title: "All of them", items: dramas }] };
}

// What a search found: a page of items, and whether there's a next one.
export async function videoSearch({ query, offset }) {
  const found = dramas.filter((d) => d.title.toLowerCase().includes(query.toLowerCase()));
  return { items: offset > 0 ? [] : found, more: false };
}

// A video and its episodes, as its page shows.
export async function videoItem({ id }) {
  const drama = dramas.find((d) => d.id === id);
  if (!drama) throw fail("unavailable", "No such drama");
  return { ...drama, intro: `What Drama ${id} is about.`, episodes: episodesOf(id) };
}

// What an episode plays through: an mp4 or an hls address, with its audio
// either in it or, as audioUrl, beside it, and headers for the app to send.
export async function videoPlay({ id, episode }) {
  if (!episodesOf(id).some((e) => e.id === episode)) throw fail("unavailable", "No such episode");
  return { url: `https://demo.video/${id}/${episode}.mp4`, type: "mp4" };
}
