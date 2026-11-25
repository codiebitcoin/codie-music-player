// api is everything the page asks the server. A 401 means the viewer isn't
// signed in, which the page turns into the login.
export class Unauthorized extends Error {}

export async function api(path, opts = {}) {
  const res = await fetch(path, {
    headers: opts.body ? { 'Content-Type': 'application/json' } : {},
    ...opts,
    body: opts.body ? JSON.stringify(opts.body) : undefined,
  })
  if (res.status === 401) throw new Unauthorized()
  const text = await res.text()
  let data = null
  try {
    data = text ? JSON.parse(text) : null
  } catch {
    // A plain error body, as http.Error writes it.
  }
  if (!res.ok) throw new Error((data && data.error) || `request failed (${res.status})`)
  return data
}

export const get = (path) => api(path)
export const post = (path, body) => api(path, { method: 'POST', body })
export const del = (path) => api(path, { method: 'DELETE' })

// coverUrl is a picture as the page fetches it: through the server, which
// sends the source's own headers.
export const coverUrl = (src, u) =>
  u ? `/api/cover?src=${encodeURIComponent(src)}&u=${encodeURIComponent(u)}` : ''

// clock says a duration in milliseconds as m:ss.
export function clock(ms) {
  const s = Math.max(0, Math.round((ms || 0) / 1000))
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
}

// artists joins a track's artists as the credits read.
export function artists(track) {
  return (track.artists || []).map((a) => a.name).join(', ')
}
