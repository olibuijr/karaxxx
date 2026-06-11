import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { useAuth } from '../lib/auth'
import { fetchFavoriteIds, fetchVideo } from '../api'
import type { FavoriteSort, Video } from '../types'
import VideoCard from '../components/VideoCard'
import VideoCardSkeleton from '../components/VideoCardSkeleton'

const FAVORITE_SORT_OPTIONS: { label: string; value: FavoriteSort }[] = [
  { label: 'Recent', value: 'recent' },
  { label: 'Most viewed', value: 'views' },
  { label: 'Longest', value: 'duration' },
  { label: 'Title', value: 'title' },
]

function isFavoriteSort(value: string | null): value is FavoriteSort {
  return value === 'recent' || value === 'views' || value === 'duration' || value === 'title'
}

export default function Favorites() {
  const { token, user } = useAuth()
  const navigate = useNavigate()
  const [sp] = useSearchParams()
  const [videos, setVideos] = useState<Video[]>([])
  const [loading, setLoading] = useState(true)
  const [favError, setFavError] = useState<string | null>(null)
  const sortParam = sp.get('sort')
  const sort: FavoriteSort = isFavoriteSort(sortParam) ? sortParam : 'recent'

  const sortButtons = useMemo(() => FAVORITE_SORT_OPTIONS, [])

  useEffect(() => {
    if (!token) {
      setVideos([])
      setFavError(null)
      setLoading(false)
      return
    }

    setLoading(true)
    setFavError(null)
    fetchFavoriteIds(token, sort)
      .then(async (ids) => {
        const results = await Promise.allSettled(
          ids.map((id) => fetchVideo(id).catch(() => null))
        )
        const vids = results
          .filter((result): result is PromiseFulfilledResult<Video | null> => result.status === 'fulfilled')
          .map((result) => result.value)
          .filter((video): video is Video => video !== null)
        setVideos(vids)
      })
      .catch(() => {
        setVideos([])
        setFavError("Couldn't load your favorites.")
      })
      .finally(() => setLoading(false))
  }, [sort, token])

  function setSort(nextSort: FavoriteSort) {
    const params = new URLSearchParams(sp)
    if (nextSort === 'recent') params.delete('sort')
    else params.set('sort', nextSort)
    const qs = params.toString()
    navigate(qs ? `/favorites?${qs}` : '/favorites', { viewTransition: true })
  }

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
        <div className="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <h1 className="text-lg font-bold tracking-tight md:text-xl">Your Favorites</h1>
            <p className="text-xs text-muted mt-1">{videos.length} videos</p>
          </div>
          <div className="flex flex-wrap gap-2">
            {sortButtons.map((option) => (
              <button
                key={option.value}
                type="button"
                onClick={() => setSort(option.value)}
                className={`min-h-[40px] rounded-full px-3 py-2 text-xs font-semibold transition-all duration-150 focus-visible:ring-2 focus-visible:ring-orange/40 focus-visible:outline-none ${
                  sort === option.value
                    ? 'bg-gradient-to-br from-orange to-red text-white shadow-[0_2px_12px_-2px_rgba(249,115,22,0.5)]'
                    : 'bg-white/[0.04] text-muted hover:bg-white/8 hover:text-text'
                }`}
              >
                {option.label}
              </button>
            ))}
          </div>
        </div>
      </div>

      {loading ? (
        <div className="grid gap-2.5 p-2.5
                        grid-cols-1
                        sm:grid-cols-2 sm:gap-3 sm:p-3
                        md:grid-cols-3
                        lg:grid-cols-4 lg:gap-3.5 lg:p-4
                        xl:grid-cols-5
                        2xl:grid-cols-6">
          {Array.from({ length: 12 }).map((_, index) => (
            <VideoCardSkeleton key={index} index={index} />
          ))}
        </div>
      ) : favError ? (
        <div className="text-center py-16 text-muted">{favError}</div>
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
