import { useEffect, useState, useCallback, useRef } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import VideoCard from '../components/VideoCard'
import VideoCardSkeleton from '../components/VideoCardSkeleton'
import FilterSelect from '../components/FilterSelect'
import { clearWatchHistory, fetchBrowse, fetchCategories, subscribeProgress, removeWatchHistory } from '../api'
import {
  Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger,
} from '../components/ui/dialog'
import { parseCategories, toggleCategoryParam } from '../lib/categories'
import type { Video, CrawlProgress } from '../types'
import { SOURCES } from '../types'
import { useAuth } from '../lib/auth'
import { XIcon } from 'lucide-react'

type BrowseSort = 'recent' | 'new' | 'views' | 'duration'
type Density = 'compact' | 'comfortable' | 'large' | 'theatre'

const SORT_VALUES: readonly BrowseSort[] = ['recent', 'new', 'views', 'duration']
const DENSITY_VALUES: readonly Density[] = ['compact', 'comfortable', 'large', 'theatre']
const DENSITY_OPTIONS: ReadonlyArray<{ key: Density; icon: string; label: string }> = [
  { key: 'compact', icon: '\u25A6', label: 'Compact' },
  { key: 'comfortable', icon: '\u25A3', label: 'Comfortable' },
  { key: 'large', icon: '\u25A1', label: 'Large' },
  { key: 'theatre', icon: '\u25A0', label: 'Theatre' },
]

function readStored(key: string): string | null {
  if (typeof window === 'undefined') return null
  try { return window.localStorage.getItem(key) } catch { return null }
}

function writeStored(key: string, value: string): void {
  if (typeof window === 'undefined') return
  try { window.localStorage.setItem(key, value) } catch { /* ignore */ }
}

function isBrowseSort(value: string | null): value is BrowseSort {
  return value !== null && SORT_VALUES.includes(value as BrowseSort)
}
function isDensity(value: string | null): value is Density {
  return value !== null && DENSITY_VALUES.includes(value as Density)
}
function isSourceValue(value: string | null): value is string {
  return value !== null && SOURCES.some((source) => source.value === value)
}

function EmptyState({ icon, title, message, action }: { icon: string; title: string; message: string; action?: { label: string; onClick: () => void } }) {
  return (
    <div className="flex flex-col items-center justify-center py-20 px-4 text-center animate-fade-in">
      <svg className="w-16 h-16 text-muted/20 mb-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1">
        <path d={icon} />
      </svg>
      <h3 className="text-base font-semibold text-muted mb-1">{title}</h3>
      <p className="text-sm text-muted/60 max-w-xs">{message}</p>
      {action && (
        <button onClick={action.onClick}
          className="mt-4 px-4 py-2 rounded-full bg-red/15 text-red text-xs font-semibold hover:bg-red/25 transition-colors">
          {action.label}
        </button>
      )}
    </div>
  )
}

