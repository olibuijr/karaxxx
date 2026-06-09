import { useState, useEffect, useRef } from 'react'
import { useAuth } from '../lib/auth'
import { CheckIcon, PlusIcon } from 'lucide-react'

interface Playlist {
  id: number
  name: string
  video_count: number
  created_at: string
}

export default function PlaylistButton({ videoId }: { videoId: string }) {
  const { token } = useAuth()
  const [open, setOpen] = useState(false)
  const [playlists, setPlaylists] = useState<Playlist[]>([])
  const [contained, setContained] = useState<Set<number>>(new Set())
  const [loading, setLoading] = useState(false)
  const [creating, setCreating] = useState(false)
  const [newName, setNewName] = useState('')
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open || !token) return
    setLoading(true)
    fetch('/api/playlists', { headers: { Authorization: `Bearer ${token}` } })
      .then(r => r.json())
      .then(async (list: Playlist[]) => {
        setPlaylists(list)
        const containedIds = new Set<number>()
        await Promise.all(list.map(async pl => {
          try {
            const res = await fetch(`/api/playlists/${pl.id}`, { headers: { Authorization: `Bearer ${token}` } })
            const videos: { id: string }[] = await res.json()
            if (videos.some(v => v.id === videoId)) containedIds.add(pl.id)
          } catch {}
        }))
        setContained(containedIds)
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [open, token, videoId])

  useEffect(() => {
    function onMouseDown(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onMouseDown)
    return () => document.removeEventListener('mousedown', onMouseDown)
  }, [])

  const toggle = async (pl: Playlist) => {
    if (!token) return
    const isContained = contained.has(pl.id)
    const url = isContained
      ? `/api/playlists/${pl.id}/videos/${videoId}`
      : `/api/playlists/${pl.id}/videos`
    const method = isContained ? 'DELETE' : 'POST'
    const body = isContained ? undefined : JSON.stringify({ video_id: videoId })
    const headers: Record<string, string> = { Authorization: `Bearer ${token}` }
    if (!isContained) headers['Content-Type'] = 'application/json'
    const res = await fetch(url, { method, headers, body })
    if (res.ok) {
      setContained(prev => {
        const next = new Set(prev)
        if (isContained) next.delete(pl.id)
        else next.add(pl.id)
        return next
      })
      setPlaylists(prev => prev.map(p =>
        p.id === pl.id ? { ...p, video_count: p.video_count + (isContained ? -1 : 1) } : p
      ))
    }
  }

  const create = async () => {
    if (!token || !newName.trim()) return
    const res = await fetch('/api/playlists', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
      body: JSON.stringify({ name: newName.trim() }),
    })
    if (res.ok) {
      const data = await res.json()
      const newPl: Playlist = { id: data.id, name: newName.trim(), video_count: 0, created_at: new Date().toISOString() }
      setPlaylists(prev => [newPl, ...prev])
      setNewName('')
      setCreating(false)
    }
  }

  if (!token) return null

  return (
    <div className="relative" ref={ref}>
      <button
        onClick={() => setOpen(v => !v)}
        className="flex items-center gap-1.5 px-3 py-1.5 rounded-full text-xs font-semibold
                   bg-card border border-border text-muted hover:text-text hover:border-red/40 transition-colors"
        aria-label="Add to playlist"
      >
        <PlusIcon className="w-3.5 h-3.5" />
        Playlist
      </button>
      {open && (
        <div className="absolute right-0 top-full mt-1 z-50 w-56 rounded-lg bg-card border border-border shadow-xl
                        overflow-hidden">
          {loading ? (
            <div className="px-3 py-2 text-xs text-muted">Loading...</div>
          ) : (
            <div className="max-h-60 overflow-y-auto">
              {playlists.map(pl => (
                <button
                  key={pl.id}
                  onClick={() => toggle(pl)}
                  className="flex items-center gap-2 w-full px-3 py-2 text-xs text-left text-text
                             hover:bg-white/5 transition-colors"
                >
                  <span className="flex-1 truncate">{pl.name}</span>
                  {contained.has(pl.id) && <CheckIcon className="w-3.5 h-3.5 text-orange flex-shrink-0" />}
                </button>
              ))}
              {playlists.length === 0 && !creating && (
                <div className="px-3 py-2 text-xs text-muted">No playlists yet</div>
              )}
            </div>
          )}
          <div className="border-t border-border">
            {creating ? (
              <form
                onSubmit={e => { e.preventDefault(); create() }}
                className="flex gap-1 p-2"
              >
                <input
                  value={newName}
                  onChange={e => setNewName(e.target.value)}
                  placeholder="Playlist name..."
                  autoFocus
                  className="flex-1 px-2 py-1 text-xs rounded bg-bg border border-border text-text outline-none
                             focus:border-red/40"
                />
                <button
                  type="submit"
                  className="px-2 py-1 text-xs font-semibold rounded bg-orange text-black hover:bg-orange/90 transition-colors"
                >
                  Create
                </button>
              </form>
            ) : (
              <button
                onClick={() => setCreating(true)}
                className="flex items-center gap-2 w-full px-3 py-2 text-xs text-muted hover:text-text hover:bg-white/5 transition-colors"
              >
                <PlusIcon className="w-3.5 h-3.5" />
                Create new
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
