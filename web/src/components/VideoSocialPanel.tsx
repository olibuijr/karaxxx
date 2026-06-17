import { useEffect, useState, type FormEvent } from 'react'
import { fetchVideoSocial, formatViews, postVideoComment, toggleVideoReaction } from '../api'
import type { VideoSocial } from '../types'

const reactionIcons: Record<string, { label: string; path: string }> = {
  like: {
    label: 'Like',
    path: 'M8 21H4V9h4v12Zm3-12 3-6c2 0 3 1.5 2.4 3.2L15.8 9H20c1.5 0 2.5 1.2 2.2 2.7l-1.2 6A4 4 0 0 1 17 21h-7V9h1Z',
  },
  fire: {
    label: 'Heat',
    path: 'M12 22c-4 0-7-3-7-7 0-3 2-5.5 4-7.5.5 2 1.8 3 3 3 1.8 0 3-1.5 3-4.5 3 2.4 5 5.3 5 9 0 4-3 7-8 7Z',
  },
  heart: {
    label: 'Heart',
    path: 'M12 21S4 16.2 4 9.8C4 6.6 6.2 4 9 4c1.6 0 2.6.7 3 1.6C12.4 4.7 13.4 4 15 4c2.8 0 5 2.6 5 5.8C20 16.2 12 21 12 21Z',
  },
  peach: {
    label: 'Peach',
    path: 'M13 5c1.5-2 3.5-2.5 6-2-1 2.5-3 4-5.5 4.2C18 8 21 11 21 15c0 4-3.2 7-8 7S3 19 3 15c0-4 3-7 7.5-7.8C9 5.7 8.5 4 9 2c2 .5 3.2 1.5 4 3Z',
  },
  spark: {
    label: 'Spark',
    path: 'M12 2l1.6 6.4L20 10l-6.4 1.6L12 18l-1.6-6.4L4 10l6.4-1.6L12 2Zm6 12 .8 3.2L22 18l-3.2.8L18 22l-.8-3.2L14 18l3.2-.8L18 14Z',
  },
}

export default function VideoSocialPanel({ videoId, token, initialWatchCount = 0 }: { videoId: string; token: string | null; initialWatchCount?: number }) {
  const [social, setSocial] = useState<VideoSocial | null>(null)
  const [comment, setComment] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!token || !videoId) return
    fetchVideoSocial(videoId, token).then(setSocial).catch(() => setSocial(null))
  }, [videoId, token])

  async function react(reaction: string) {
    if (!token || busy) return
    setBusy(true)
    try {
      setSocial(await toggleVideoReaction(videoId, token, reaction))
    } finally {
      setBusy(false)
    }
  }

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!token || !comment.trim() || busy) return
    setBusy(true)
    try {
      setSocial(await postVideoComment(videoId, token, comment))
      setComment('')
    } finally {
      setBusy(false)
    }
  }

  const watchCount = social?.watch_count ?? initialWatchCount
  const selected = new Set(social?.user_reactions || [])

  return (
    <section className="rounded-lg border border-border bg-card/80 p-4 space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-sm font-bold text-text">Community</h2>
          <p className="text-xs text-muted">
            Watched {formatViews(watchCount)} times by KaraXXX users. Aggregate data is anonymous and used for quality improvements.
          </p>
        </div>
        <div className="flex flex-wrap gap-1.5">
          {Object.entries(reactionIcons).map(([key, icon]) => (
            <button
              key={key}
              onClick={() => react(key)}
              disabled={!token || busy}
              className={`flex items-center gap-1 rounded-full border px-2.5 py-1 text-xs font-semibold transition-colors
                ${selected.has(key) ? 'border-orange bg-orange text-black' : 'border-border bg-bg text-muted hover:text-text hover:border-orange/40'}`}
              title={icon.label}
            >
              <svg viewBox="0 0 24 24" className="h-3.5 w-3.5" fill="currentColor" aria-hidden="true">
                <path d={icon.path} />
              </svg>
              <span>{social?.reactions?.[key] || 0}</span>
            </button>
          ))}
        </div>
      </div>

      <form onSubmit={submit} className="flex gap-2">
        <input
          value={comment}
          onChange={e => setComment(e.target.value)}
          maxLength={500}
          placeholder="Add a comment..."
          className="min-w-0 flex-1 rounded-md border border-border bg-bg px-3 py-2 text-sm text-text outline-none focus:border-orange/50"
        />
        <button
          type="submit"
          disabled={!comment.trim() || busy}
          className="rounded-md bg-orange px-3 py-2 text-sm font-bold text-black disabled:opacity-50"
        >
          Post
        </button>
      </form>

      <div className="space-y-2">
        {(social?.comments || []).length === 0 ? (
          <p className="text-xs text-muted">No comments yet.</p>
        ) : (
          social!.comments.slice().reverse().map(c => (
            <div key={c.id} className="rounded-md border border-white/5 bg-bg/70 px-3 py-2">
              <div className="flex items-center justify-between gap-3">
                <span className="text-xs font-semibold text-orange">{c.display_name}</span>
                <span className="text-[10px] text-muted">{c.created_at?.slice(0, 16)}</span>
              </div>
              <p className="mt-1 text-sm text-text leading-5">{c.body}</p>
            </div>
          ))
        )}
      </div>
    </section>
  )
}
