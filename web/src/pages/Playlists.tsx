import { useEffect, useState, useRef } from 'react'
import { Link } from 'react-router-dom'
import { useAuth } from '../lib/auth'
import type { Video } from '../types'
import VideoCard from '../components/VideoCard'
import { EllipsisIcon, PlusIcon, CheckIcon, PencilIcon, Trash2Icon, XIcon } from 'lucide-react'

interface Playlist {
  id: number
  name: string
  video_count: number
  created_at: string
}

export default function Playlists() {
  const { token, user } = useAuth()
  const [playlists, setPlaylists] = useState<Playlist[]>([])
  const [loading, setLoading] = useState(true)
  const [playlistsError, setPlaylistsError] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const [newName, setNewName] = useState('')
  const [expanded, setExpanded] = useState<number | null>(null)
  const [playlistVideos, setPlaylistVideos] = useState<Record<number, Video[]>>({})
  const [playlistVideoErrors, setPlaylistVideoErrors] = useState<Record<number, string>>({})
  const [expLoading, setExpLoading] = useState<Record<number, boolean>>({})
  const [renaming, setRenaming] = useState<number | null>(null)
  const [renameVal, setRenameVal] = useState('')
  const [menuOpen, setMenuOpen] = useState<number | null>(null)
  const menuRef = useRef<HTMLDivElement>(null)

  const fetchPlaylists = () => {
    if (!token) {
      setPlaylists([])
      setPlaylistsError(null)
      setLoading(false)
      return
    }

    setLoading(true)
    setPlaylistsError(null)
    fetch('/api/playlists', { headers: { Authorization: `Bearer ${token}` } })
      .then(r => {
        if (!r.ok) throw new Error('Playlists failed')
        return r.json() as Promise<Playlist[]>
      })
      .then(setPlaylists)
      .catch(() => {
        setPlaylists([])
        setPlaylistsError("Couldn't load your playlists.")
      })
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    fetchPlaylists()
  }, [token])

  useEffect(() => {
    function onMouseDown(e: MouseEvent) {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) setMenuOpen(null)
    }
    document.addEventListener('mousedown', onMouseDown)
    return () => document.removeEventListener('mousedown', onMouseDown)
  }, [])

  const create = async () => {
    if (!token || !newName.trim()) return
    const res = await fetch('/api/playlists', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
      body: JSON.stringify({ name: newName.trim() }),
    })
    if (res.ok) {
      setNewName('')
      setCreating(false)
      fetchPlaylists()
    }
  }

  const rename = async (id: number) => {
    if (!token || !renameVal.trim()) return
    await fetch(`/api/playlists/${id}`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
      body: JSON.stringify({ name: renameVal.trim() }),
    })
    setRenaming(null)
    fetchPlaylists()
  }

  const del = async (id: number) => {
    if (!token) return
    await fetch(`/api/playlists/${id}`, {
      method: 'DELETE',
      headers: { Authorization: `Bearer ${token}` },
    })
    setMenuOpen(null)
    if (expanded === id) setExpanded(null)
    fetchPlaylists()
  }

  const toggleExpand = async (id: number) => {
    if (expanded === id) { setExpanded(null); return }
    setExpanded(id)
    if (!playlistVideos[id]) {
      setExpLoading(prev => ({ ...prev, [id]: true }))
      setPlaylistVideoErrors(prev => {
        const next = { ...prev }
        delete next[id]
        return next
      })
      try {
        const res = await fetch(`/api/playlists/${id}`, { headers: { Authorization: `Bearer ${token}` } })
        if (!res.ok) throw new Error('Playlist videos failed')
        const data: { id: string; title: string; slug: string; duration: number; views: number; thumb_uuid: string; preview_url: string; added_at: string; upload_date: string; source: string; categories: string; position: number }[] = await res.json()
        const vids = data.map(d => ({
          id: d.id,
          slug: d.slug,
          title: d.title,
          description: '',
          categories: d.categories ? d.categories.split(',').filter(Boolean) : [],
          tags: [],
          uploader: '',
          upload_date: d.upload_date || '',
          duration: d.duration,
          views: d.views,
          added_at: d.added_at,
          source: d.source,
          thumb_uuid: d.thumb_uuid,
          url_360: '',
          url_720: '',
          url_1080: '',
          preview_url: d.preview_url,
          hls_url: '',
        }))
        setPlaylistVideos(prev => ({ ...prev, [id]: vids }))
      } catch {
        setPlaylistVideoErrors(prev => ({ ...prev, [id]: "Couldn't load videos." }))
      }
      setExpLoading(prev => ({ ...prev, [id]: false }))
    }
  }

  const removeVideo = async (playlistId: number, videoId: string) => {
    if (!token) return
    await fetch(`/api/playlists/${playlistId}/videos/${videoId}`, {
      method: 'DELETE',
      headers: { Authorization: `Bearer ${token}` },
    })
    setPlaylistVideos(prev => ({
      ...prev,
      [playlistId]: (prev[playlistId] || []).filter(v => v.id !== videoId),
    }))
    setPlaylists(prev => prev.map(p =>
      p.id === playlistId ? { ...p, video_count: p.video_count - 1 } : p
    ))
  }

  function formatDate(iso: string) {
    if (!iso) return ''
    const d = new Date(iso)
    if (isNaN(d.getTime())) return ''
    return d.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
  }

  if (!user) {
    return (
      <div className="text-center py-24">
        <p className="text-muted mb-4">Sign in to manage playlists</p>
        <Link to="/" className="text-orange hover:underline font-semibold">Browse videos</Link>
      </div>
    )
  }

  return (
    <div className="max-w-[1800px] mx-auto px-3 py-3 md:px-6 md:py-5">
      <div className="flex items-center justify-between mb-4">
        <div>
          <h1 className="text-lg font-bold tracking-tight md:text-xl">Your Playlists</h1>
          <p className="text-xs text-muted mt-1">{playlists.length} playlists</p>
        </div>
        {!creating && (
          <button
            onClick={() => setCreating(true)}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded-full text-xs font-semibold
                       bg-orange text-black hover:bg-orange/90 transition-colors"
          >
            <PlusIcon className="w-3.5 h-3.5" />
            Create Playlist
          </button>
        )}
      </div>

      {creating && (
        <form
          onSubmit={e => { e.preventDefault(); create() }}
          className="flex gap-2 mb-4"
        >
          <input
            value={newName}
            onChange={e => setNewName(e.target.value)}
            placeholder="Playlist name..."
            autoFocus
            className="flex-1 px-3 py-2 text-sm rounded-lg bg-card border border-border text-text outline-none
                       focus:border-red/40 transition-colors"
          />
          <button
            type="submit"
            className="px-3 py-2 text-sm font-semibold rounded-lg bg-orange text-black hover:bg-orange/90 transition-colors"
          >
            Create
          </button>
          <button
            type="button"
            onClick={() => { setCreating(false); setNewName('') }}
            className="px-3 py-2 text-sm text-muted hover:text-text transition-colors"
          >
            Cancel
          </button>
        </form>
      )}

      {loading ? (
        <div className="text-center py-16 text-muted">Loading...</div>
      ) : playlistsError ? (
        <div className="text-center py-16 text-muted">{playlistsError}</div>
      ) : playlists.length === 0 ? (
        <div className="text-center py-16 text-muted">
          No playlists yet. Create one to start organizing videos.
        </div>
      ) : (
        <div className="space-y-2">
          {playlists.map(pl => (
            <div key={pl.id} className="rounded-lg bg-card border border-border overflow-hidden">
              <div
                onClick={() => toggleExpand(pl.id)}
                className="flex items-center gap-3 px-4 py-3 cursor-pointer hover:bg-white/[0.02] transition-colors"
              >
                <div className="flex-1 min-w-0">
                  {renaming === pl.id ? (
                    <form
                      onSubmit={e => { e.preventDefault(); rename(pl.id) }}
                      onClick={e => e.stopPropagation()}
                      className="flex gap-2"
                    >
                      <input
                        value={renameVal}
                        onChange={e => setRenameVal(e.target.value)}
                        autoFocus
                        className="flex-1 px-2 py-1 text-sm rounded bg-bg border border-border text-text outline-none
                                   focus:border-red/40"
                      />
                      <button
                        type="submit"
                        className="text-orange hover:text-orange/80 transition-colors"
                      >
                        <CheckIcon className="w-4 h-4" />
                      </button>
                      <button
                        type="button"
                        onClick={() => setRenaming(null)}
                        className="text-muted hover:text-text transition-colors"
                      >
                        <XIcon className="w-4 h-4" />
                      </button>
                    </form>
                  ) : (
                    <span className="text-sm font-semibold text-text">{pl.name}</span>
                  )}
                  <div className="text-xs text-muted mt-0.5">
                    {pl.video_count} videos · Created {formatDate(pl.created_at)}
                  </div>
                </div>
                <div className="relative" onClick={e => e.stopPropagation()} ref={menuRef}>
                  <button
                    onClick={() => setMenuOpen(menuOpen === pl.id ? null : pl.id)}
                    className="p-1 text-muted hover:text-text transition-colors rounded hover:bg-white/5"
                    aria-label="Playlist actions"
                  >
                    <EllipsisIcon className="w-4 h-4" />
                  </button>
                  {menuOpen === pl.id && (
                    <div className="absolute right-0 top-full mt-1 z-50 w-36 rounded-lg bg-card border border-border shadow-xl overflow-hidden">
                      <button
                        onClick={() => { setRenameVal(pl.name); setRenaming(pl.id); setMenuOpen(null) }}
                        className="flex items-center gap-2 w-full px-3 py-2 text-xs text-left text-text hover:bg-white/5 transition-colors"
                      >
                        <PencilIcon className="w-3.5 h-3.5" />
                        Rename
                      </button>
                      <button
                        onClick={() => del(pl.id)}
                        className="flex items-center gap-2 w-full px-3 py-2 text-xs text-left text-red hover:bg-red/10 transition-colors"
                      >
                        <Trash2Icon className="w-3.5 h-3.5" />
                        Delete
                      </button>
                    </div>
                  )}
                </div>
              </div>

              {expanded === pl.id && (
                <div className="border-t border-border p-3">
                  {expLoading[pl.id] ? (
                    <div className="text-center py-8 text-muted text-xs">Loading videos...</div>
                  ) : playlistVideoErrors[pl.id] ? (
                    <div className="text-center py-8 text-muted text-xs">{playlistVideoErrors[pl.id]}</div>
                  ) : !playlistVideos[pl.id] || playlistVideos[pl.id].length === 0 ? (
                    <div className="text-center py-8 text-muted text-xs">No videos in this playlist</div>
                  ) : (
                    <div className="grid gap-2.5
                                    grid-cols-1
                                    sm:grid-cols-2 sm:gap-3
                                    md:grid-cols-3
                                    lg:grid-cols-4 lg:gap-3.5
                                    xl:grid-cols-5">
                      {playlistVideos[pl.id].map(v => (
                        <div key={v.id} className="relative group">
                          <VideoCard video={v} />
                          <button
                            onClick={() => removeVideo(pl.id, v.id)}
                            className="absolute top-2 right-2 z-20 w-6 h-6 rounded-full bg-black/60 text-white/80
                                       flex items-center justify-center opacity-0 group-hover:opacity-100
                                       hover:bg-red/80 hover:text-white transition-all duration-200"
                            aria-label="Remove from playlist"
                          >
                            <XIcon className="w-3.5 h-3.5" />
                          </button>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
