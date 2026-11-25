import { useEffect, useMemo, useState } from 'react'
import { del, get, post } from '../api.js'
import { artists, clock, coverUrl } from '../api.js'
import { navigate, useData } from '../App.jsx'

// Shell is the desktop's window: a sidebar, the page, and the player along the foot.
export default function Shell({ route, player, onLogout }) {
  const { data } = useData('/api/sources')
  const sources = data?.sources || []
  const [src, setSrc] = useState('')
  const source = sources.find((s) => s.id === src) || sources[0]

  useEffect(() => {
    if (!src && sources[0]) setSrc(sources[0].id)
  }, [sources, src])

  const path = route.split('?')[0]
  const query = new URLSearchParams(route.split('?')[1] || '')

  const open = (to) => navigate(to)

  return (
    <div className="shell">
      <aside className="sidebar">
        <Sidebar sources={sources} source={source} onSource={setSrc} path={path} query={query} open={open} onLogout={onLogout} />
      </aside>
      <main className="main">
        <div className="page">
          {source && (
            <Page source={source} path={path} query={query} open={open} player={player} />
          )}
        </div>
        <PlayerBar player={player} open={open} />
      </main>
      <audio ref={player.audio} />
    </div>
  )
}

function Sidebar({ sources, source, onSource, path, query, open, onLogout }) {
  const can = (what) => source?.can?.includes(what)
  const here = (page) => path === page && (!query.get('page') || page !== '/browse')
  const items = useMemo(() => navItems(source), [source])

  return (
    <>
      {sources.length > 1 && (
        <select className="source-switch" value={source?.id || ''} onChange={(e) => { onSource(e.target.value); open('/') }}>
          {sources.map((s) => (
            <option key={s.id} value={s.id}>{s.name}</option>
          ))}
        </select>
      )}
      {can('search') && <SearchBox source={source} open={open} />}
      <nav className="nav">
        {items.map((item) => (
          <button key={item.to} className={here(item.match) ? 'nav-item on' : 'nav-item'} onClick={() => open(item.to)}>
            <span className="nav-icon">{item.icon}</span>
            {item.label}
          </button>
        ))}
      </nav>
      <button className="nav-item logout" onClick={async () => { await post('/api/logout'); onLogout() }}>Sign out</button>
    </>
  )
}

// navItems is the source's own navigation, or the pages the desktop shows.
function navItems(source) {
  if (!source) return []
  const base = `?src=${source.id}`
  const builtin = {
    home: { to: `/${base}`, match: '/', icon: '⌂', label: 'Home' },
    liked: { to: `/liked${base}`, match: '/liked', icon: '♡', label: 'Liked Songs' },
    savedAlbums: { to: `/albums${base}`, match: '/albums', icon: '▣', label: 'Saved Albums' },
    stations: { to: `/radio${base}`, match: '/radio', icon: '◉', label: 'Radio' },
  }
  const nav = source.navigation?.length ? source.navigation : [{ builtin: 'home' }, { builtin: 'liked' }, { builtin: 'stations' }]
  const items = []
  for (const n of nav) {
    if (n.builtin) {
      const b = builtin[n.builtin]
      if (!b) continue
      if (n.builtin !== 'home' && !source.can?.includes(n.builtin === 'stations' ? 'stations' : n.builtin)) continue
      items.push({ ...b, label: n.title || b.label })
    } else if (n.page && source.can?.includes('browse')) {
      items.push({ to: `/browse${base}&page=${encodeURIComponent(n.page)}`, match: '/browse', icon: '›', label: n.title })
    }
  }
  if (source.id === 'local') items.push({ to: `/library${base}`, match: '/library', icon: '☰', label: 'Folders' })
  return items
}

function SearchBox({ source, open }) {
  const [q, setQ] = useState('')
  const [tips, setTips] = useState([])
  useEffect(() => {
    if (!q || !source?.can?.includes('suggest')) { setTips([]); return }
    const t = setTimeout(() => {
      get(`/api/suggest?src=${source.id}&q=${encodeURIComponent(q)}`).then((d) => setTips(d || [])).catch(() => {})
    }, 200)
    return () => clearTimeout(t)
  }, [q, source])

  const go = (text) => {
    setTips([])
    open(`/search?src=${source.id}&q=${encodeURIComponent(text)}`)
  }
  return (
    <div className="search">
      <input className="input" placeholder="Search" value={q} onChange={(e) => setQ(e.target.value)}
        onKeyDown={(e) => e.key === 'Enter' && q && go(q)} />
      {tips.length > 0 && (
        <div className="suggest">
          {tips.slice(0, 6).map((t) => <button key={t} onClick={() => go(t)}>{t}</button>)}
        </div>
      )}
    </div>
  )
}

