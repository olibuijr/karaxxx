import { useState, useEffect } from 'react'
import { useAuth } from '../lib/auth'

export default function FavoriteButton({ videoId }: { videoId: string }) {
  const { token } = useAuth()
  const [fav, setFav] = useState(false)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!token) return
    fetch(`/api/fav/video/${videoId}`, { headers: { Authorization: `Bearer ${token}` } })
      .then(r => r.json())
      .then(d => setFav(d.favorited))
      .catch(() => {})
  }, [token, videoId])

  const toggle = async (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    if (!token || busy) return
    setBusy(true)
    const method = fav ? 'DELETE' : 'POST'
    const res = await fetch(`/api/fav/video/${videoId}`, {
      method,
      headers: { Authorization: `Bearer ${token}` },
    })
    if (res.ok) setFav(!fav)
    setBusy(false)
  }

  if (!token) return null

  return (
    <button
      onClick={toggle}
      className={`absolute top-2 right-2 z-10 w-7 h-7 rounded-full flex items-center justify-center
                  transition-all duration-200 backdrop-blur-sm
                  ${fav ? 'bg-red/80 text-white scale-110' : 'bg-black/50 text-white/60 hover:text-white hover:bg-black/70'}`}
      aria-label={fav ? 'Remove from favorites' : 'Add to favorites'}
    >
      <svg width="14" height="14" viewBox="0 0 24 24" fill={fav ? 'currentColor' : 'none'} stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
        <path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z"/>
      </svg>
    </button>
  )
}
