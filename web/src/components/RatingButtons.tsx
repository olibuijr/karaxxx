import { useState, useEffect, useCallback } from 'react'
import { useAuth } from '../lib/auth'

interface RatingData {
  rating: number
  up_count: number
  down_count: number
}

export default function RatingButtons({ videoId, compact }: { videoId: string; compact?: boolean }) {
  const { token } = useAuth()
  const [data, setData] = useState<RatingData>({ rating: 0, up_count: 0, down_count: 0 })
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!token) return
    fetch(`/api/rate/${videoId}`, { headers: { Authorization: `Bearer ${token}` } })
      .then(r => r.json())
      .then(d => setData(d))
      .catch(() => {})
  }, [token, videoId])

  const rate = useCallback(async (value: 1 | -1) => {
    if (!token || busy) return
    setBusy(true)
    const newRating = data.rating === value ? 0 : value
    const delta = (() => {
      if (data.rating === 0) return value === 1 ? { up: 1 } : { down: 1 }
      if (data.rating === value) return value === 1 ? { up: -1 } : { down: -1 }
      return value === 1 ? { up: 1, down: -1 } : { down: 1, up: -1 }
    })()
    setData(prev => ({
      ...prev,
      rating: newRating,
      up_count: prev.up_count + (delta.up ?? 0),
      down_count: prev.down_count + (delta.down ?? 0),
    }))
    const res = await fetch(`/api/rate/${videoId}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
      body: JSON.stringify({ rating: newRating }),
    })
    if (res.ok) {
      const d = await res.json()
      setData({ rating: d.rating, up_count: d.up_count, down_count: d.down_count })
    } else {
      setData(prev => ({
        ...prev,
        up_count: prev.up_count - (delta.up ?? 0),
        down_count: prev.down_count - (delta.down ?? 0),
      }))
    }
    setBusy(false)
  }, [token, busy, videoId, data.rating])

  if (!token) return null

  return (
    <div className={`flex items-center gap-1 ${compact ? 'scale-75 origin-left' : ''}`}>
      <button
        onClick={() => rate(1)}
        disabled={busy}
        className={`flex items-center gap-1 px-2 py-1 rounded-full text-xs font-semibold transition-colors
          ${data.rating === 1 ? 'bg-orange/20 text-orange' : 'text-muted hover:text-text hover:bg-white/5'}`}
        aria-label="Like"
      >
        <svg width="16" height="16" viewBox="0 0 24 24" fill={data.rating === 1 ? 'currentColor' : 'none'} stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M7 22V11M2 13v7a2 2 0 0 0 2 2h12.4a2 2 0 0 0 1.98-1.72l1.4-8A2 2 0 0 0 17.8 10H13V5a3 3 0 0 0-3-3l-3 8v12Z" />
        </svg>
        {!compact && <span>{data.up_count}</span>}
      </button>
      <button
        onClick={() => rate(-1)}
        disabled={busy}
        className={`flex items-center gap-1 px-2 py-1 rounded-full text-xs font-semibold transition-colors
          ${data.rating === -1 ? 'bg-red/20 text-red' : 'text-muted hover:text-text hover:bg-white/5'}`}
        aria-label="Dislike"
      >
        <svg width="16" height="16" viewBox="0 0 24 24" fill={data.rating === -1 ? 'currentColor' : 'none'} stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M17 2v9M22 11v-7a2 2 0 0 0-2-2H7.6a2 2 0 0 0-1.98 1.72l-1.4 8A2 2 0 0 0 6.2 14H11v5a3 3 0 0 0 3 3l3-8V2Z" />
        </svg>
        {!compact && <span>{data.down_count}</span>}
      </button>
    </div>
  )
}
