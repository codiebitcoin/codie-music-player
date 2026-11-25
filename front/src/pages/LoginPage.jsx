import { useState } from 'react'
import { post } from '../api.js'

export default function LoginPage({ onLogin }) {
  const [user, setUser] = useState('')
  const [pass, setPass] = useState('')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (e) => {
    e.preventDefault()
    setErr('')
    setBusy(true)
    try {
      await post('/api/login', { user, pass })
      onLogin()
    } catch (e2) {
      setErr(e2.message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="page-center">
      <form className="login-card" onSubmit={submit}>
        <div className="mark">♪</div>
        <div className="login-title">Codie Music Player</div>
        <p className="login-sub">The same sources as the desktop app, in the browser.</p>
        <input className="input" placeholder="Username" value={user} onChange={(e) => setUser(e.target.value)} autoFocus />
        <input className="input" type="password" placeholder="Password" value={pass} onChange={(e) => setPass(e.target.value)} />
        {err && <div className="login-err">{err}</div>}
        <button className="btn btn-accent" disabled={busy || !user || !pass}>
          {busy ? 'Signing in…' : 'Sign in'}
        </button>
      </form>
    </div>
  )
}
