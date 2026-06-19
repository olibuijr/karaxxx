import { useEffect, useId, useMemo, useRef, useState, type KeyboardEvent } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { fetchSearchSuggest, formatDuration } from '../api'
import type { SearchSuggestResponse, Video } from '../types'
import { toggleCategoryParam } from '../lib/categories'

type SearchDropdownVariant = 'desktop' | 'mobile'

interface SearchDropdownProps {
  variant?: SearchDropdownVariant
  onNavigate?: () => void
}

type SearchAction =
  | { type: 'category'; category: { name: string; count: number } }
  | { type: 'video'; video: Video }
  | { type: 'footer'; term: string }

const EMPTY_RESULTS: SearchSuggestResponse = { categories: [], videos: [] }

function getVideoThumb(video: Video): string {
  const thumb = video.thumb_uuid
  if (!thumb) return ''
  if (video.source && video.source !== 'xnxx') return `/media?url=${encodeURIComponent(thumb)}`
  if (/^https?:\/\//i.test(thumb)) return `/media?url=${encodeURIComponent(thumb)}`
  return `/thumb/${thumb}/0/mozaique_listing.jpg`
}

function formatSourceLabel(source: string): string {
  if (source === 'xnxx') return 'XNXX'
  if (source === 'xvideos') return 'xVideos'
  if (source === 'xhamster') return 'xHamster'
  if (source === 'eporner') return 'EPorner'
  if (source === 'tnaflix') return 'TNAFlix'
  if (source === 'drtuber') return 'DrTuber'
  return source
}

export default function SearchDropdown({
  variant = 'desktop',
  onNavigate,
}: SearchDropdownProps) {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<SearchSuggestResponse>(EMPTY_RESULTS)
  const [loading, setLoading] = useState(false)
  const [focused, setFocused] = useState(false)
  const [highlightedIndex, setHighlightedIndex] = useState(-1)
  const ref = useRef<HTMLDivElement>(null)
  const navigate = useNavigate()
  const location = useLocation()
  const listboxId = useId()
  const trimmedQuery = query.trim()
  const showFooter = trimmedQuery.length > 0
  const hasMatches = results.categories.length > 0 || results.videos.length > 0
  const showEmptyState = trimmedQuery.length >= 2 && !loading && !hasMatches
  const open = focused && (showFooter || hasMatches || showEmptyState || (loading && trimmedQuery.length >= 2))

  const inputClasses = variant === 'mobile'
    ? 'w-full pl-8 pr-3 py-1.5 rounded-md border border-border bg-bg/80 text-text text-sm outline-none focus:border-orange/50 focus:ring-2 focus:ring-orange/15 transition-all duration-200 placeholder:text-muted/50'
    : 'w-full pl-9 pr-3 py-2 rounded-full border border-border bg-card/80 text-text text-sm outline-none hover:border-border hover:bg-card focus:border-orange/50 focus:ring-2 focus:ring-orange/15 focus:bg-card transition-all duration-200 placeholder:text-muted/50'
  const iconClasses = variant === 'mobile'
    ? 'absolute left-3 top-1/2 -translate-y-1/2 text-muted/60 pointer-events-none'
    : 'absolute left-3.5 top-1/2 -translate-y-1/2 text-muted/60 pointer-events-none'
  const iconSize = variant === 'mobile' ? 14 : 15

  const actions = useMemo<SearchAction[]>(() => {
    const next: SearchAction[] = [
      ...results.categories.map((category) => ({ type: 'category' as const, category })),
      ...results.videos.map((video) => ({ type: 'video' as const, video })),
    ]
    if (showFooter) {
      next.push({ type: 'footer', term: trimmedQuery })
    }
    return next
  }, [results.categories, results.videos, showFooter, trimmedQuery])

  useEffect(() => {
    const params = new URLSearchParams(location.search)
    const nextQuery = params.get('q') ?? ''
    setQuery(nextQuery)
    setHighlightedIndex(-1)
  }, [location.search])

  useEffect(() => {
    function handleClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setFocused(false)
        setHighlightedIndex(-1)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [])

  useEffect(() => {
    setHighlightedIndex(-1)
    if (trimmedQuery.length < 2) {
      setLoading(false)
      setResults(EMPTY_RESULTS)
      return
    }

    const controller = new AbortController()
    setLoading(true)
    setResults(EMPTY_RESULTS)
    const timeoutId = window.setTimeout(() => {
      void fetchSearchSuggest(trimmedQuery, controller.signal)
        .then((data) => {
          setResults(data)
        })
        .catch((error: unknown) => {
          if (error instanceof DOMException && error.name === 'AbortError') return
          setResults(EMPTY_RESULTS)
        })
        .finally(() => {
          if (!controller.signal.aborted) {
            setLoading(false)
          }
        })
    }, 180)

    return () => {
      window.clearTimeout(timeoutId)
      controller.abort()
    }
  }, [trimmedQuery])

  function closeDropdown() {
    setFocused(false)
    setHighlightedIndex(-1)
    onNavigate?.()
  }

  function openAllResults() {
    if (!trimmedQuery) return
    navigate(`/search?q=${encodeURIComponent(trimmedQuery)}`, { viewTransition: true })
    closeDropdown()
  }

  function activateAction(action: SearchAction) {
    if (action.type === 'category') {
      const params = new URLSearchParams(location.search)
      const nextCategories = toggleCategoryParam(params.get('cat'), action.category.name)
      if (nextCategories) params.set('cat', nextCategories)
      else params.delete('cat')
      const qs = params.toString()
      navigate(qs ? `/?${qs}` : '/', { viewTransition: true })
      closeDropdown()
      return
    }
    if (action.type === 'video') {
      navigate(`/play/${action.video.id}`, { viewTransition: true })
      closeDropdown()
      return
    }
    openAllResults()
  }

  function handleKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === 'ArrowDown') {
      if (actions.length === 0) return
      event.preventDefault()
      setHighlightedIndex((prev) => (prev < actions.length - 1 ? prev + 1 : 0))
      return
    }

    if (event.key === 'ArrowUp') {
      if (actions.length === 0) return
      event.preventDefault()
      setHighlightedIndex((prev) => (prev > 0 ? prev - 1 : actions.length - 1))
      return
    }

    if (event.key === 'Enter') {
      if (!trimmedQuery) return
      event.preventDefault()
      if (highlightedIndex >= 0 && highlightedIndex < actions.length) {
        activateAction(actions[highlightedIndex])
      } else {
        openAllResults()
      }
      return
    }

    if (event.key === 'Escape') {
      event.preventDefault()
      closeDropdown()
    }
  }

  return (
    <div ref={ref} className="relative w-full">
      <svg
        className={iconClasses}
        width={iconSize}
        height={iconSize}
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
      >
        <circle cx="11" cy="11" r="7" />
        <path d="m21 21-4.3-4.3" />
      </svg>
      <input
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        onFocus={() => setFocused(true)}
        onKeyDown={handleKeyDown}
        placeholder="Search videos..."
        role="combobox"
        aria-expanded={open}
        aria-controls={listboxId}
        aria-autocomplete="list"
        aria-activedescendant={open && highlightedIndex >= 0 ? `${listboxId}-option-${highlightedIndex}` : undefined}
        className={inputClasses}
      />

      {open && (
        <div
          id={listboxId}
          role="listbox"
          className="absolute left-0 right-0 top-full z-50 mt-2 overflow-hidden rounded-lg border border-white/10 bg-[#1b1b26]/95 backdrop-blur-xl shadow-[inset_0_1px_0_rgba(255,255,255,0.08),0_16px_48px_-16px_rgba(0,0,0,0.8)]"
        >
          <div className="max-h-[28rem] overflow-y-auto py-2">
            {results.categories.length > 0 && (
              <div className="px-2 pb-1">
                <div className="px-3 pb-1 text-[11px] font-semibold uppercase tracking-widest text-muted">
                  Categories
                </div>
                {results.categories.map((category, index) => {
                  const isActive = highlightedIndex === index
                  return (
                    <button
                      key={`category-${category.name}`}
                      type="button"
                      id={`${listboxId}-option-${index}`}
                      role="option"
                      aria-selected={isActive}
                      onMouseEnter={() => setHighlightedIndex(index)}
                      onClick={() => activateAction({ type: 'category', category })}
                      className={`flex w-full items-center justify-between rounded-md px-3 py-2 text-left text-sm font-medium transition-colors ${
                        isActive
                          ? 'bg-orange/10 text-orange'
                          : 'text-muted hover:bg-white/5 hover:text-text'
                      }`}
                    >
                      <span className="capitalize">{category.name}</span>
                      <span className="text-xs tabular-nums text-muted">{category.count}</span>
                    </button>
                  )
                })}
              </div>
            )}

            {results.videos.length > 0 && (
              <div className="px-2 pb-1">
                <div className="px-3 pb-1 text-[11px] font-semibold uppercase tracking-widest text-muted">
                  Videos
                </div>
                {results.videos.map((video, videoIndex) => {
                  const optionIndex = results.categories.length + videoIndex
                  const thumb = getVideoThumb(video)
                  const isActive = highlightedIndex === optionIndex
                  return (
                    <button
                      key={`video-${video.id}`}
                      type="button"
                      id={`${listboxId}-option-${optionIndex}`}
                      role="option"
                      aria-selected={isActive}
                      onMouseEnter={() => setHighlightedIndex(optionIndex)}
                      onClick={() => activateAction({ type: 'video', video })}
                      className={`flex w-full items-center gap-3 rounded-md px-3 py-2 text-left transition-colors ${
                        isActive
                          ? 'bg-orange/10 text-orange'
                          : 'text-muted hover:bg-white/5 hover:text-text'
                      }`}
                    >
                      <div className="h-11 w-16 overflow-hidden rounded-md bg-bg/80 flex-shrink-0">
                        {thumb ? (
                          <img
                            src={thumb}
                            alt={video.title}
                            className="h-full w-full object-cover"
                            loading="lazy"
                          />
                        ) : (
                          <div className="flex h-full w-full items-center justify-center text-[10px] text-muted">
                            No image
                          </div>
                        )}
                      </div>
                      <div className="min-w-0 flex-1">
                        <div className={`truncate text-sm font-medium ${isActive ? 'text-orange' : 'text-text'}`}>
                          {video.title}
                        </div>
                        <div className="truncate text-xs text-muted">
                          {formatSourceLabel(video.source)} · {formatDuration(video.duration)}
                        </div>
                      </div>
                    </button>
                  )
                })}
              </div>
            )}

            {loading && trimmedQuery.length >= 2 && !hasMatches && (
              <div className="px-5 py-2 text-sm text-muted">Searching...</div>
            )}

            {showEmptyState && (
              <div className="px-5 py-2 text-sm text-muted">No matches</div>
            )}

            {showFooter && (
              <div className="mt-1 border-t border-white/10 px-2 pt-2">
                <button
                  type="button"
                  id={`${listboxId}-option-${actions.length - 1}`}
                  role="option"
                  aria-selected={highlightedIndex === actions.length - 1}
                  onMouseEnter={() => setHighlightedIndex(actions.length - 1)}
                  onClick={openAllResults}
                  className={`flex w-full items-center rounded-md px-3 py-2 text-left text-sm font-medium transition-colors ${
                    highlightedIndex === actions.length - 1
                      ? 'bg-orange/10 text-orange'
                      : 'text-muted hover:bg-white/5 hover:text-text'
                  }`}
                >
                  Open all results for "{trimmedQuery}"
                </button>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
