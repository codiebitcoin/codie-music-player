import { useEffect, useState } from 'react'
import { get } from './api.js'
import { usePlayer } from './player.js'
import LoginPage from './pages/LoginPage.jsx'
import Shell from './pages/Shell.jsx'

// The path is the state, and navigate uses the History API.
export function navigate(path) {
  window.history.pushState({}, '', path)
  window.dispatchEvent(new PopStateEvent('popstate'))
}

export default function App() {
  const [route, setRoute] = useState(window.location.pathname + window.location.search)
  const [authed, setAuthed] = useState(null)
  const player = usePlayer()

  useEffect(() => {
    const onPop = () => setRoute(window.location.pathname + window.location.search)
    window.addEventListener('popstate', onPop)
    return () => window.removeEventListener('popstate', onPop)
  }, [])

  useEffect(() => {
    fetch('/api/sources')
      .then((res) => setAuthed(res.status !== 401))
      .catch(() => setAuthed(false))
  }, [])

  if (authed === null) return <div className="page-center">Loading…</div>
  if (!authed) return <LoginPage onLogin={() => setAuthed(true)} />
  return <Shell route={route} player={player} onLogout={() => setAuthed(false)} />
}

// useData loads path whenever it changes, and hands back the answer.
export function useData(path) {
  const [data, setData] = useState(null)
  const [err, setErr] = useState('')
  const [loading, setLoading] = useState(true)
  useEffect(() => {
    let gone = false
    setLoading(true)
    setErr('')
    get(path)
      .then((d) => !gone && setData(d))
      .catch((e) => !gone && setErr(e.message))
      .finally(() => !gone && setLoading(false))
    return () => {
      gone = true
    }
  }, [path])
  return { data, err, loading, reload: () => setData((d) => d) }
}
