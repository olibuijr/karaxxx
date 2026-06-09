import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useAuth } from '../lib/auth'
import { fetchVideo } from '../api'
import type { Video } from '../types'
import VideoCard from '../components/VideoCard'

export default function Favorites() {
  const { token, user } = useAuth()
  const [videos, setVideos] = useState<Video[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!token) { setLoading(false); return }
    fetch('/api/fav/videos', { headers: { Authorization: `Bearer ${token}` } })
      .then(r => r.json())
      .then(async (ids: unknown) => {
        if (!Array.isArray(ids)) { setVideos([]); return }
        const results = await Promise.allSettled(
          ids.map((id: string) => fetchVideo(id).catch(() => null))
        )
        const vids = results
          .filter((r): r is PromiseFulfilledResult<Video> => r.status === 'fulfilled' && r.value !== null)
          .map(r => r.value)
        setVideos(vids)
      })
      .finally(() => setLoading(false))
  }, [token])

  if (!user) {
    return (
      <div className="text-center py-24">
        <p className="text-muted mb-4">Sign in to see your favorites.</p>
        <Link to="/" className="text-orange hover:underline font-semibold">Browse videos</Link>
      </div>
    )
  }

  return (
    <div className="max-w-[1800px] mx-auto">
      <div className="px-3 py-3 md:px-6 md:py-5">
        <h1 className="text-lg font-bold tracking-tight md:text-xl">Your Favorites</h1>
        <p className="text-xs text-muted mt-1">{videos.length} videos</p>
      </div>

      {loading ? (
        <div className="text-center py-16 text-muted">Loading...</div>
      ) : videos.length === 0 ? (
        <div className="text-center py-16 text-muted">
          No favorites yet. Heart videos to save them here.
        </div>
      ) : (
        <div className="grid gap-2.5 p-2.5
                        grid-cols-1
                        sm:grid-cols-2 sm:gap-3 sm:p-3
                        md:grid-cols-3
                        lg:grid-cols-4 lg:gap-3.5 lg:p-4
                        xl:grid-cols-5
                        2xl:grid-cols-6">
          {videos.map(v => (
            <VideoCard key={v.id} video={v} />
          ))}
        </div>
      )}
    </div>
  )
}
