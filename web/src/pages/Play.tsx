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

function formatTime(s: number): string {
  if (!s || s < 0) return '0:00'
  const m = Math.floor(s / 60)
  const sec = Math.floor(s % 60)
  return `${m}:${sec.toString().padStart(2, '0')}`
}

function proxiedMedia(url: string): string {
  return `/media?url=${encodeURIComponent(url)}`
}

function xnxxPoster(thumb: string): string {
  if (/^https?:\/\//i.test(thumb)) return proxiedMedia(thumb)
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
  const [showControls, setShowControls] = useState(true)
  const controlsTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [currentTime, setCurrentTime] = useState(0)
  const [duration, setDuration] = useState(0)
  const [volume, setVolume] = useState(1)
  const [muted, setMuted] = useState(false)
  const [showVolumeSlider, setShowVolumeSlider] = useState(false)
  const progressRef = useRef<HTMLDivElement>(null)

  const [autoplayDisabled, setAutoplayDisabled] = useState(() => sessionStorage.getItem('kxxx_autoplay_disabled') === 'true')
  const [countdown, setCountdown] = useState<number | null>(null)
  const [nextVideo, setNextVideo] = useState<string | null>(null)

  const [gestureOverlay, setGestureOverlay] = useState<{ type: string; value: string } | null>(null)
  const lastTapRef = useRef<{ x: number; time: number } | null>(null)
  const touchStartRef = useRef<{ x: number; y: number; time: number } | null>(null)

  // Keyboard shortcut help
  const [showShortcuts, setShowShortcuts] = useState(false)

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

  // Track time/duration
  useEffect(() => {
    const vid = videoRef.current
    if (!vid) return
    const onTime = () => setCurrentTime(vid.currentTime)
    const onDur = () => { setDuration(vid.duration); setVolume(vid.volume) }
    const onVol = () => setVolume(vid.volume)
    const onMut = () => { setMuted(vid.muted); setVolume(vid.volume) }
    vid.addEventListener('timeupdate', onTime)
    vid.addEventListener('loadedmetadata', onDur)
    vid.addEventListener('volumechange', onVol)
    vid.addEventListener('volumechange', onMut)
    return () => {
      vid.removeEventListener('timeupdate', onTime)
      vid.removeEventListener('loadedmetadata', onDur)
      vid.removeEventListener('volumechange', onVol)
      vid.removeEventListener('volumechange', onMut)
    }
  }, [v])

  // Watch position tracking
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
        navigator.sendBeacon(`/api/watch/${id}`, new Blob([JSON.stringify({ position: Math.floor(vid.currentTime) })], { type: 'application/json' }))
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

  // Auto-hide controls in theater mode — handled by handleMouseMove

  // Keyboard shortcuts
  useEffect(() => {
    const handle = (e: KeyboardEvent) => {
      const vid = videoRef.current
      if (!vid) return

      // Don't capture when typing in an input
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return

      switch (e.code) {
        case 'Space':
          e.preventDefault()
          vid.paused ? vid.play().catch(() => {}) : vid.pause()
          break
        case 'KeyF':
          e.preventDefault()
          toggleFullscreen()
          break
        case 'KeyT':
          e.preventDefault()
          toggleTheater()
          break
        case 'ArrowLeft':
        case 'KeyJ':
          e.preventDefault()
          vid.currentTime = Math.max(0, vid.currentTime - 10)
          showGesture('seek', '-10s')
          break
        case 'ArrowRight':
        case 'KeyL':
          e.preventDefault()
          vid.currentTime = Math.min(duration || 0, vid.currentTime + 10)
          showGesture('seek', '+10s')
          break
        case 'ArrowUp':
          e.preventDefault()
          vid.volume = Math.min(1, vid.volume + 0.1)
          showGesture('volume', `${Math.round(vid.volume * 100)}%`)
          break
        case 'ArrowDown':
          e.preventDefault()
          vid.volume = Math.max(0, vid.volume - 0.1)
          showGesture('volume', `${Math.round(vid.volume * 100)}%`)
          break
        case 'KeyM':
          e.preventDefault()
          vid.muted = !vid.muted
          break
        case 'Slash':
          e.preventDefault()
          setShowShortcuts(s => !s)
          break
      }
    }
    window.addEventListener('keydown', handle)
    return () => window.removeEventListener('keydown', handle)
  }, [duration])

  const showGesture = (type: string, value: string) => {
    setGestureOverlay({ type, value })
  }

  const toggleTheater = useCallback(() => {
    setTheaterMode(prev => {
      const next = !prev
      localStorage.setItem('kxxx_theater', String(next))
      if (next) setShowControls(true)
      return next
    })
  }, [])

  const handleMouseMove = useCallback(() => {
    if (!theaterMode) return
    setShowControls(true)
    if (controlsTimer.current) clearTimeout(controlsTimer.current)
    controlsTimer.current = setTimeout(() => setShowControls(false), 3000)
  }, [theaterMode])

  const handleControlsMouseEnter = useCallback(() => {
    if (controlsTimer.current) clearTimeout(controlsTimer.current)
  }, [])

  const handleControlsMouseLeave = useCallback(() => {
    if (theaterMode) {
      controlsTimer.current = setTimeout(() => setShowControls(false), 2000)
    }
  }, [theaterMode])

  const handleEnded = useCallback(async () => {
    if (autoplayDisabled || !id) return
    const related = await fetchRelated(id)
    const nextId = related.length > 0 ? related[0].id : await fetchRandom(v?.source, v?.categories?.[0]).catch(() => null)
    if (!nextId) return
    setNextVideo(nextId)
    setCountdown(5)
  }, [autoplayDisabled, id, v?.source, v?.categories])

  useEffect(() => {
    if (countdown === null) return
    if (countdown <= 0) {
      if (nextVideo) navigate(`/play/${nextVideo}`, { viewTransition: true })
      return
    }
    const t = setTimeout(() => setCountdown(countdown - 1), 1000)
    return () => clearTimeout(t)
  }, [countdown, nextVideo, navigate])

  // Progress bar click
  const handleProgressClick = useCallback((e: React.MouseEvent) => {
    const vid = videoRef.current
    const el = progressRef.current
    if (!vid || !el || !duration) return
    const rect = el.getBoundingClientRect()
    const pct = (e.clientX - rect.left) / rect.width
    vid.currentTime = pct * duration
  }, [duration])

  const togglePlay = useCallback(() => {
    const vid = videoRef.current
    if (!vid) return
    vid.paused ? vid.play().catch(() => {}) : vid.pause()
  }, [])

  const toggleFullscreen = () => {
    const el = videoRef.current?.parentElement
    if (!el) return
    if (document.fullscreenElement) {
      document.exitFullscreen().catch(() => {})
    } else {
      el.requestFullscreen().catch(() => {})
    }
  }

  // Touch gestures
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

  const progressPct = duration > 0 ? (currentTime / duration) * 100 : 0

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-[60dvh]">
        <div className="flex flex-col items-center gap-3">
          <svg className="animate-spin h-8 w-8 text-red" viewBox="0 0 24 24" fill="none">
            <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="3" opacity="0.2" />
            <path d="M12 2a10 10 0 0 1 10 10" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
          </svg>
          <span className="text-sm text-muted">Loading video...</span>
        </div>
      </div>
    )
  }

  if (!v) {
    return (
      <div className="flex items-center justify-center min-h-[60dvh]">
        <div className="text-center space-y-3">
          <svg className="w-12 h-12 mx-auto text-muted/40" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
            <circle cx="12" cy="12" r="10" />
            <line x1="15" y1="9" x2="9" y2="15" />
            <line x1="9" y1="9" x2="15" y2="15" />
          </svg>
          <p className="text-muted">Video not found.</p>
          <Link to="/" className="inline-block text-sm text-red hover:underline">Browse videos</Link>
        </div>
      </div>
    )
  }

  const qualities = [
    v.url_1080 && '1080',
    v.url_720 && '720',
    v.url_360 && '360',
  ].filter(Boolean) as Array<'1080' | '720' | '360'>
  const resolvedQuality = qualities.includes(quality) ? quality : qualities[0]
  const src = resolvedQuality ? `/vid/${id}/${resolvedQuality}` : ''

  const isNotXnxx = v.source && v.source !== 'xnxx'
  const posterSrc = v.thumb_uuid
    ? (isNotXnxx ? proxiedMedia(v.thumb_uuid) : xnxxPoster(v.thumb_uuid))
    : undefined
  const previewSrc = v.preview_url
    ? (isNotXnxx ? proxiedMedia(v.preview_url) : (v.thumb_uuid ? xnxxPreview(v.thumb_uuid) : ''))
    : !isNotXnxx && v.thumb_uuid ? xnxxPreview(v.thumb_uuid) : ''

  return (
    <div className={`mx-auto px-3 py-3 md:px-6 md:py-5 ${theaterMode ? 'max-w-none' : 'max-w-5xl'}`}
         onMouseMove={handleMouseMove}>

      {/* Player */}
      <div className={`relative ${theaterMode ? 'fixed inset-0 z-50 bg-black' : 'aspect-video'} overflow-hidden
                      ${theaterMode ? '' : 'rounded-lg mb-4 shadow-elevated'}`
                      }
           onTouchStart={handleTouchStart}
           onTouchEnd={handleTouchEnd}>

        {previewSrc || src ? (
          <video
            ref={videoRef}
            key={`${id}-${resolvedQuality || quality}`}
            className="w-full h-full object-contain bg-black"
            poster={posterSrc}
            onClick={togglePlay}
            onDoubleClick={toggleFullscreen}
            onEnded={handleEnded}
            onPlay={() => setPlaying(true)}
            onPause={() => setPlaying(false)}
            onMouseMove={handleMouseMove}
            playsInline
          >
            {src && <source src={src} type="video/mp4" />}
            {v.hls_url && <source src={v.hls_url} type="application/vnd.apple.mpegurl" />}
            {previewSrc && <source src={previewSrc} type="video/mp4" />}
          </video>
        ) : (
          <div className="flex items-center justify-center h-full text-muted bg-bg">
            <div className="text-center space-y-2">
              <svg className="w-10 h-10 mx-auto opacity-50" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
                <polygon points="5 3 19 12 5 21 5 3" />
              </svg>
              <p className="text-sm">No stream available</p>
            </div>
          </div>
        )}

        {/* Autoplay countdown */}
        {countdown !== null && nextVideo && (
          <div className="absolute inset-0 bg-black/70 flex items-center justify-center z-50 animate-fade-in">
            <div className="text-center">
              <p className="text-white text-lg mb-3">Next video in <span className="font-bold text-red">{countdown}</span></p>
              <button
                onClick={() => { setCountdown(null); setAutoplayDisabled(true); sessionStorage.setItem('kxxx_autoplay_disabled', 'true') }}
                className="px-5 py-2 bg-white/10 hover:bg-white/20 text-white rounded-full text-sm transition-colors"
              >
                Cancel autoplay
              </button>
            </div>
          </div>
        )}

        {/* Gesture overlay */}
        {gestureOverlay && (
          <div className="absolute inset-0 flex items-center justify-center pointer-events-none z-40">
            <div className="bg-black/70 backdrop-blur-sm text-white text-xl font-bold px-5 py-2.5 rounded-lg animate-scale-in
                           border border-white/10">
              {gestureOverlay.type === 'seek' ? gestureOverlay.value : `Vol: ${gestureOverlay.value}`}
            </div>
          </div>
        )}

        {/* Keyboard shortcuts help */}
        {showShortcuts && (
          <div className="absolute inset-0 bg-black/80 flex items-center justify-center z-50"
               onClick={() => setShowShortcuts(false)}>
            <div className="bg-card border border-border rounded-xl p-6 max-w-xs w-full space-y-3 animate-scale-in"
                 onClick={e => e.stopPropagation()}>
              <h3 className="text-sm font-bold text-text">Keyboard Shortcuts</h3>
              <div className="space-y-2 text-xs text-muted">
                {[
                  ['Space', 'Play / Pause'],
                  ['← / →', 'Seek -10s / +10s'],
                  ['↑ / ↓', 'Volume up / down'],
                  ['F', 'Fullscreen'],
                  ['T', 'Theater mode'],
                  ['M', 'Mute'],
                  ['?', 'Toggle shortcuts'],
                ].map(([key, desc]) => (
                  <div key={key} className="flex items-center justify-between">
                    <kbd className="px-1.5 py-0.5 rounded bg-white/10 text-text text-[10px] font-mono">{key}</kbd>
                    <span className="text-muted">{desc}</span>
                  </div>
                ))}
              </div>
              <button onClick={() => setShowShortcuts(false)}
                className="w-full text-center text-xs text-muted hover:text-text py-2 transition-colors">
                Close
              </button>
            </div>
          </div>
        )}

        {/* Custom controls overlay */}
        {!theaterMode && (
          <div className={`absolute inset-x-0 bottom-0 video-controls ${showControls ? '' : 'opacity-0 pointer-events-none'}`}
               onMouseEnter={handleControlsMouseEnter}
               onMouseLeave={handleControlsMouseLeave}>

            {/* Progress bar */}
            <div ref={progressRef} className="video-progress-track group/progress"
                 onClick={handleProgressClick}>
              <div className="video-progress-fill" style={{ width: `${progressPct}%` }} />
            </div>

            {/* Controls row */}
            <div className="flex items-center gap-3 mt-1">
              {/* Play/Pause */}
              <button onClick={togglePlay}
                className="text-white hover:text-red transition-colors w-8 h-8 flex items-center justify-center">
                {playing ? (
                  <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
                    <rect x="6" y="4" width="4" height="16" /><rect x="14" y="4" width="4" height="16" />
                  </svg>
                ) : (
                  <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
                    <polygon points="8,5 19,12 8,19" />
                  </svg>
                )}
              </button>

              {/* Time display */}
              <span className="text-white text-[11px] font-mono tabular-nums min-w-[90px]">
                {formatTime(currentTime)} / {formatTime(duration)}
              </span>

              {/* Volume */}
              <div className="relative flex items-center"
                   onMouseEnter={() => setShowVolumeSlider(true)}
                   onMouseLeave={() => setShowVolumeSlider(false)}>
                <button onClick={() => { const vid = videoRef.current; if (vid) vid.muted = !vid.muted }}
                  className="text-white/70 hover:text-white transition-colors w-7 h-7 flex items-center justify-center">
                  {muted || volume === 0 ? (
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                      <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5" />
                      <line x1="23" y1="9" x2="17" y2="15" /><line x1="17" y1="9" x2="23" y2="15" />
                    </svg>
                  ) : (
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                      <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5" />
                      <path d="M19.07 4.93a10 10 0 0 1 0 14.14M15.54 8.46a5 5 0 0 1 0 7.07" />
                    </svg>
                  )}
                </button>
                {showVolumeSlider && (
                  <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-2 bg-card border border-border rounded-lg p-2 shadow-elevated animate-scale-in">
                    <input type="range" min="0" max="1" step="0.05"
                      value={muted ? 0 : volume}
                      onChange={e => { const vid = videoRef.current; if (vid) { vid.volume = parseFloat(e.target.value); vid.muted = false }}}
                      className="w-20 h-1 accent-red appearance-none bg-white/20 rounded-full cursor-pointer"
                      style={{ writingMode: 'vertical-lr', direction: 'rtl', height: '80px', width: '4px' }}
                      aria-label="Volume"
                    />
                  </div>
                )}
              </div>

              <div className="flex-1" />

              {/* Quality selector */}
              {qualities.length > 1 && (
                <div className="flex gap-1">
                  {qualities.map(q => (
                    <button key={q} onClick={() => setQuality(q as '360' | '720' | '1080')}
                      className={`px-2 py-0.5 rounded text-[10px] font-semibold transition-colors
                                  ${quality === q
                                    ? 'bg-red text-white'
                                    : 'bg-white/10 text-white/70 hover:bg-white/20'
                                  }`}>
                      {q}p
                    </button>
                  ))}
                </div>
              )}

              {/* Theater mode */}
              {theaterMode && (
                <button onClick={toggleTheater}
                  className="text-white/70 hover:text-white text-[10px] px-2 py-1 rounded bg-white/10 hover:bg-white/20 transition-colors">
                  Exit Theater
                </button>
              )}

              {/* Fullscreen */}
              <button onClick={toggleFullscreen} title="Fullscreen (F)"
                className="text-white/70 hover:text-white transition-colors w-7 h-7 flex items-center justify-center">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <polyline points="15 3 21 3 21 9" /><polyline points="9 21 3 21 3 15" />
                  <line x1="21" y1="3" x2="14" y2="10" /><line x1="3" y1="21" x2="10" y2="14" />
                </svg>
              </button>
            </div>
          </div>
        )}

        {/* Theater mode controls */}
        {theaterMode && (
          <div className={`absolute inset-x-0 bottom-0 video-controls ${showControls ? '' : 'opacity-0 pointer-events-none'}`}
               onMouseEnter={handleControlsMouseEnter}
               onMouseLeave={handleControlsMouseLeave}>

            {/* Progress bar */}
            <div ref={progressRef} className="video-progress-track group/progress"
                 onClick={handleProgressClick}>
              <div className="video-progress-fill" style={{ width: `${progressPct}%` }} />
            </div>

            <div className="flex items-center gap-3 mt-1">
              <button onClick={togglePlay}
                className="text-white hover:text-red transition-colors w-8 h-8 flex items-center justify-center">
                {playing ? (
                  <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
                    <rect x="6" y="4" width="4" height="16" /><rect x="14" y="4" width="4" height="16" />
                  </svg>
                ) : (
                  <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor">
                    <polygon points="8,5 19,12 8,19" />
                  </svg>
                )}
              </button>

              <span className="text-white text-[11px] font-mono tabular-nums">
                {formatTime(currentTime)} / {formatTime(duration)}
              </span>

              {/* Volume in theater */}
              <div className="relative flex items-center"
                   onMouseEnter={() => setShowVolumeSlider(true)}
                   onMouseLeave={() => setShowVolumeSlider(false)}>
                <button onClick={() => { const vid = videoRef.current; if (vid) vid.muted = !vid.muted }}
                  className="text-white/70 hover:text-white transition-colors w-7 h-7 flex items-center justify-center">
                  {muted || volume === 0 ? (
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                      <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5" />
                      <line x1="23" y1="9" x2="17" y2="15" /><line x1="17" y1="9" x2="23" y2="15" />
                    </svg>
                  ) : (
                    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                      <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5" />
                      <path d="M19.07 4.93a10 10 0 0 1 0 14.14M15.54 8.46a5 5 0 0 1 0 7.07" />
                    </svg>
                  )}
                </button>
              </div>

              <div className="flex-1" />

              <button onClick={() => setShowShortcuts(true)}
                className="text-white/50 hover:text-white text-[10px] px-2 py-1 rounded transition-colors"
                title="Keyboard shortcuts">
                ?
              </button>

              <button onClick={toggleTheater}
                className="text-white/70 hover:text-white text-[10px] px-2 py-1 rounded bg-white/10 hover:bg-white/20 transition-colors">
                Exit Theater
              </button>

              <button onClick={toggleFullscreen}
                className="text-white/70 hover:text-white transition-colors w-7 h-7 flex items-center justify-center">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                  <polyline points="15 3 21 3 21 9" /><polyline points="9 21 3 21 3 15" />
                  <line x1="21" y1="3" x2="14" y2="10" /><line x1="3" y1="21" x2="10" y2="14" />
                </svg>
              </button>
            </div>
          </div>
        )}
      </div>

      {/* Info (hidden in theater) */}
      <div className={`space-y-3 transition-all duration-300 ${theaterMode ? 'opacity-0 h-0 overflow-hidden pointer-events-none' : 'animate-slide-up'}`}>
        <h1 className="text-lg font-bold leading-tight md:text-xl flex items-center gap-2">
          {v.title}
          {id && <FavoriteButton videoId={id} />}
          {id && <RatingButtons videoId={id} compact />}
        </h1>

        <div className="flex flex-wrap items-center gap-3 text-sm text-muted">
          {v.views > 0 && <span>{formatViews(v.views)} views</span>}
          {v.watch_count != null && v.watch_count > 0 && (
            <><span className="text-border">|</span><span>{formatViews(v.watch_count)} KaraXXX watches</span></>
          )}
          {v.duration > 0 && (
            <><span className="text-border">|</span><span>{formatDuration(v.duration)}</span></>
          )}
          {v.uploader && (
            <><span className="text-border">|</span>
              <Link to={`/?uploader=${encodeURIComponent(v.uploader)}`} className="text-red hover:underline">{v.uploader}</Link>
            </>
          )}
          {v.upload_date && (
            <><span className="text-border">|</span><span>{v.upload_date}</span></>
          )}
          <span className="text-border">|</span>
          <button onClick={() => setShowShortcuts(true)}
            className="text-muted hover:text-text transition-colors text-xs">
            Shortcuts (?)
          </button>
        </div>

        {/* Quality + actions */}
        <div className="flex gap-1.5 flex-wrap items-center">
          {qualities.length > 1 && qualities.map(q => (
            <button key={q} onClick={() => setQuality(q as '360' | '720' | '1080')}
              className={`px-3 py-1 rounded-full text-xs font-semibold transition-colors
                          ${quality === q
                            ? 'bg-red text-white shadow-[0_2px_8px_-2px_rgba(225,29,72,0.5)]'
                            : 'bg-card border border-border text-muted hover:text-text hover:border-red/40'
                          }`}>
              {q}p
            </button>
          ))}
          {id && <PlaylistButton videoId={id} />}
          <button onClick={toggleTheater}
            className={`px-3 py-1 rounded-full text-xs font-semibold transition-colors ${theaterMode ? 'bg-red text-white' : 'bg-card border border-border text-muted hover:text-text'}`}>
            Theater
          </button>
          {!autoplayDisabled && (
            <button onClick={() => { setAutoplayDisabled(true); sessionStorage.setItem('kxxx_autoplay_disabled', 'true') }}
              className="px-3 py-1 rounded-full text-xs font-semibold transition-colors bg-card border border-border text-muted hover:text-text">
              Autoplay on
            </button>
          )}
        </div>

        {/* Categories */}
        {v.categories && v.categories.length > 0 && (
          <div className="flex gap-1.5 flex-wrap">
            {v.categories.filter(c => c !== 'uncategorized').map((cat, i) => (
              <Link key={cat} to={`/?cat=${encodeURIComponent(cat)}`
                } className={`text-[11px] px-2 py-0.5 rounded-full font-semibold capitalize transition-colors
                            ${i === 0 ? 'bg-red/15 text-red' : 'bg-orange/10 text-orange hover:bg-orange/20'
                  }`}>
                {cat}
              </Link>
            ))}
          </div>
        )}

        {/* Tags */}
        {v.tags && v.tags.length > 0 && (
          <div className="flex gap-1 flex-wrap">
            {v.tags.slice(0, 15).map(tag => (
              <Link key={tag} to={`/search?q=${encodeURIComponent(tag)}`}
                className="text-[10px] px-1.5 py-0.5 rounded bg-white/5 text-muted
                           hover:text-text hover:bg-white/10 transition-colors capitalize">
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
