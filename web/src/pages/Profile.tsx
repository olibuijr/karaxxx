import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useAuth } from '../lib/auth'
import { fetchProfile } from '../api'
import type { Video } from '../types'
import VideoCard from '../components/VideoCard'

interface ProfileData {
  username: string
  account_age_days: number
  total_watched: number
  total_watch_time_seconds: number
  favorite_categories: string[]
  top_categories: { name: string; count: number }[]
  playlist_count: number
  favorite_count: number
  ratings_given: number
  rating_ratio: number
  recently_watched: Video[]
  top_rated: Video[]
}

export default function Profile() {
  const { token, user } = useAuth()
  const navigate = useNavigate()
  const [data, setData] = useState<ProfileData | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!token) { setLoading(false); return }
    fetchProfile(token)
      .then(setData)
      .catch(() => setData(null))
      .finally(() => setLoading(false))
  }, [token])

  if (!user) {
    return (
      <div className="text-center py-24">
        <p className="text-muted mb-4">Sign in to see your profile.</p>
        <Link to="/" className="text-orange hover:underline font-semibold">Browse videos</Link>
      </div>
    )
  }

  if (loading) return <div className="text-center py-24 text-muted">Loading...</div>
  if (!data) return <div className="text-center py-24 text-muted">Failed to load profile.</div>

  const formatTime = (s: number) => {
    const h = Math.floor(s / 3600)
    const m = Math.floor((s % 3600) / 60)
    return h > 0 ? `${h}h ${m}m` : `${m}m`
  }

  return (
    <div className="max-w-5xl mx-auto p-4 md:p-8 space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-text">{data.username}</h1>
          <p className="text-sm text-muted">Member for {data.account_age_days} days</p>
        </div>
        <button onClick={() => navigate(-1)} className="text-sm text-muted hover:text-text">Back</button>
      </div>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
        <div className="bg-card border border-border rounded-lg p-4 text-center">
          <div className="text-2xl font-bold text-text">{data.total_watched}</div>
          <div className="text-xs text-muted mt-1">Videos Watched</div>
        </div>
        <div className="bg-card border border-border rounded-lg p-4 text-center">
          <div className="text-2xl font-bold text-text">{formatTime(data.total_watch_time_seconds)}</div>
          <div className="text-xs text-muted mt-1">Watch Time</div>
        </div>
        <div className="bg-card border border-border rounded-lg p-4 text-center">
          <div className="text-2xl font-bold text-text">{data.playlist_count}</div>
          <div className="text-xs text-muted mt-1">Playlists</div>
        </div>
        <div className="bg-card border border-border rounded-lg p-4 text-center">
          <div className="text-2xl font-bold text-text">{data.favorite_count}</div>
          <div className="text-xs text-muted mt-1">Favorites</div>
        </div>
      </div>

      {data.top_categories.length > 0 && (
        <div className="bg-card border border-border rounded-lg p-4">
          <h2 className="text-sm font-semibold text-text mb-3">Top Categories</h2>
          <div className="space-y-2">
            {data.top_categories.slice(0, 6).map((c: any) => {
              const maxCount = Math.max(...data.top_categories.map((x: any) => x.count))
              const pct = (c.count / maxCount) * 100
              return (
                <div key={c.name} className="flex items-center gap-3">
                  <span className="text-xs text-muted w-24 capitalize truncate">{c.name}</span>
                  <div className="flex-1 bg-bg rounded-full h-2 overflow-hidden">
                    <div className="h-full bg-orange rounded-full" style={{ width: `${pct}%` }} />
                  </div>
                  <span className="text-xs text-muted w-8 text-right">{c.count}</span>
                </div>
              )
            })}
          </div>
        </div>
      )}

      {data.ratings_given > 0 && (
        <div className="bg-card border border-border rounded-lg p-4">
          <div className="text-sm text-muted">
            {data.ratings_given} ratings given · {Math.round(data.rating_ratio * 100)}% positive
          </div>
        </div>
      )}

      {data.recently_watched.length > 0 && (
        <div>
          <h2 className="text-sm font-semibold text-text mb-3">Recently Watched</h2>
          <div className="flex gap-2.5 overflow-x-auto pb-2 snap-x snap-mandatory">
            {data.recently_watched.map(v => (
              <div key={v.id} className="snap-start flex-shrink-0 w-48"><VideoCard video={v} /></div>
            ))}
          </div>
        </div>
      )}

      {data.top_rated.length > 0 && (
        <div>
          <h2 className="text-sm font-semibold text-text mb-3">Top Rated</h2>
          <div className="flex gap-2.5 overflow-x-auto pb-2 snap-x snap-mandatory">
            {data.top_rated.map(v => (
              <div key={v.id} className="snap-start flex-shrink-0 w-48"><VideoCard video={v} /></div>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
