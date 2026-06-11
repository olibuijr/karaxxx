import { useEffect, useState, useCallback, useRef } from 'react'
import { useSearchParams } from 'react-router-dom'
import VideoCard from '../components/VideoCard'
import VideoCardSkeleton from '../components/VideoCardSkeleton'
import FilterSelect from '../components/FilterSelect'
import { fetchBrowse, subscribeProgress } from '../api'
import type { Video, CrawlProgress, BrowseParams } from '../types'
import { SOURCES } from '../types'
import { useAuth } from '../lib/auth'

export default function Browse() {
  const [sp] = useSearchParams()
  const sort = sp.get('sort') || 'recent'
  const cat = sp.get('cat') || ''
  const q = sp.get('q') || ''
  const uploader = sp.get('uploader') || ''
  const sourceFilter = sp.get('source') || ''

  const [videos, setVideos] = useState<Video[]>([])
  const [page, setPage] = useState(0)
  const [totalPages, setTotalPages] = useState(0)
  const [totalCount, setTotalCount] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const gridRef = useRef<HTMLDivElement>(null)
  const sentinelRef = useRef<HTMLDivElement>(null)
  const previewRef = useRef<(() => void) | undefined>(undefined)
  const busyRef = useRef(false)

  const { token } = useAuth()
  const [history, setHistory] = useState<Video[]>([])

  const [density, setDensity] = useState<'compact' | 'comfortable' | 'large' | 'theatre'>(() => (localStorage.getItem('kxxx_density') as any) || 'comfortable')
  const [activeTab, setActiveTab] = useState<'browse' | 'for-you'>('browse')
  const [forYouVideos, setForYouVideos] = useState<Video[]>([])
  const [forYouLoading, setForYouLoading] = useState(false)

  const densityGrid = density === 'theatre'
    ? 'grid-cols-1 max-w-4xl mx-auto'
    : density === 'compact'
    ? 'grid-cols-4 xs:grid-cols-5 sm:grid-cols-6 md:grid-cols-7 lg:grid-cols-8 xl:grid-cols-10 2xl:grid-cols-12'
    : density === 'large'
    ? 'grid-cols-1 sm:grid-cols-2 lg:grid-cols-3'
    : 'grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 2xl:grid-cols-6'

  const isForYou = activeTab === 'for-you'
  const showVideos = isForYou ? forYouVideos : videos
  const showLoading = isForYou ? forYouLoading : loading
  const showTotal = isForYou ? forYouVideos.length : totalCount

  const filters = `${sort}|${cat}|${q}|${uploader}|${sourceFilter}`

  useEffect(() => {
    if (!token) { setHistory([]); return }
    fetch(`/api/watch/history?limit=8`, {
      headers: { Authorization: `Bearer ${token}` }
    })
      .then(r => r.ok ? r.json() : [])
      .then(setHistory)
      .catch(() => setHistory([]))
  }, [token])

  const removeFromHistory = useCallback(async (videoId: string) => {
    if (!token) return
    await fetch(`/api/watch/${videoId}`, {
      method: 'DELETE',
      headers: { Authorization: `Bearer ${token}` }
    })
    setHistory(prev => prev.filter(h => h.id !== videoId))
  }, [token])

  useEffect(() => {
    setVideos([])
    setPage(0)
    setTotalPages(0)
    setTotalCount(0)
    setLoading(true)
    busyRef.current = false
    fetchBrowse({ page: 1, sort: sort as BrowseParams['sort'], cat, q, uploader, source: sourceFilter || undefined })
      .then(d => {
        setVideos(d.videos)
        setPage(1)
        setTotalPages(d.total_pages)
        setTotalCount(d.count)
      })
      .finally(() => setLoading(false))
  }, [filters])

  useEffect(() => {
    if (activeTab !== 'for-you' || !token) return
    setForYouLoading(true)
    fetch(`/api/for-you`, {
      headers: { Authorization: `Bearer ${token}` }
    })
      .then(r => r.ok ? r.json() : [])
      .then(setForYouVideos)
      .catch(() => setForYouVideos([]))
      .finally(() => setForYouLoading(false))
  }, [activeTab, token])

  const loadMore = useCallback(() => {
    if (busyRef.current || page >= totalPages) return
    busyRef.current = true
    setLoadingMore(true)
    fetchBrowse({ page: page + 1, sort: sort as BrowseParams['sort'], cat, q, uploader, source: sourceFilter || undefined })
      .then(d => {
        setVideos(prev => [...prev, ...d.videos])
        setPage(d.page)
        setTotalPages(d.total_pages)
        setTotalCount(d.count)
      })
      .finally(() => {
        setLoadingMore(false)
        busyRef.current = false
      })
  }, [page, totalPages, sort, cat, q, uploader, sourceFilter])

  useEffect(() => {
    const sentinel = sentinelRef.current
    if (!sentinel || totalPages === 0) return
    const obs = new IntersectionObserver(entries => {
      if (entries[0].isIntersecting) loadMore()
    }, { rootMargin: '600px' })
    obs.observe(sentinel)
    return () => obs.disconnect()
  }, [loadMore, totalPages])

  useEffect(() => {
    const unsub = subscribeProgress((p: CrawlProgress) => {
      if (p.total_count) setTotalCount(p.total_count)
    })
    return unsub
  }, [])

  useEffect(() => {
    previewRef.current?.()
    const grid = gridRef.current
    if (!grid || showVideos.length === 0) return

    const vids = () => grid.querySelectorAll<HTMLVideoElement>('video[data-preview]')

    function tick() {
      const els = vids()
      if (els.length === 0) return
      let best: HTMLVideoElement | null = null
      let bestRatio = 0
      const ch = window.innerHeight
      els.forEach(v => {
        const r = v.getBoundingClientRect()
        const vis = Math.max(0, Math.min(r.bottom, ch) - Math.max(r.top, 0))
        const ratio = vis / r.height
        if (ratio > bestRatio) { bestRatio = ratio; best = v }
      })
      els.forEach(v => {
        if (v === best && bestRatio > 0.2) {
          v.play().catch(() => {})
          v.classList.add('playing')
        } else {
          v.pause()
          v.classList.remove('playing')
        }
      })
    }

    if (window.innerWidth < 768) {
      tick()
      window.addEventListener('scroll', tick, { passive: true })
      window.addEventListener('resize', tick)
      previewRef.current = () => {
        window.removeEventListener('scroll', tick)
        window.removeEventListener('resize', tick)
      }
      return () => {
        window.removeEventListener('scroll', tick)
        window.removeEventListener('resize', tick)
      }
    }
  }, [showVideos])

  const label = q
    ? `Search: "${q}" (${showTotal || 0} results)`
    : uploader
    ? `Uploader: ${uploader}`
    : ''

  const sortHref = (s: string) => {
    const p = new URLSearchParams(sp)
    if (s === 'recent') p.delete('sort')
    else p.set('sort', s)
    const qs = p.toString()
    return `/?${qs}`
  }

  const sorts: { label: string; value: string }[] = [
    { label: 'Recent', value: 'recent' },
    { label: 'New', value: 'new' },
    { label: 'Popular', value: 'views' },
    { label: 'Longest', value: 'duration' },
  ]

  const sourceHref = (src: string) => {
    const p = new URLSearchParams(sp)
    if (src === '') p.delete('source')
    else p.set('source', src)
    const qs = p.toString()
    return `/?${qs}`
  }

  const sources = SOURCES

  return (
    <>
      {label && <div className="px-3 py-1 text-xs text-muted md:px-6">{label}</div>}

      <div className="flex items-center gap-2 px-2.5 py-2 md:px-6">
        <div className="w-32">
          <span className="text-[11px] font-semibold text-muted/70 uppercase tracking-widest mb-1 block">Sort</span>
          <FilterSelect options={sorts} current={sort} getHref={sortHref} />
        </div>
        <div className="w-32">
          <span className="text-[11px] font-semibold text-muted/70 uppercase tracking-widest mb-1 block">Source</span>
          <FilterSelect options={sources} current={sourceFilter} getHref={sourceHref} />
        </div>
        <span className="w-px h-6 bg-white/10 mx-1" aria-hidden />
        {[
          { key: 'compact', icon: '\u25A6' },
          { key: 'comfortable', icon: '\u25A3' },
          { key: 'large', icon: '\u25A1' },
          { key: 'theatre', icon: '\u25A0' },
        ].map(d => (
          <button key={d.key} onClick={() => { setDensity(d.key as any); localStorage.setItem('kxxx_density', d.key) }}
            className={`px-2 py-1 rounded-md text-xs font-semibold transition-all duration-150 ${density === d.key ? 'bg-white/10 text-orange shadow-inner' : 'text-muted hover:text-text hover:bg-white/5'}`}>
            {d.icon}
          </button>
        ))}
      </div>

      {token && (
        <div className="flex items-center gap-2 px-2.5 md:px-6 py-1">
          <button onClick={() => setActiveTab('browse')}
            className={`px-3 py-1 rounded-full text-xs font-semibold transition-all duration-150 ${activeTab === 'browse' ? 'bg-gradient-to-br from-orange to-red text-white shadow-[0_2px_12px_-2px_rgba(249,115,22,0.5)]' : 'text-muted hover:text-text hover:bg-white/5'}`}>
            Browse
          </button>
          <button onClick={() => setActiveTab('for-you')}
            className={`px-3 py-1 rounded-full text-xs font-semibold transition-all duration-150 ${activeTab === 'for-you' ? 'bg-gradient-to-br from-orange to-red text-white shadow-[0_2px_12px_-2px_rgba(249,115,22,0.5)]' : 'text-muted hover:text-text hover:bg-white/5'}`}>
            For You
          </button>
        </div>
      )}

      {!isForYou && history.length > 0 && (
        <div className="px-2.5 md:px-6">
          <h2 className="text-sm font-bold mb-2">Continue Watching</h2>
          <div className="overflow-x-auto flex gap-2.5 pb-2 snap-x snap-mandatory">
            {history.map(h => (
              <div key={h.id} className="relative flex-shrink-0 w-48 snap-start">
                <button
                  onClick={(e) => { e.preventDefault(); e.stopPropagation(); removeFromHistory(h.id) }}
                  className="absolute top-1 right-1 z-20 w-5 h-5 rounded-full bg-black/60 text-white text-xs flex items-center justify-center hover:bg-red/80 transition-colors"
                >
                  X
                </button>
                <VideoCard video={h} />
              </div>
            ))}
          </div>
        </div>
      )}

      {showLoading && showVideos.length === 0 && (
        <div className={`grid gap-2.5 p-2.5 ${densityGrid}`}>
          {Array.from({ length: 12 }).map((_, i) => (
            <VideoCardSkeleton key={i} index={i} />
          ))}
        </div>
      )}

      {!showLoading && showVideos.length === 0 && (
        <div className="text-center py-16 text-muted">
          {q ? `No results for "${q}".` : uploader ? `No videos for ${uploader}.` : 'No videos yet.'}
        </div>
      )}

      <div ref={gridRef}
           className={`grid gap-2.5 p-2.5 ${densityGrid}`}>
        {showVideos.map(v => (
          <div key={v.id}>
            <VideoCard video={v} />
            {isForYou && (v as any).reason && (
              <span className="text-[10px] text-orange/80 mt-1 block">{(v as any).reason}</span>
            )}
          </div>
        ))}
      </div>

      {loadingMore && (
        <div className="text-center py-6 text-muted text-sm">Loading more...</div>
      )}

      {!isForYou && page >= totalPages && totalPages > 0 && (
        <div className="text-center pb-8 text-[11px] text-muted">
          {totalCount.toLocaleString()} total
        </div>
      )}

      <div ref={sentinelRef} className="h-1" />
    </>
  )
}