export default function Browse() {
  const navigate = useNavigate()
  const [sp] = useSearchParams()
  const [storedSort, setStoredSort] = useState<BrowseSort>(() => {
    const stored = readStored('kxxx_sort')
    return isBrowseSort(stored) ? stored : 'recent'
  })
  const [storedSource, setStoredSource] = useState<string>(() => {
    const stored = readStored('kxxx_source')
    return isSourceValue(stored) ? stored : ''
  })
  const sortParam = sp.get('sort')
  const sourceParam = sp.get('source')
  const hasSortParam = sp.has('sort')
  const hasSourceParam = sp.has('source')
  const sort = isBrowseSort(sortParam) ? sortParam : storedSort
  const cat = sp.get('cat') || ''
  const q = sp.get('q') || ''
  const uploader = sp.get('uploader') || ''
  const sourceFilter = isSourceValue(sourceParam) ? sourceParam : storedSource

  const [videos, setVideos] = useState<Video[]>([])
  const [page, setPage] = useState(0)
  const [totalPages, setTotalPages] = useState(0)
  const [totalCount, setTotalCount] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [browseError, setBrowseError] = useState<string | null>(null)
  const [loadMoreError, setLoadMoreError] = useState<string | null>(null)
  const [categoryOptions, setCategoryOptions] = useState<{ label: string; value: string }[]>([
    { label: 'All categories', value: '' },
  ])
  const gridRef = useRef<HTMLDivElement>(null)
  const sentinelRef = useRef<HTMLDivElement>(null)
  const previewRef = useRef<(() => void) | undefined>(undefined)
  const busyRef = useRef(false)

  const { token } = useAuth()
  const [history, setHistory] = useState<Video[]>([])
  const [clearHistoryOpen, setClearHistoryOpen] = useState(false)
  const [clearingHistory, setClearingHistory] = useState(false)
  const [clearError, setClearError] = useState<string | null>(null)

  const [density, setDensity] = useState<Density>(() => {
    const stored = readStored('kxxx_density')
    return isDensity(stored) ? stored : 'comfortable'
  })
  const [activeTab, setActiveTab] = useState<'browse' | 'for-you'>('browse')
  const [forYouVideos, setForYouVideos] = useState<Array<Video & { reason?: string }>>([])
  const [forYouLoading, setForYouLoading] = useState(false)

  const activeCategories = parseCategories(cat)
  const activeFilterCount = activeCategories.length + (sourceFilter ? 1 : 0)
  const activeSource = sourceFilter ? SOURCES.find((source) => source.value === sourceFilter) ?? null : null

  const densityGrid = density === 'theatre'
    ? 'grid grid-cols-1 gap-4 p-0 sm:p-3 max-w-4xl mx-auto'
    : density === 'compact'
    ? 'grid grid-cols-2 gap-1.5 p-1.5 sm:grid-cols-3 sm:gap-2 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-7'
    : density === 'large'
    ? 'grid grid-cols-1 gap-3 p-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4'
    : 'grid grid-cols-2 gap-2.5 p-2.5 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5'

  const isForYou = activeTab === 'for-you'
  const showVideos = isForYou ? forYouVideos : videos
  const showLoading = isForYou ? forYouLoading : loading
  const showTotal = isForYou ? forYouVideos.length : totalCount

  const filters = `${sort}|${cat}|${q}|${uploader}|${sourceFilter}`

  // Store preferences from URL params
  useEffect(() => {
    if (!hasSortParam) return
    const nextSort = isBrowseSort(sortParam) ? sortParam : 'recent'
    setStoredSort(nextSort)
    writeStored('kxxx_sort', nextSort)
  }, [hasSortParam, sortParam])

  useEffect(() => {
    if (!hasSourceParam) return
    const nextSource = isSourceValue(sourceParam) ? sourceParam : ''
    setStoredSource(nextSource)
    writeStored('kxxx_source', nextSource)
  }, [hasSourceParam, sourceParam])

  useEffect(() => {
    fetchCategories(80)
      .then((categories) => {
        setCategoryOptions([
          { label: 'All categories', value: '' },
          ...categories.map((name) => ({ label: name, value: name })),
        ])
      })
      .catch(() => setCategoryOptions([{ label: 'All categories', value: '' }]))
  }, [])

  useEffect(() => {
    if (!token) { setHistory([]); return }
    setClearError(null)
    fetch(`/api/watch/history?limit=8`, { headers: { Authorization: `Bearer ${token}` }})
      .then((response) => { if (!response.ok) return [] as Video[]; return response.json() as Promise<Video[]> })
      .then(setHistory)
      .catch(() => setHistory([]))
  }, [token])

  const removeFromHistory = useCallback(async (videoId: string) => {
    if (!token) return
    await removeWatchHistory(token, videoId)
    setHistory(prev => prev.filter(h => h.id !== videoId))
  }, [token])

  const updateHref = useCallback((mutate: (params: URLSearchParams) => void) => {
    const params = new URLSearchParams(sp)
    mutate(params)
    const qs = params.toString()
    return qs ? `/?${qs}` : '/'
  }, [sp])

  const handleClearHistoryConfirm = useCallback(async () => {
    if (!token || clearingHistory) return
    setClearingHistory(true)
    setClearError(null)
    try {
      await clearWatchHistory(token)
      setHistory([])
      setClearHistoryOpen(false)
    } catch { setClearError("Couldn't clear history.") }
    finally { setClearingHistory(false) }
  }, [clearingHistory, token])

  const handleRemoveCategoryFilter = useCallback((category: string) => {
    const href = updateHref((params) => {
      const nextCat = toggleCategoryParam(params.get('cat'), category)
      if (nextCat) params.set('cat', nextCat); else params.delete('cat')
    })
    navigate(href, { viewTransition: true })
  }, [navigate, updateHref])

  const handleRemoveSourceFilter = useCallback(() => {
    setStoredSource('')
    writeStored('kxxx_source', '')
    const href = updateHref((params) => params.delete('source'))
    navigate(href, { viewTransition: true })
  }, [navigate, updateHref])

  const handleClearActiveFilters = useCallback(() => {
    setStoredSource('')
    writeStored('kxxx_source', '')
    const href = updateHref((params) => { params.delete('cat'); params.delete('source') })
    navigate(href, { viewTransition: true })
  }, [navigate, updateHref])

  // Fetch browse data
  useEffect(() => {
    setVideos([])
    setPage(0)
    setTotalPages(0)
    setTotalCount(0)
    setLoading(true)
    setBrowseError(null)
    setLoadMoreError(null)
    busyRef.current = false
    fetchBrowse({ page: 1, sort, cat, q, uploader, source: sourceFilter || undefined })
      .then(d => { setVideos(d.videos); setPage(1); setTotalPages(d.total_pages); setTotalCount(d.count) })
      .catch(() => { setVideos([]); setBrowseError('Could not load videos right now.') })
      .finally(() => setLoading(false))
  }, [filters])

  // For You tab
  useEffect(() => {
    if (activeTab !== 'for-you' || !token) return
    setForYouLoading(true)
    fetch(`/api/for-you`, { headers: { Authorization: `Bearer ${token}` }})
      .then(r => { if (!r.ok) return [] as Array<Video & { reason?: string }>; return r.json() as Promise<Array<Video & { reason?: string }>> })
      .then(setForYouVideos)
      .catch(() => setForYouVideos([]))
      .finally(() => setForYouLoading(false))
  }, [activeTab, token])

  // Infinite scroll
  const loadMore = useCallback(() => {
    if (busyRef.current || page >= totalPages) return
    busyRef.current = true
    setLoadingMore(true)
    setLoadMoreError(null)
    fetchBrowse({ page: page + 1, sort, cat, q, uploader, source: sourceFilter || undefined })
      .then(d => { setVideos(prev => [...prev, ...d.videos]); setPage(d.page); setTotalPages(d.total_pages); setTotalCount(d.count) })
      .catch(() => setLoadMoreError('Could not load more.'))
      .finally(() => { setLoadingMore(false); busyRef.current = false })
  }, [page, totalPages, sort, cat, q, uploader, sourceFilter])

  useEffect(() => {
    const sentinel = sentinelRef.current
    if (!sentinel || totalPages === 0) return
    const obs = new IntersectionObserver(entries => { if (entries[0].isIntersecting) loadMore() }, { rootMargin: '600px' })
    obs.observe(sentinel)
    return () => obs.disconnect()
  }, [loadMore, totalPages])

  // Live count updates
  useEffect(() => {
    const unsub = subscribeProgress((p: CrawlProgress) => { if (p.total_count) setTotalCount(p.total_count) })
    return unsub
  }, [])

  // Mobile hover preview management
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
        if (v === best && bestRatio > 0.2) { v.play().catch(() => {}); v.classList.add('playing') }
        else { v.pause(); v.classList.remove('playing') }
      })
    }
    if (window.innerWidth < 768) {
      tick()
      window.addEventListener('scroll', tick, { passive: true })
      window.addEventListener('resize', tick)
      previewRef.current = () => { window.removeEventListener('scroll', tick); window.removeEventListener('resize', tick) }
      return () => { window.removeEventListener('scroll', tick); window.removeEventListener('resize', tick) }
    }
  }, [showVideos])

  const label = q ? `Search: "${q}" (${showTotal || 0} results)` : uploader ? `Uploader: ${uploader}` : ''

  const sortHref = (s: string) => updateHref((params) => params.set('sort', s))
  const sourceHref = (src: string) => updateHref((params) => params.set('source', src))
  const categoryHref = (value: string) => updateHref((params) => {
    if (value === '') { params.delete('cat'); return }
    const nextCat = toggleCategoryParam(params.get('cat'), value)
    if (nextCat) params.set('cat', nextCat); else params.delete('cat')
  })

  const sorts: { label: string; value: string }[] = [
    { label: 'Recent', value: 'recent' }, { label: 'New', value: 'new' },
    { label: 'Popular', value: 'views' }, { label: 'Longest', value: 'duration' },
  ]

  return (
    <>
      {/* Search label or uploader */}
      {label && (
        <div className="px-3 py-2 text-xs text-muted md:px-6 border-b border-border/50">
          {label}
        </div>
      )}

      {/* Filters bar */}
      <div className="flex items-center gap-2 px-2.5 py-2 md:px-6 border-b border-border/50">
        <div className="hidden lg:block w-32">
          <span className="text-[11px] font-semibold text-muted/70 uppercase tracking-widest mb-1 block">Category</span>
          <FilterSelect options={categoryOptions} current={activeCategories[0] || ''} getHref={categoryHref} />
        </div>
        <div className="w-32">
          <span className="text-[11px] font-semibold text-muted/70 uppercase tracking-widest mb-1 block">Sort</span>
          <FilterSelect options={sorts} current={sort} getHref={sortHref} />
        </div>
        <div className="w-32">
          <span className="text-[11px] font-semibold text-muted/70 uppercase tracking-widest mb-1 block">Source</span>
          <FilterSelect options={SOURCES} current={sourceFilter} getHref={sourceHref} />
        </div>
        <span className="w-px h-6 bg-border mx-1" aria-hidden />
        {DENSITY_OPTIONS.map(d => (
          <button key={d.key} onClick={() => { setDensity(d.key); writeStored('kxxx_density', d.key) }}
            className="relative group px-2 py-1 rounded-md text-xs font-semibold transition-all duration-150"
            title={d.label}>
            <span className={`${density === d.key ? 'text-red' : 'text-muted hover:text-text'}`}>{d.icon}</span>
            {density === d.key && <span className="absolute -bottom-0.5 left-1/2 -translate-x-1/2 w-4 h-0.5 bg-red rounded-full" />}
          </button>
        ))}
      </div>

      {/* Tab bar */}
      {token && (
        <div className="flex items-center gap-2 px-2.5 md:px-6 py-2 border-b border-border/50">
          <button onClick={() => setActiveTab('browse')}
            className={`px-3 py-1.5 rounded-full text-xs font-semibold transition-all duration-200 ${
              activeTab === 'browse'
                ? 'bg-gradient-to-br from-red to-orange text-white shadow-[0_2px_12px_-2px_rgba(225,29,72,0.4)]'
                : 'text-muted hover:text-text hover:bg-white/5'
            }`}>
            Browse
          </button>
          <button onClick={() => setActiveTab('for-you')}
            className={`px-3 py-1.5 rounded-full text-xs font-semibold transition-all duration-200 ${
              activeTab === 'for-you'
                ? 'bg-gradient-to-br from-red to-orange text-white shadow-[0_2px_12px_-2px_rgba(225,29,72,0.4)]'
                : 'text-muted hover:text-text hover:bg-white/5'
            }`}>
            For You
          </button>
        </div>
      )}

      {/* Active filter chips */}
      {activeFilterCount > 0 && (
        <div className="px-2.5 py-2 md:px-6 animate-fade-in">
          <div className="flex items-center gap-2 overflow-x-auto whitespace-nowrap rounded-2xl border border-border/50 bg-card/50 px-2.5 py-2 text-xs backdrop-blur-sm">
            {activeCategories.map((category) => (
              <div key={category}
                className="inline-flex flex-shrink-0 items-center gap-1 rounded-full border border-red/20 bg-red/10 px-2.5 py-1 text-xs font-semibold text-red capitalize">
                <span>{category}</span>
                <button type="button" aria-label={`Remove ${category}`}
                  onClick={() => handleRemoveCategoryFilter(category)}
                  className="flex items-center justify-center rounded-full p-0.5 transition-colors hover:bg-red/20 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red/40">
                  <XIcon className="w-3 h-3" />
                </button>
              </div>
            ))}
            {activeSource && (
              <div className="inline-flex flex-shrink-0 items-center gap-1 rounded-full border border-red/20 bg-red/10 px-2.5 py-1 text-xs font-semibold text-red">
                <span>{activeSource.label}</span>
                <button type="button" aria-label={`Remove ${activeSource.label}`}
                  onClick={handleRemoveSourceFilter}
                  className="flex items-center justify-center rounded-full p-0.5 transition-colors hover:bg-red/20 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red/40">
                  <XIcon className="w-3 h-3" />
                </button>
              </div>
            )}
            {activeFilterCount >= 2 && (
              <button type="button" onClick={handleClearActiveFilters}
                className="inline-flex flex-shrink-0 items-center rounded-full border border-border/50 bg-white/[0.04] px-3 py-1.5 text-xs font-semibold text-text backdrop-blur transition-colors hover:bg-white/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red/40">
                Clear all
              </button>
            )}
          </div>
        </div>
      )}

      {/* Continue Watching */}
      {!isForYou && history.length > 0 && (
        <div className="px-2.5 md:px-6 animate-slide-up">
          <div className="mb-2 flex items-center justify-between gap-3">
            <div className="flex items-center gap-2">
              <h2 className="text-sm font-bold">Continue Watching</h2>
              {clearError && <span className="text-xs text-red" role="alert">{clearError}</span>}
            </div>
            <Dialog open={clearHistoryOpen} onOpenChange={(open) => { setClearHistoryOpen(open); if (open) setClearError(null) }}>
              <DialogTrigger
                render={
                  <button type="button"
                    className="rounded-full border border-border/50 bg-card/50 px-3 py-1.5 text-xs font-semibold text-muted backdrop-blur transition-colors hover:bg-white/10 hover:text-text focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red/40"
                  />
                }
              >
                Clear history
              </DialogTrigger>
              <DialogContent showCloseButton={false} className="border-border bg-bg/90 text-text backdrop-blur-xl">
                <DialogHeader>
                  <DialogTitle>Clear watch history?</DialogTitle>
                  <DialogDescription className="text-muted">This removes every video from your Continue Watching list. This can&apos;t be undone.</DialogDescription>
                </DialogHeader>
                <DialogFooter className="border-border bg-card/50">
                  <DialogClose
                    render={
                      <button type="button"
                        className="inline-flex items-center justify-center rounded-md border border-border bg-card px-3 py-2 text-sm font-semibold text-text transition-colors hover:bg-white/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red/40"
                      />
                    }
                  >
                    Cancel
                  </DialogClose>
                  <button type="button" onClick={handleClearHistoryConfirm} disabled={clearingHistory}
                    className="inline-flex items-center justify-center rounded-md bg-red px-3 py-2 text-sm font-semibold text-white transition-colors hover:bg-red/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red/40 disabled:cursor-not-allowed disabled:opacity-60">
                    {clearingHistory ? 'Clearing…' : 'Clear history'}
                  </button>
                </DialogFooter>
              </DialogContent>
            </Dialog>
          </div>
          <div className="overflow-x-auto flex gap-2.5 pb-2 snap-x snap-mandatory">
            {history.map(h => (
              <div key={h.id} className="relative flex-shrink-0 w-48 snap-start">
                <button type="button" aria-label="Remove from history"
                  onClick={(e) => { e.preventDefault(); e.stopPropagation(); removeFromHistory(h.id) }}
                  className="absolute top-1 right-1 z-20 flex items-center justify-center w-5 h-5 rounded-full bg-black/60 text-white transition-colors hover:bg-red/80">
                  <XIcon className="w-3 h-3" />
                </button>
                <VideoCard video={h} />
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Loading skeleton */}
      {showLoading && showVideos.length === 0 && (
        <div className={densityGrid}>
          {Array.from({ length: 12 }).map((_, i) => <VideoCardSkeleton key={i} index={i} />)}
        </div>
      )}

      {/* Empty state */}
      {!showLoading && showVideos.length === 0 && !browseError && (
        <EmptyState
          icon="M17 1l4 4-4 4M3 11l4-4 4 4M7 5V1h10M7 19l4 4 4-4M12 3v18"
          title="No videos to show"
          message={q ? `No results for "${q}".` : uploader ? `No videos for ${uploader}.` : activeFilterCount > 0 ? 'Try adjusting your filters.' : 'No videos yet. Crawl is working in the background.'}
          action={activeFilterCount > 0 ? { label: 'Clear filters', onClick: handleClearActiveFilters } : undefined}
        />
      )}

      {/* Error state */}
      {!showLoading && browseError && (
        <EmptyState
          icon="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0zM12 9v4M12 17h.01"
          title="Something went wrong"
          message={browseError}
          action={{ label: 'Try again', onClick: () => window.location.reload() }}
        />
      )}

      {/* Video grid */}
      <div ref={gridRef} className={densityGrid}>
        {showVideos.map(v => (
          <div key={v.id}>
            <VideoCard video={v} />
            {isForYou && 'reason' in v && typeof v.reason === 'string' && (
              <span className="text-[10px] text-red/80 mt-1 block px-1">{v.reason}</span>
            )}
          </div>
        ))}
      </div>

      {/* Loading more */}
      {loadingMore && (
        <div className="text-center py-6">
          <svg className="animate-spin h-5 w-5 mx-auto text-muted" viewBox="0 0 24 24" fill="none">
            <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="3" opacity="0.2" />
            <path d="M12 2a10 10 0 0 1 10 10" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
          </svg>
        </div>
      )}

      {loadMoreError && (
        <div className="text-center py-4">
          <span className="text-xs text-muted">{loadMoreError}</span>
        </div>
      )}

      {/* End of results */}
      {!isForYou && page >= totalPages && totalPages > 0 && (
        <div className="text-center pb-8">
          <span className="text-[11px] text-muted/60">{totalCount.toLocaleString()} total videos</span>
        </div>
      )}

      <div ref={sentinelRef} className="h-1" />
    </>
  )
}