function Page({ source, path, query, open, player }) {
  const src = query.get('src') || source.id
  if (path === '/search') return <SearchPage src={src} q={query.get('q') || ''} open={open} player={player} />
  if (path === '/playlist') return <TracksPage title="Playlist" path={`/api/source/${src}/playlist?id=${encodeURIComponent(query.get('id') || '')}`} pick={(d) => d?.tracks} open={open} player={player} src={src} />
  if (path === '/album') return <AlbumPage src={src} id={query.get('id') || ''} open={open} player={player} />
  if (path === '/artist') return <ArtistPage src={src} id={query.get('id') || ''} open={open} player={player} />
  if (path === '/liked') return <TracksPage title="Liked Songs" path={`/api/source/${src}/liked`} pick={(d) => d?.tracks} open={open} player={player} src={src} />
  if (path === '/radio') return <RadioPage src={src} open={open} player={player} />
  if (path === '/browse') return <BrowsePage src={src} page={query.get('page') || ''} open={open} player={player} />
  if (path === '/library') return <LibraryPage />
  return <HomePage src={src} open={open} player={player} />
}

function HomePage({ src, open, player }) {
  const { data, err, loading } = useData(`/api/source/${src}/home`)
  if (loading) return <p className="muted">Loading…</p>
  if (err && !/not supported|no such/i.test(err)) return <p className="error">{err}</p>
  if (!data?.sections?.length) return <RecentAndPlaylists src={src} open={open} player={player} />
  return (
    <>
      <h1>Home</h1>
      {data.sections.map((section, i) => (
        <Section key={i} section={section} src={src} open={open} player={player} />
      ))}
    </>
  )
}

function RecentAndPlaylists({ src, open, player }) {
  const recent = useData(`/api/source/${src}/recent`)
  const lists = useData(`/api/source/${src}/playlists`)
  const tracks = Array.isArray(recent.data) ? recent.data : []
  const playlists = Array.isArray(lists.data) ? lists.data : []
  if (!tracks.length && !playlists.length) {
    return (
      <>
        <h1>Home</h1>
        <p className="muted">Nothing here yet. Search for a song, or add a folder of your own music.</p>
      </>
    )
  }
  return (
    <>
      <h1>Home</h1>
      {tracks.length > 0 && <TrackList title="Recently Played" tracks={tracks.slice(0, 8)} src={src} player={player} />}
      {playlists.length > 0 && (
        <>
          <h2>Playlists</h2>
          <div className="cards">
            {playlists.map((p) => (
              <Card key={p.id} src={src} image={p.imageUrl} title={p.name} sub={`${p.trackCount || ''} songs`} onClick={() => open(`/playlist?src=${src}&id=${encodeURIComponent(p.id)}`)} />
            ))}
          </div>
        </>
      )}
    </>
  )
}

function Section({ section, src, open, player }) {
  return (
    <section>
      {section.title && <h2>{section.title}</h2>}
      {section.tracks?.length > 0 && <TrackList tracks={section.tracks} src={src} player={player} />}
      {section.albums?.length > 0 && (
        <div className="cards">
          {section.albums.map((a) => <Card key={a.id} src={src} image={a.imageUrl} title={a.name} sub={(a.artists || []).map((x) => x.name).join(', ')} onClick={() => open(`/album?src=${src}&id=${encodeURIComponent(a.id)}`)} />)}
        </div>
      )}
      {section.playlists?.length > 0 && (
        <div className="cards">
          {section.playlists.map((p) => <Card key={p.id} src={src} image={p.imageUrl} title={p.name} sub={p.owner} onClick={() => open(`/playlist?src=${src}&id=${encodeURIComponent(p.id)}`)} />)}
        </div>
      )}
      {section.artists?.length > 0 && (
        <div className="cards">
          {section.artists.map((a) => <Card key={a.id} src={src} image={a.imageUrl} title={a.name} round onClick={() => open(`/artist?src=${src}&id=${encodeURIComponent(a.id)}`)} />)}
        </div>
      )}
      {section.stations?.length > 0 && (
        <div className="cards">
          {section.stations.map((s) => <Card key={s.id} src={src} image={s.imageUrl} title={s.name} sub={s.description} onClick={() => playStation(src, s, player)} />)}
        </div>
      )}
    </section>
  )
}

