import { useRef, useCallback, useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import type { Video } from '../types'

function proxiedMedia(url: string): string {
  return `/media?url=${encodeURIComponent(url)}`
}

function xnxxPoster(thumb: string): string {
  if (/^https?:\/\//i.test(thumb)) return proxiedMedia(thumb)
  // 960x540 listing mosaic is the highest public XNXX thumbnail asset; use it
  // instead of a hard-coded xn_N_t frame so cards are crisp and never pick a
  // random low-res frame number unrelated to this video.
  return `/thumb/${thumb}/0/mozaique_listing.jpg`
}

function xnxxPreview(thumb: string): string {
  if (/^https?:\/\//i.test(thumb)) {
    try {
      const url = new URL(thumb)
      url.pathname = url.pathname.replace(/\/[^/]+$/, '/preview.mp4')
      return proxiedMedia(url.toString())
    } catch {
      return ''
    }
  }
  return `/thumb/${thumb}/0/preview.mp4`
}
import { formatDuration, formatViews, timeAgo } from '../api'
import FavoriteButton from './FavoriteButton'
import { toggleCategoryParam } from '../lib/categories'

const SOURCE_BADGE: Record<string, { label: string; color: string }> = {
  xvideos:  { label: 'XV', color: 'bg-gradient-to-br from-green-500 to-green-700 text-white' },
  xhamster: { label: 'XH', color: 'bg-gradient-to-br from-orange to-red text-white' },
  eporner:  { label: 'EP', color: 'bg-gradient-to-br from-blue-500 to-blue-700 text-white' },
  tnaflix:  { label: 'TF', color: 'bg-gradient-to-br from-emerald-500 to-emerald-700 text-white' },
  drtuber:  { label: 'DT', color: 'bg-gradient-to-br from-violet-500 to-violet-700 text-white' },
  heavyfetish: { label: 'HF', color: 'bg-gradient-to-br from-fuchsia-600 to-pink-700 text-white' },
  punishbang: { label: 'PB', color: 'bg-gradient-to-br from-red-700 to-stone-900 text-white' },
  sunporno: { label: 'SP', color: 'bg-gradient-to-br from-yellow-500 to-orange text-black' },
}

export default function VideoCard({ video }: { video: Video }) {
  const navigate = useNavigate()
  const location = useLocation()
  const [showPlayOverlay, setShowPlayOverlay] = useState(false)
  const isNotXnxx = video.source && video.source !== 'xnxx'
  const thumb = video.thumb_uuid
    ? (isNotXnxx ? proxiedMedia(video.thumb_uuid) : xnxxPoster(video.thumb_uuid))
    : ''
  const preview = video.preview_url
    ? (isNotXnxx ? proxiedMedia(video.preview_url) : (video.thumb_uuid ? xnxxPreview(video.thumb_uuid) : ''))
    : !isNotXnxx && video.thumb_uuid ? xnxxPreview(video.thumb_uuid) : ''
  const vidRef = useRef<HTMLVideoElement>(null)

  const watchedPos = video.watched_position
  const progressPct = watchedPos && video.duration > 0
    ? Math.min(100, (watchedPos / video.duration) * 100)
    : 0

  const onEnter = useCallback(() => {
    setShowPlayOverlay(true)
    const v = vidRef.current
    if (v && 'ontouchstart' in window === false) {
      v.play().catch(() => {})
      v.classList.add('playing')
    }
  }, [])

  const onLeave = useCallback(() => {
    setShowPlayOverlay(false)
    const v = vidRef.current
    if (v) {
      v.pause()
      v.classList.remove('playing')
    }
  }, [])

  const navigateToCategory = useCallback((category: string) => {
    const params = new URLSearchParams(location.search)
    const nextCategories = toggleCategoryParam(params.get('cat'), category)
    if (nextCategories) params.set('cat', nextCategories)
    else params.delete('cat')
    const qs = params.toString()
    navigate(qs ? `/?${qs}` : '/', { viewTransition: true })
  }, [location.search, navigate])

  return (
    <Link viewTransition
      to={`/play/${video.id}`}
      onMouseEnter={onEnter}
      onMouseLeave={onLeave}
      className="group flex flex-col overflow-hidden rounded-xl bg-card border border-border
                 shadow-card
                 hover:border-red/30 hover:-translate-y-1
                 hover:shadow-glow
                 active:scale-[0.975] transition-all duration-200"
    >
      {/* Thumbnail */}
      <div className="relative aspect-video bg-bg overflow-hidden flex-shrink-0">
        {thumb && (
          <>
            <img
              src={thumb}
              loading="lazy"
              alt={video.title}
              decoding="async"
              sizes="(max-width: 640px) 50vw, (max-width: 1024px) 33vw, 20vw"
              className="w-full h-full object-cover transition-transform duration-500 group-hover:scale-105"
            />
            {/* Gradient overlay */}
            <div className="absolute inset-0 bg-gradient-to-t from-bg/90 via-transparent to-transparent
                            opacity-80 group-hover:opacity-100 transition-opacity duration-300 pointer-events-none" />
          </>
        )}

        {/* Progress bar */}
        {progressPct > 0 && (
          <>
            <div className="absolute bottom-0 left-0 right-0 h-[3px] bg-white/10 z-10" />
            <div className="absolute bottom-0 h-[3px] bg-gradient-to-r from-red to-orange z-10
                            shadow-[0_0_6px_rgba(225,29,72,0.5)]"
                 style={{ width: `${progressPct}%` }} />
          </>
        )}

        {/* Hover preview video */}
        {preview && (
          <video
            ref={vidRef}
            data-preview
            src={preview}
            muted
            loop
            playsInline
            preload="none"
            className="absolute inset-0 w-full h-full object-cover opacity-0 group-hover:opacity-100 transition-opacity duration-300"
          />
        )}

        {/* Play icon overlay on hover */}
        <div className={`absolute inset-0 flex items-center justify-center z-10 video-card-play-icon ${showPlayOverlay ? 'visible' : ''}`}>
          <div className="w-12 h-12 rounded-full bg-red/80 backdrop-blur-sm flex items-center justify-center
                          shadow-[0_0_24px_rgba(225,29,72,0.5)] transition-transform duration-200 group-hover:scale-110">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="white">
              <polygon points="8,5 19,12 8,19" />
            </svg>
          </div>
        </div>

        {/* Duration badge */}
        {video.duration > 0 && (
          <span className="absolute bottom-2 right-2 bg-black/70 backdrop-blur-md
                           text-white text-[10px] px-2 py-0.5 rounded-md
                           font-semibold tabular-nums tracking-wide border border-white/10 z-10
                           shadow-[0_2px_8px_rgba(0,0,0,0.4)]">
            {formatDuration(video.duration)}
          </span>
        )}

        {/* Source badge */}
        {isNotXnxx && SOURCE_BADGE[video.source] && (
          <span className={`absolute top-2 left-2 text-[9px] px-1.5 py-0.5 rounded-full
                            font-bold uppercase tracking-wider z-10 ${SOURCE_BADGE[video.source].color}`}>
            {SOURCE_BADGE[video.source].label}
          </span>
        )}

        {/* Favorites button */}
        <FavoriteButton videoId={video.id} />
      </div>

      {/* Info */}
      <div className="flex flex-col gap-1 p-2.5 flex-1 min-w-0">
        <h3 className="text-[13px] leading-tight font-semibold text-text
                       line-clamp-2 tracking-tight">
          {video.title}
        </h3>

        <div className="flex items-center gap-1.5 mt-auto">
          {video.views > 0 && (
            <span className="text-[11px] text-muted">{formatViews(video.views)} views</span>
          )}
          {video.watch_count != null && video.watch_count > 0 && (
            <>
              <span className="text-[11px] text-muted/40">·</span>
              <span className="text-[11px] text-muted">{formatViews(video.watch_count)} watched</span>
            </>
          )}
          {video.upload_date && (
            <>
              <span className="text-[11px] text-muted/40">·</span>
              <span className="text-[11px] text-muted">{timeAgo(video.upload_date)}</span>
            </>
          )}
        </div>

        {/* Category badges */}
        {video.categories && video.categories.length > 0 && (
          <div className="flex gap-1 flex-wrap mt-1">
            {video.categories.filter(c => c !== 'uncategorized').slice(0, 3).map((cat, i) => (
              <span
                key={cat}
                onClick={(e) => {
                  e.preventDefault()
                  e.stopPropagation()
                  navigateToCategory(cat)
                }}
                className={`text-[10px] px-1.5 py-px rounded-full font-semibold capitalize cursor-pointer
                            border border-transparent transition-colors
                            ${i === 0
                              ? 'bg-red/15 text-red hover:bg-red/25'
                              : 'bg-orange/10 text-orange hover:bg-orange/20'
                            }`}
                aria-label={`Filter by ${cat}`}
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
