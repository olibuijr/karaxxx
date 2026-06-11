import { useEffect, useState, type FormEvent } from 'react'
import { Link, useParams } from 'react-router-dom'
import { fetchWall, postWallComment } from '../api'
import { useAuth } from '../lib/auth'
import type { SocialComment, Video } from '../types'
import VideoCard from '../components/VideoCard'
import CategoryIcon from '../components/CategoryIcon'

interface WallData {
  user: {
    id: number
    username: string
    public_name: string
    is_self: boolean
  }
  favorite_categories: string[]
  favorite_videos: Video[]
  comments: SocialComment[]
  privacy_note: string
}

export default function Wall() {
  const { username = '' } = useParams()
  const { token } = useAuth()
  const [data, setData] = useState<WallData | null>(null)
  const [comment, setComment] = useState('')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!token || !username) return
    setLoading(true)
    fetchWall(username, token)
      .then(setData)
      .catch(() => setData(null))
      .finally(() => setLoading(false))
  }, [username, token])

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!token || !username || !comment.trim()) return
    setBusy(true)
    try {
      setData(await postWallComment(username, token, comment))
      setComment('')
    } finally {
      setBusy(false)
    }
  }

  if (loading) return <div className="text-center py-24 text-muted">Loading...</div>
  if (!data) return <div className="text-center py-24 text-muted">Wall not found.</div>

  return (
    <div className="mx-auto max-w-6xl space-y-6 p-4 md:p-8">
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-text">{data.user.public_name}</h1>
          <p className="text-sm text-muted">@{data.user.username} · {data.user.is_self ? 'your wall' : 'public wall'}</p>
        </div>
        {data.user.is_self && (
          <Link to="/profile" className="text-sm font-semibold text-orange hover:underline">Edit privacy settings</Link>
        )}
      </header>

      <p className="rounded-lg border border-border bg-card/80 p-3 text-xs leading-5 text-muted">
        {data.privacy_note}
      </p>

      {data.favorite_categories.length > 0 && (
        <section className="rounded-lg border border-border bg-card/80 p-4">
          <h2 className="mb-3 text-sm font-bold text-text">Favorite Categories</h2>
          <div className="flex flex-wrap gap-2">
            {data.favorite_categories.map(cat => (
              <Link
                key={cat}
                to={`/?cat=${encodeURIComponent(cat)}`}
                className="flex items-center gap-1.5 rounded-full border border-orange/20 bg-orange/10 px-2.5 py-1 text-xs font-semibold text-orange capitalize hover:bg-orange/20"
              >
                <CategoryIcon category={cat} className="h-3.5 w-3.5" />
                {cat}
              </Link>
            ))}
          </div>
        </section>
      )}

      <section>
        <h2 className="mb-3 text-sm font-bold text-text">Favorite Media</h2>
        {data.favorite_videos.length === 0 ? (
          <p className="rounded-lg border border-border bg-card/80 p-4 text-sm text-muted">No public favorites yet.</p>
        ) : (
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
            {data.favorite_videos.map(v => <VideoCard key={v.id} video={v} />)}
          </div>
        )}
      </section>

      <section className="rounded-lg border border-border bg-card/80 p-4 space-y-4">
        <h2 className="text-sm font-bold text-text">Wall Comments</h2>
        <form onSubmit={submit} className="flex gap-2">
          <input
            value={comment}
            onChange={e => setComment(e.target.value)}
            maxLength={500}
            placeholder="Leave a wall comment..."
            className="min-w-0 flex-1 rounded-md border border-border bg-bg px-3 py-2 text-sm text-text outline-none focus:border-orange/50"
          />
          <button type="submit" disabled={!comment.trim() || busy} className="rounded-md bg-orange px-3 py-2 text-sm font-bold text-black disabled:opacity-45">
            Post
          </button>
        </form>
        <div className="space-y-2">
          {data.comments.length === 0 ? (
            <p className="text-xs text-muted">No wall comments yet.</p>
          ) : (
            data.comments.map(c => (
              <div key={c.id} className="rounded-md border border-white/5 bg-bg/70 px-3 py-2">
                <div className="flex items-center justify-between gap-3">
                  <span className="text-xs font-semibold text-orange">{c.display_name}</span>
                  <span className="text-[10px] text-muted">{c.created_at?.slice(0, 16)}</span>
                </div>
                <p className="mt-1 text-sm leading-5 text-text">{c.body}</p>
              </div>
            ))
          )}
        </div>
      </section>
    </div>
  )
}