function SearchPage({ src, q, open, player }) {
  const [kind, setKind] = useState('tracks')
  const { data, err, loading } = useData(`/api/search?src=${src}&q=${encodeURIComponent(q)}&kind=${kind}`)
  const kinds = ['tracks', 'albums', 'artists', 'playlists']
  return (
    <>
      <h1>Search · {q}</h1>
      <div className="tabs">
        {kinds.map((k) => <button key={k} className={k === kind ? 'on' : ''} onClick={() => setKind(k)}>{k}</button>)}
      </div>
      {loading && <p className="muted">Searching…</p>}
      {err && <p className="error">{err}</p>}
      {data && kind === 'tracks' && <TrackList tracks={data.tracks || []} src={src} player={player} />}
      {data && kind !== 'tracks' && (
        <div className="cards">
          {(data[kind] || []).map((item) => (
            <Card key={item.id} src={src} image={item.imageUrl} title={item.name}
              onClick={() => open(`/${kind === 'playlists' ? 'playlist' : kind.replace(/s$/, '')}?src=${src}&id=${encodeURIComponent(item.id)}`)} />
          ))}
        </div>
      )}
    </>
  )
}

function TracksPage({ title, path, pick, src, player }) {
  const { data, err, loading } = useData(path)
  const tracks = pick(data) || []
  const name = data?.name || data?.title || title
  return (
    <>
      <h1>{name}</h1>
      {loading && <p className="muted">Loading…</p>}
      {err && <p className="error">{err}</p>}
      <TrackList tracks={tracks} src={src} player={player} />
    </>
  )
}

function AlbumPage({ src, id, player }) {
  const { data, err, loading } = useData(`/api/source/${src}/album?id=${encodeURIComponent(id)}`)
  return (
    <>
      <h1>{data?.album?.name || 'Album'}</h1>
      {loading && <p className="muted">Loading…</p>}
      {err && <p className="error">{err}</p>}
      <TrackList tracks={data?.tracks || []} src={src} player={player} />
    </>
  )
}

function ArtistPage({ src, id, open, player }) {
  const { data, err, loading } = useData(`/api/source/${src}/artist?id=${encodeURIComponent(id)}`)
  return (
    <>
      <h1>{data?.artist?.name || 'Artist'}</h1>
      {loading && <p className="muted">Loading…</p>}
      {err && <p className="error">{err}</p>}
      <TrackList tracks={data?.tracks || []} src={src} player={player} />
      {data?.albums?.length > 0 && (
        <>
          <h2>Albums</h2>
          <div className="cards">
            {data.albums.map((a) => <Card key={a.id} src={src} image={a.imageUrl} title={a.name} sub={a.released} onClick={() => open(`/album?src=${src}&id=${encodeURIComponent(a.id)}`)} />)}
          </div>
        </>
      )}
    </>
  )
}

function BrowsePage({ src, page, open, player }) {
  const { data, err, loading } = useData(`/api/source/${src}/browse?page=${encodeURIComponent(page)}`)
  return (
    <>
      <h1>{data?.title || page}</h1>
      {data?.subtitle && <p className="muted">{data.subtitle}</p>}
      {loading && <p className="muted">Loading…</p>}
      {err && <p className="error">{err}</p>}
      {(data?.sections || []).map((section, i) => <Section key={i} section={section} src={src} open={open} player={player} />)}
    </>
  )
}

function RadioPage({ src, player }) {
  const { data, err, loading } = useData(`/api/source/${src}/stations`)
  const stations = Array.isArray(data) ? data : []
  return (
    <>
      <h1>Radio</h1>
      {loading && <p className="muted">Loading…</p>}
      {err && <p className="error">{err}</p>}
      <div className="cards">
        {stations.map((s) => <Card key={s.id} src={src} image={s.imageUrl} title={s.name} sub={s.description} onClick={() => playStation(src, s, player)} />)}
      </div>
    </>
  )
}

async function playStation(src, station, player) {
  const tracks = await get(`/api/source/${src}/station?id=${encodeURIComponent(station.id)}`)
  player.play(tracks || [])
}

