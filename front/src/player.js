import { useEffect, useRef, useState } from 'react'
import { get } from './api.js'

// usePlayer is the queue and the one audio element it plays through. A track
// is asked of its source as it comes up; the browser plays what comes back.
export function usePlayer() {
  const audio = useRef(null)
  const [queue, setQueue] = useState([])
  const [at, setAt] = useState(-1)
  const [playing, setPlaying] = useState(false)
  const [pos, setPos] = useState(0)
  const [dur, setDur] = useState(0)
  const [volume, setVolume] = useState(0.8)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const gen = useRef(0)

  const current = at >= 0 ? queue[at] : null

  const load = async (track, shouldPlay) => {
    const mine = ++gen.current
    setBusy(true)
    setErr('')
    try {
      const stream = await get(
        `/api/stream?src=${encodeURIComponent(track.source)}&id=${encodeURIComponent(track.id)}`,
      )
      if (mine !== gen.current) return
      const el = audio.current
      el.src = stream.url
      if (shouldPlay) await el.play()
    } catch (e) {
      if (mine === gen.current) setErr(e.message)
    } finally {
      if (mine === gen.current) setBusy(false)
    }
  }

  // play replaces the queue with tracks and starts the one at index.
  const play = (tracks, index = 0) => {
    const list = tracks.filter((t) => t && t.id)
    if (list.length === 0) return
    const i = Math.min(Math.max(index, 0), list.length - 1)
    setQueue(list)
    setAt(i)
    setPlaying(true)
    load(list[i], true)
  }

  const go = (i) => {
    if (i < 0 || i >= queue.length) return
    setAt(i)
    setPlaying(true)
    load(queue[i], true)
  }

  const toggle = () => {
    const el = audio.current
    if (!el || !el.src) return
    if (el.paused) el.play()
    else el.pause()
  }

  const seek = (ms) => {
    const el = audio.current
    if (el && Number.isFinite(ms)) el.currentTime = ms / 1000
  }

  useEffect(() => {
    const el = audio.current
    const onTime = () => setPos(el.currentTime * 1000)
    const onMeta = () => setDur((Number.isFinite(el.duration) ? el.duration : 0) * 1000)
    const onPlay = () => setPlaying(true)
    const onPause = () => setPlaying(false)
    const onEnded = () => {
      setAt((i) => {
        if (i + 1 < queue.length) {
          const next = i + 1
          load(queue[next], true)
          return next
        }
        return i
      })
    }
    el.addEventListener('timeupdate', onTime)
    el.addEventListener('durationchange', onMeta)
    el.addEventListener('play', onPlay)
    el.addEventListener('pause', onPause)
    el.addEventListener('ended', onEnded)
    return () => {
      el.removeEventListener('timeupdate', onTime)
      el.removeEventListener('durationchange', onMeta)
      el.removeEventListener('play', onPlay)
      el.removeEventListener('pause', onPause)
      el.removeEventListener('ended', onEnded)
    }
  }, [queue])

  useEffect(() => {
    if (audio.current) audio.current.volume = volume
  }, [volume])

  return {
    audio,
    queue,
    at,
    current,
    playing,
    pos,
    dur,
    volume,
    busy,
    err,
    play,
    go,
    toggle,
    seek,
    setVolume,
    next: () => go(at + 1),
    prev: () => (pos > 3000 ? seek(0) : go(at - 1)),
  }
}
