import { useRef, useCallback } from 'react'
import { Link } from 'react-router-dom'
import type { Video } from '../types'
import { formatDuration, formatViews, timeAgo } from '../api'
import FavoriteButton from './FavoriteButton'

export default function VideoCard({ video }: { video: Video }) {
  const isNotXnxx = video.source && video.source !== 'xnxx'
  const sourceBadge: Record<string, { label: string; color: string }> = {
    xhamster: { label: 'XH', color: 'bg-orange text-black' },
    eporner:  { label: 'EP', color: 'bg-blue-500 text-white' },
    tnaflix:  { label: 'TF', color: 'bg-emerald-500 text-white' },
    drtuber:  { label: 'DT', color: 'bg-violet-500 text-white' },
  }
  const thumb = video.thumb_uuid
    ? (isNotXnxx ? `/media?url=${encodeURIComponent(video.thumb_uuid)}` : `/thumb/${video.thumb_uuid}/0/xn_23_t.jpg`)
    : ''
  const preview = video.preview_url
    ? (isNotXnxx ? `/media?url=${encodeURIComponent(video.preview_url)}` : `/thumb/${video.thumb_uuid}/0/preview.mp4`)
    : isNotXnxx && video.thumb_uuid ? `/media?url=${encodeURIComponent(video.thumb_uuid)}` : ''
  const vidRef = useRef<HTMLVideoElement>(null)

  const watchedPos = (video as any).watched_position as number | undefined
  const progressPct = watchedPos && video.duration > 0
    ? Math.min(100, (watchedPos / video.duration) * 100)
    : 0

  const onEnter = useCallback(() => {
    const v = vidRef.current
    if (v && 'ontouchstart' in window === false) {
      v.play().catch(() => {})
      v.classList.add('playing')
    }
  }, [])

  const onLeave = useCallback(() => {
    const v = vidRef.current
    if (v) {
      v.pause()
      v.classList.remove('playing')
    }
  }, [])

  return (
    <Link
      to={`/play/${video.id}`}
      onMouseEnter={onEnter}
      onMouseLeave={onLeave}
      className="group flex flex-col overflow-hidden rounded-xl bg-card border border-white/[0.06]
                 shadow-[0_1px_2px_rgba(0,0,0,0.4),0_8px_24px_-12px_rgba(0,0,0,0.5)]
                 hover:border-orange/30 hover:-translate-y-1
                 hover:shadow-[0_0_0_1px_rgba(249,115,22,0.15),0_12px_40px_-8px_rgba(0,0,0,0.7)]
                 active:scale-[0.975] transition-all duration-200"
    >
      <div className="relative aspect-video bg-bg overflow-hidden flex-shrink-0">
        {thumb && (
          <>
            <img
              src={thumb}
              loading="lazy"
              alt=""
              className="w-full h-full object-cover transition-transform duration-300 group-hover:scale-105"
            />
            <div className="absolute inset-0 bg-gradient-to-t from-bg/80 via-transparent to-transparent
                            opacity-85 group-hover:opacity-100 transition-opacity duration-300 pointer-events-none" />
          </>
        )}
        {progressPct > 0 && (
          <>
            <div className="absolute bottom-0 left-0 right-0 h-[3px] bg-white/10 z-10" />
            <div className="absolute bottom-0 h-[3px] bg-gradient-to-r from-red to-orange z-10
                            shadow-[0_0_6px_rgba(249,115,22,0.6)]"
                 style={{ width: `${progressPct}%` }} />
          </>
        )}
        {preview && (
          <video
            ref={vidRef}
            data-preview
            src={preview}
            muted
            loop
            playsInline
            preload="none"
            className="absolute inset-0 w-full h-full object-cover opacity-0 group-hover:opacity-100 transition-opacity duration-200"
          />
        )}
        {video.duration > 0 && (
          <span className="absolute bottom-2 right-2 bg-black/65 backdrop-blur-md
                           text-white text-[10px] px-2 py-0.5 rounded-md
                           font-semibold tabular-nums tracking-wide border border-white/10 z-10
                           shadow-[0_2px_8px_rgba(0,0,0,0.4)]">
            {formatDuration(video.duration)}
          </span>
        )}
        {isNotXnxx && sourceBadge[video.source] && (
          <span className={`absolute top-2 left-2 text-[9px] px-1.5 py-0.5 rounded-full
                            font-bold uppercase tracking-wider z-10 ${sourceBadge[video.source].color}`}>
            {sourceBadge[video.source].label}
          </span>
        )}
        <FavoriteButton videoId={video.id} />
      </div>

      <div className="flex flex-col gap-1 p-2.5 flex-1">
        <h3 className="text-[13px] leading-tight font-semibold text-text
                       line-clamp-2 tracking-tight">
          {video.title}
        </h3>

        <div className="flex items-center gap-1.5 mt-auto">
          {video.views > 0 && (
            <span className="text-[11px] text-muted">{formatViews(video.views)} views</span>
          )}
          {video.upload_date && (
            <>
              <span className="text-[11px] text-muted">·</span>
              <span className="text-[11px] text-muted">{timeAgo(video.upload_date)}</span>
            </>
          )}
        </div>

        {video.categories && video.categories.length > 0 && (
          <div className="flex gap-1 flex-wrap mt-1">
            {video.categories.filter(c => c !== 'uncategorized').slice(0, 3).map((cat, i) => (
              <span
                key={cat}
                onClick={(e) => { e.preventDefault(); e.stopPropagation(); window.location.href = `/?cat=${encodeURIComponent(cat)}` }}
                className={`text-[10px] px-1.5 py-px rounded-full font-semibold capitalize cursor-pointer
                            border border-transparent transition-colors
                            ${i === 0
                              ? 'bg-orange/15 text-orange hover:bg-orange/25'
                              : 'bg-red/10 text-red hover:bg-red/20'
                            }`}
              >
                {cat}
              </span>
            ))}
          </div>
        )}
      </div>
    </Link>
  )
}