function LibraryPage() {
  const { data, err } = useData('/api/folders')
  const [path, setPath] = useState('')
  const [note, setNote] = useState('')
  const add = async (e) => {
    e.preventDefault()
    await post('/api/folders', { path })
    setNote('Added. The folder is scanned in the background; reload Home in a moment.')
    setPath('')
  }
  return (
    <>
      <h1>Folders</h1>
      <p className="muted">{data?.tracks || 0} songs indexed.</p>
      {err && <p className="error">{err}</p>}
      <ul className="folders">
        {(data?.folders || []).map((f) => (
          <li key={f}>
            <span>{f}</span>
            <button onClick={() => del(`/api/folders?path=${encodeURIComponent(f)}`).then(() => location.reload())}>Remove</button>
          </li>
        ))}
      </ul>
      <form className="folder-add" onSubmit={add}>
        <input className="input" placeholder="/path/to/music" value={path} onChange={(e) => setPath(e.target.value)} />
        <button className="btn" disabled={!path}>Add folder</button>
      </form>
      {note && <p className="muted">{note}</p>}
    </>
  )
}

function TrackList({ title, tracks, src, player }) {
  if (!tracks.length) return null
  const stamped = tracks.map((t) => ({ ...t, source: t.source || src }))
  return (
    <>
      {title && <h2>{title}</h2>}
      <ol className="tracks">
        {stamped.map((t, i) => (
          <li key={t.id + i} className={player.current?.id === t.id ? 'on' : ''} onDoubleClick={() => player.play(stamped, i)}>
            <button className="play-mini" onClick={() => player.play(stamped, i)}>▶</button>
            <span className="track-name">{t.name}</span>
            <span className="track-artist">{artists(t)}</span>
            <span className="track-album">{t.album?.name}</span>
            <span className="track-time">{clock(t.duration)}</span>
          </li>
        ))}
      </ol>
    </>
  )
}

function Card({ src, image, title, sub, onClick, round }) {
  return (
    <button className="card" onClick={onClick}>
      <span className={round ? 'cover round' : 'cover'} style={image ? { backgroundImage: `url(${coverUrl(src, image)})` } : {}} />
      <span className="card-title">{title}</span>
      {sub && <span className="card-sub">{sub}</span>}
    </button>
  )
}

function PlayerBar({ player, open }) {
  const t = player.current
  const dur = player.dur || t?.duration || 0
  return (
    <footer className="player">
      <div className="now">
        <span className="cover mini" style={t?.album?.imageUrl ? { backgroundImage: `url(${coverUrl(t.source, t.album.imageUrl)})` } : {}} />
        <span className="now-text">
          <strong>{t ? t.name : 'Not Playing'}</strong>
          <em>{player.err || (t ? artists(t) : '')}</em>
        </span>
      </div>
      <div className="transport">
        <div className="transport-buttons">
          <button onClick={player.prev} disabled={!t}>⏮</button>
          <button className="play" onClick={player.toggle} disabled={!t}>{player.playing ? '⏸' : '▶'}</button>
          <button onClick={player.next} disabled={!t}>⏭</button>
        </div>
        <div className="scrub">
          <span>{clock(player.pos)}</span>
          <input type="range" min="0" max={dur || 0} value={Math.min(player.pos, dur || 0)}
            onChange={(e) => player.seek(Number(e.target.value))} />
          <span>{clock(dur)}</span>
        </div>
      </div>
      <div className="volume">
        {t && <Lyrics t={t} open={open} />}
        <input type="range" min="0" max="1" step="0.01" value={player.volume} onChange={(e) => player.setVolume(Number(e.target.value))} />
      </div>
    </footer>
  )
}

function Lyrics({ t }) {
  const [open, setOpen] = useState(false)
  const [lines, setLines] = useState(null)
  useEffect(() => { setOpen(false); setLines(null) }, [t?.id])
  const show = async () => {
    setOpen(true)
    if (lines) return
    try {
      const data = await get(`/api/lyrics?src=${t.source}&id=${encodeURIComponent(t.id)}`)
      setLines(data?.lines?.length ? data.lines.map((l) => l.text) : (data?.lrc || '').split('\n'))
    } catch (e) {
      setLines([e.message])
    }
  }
  return (
    <>
      <button onClick={show}>Lyrics</button>
      {open && (
        <div className="lyrics" onClick={() => setOpen(false)}>
          {(lines || ['Loading…']).map((l, i) => <p key={i}>{l}</p>)}
        </div>
      )}
    </>
  )
}
