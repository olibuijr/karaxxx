import { useEffect, useState, useRef, useCallback } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import { fetchVideo, formatDuration, formatViews, fetchRelated, fetchRandom } from '../api'
import type { Video } from '../types'
import FavoriteButton from '../components/FavoriteButton'
import RatingButtons from '../components/RatingButtons'
import PlaylistButton from '../components/PlaylistButton'
import VideoSocialPanel from '../components/VideoSocialPanel'
import { toast } from 'sonner'
import { useAuth } from '../lib/auth'

export default function Play() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [v, setV] = useState<Video | null>(null)
  const [loading, setLoading] = useState(true)
  const [quality, setQuality] = useState<'1080' | '720' | '360'>('720')
  const videoRef = useRef<HTMLVideoElement>(null)
  const { token } = useAuth()
  const [playing, setPlaying] = useState(true)

  const [theaterMode, setTheaterMode] = useState(() => localStorage.getItem('kxxx_theater') === 'true')
  const [showTheaterUi, setShowTheaterUi] = useState(true)
  const inactivityRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const [autoplayDisabled, setAutoplayDisabled] = useState(() => sessionStorage.getItem('kxxx_autoplay_disabled') === 'true')
  const [countdown, setCountdown] = useState<number | null>(null)
  const [nextVideo, setNextVideo] = useState<string | null>(null)

  const [gestureOverlay, setGestureOverlay] = useState<{ type: string; value: string } | null>(null)
  const lastTapRef = useRef<{ x: number; time: number } | null>(null)
  const touchStartRef = useRef<{ x: number; y: number; time: number } | null>(null)

  useEffect(() => {
    setCountdown(null)
    setNextVideo(null)
    setGestureOverlay(null)
  }, [id])

  useEffect(() => {
    if (!id) return
    setLoading(true)
    fetchVideo(id)
      .then(v => {
        setV(v)
        if (v.url_1080) setQuality('1080')
        else if (v.url_720) setQuality('720')
        else setQuality('360')
      })
      .catch(() => setV(null))
      .finally(() => setLoading(false))
  }, [id])

  useEffect(() => {
    const vid = videoRef.current
    if (!vid || !v) return
    const watched = v.watched_position
    if (!watched || watched <= 0) return
    const onMeta = () => { vid.currentTime = watched }
    vid.addEventListener('loadedmetadata', onMeta, { once: true })
    return () => vid.removeEventListener('loadedmetadata', onMeta)
  }, [v])

  useEffect(() => {
    if (!v) return
    const watched = v.watched_position
    if (!watched || watched <= 5) return
    const mins = Math.floor(watched / 60)
    const secs = Math.floor(watched % 60)
    toast(`Resume from ${mins}:${secs.toString().padStart(2, '0')}?`, {
      action: {
        label: 'Resume',
        onClick: () => {
          const vid = videoRef.current
          if (vid) { vid.currentTime = watched; vid.play().catch(() => {}) }
        }
      },
      duration: 8000,
    })
  }, [v])

  useEffect(() => {
    if (!id || !token) return
    const vid = videoRef.current
    if (!vid) return

    let interval: ReturnType<typeof setInterval> | null = null

    const save = () => {
      if (!vid) return
      const pos = Math.floor(vid.currentTime)
      fetch(`/api/watch/${id}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify({ position: pos }),
      }).catch(() => {})
    }

    const onPlay = () => {
      interval = setInterval(() => {
        if (!vid.paused) {
          fetch(`/api/watch/${id}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
            body: JSON.stringify({ position: Math.floor(vid.currentTime) }),
          }).catch(() => {})
        }
      }, 5000)
    }

    const onPause = () => {
      if (interval) { clearInterval(interval); interval = null }
      save()
    }

    const onSeeking = () => save()

    vid.addEventListener('play', onPlay)
    vid.addEventListener('pause', onPause)
    vid.addEventListener('seeking', onSeeking)

    return () => {
      if (interval) clearInterval(interval)
      vid.removeEventListener('play', onPlay)
      vid.removeEventListener('pause', onPause)
      vid.removeEventListener('seeking', onSeeking)
    }
  }, [id, token])

  useEffect(() => {
    if (!id || !token) return
    fetch(`/api/watch/${id}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
      body: JSON.stringify({ event: 'play' }),
    }).catch(() => {})
  }, [id, token])

  useEffect(() => {
    if (!id || !token) return
    const onUnload = () => {
      const vid = videoRef.current
      if (vid) {
        navigator.sendBeacon(`/api/watch/${id}`, JSON.stringify({ position: Math.floor(vid.currentTime) }))
      }
    }
    window.addEventListener('beforeunload', onUnload)
    return () => window.removeEventListener('beforeunload', onUnload)
  }, [id, token])

  useEffect(() => {
    if (!gestureOverlay) return
    const t = setTimeout(() => setGestureOverlay(null), 1000)
    return () => clearTimeout(t)
  }, [gestureOverlay])

  const toggleTheater = useCallback(() => {
    setTheaterMode(prev => {
      const next = !prev
      localStorage.setItem('kxxx_theater', String(next))
      return next
    })
  }, [])

  const handleMouseMove = useCallback(() => {
    if (!theaterMode) return
    setShowTheaterUi(true)
    if (inactivityRef.current) clearTimeout(inactivityRef.current)
    inactivityRef.current = setTimeout(() => setShowTheaterUi(false), 3000)
  }, [theaterMode])

  const handleEnded = useCallback(async () => {
    if (autoplayDisabled || !id) return
    const related = await fetchRelated(id)
    const nextId = related.length > 0 ? related[0].id : await fetchRandom(v?.source, v?.categories?.[0]).catch(() => null)
    if (!nextId) return
    setNextVideo(nextId)
    setCountdown(5)
  }, [autoplayDisabled, id])

  useEffect(() => {
    if (countdown === null) return
    if (countdown <= 0) {
      if (nextVideo) navigate(`/play/${nextVideo}`, { viewTransition: true })
      return
    }
    const t = setTimeout(() => setCountdown(countdown - 1), 1000)
    return () => clearTimeout(t)
  }, [countdown, nextVideo, navigate])

  const handleTouchStart = (e: React.TouchEvent) => {
    touchStartRef.current = {
      x: e.touches[0].clientX,
      y: e.touches[0].clientY,
      time: Date.now(),
    }
  }

  const handleTouchEnd = (e: React.TouchEvent) => {
    const touchStart = touchStartRef.current
    if (!touchStart) return
    const dx = e.changedTouches[0].clientX - touchStart.x
    const dy = e.changedTouches[0].clientY - touchStart.y
    const elapsed = Date.now() - touchStart.time
    const player = (e.target as HTMLElement).closest('.aspect-video')
    if (!player) return
    const rect = player.getBoundingClientRect()
    const relX = e.changedTouches[0].clientX - rect.left
    const vid = videoRef.current

    if (elapsed > 500 && Math.abs(dx) < 10 && Math.abs(dy) < 10) return

    const now = Date.now()
    if (lastTapRef.current && now - lastTapRef.current.time < 300 && Math.abs(e.changedTouches[0].clientX - lastTapRef.current.x) < 30) {
      if (vid) {
        if (relX < rect.width / 2) {
          vid.currentTime = Math.max(0, vid.currentTime - 10)
          setGestureOverlay({ type: 'seek', value: '-10s' })
        } else {
          vid.currentTime = Math.min(vid.duration || 0, vid.currentTime + 10)
          setGestureOverlay({ type: 'seek', value: '+10s' })
        }
      }
      lastTapRef.current = null
      return
    }

    lastTapRef.current = { x: e.changedTouches[0].clientX, time: now }

    if (elapsed > 500) return

    if (Math.abs(dx) > 50 && Math.abs(dx) > Math.abs(dy) * 1.5) {
      if (vid) {
        const seekAmount = dx < 0 ? 15 : -15
        vid.currentTime = Math.max(0, Math.min(vid.duration || 0, vid.currentTime + seekAmount))
        setGestureOverlay({ type: 'seek', value: `${seekAmount > 0 ? '+' : ''}${seekAmount}s` })
      }
      return
    }

    if (Math.abs(dy) > 50 && Math.abs(dy) > Math.abs(dx) * 1.5 && relX > rect.width / 2) {
      if (vid) {
        const step = dy < 0 ? 0.1 : -0.1
        vid.volume = Math.max(0, Math.min(1, vid.volume + step))
        setGestureOverlay({ type: 'volume', value: `${Math.round(vid.volume * 100)}%` })
      }
    }
  }

  const toggleFullscreen = () => {
    const el = videoRef.current?.parentElement
    if (!el) return
    if (document.fullscreenElement) {
      document.exitFullscreen().catch(() => {})
    } else {
      el.requestFullscreen().catch(() => {})
    }
  }

  if (loading) return <div className="text-center py-24 text-muted">Loading...</div>
  if (!v) return <div className="text-center py-24 text-muted">Video not found.</div>

  const src = quality === '1080' ? v.url_1080 || v.url_720 || v.url_360
    : quality === '720' ? v.url_720 || v.url_1080 || v.url_360
    : v.url_360 || v.url_720 || v.url_1080

  const isNotXnxx = v.source && v.source !== 'xnxx'
  const posterSrc = v.thumb_uuid
    ? (isNotXnxx ? `/media?url=${encodeURIComponent(v.thumb_uuid)}` : `/thumb/${v.thumb_uuid}/0/xn_23_t.jpg`)
    : undefined
  const previewSrc = v.preview_url
    ? (isNotXnxx ? `/media?url=${encodeURIComponent(v.preview_url)}` : (v.thumb_uuid ? `/thumb/${v.thumb_uuid}/0/preview.mp4` : ''))
    : !isNotXnxx && v.thumb_uuid ? `/thumb/${v.thumb_uuid}/0/preview.mp4` : ''

  const qualities = [
    v.url_1080 && '1080',
    v.url_720 && '720',
    v.url_360 && '360',
  ].filter(Boolean) as string[]

  return (
    <div className={`mx-auto px-3 py-3 md:px-6 md:py-5 ${theaterMode ? 'max-w-none' : 'max-w-5xl'}`}
         onMouseMove={handleMouseMove}>
      {/* Player */}
      <div className="relative aspect-video bg-bg rounded-lg overflow-hidden mb-4 shadow-2xl shadow-black/40"
           onTouchStart={handleTouchStart}
           onTouchEnd={handleTouchEnd}>
        {previewSrc || src ? (
          <video
            ref={videoRef}
            key={`${id}-${quality}`}
            controls
            autoPlay
            playsInline
            className="w-full h-full"
            poster={posterSrc}
            onEnded={handleEnded}
            onPlay={() => setPlaying(true)}
            onPause={() => setPlaying(false)}
          >
            {src && <source src={`/vid/${id}/${quality}`} type="video/mp4" />}
            {v.hls_url && <source src={v.hls_url} type="application/vnd.apple.mpegurl" />}
            {previewSrc && <source src={previewSrc} type="video/mp4" />}
          </video>
        ) : (
          <div className="flex items-center justify-center h-full text-muted">No stream available</div>
        )}

        {countdown !== null && nextVideo && (
          <div className="absolute inset-0 bg-black/70 flex items-center justify-center z-50">
            <div className="text-center">
              <p className="text-white text-lg mb-3">Next video in {countdown}...</p>
              <button onClick={() => { setCountdown(null); setAutoplayDisabled(true); sessionStorage.setItem('kxxx_autoplay_disabled', 'true') }}
                className="px-5 py-2 bg-white/10 hover:bg-white/20 text-white rounded-full text-sm transition-colors">
                Cancel
              </button>
            </div>
          </div>
        )}

        {gestureOverlay && (
          <div className="absolute inset-0 flex items-center justify-center pointer-events-none z-40">
            <div className="bg-black/60 backdrop-blur-sm text-white text-xl font-bold px-5 py-2.5 rounded-lg">
              {gestureOverlay.type === 'seek' ? gestureOverlay.value : `Vol: ${gestureOverlay.value}`}
            </div>
          </div>
        )}

        {theaterMode && (
          <div className={`absolute bottom-0 left-0 right-0 bg-gradient-to-t from-black/80 via-black/30 to-transparent px-4 pb-3 pt-10 transition-opacity duration-300 ${showTheaterUi ? 'opacity-100' : 'opacity-0 pointer-events-none'}`}>
            <div className="flex items-center gap-3">
              <button onClick={() => { const vid = videoRef.current; if (vid) { vid.paused ? vid.play().catch(() => {}) : vid.pause() } }}
                className="text-white text-lg w-8 h-8 flex items-center justify-center rounded-full bg-white/10 hover:bg-white/20 transition-colors">
                {playing ? '\u23F8' : '\u25B6'}
              </button>
              <span className="text-white text-xs font-mono">
                {videoRef.current ? `${formatDuration(Math.floor(videoRef.current.currentTime))} / ${formatDuration(v.duration)}` : formatDuration(v.duration)}
              </span>
              <div className="flex-1" />
              <button onClick={toggleTheater}
                className="text-white/70 hover:text-white text-[11px] px-2 py-1 rounded bg-white/10 hover:bg-white/20 transition-colors">
                Exit Theater
              </button>
              <button onClick={toggleFullscreen}
                className="text-white/70 hover:text-white text-lg w-8 h-8 flex items-center justify-center rounded-full bg-white/10 hover:bg-white/20 transition-colors">
                \u26F6
              </button>
            </div>
          </div>
        )}
      </div>

      {/* Info */}
      <div className={`space-y-3 transition-all duration-300 ${theaterMode ? 'opacity-0 h-0 overflow-hidden pointer-events-none' : ''}`}>
        <h1 className="text-lg font-bold leading-tight md:text-xl flex items-center gap-2">
          {v.title}
          {id && <FavoriteButton videoId={id} />}
          {id && <RatingButtons videoId={id} compact />}
        </h1>

        <div className="flex flex-wrap items-center gap-3 text-sm text-muted">
          {v.views > 0 && <span>{formatViews(v.views)} views</span>}
          {v.watch_count != null && v.watch_count > 0 && (
            <>
              <span className="text-border">|</span>
              <span>{formatViews(v.watch_count)} KaraXXX watches</span>
            </>
          )}
          {v.duration > 0 && (
            <>
              <span className="text-border">|</span>
              <span>{formatDuration(v.duration)}</span>
            </>
          )}
          {v.uploader && (
            <>
              <span className="text-border">|</span>
              <Link to={`/?uploader=${encodeURIComponent(v.uploader)}`} className="text-orange hover:underline">
                {v.uploader}
              </Link>
            </>
          )}
          {v.upload_date && (
            <>
              <span className="text-border">|</span>
              <span>{v.upload_date}</span>
            </>
          )}
        </div>

        {/* Quality selector + theater toggle */}
        <div className="flex gap-1.5 flex-wrap items-center">
          {qualities.length > 1 && qualities.map(q => (
            <button
              key={q}
              onClick={() => setQuality(q as '360' | '720' | '1080')}
              className={`px-3 py-1 rounded-full text-xs font-semibold transition-colors
                          ${quality === q
                            ? 'bg-orange text-black'
                            : 'bg-card border border-border text-muted hover:text-text hover:border-red/40'
                          }`}
            >
              {q}p
            </button>
          ))}
          {id && <PlaylistButton videoId={id} />}
          <button onClick={toggleTheater}
            className={`px-3 py-1 rounded-full text-xs font-semibold transition-colors ${theaterMode ? 'bg-orange text-black' : 'bg-card border border-border text-muted hover:text-text'}`}>
            Theater
          </button>
        </div>

        {/* Categories */}
        {v.categories && v.categories.length > 0 && (
          <div className="flex gap-1.5 flex-wrap">
            {v.categories.filter(c => c !== 'uncategorized').map((cat, i) => (
              <Link
                key={cat}
                to={`/?cat=${encodeURIComponent(cat)}`}
                className={`text-[11px] px-2 py-0.5 rounded-full font-semibold capitalize
                            transition-colors
                            ${i === 0
                              ? 'bg-orange/15 text-orange'
                              : 'bg-red/10 text-red hover:bg-red/20'
                            }`}
              >
                {cat}
              </Link>
            ))}
          </div>
        )}

        {/* Tags */}
        {v.tags && v.tags.length > 0 && (
          <div className="flex gap-1 flex-wrap">
            {v.tags.slice(0, 15).map(tag => (
              <Link
                key={tag}
                to={`/search?q=${encodeURIComponent(tag)}`}
                className="text-[10px] px-1.5 py-0.5 rounded bg-white/5 text-muted
                           hover:text-text hover:bg-white/10 transition-colors capitalize"
              >
                {tag}
              </Link>
            ))}
          </div>
        )}

        {v.description && (
          <p className="text-sm text-muted leading-relaxed max-w-prose">{v.description}</p>
        )}

        {id && (
          <VideoSocialPanel videoId={id} token={token} initialWatchCount={v.watch_count || 0} />
        )}
      </div>
    </div>
  )
}
