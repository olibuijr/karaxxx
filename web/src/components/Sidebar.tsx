import { useEffect, useState, useCallback, useMemo } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { fetchCategories, fetchTags } from '../api'
import type { TagCount, Video } from '../types'
import { SOURCES } from '../types'
import { useAuth } from '../lib/auth'
import CategoryIcon from './CategoryIcon'
import FilterSelect from './FilterSelect'
import BrandLogo from './BrandLogo'
import SearchDropdown from './SearchDropdown'
import { parseCategories, toggleCategoryParam } from '../lib/categories'

type Sort = 'recent' | 'new' | 'views' | 'duration'

const SORT_VALUES: Sort[] = ['recent', 'new', 'views', 'duration']
const INITIAL_CATEGORY_COUNT = 24

function readStoredPreference(key: string): string | null {
  if (typeof window === 'undefined') return null

  try {
    return window.localStorage.getItem(key)
  } catch {
    return null
  }
}

function getVideoThumb(video: Video): string {
  const thumb = video.thumb_uuid
  if (!thumb) return ''
  if (video.source && video.source !== 'xnxx') return `/media?url=${encodeURIComponent(thumb)}`
  if (/^https?:\/\//i.test(thumb)) return `/media?url=${encodeURIComponent(thumb)}`
  return `/thumb/${thumb}/0/mozaique_listing.jpg`
}

export default function Sidebar({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [cats, setCats] = useState<string[]>([])
  const [pinnedCats, setPinnedCats] = useState<string[]>([])
  const [catsOpen, setCatsOpen] = useState(() => localStorage.getItem('kxxx_cats_open') !== 'false')
  const [catsExpanded, setCatsExpanded] = useState(false)
  const [tags, setTags] = useState<TagCount[]>([])
  const [tagsExpanded, setTagsExpanded] = useState(false)
  const [sp] = useSearchParams()
  const curCats = parseCategories(sp.get('cat'))
  const hasSortParam = sp.has('sort')
  const sortParam = sp.get('sort')
  const storedSort = readStoredPreference('kxxx_sort')
  const curSort = hasSortParam
    ? (SORT_VALUES.includes((sortParam ?? 'recent') as Sort) ? (sortParam as Sort) : 'recent')
    : (SORT_VALUES.includes((storedSort ?? 'recent') as Sort) ? (storedSort as Sort) : 'recent')
  const hasSourceParam = sp.has('source')
  const sourceParam = sp.get('source')
  const storedSource = readStoredPreference('kxxx_source') ?? ''
  const curSource = hasSourceParam
    ? (SOURCES.some((source) => source.value === (sourceParam ?? '')) ? (sourceParam ?? '') : '')
    : (SOURCES.some((source) => source.value === storedSource) ? storedSource : '')
  const { token } = useAuth()

  interface SuggestionGroup {
    category: string
    reason: string
    videos: Video[]
  }

  const [suggestions, setSuggestions] = useState<SuggestionGroup[]>([])
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set())

  useEffect(() => {
    fetchCategories(80).then(setCats).catch(() => {})
    fetchTags(24).then(setTags).catch(() => {})
  }, [])

  useEffect(() => {
    if (!token) { setPinnedCats([]); return }
    fetch('/api/fav/categories', { headers: { Authorization: `Bearer ${token}` } })
      .then((response) => {
        if (!response.ok) return [] as string[]
        return response.json() as Promise<string[]>
      })
      .then(setPinnedCats)
      .catch(() => setPinnedCats([]))
  }, [token])

  useEffect(() => {
    if (!token) { setSuggestions([]); return }
    fetch('/api/suggestions', { headers: { Authorization: `Bearer ${token}` } })
      .then((response) => {
        if (!response.ok) return [] as SuggestionGroup[]
        return response.json() as Promise<SuggestionGroup[]>
      })
      .then(setSuggestions)
      .catch(() => setSuggestions([]))
  }, [token])

  useEffect(() => {
    localStorage.setItem('kxxx_cats_open', String(catsOpen))
  }, [catsOpen])

  const toggleGroup = (cat: string) => {
    setExpandedGroups(prev => {
      const next = new Set(prev)
      if (next.has(cat)) next.delete(cat); else next.add(cat)
      return next
    })
  }

  const togglePin = useCallback(async (cat: string) => {
    if (!token) return
    const isPinned = pinnedCats.includes(cat)
    const method = isPinned ? 'DELETE' : 'POST'
    const res = await fetch(`/api/fav/category?cat=${encodeURIComponent(cat)}`, {
      method,
      headers: { Authorization: `Bearer ${token}` },
    })
    if (res.ok) {
      setPinnedCats(prev => isPinned ? prev.filter(c => c !== cat) : [...prev, cat])
    }
  }, [token, pinnedCats])

  const makeHref = (options?: {
    category?: string | null
    sort?: Sort
    source?: string | null
  }) => {
    const p = new URLSearchParams(sp)

    if (options?.category !== undefined) {
      if (options.category === null) {
        p.delete('cat')
      } else {
        const nextCategories = toggleCategoryParam(p.get('cat'), options.category)
        if (nextCategories) p.set('cat', nextCategories)
        else p.delete('cat')
      }
    }

    if (options?.sort !== undefined) {
      if (options.sort === 'recent') {
        if (!hasSortParam && curSort !== 'recent') p.set('sort', 'recent')
        else p.delete('sort')
      } else {
        p.set('sort', options.sort)
      }
    }

    if (options?.source !== undefined) {
      if (!options.source) {
        if (!hasSourceParam && curSource) p.set('source', '')
        else p.delete('source')
      } else {
        p.set('source', options.source)
      }
    }

    const qs = p.toString()
    return qs ? `/?${qs}` : '/'
  }

  const sorts: { label: string; value: Sort }[] = [
    { label: 'Recent', value: 'recent' },
    { label: 'New', value: 'new' },
    { label: 'Popular', value: 'views' },
    { label: 'Longest', value: 'duration' },
  ]

  const pinnedSet = useMemo(() => new Set(pinnedCats), [pinnedCats])
  const unpinnedCats = useMemo(() => cats.filter(c => !pinnedSet.has(c)), [cats, pinnedSet])
  const visibleCats = catsExpanded ? unpinnedCats : unpinnedCats.slice(0, INITIAL_CATEGORY_COUNT)
  const hiddenCatCount = Math.max(0, unpinnedCats.length - visibleCats.length)

  const catLink = (c: string, isPinned: boolean) => (
    <Link viewTransition
      key={c}
      to={makeHref({ category: c })}
      onClick={onClose}
      className={`group px-3 py-1.5 rounded-md text-sm font-medium transition-colors capitalize flex items-center justify-between
                  ${curCats.includes(c)
                    ? 'bg-white/8 text-text'
                    : 'text-muted hover:text-text hover:bg-white/5'
                  }`}
    >
      <span className="flex min-w-0 items-center gap-2">
        <CategoryIcon category={c} className="h-3.5 w-3.5 flex-shrink-0 text-orange/80" />
        <span className="truncate">{c}</span>
      </span>
      {token && (
        <button
          onClick={(e: React.MouseEvent) => { e.preventDefault(); e.stopPropagation(); togglePin(c) }}
          className={`ml-1 flex-shrink-0 w-4 h-4 rounded transition-all
                      ${isPinned
                        ? 'text-orange opacity-100'
                        : 'text-muted max-lg:opacity-100 lg:opacity-0 lg:group-hover:opacity-100 focus-visible:opacity-100 hover:text-orange'
                      }`}
          title={isPinned ? 'Unpin' : 'Pin to top'}
          aria-label={isPinned ? `Unpin ${c}` : `Pin ${c} to top`}
        >
          <svg viewBox="0 0 24 24" fill="currentColor" className="w-full h-full">
            <path d="M16 12V4h1V2H7v2h1v8l-2 2v2h5.2v6h1.6v-6H18v-2l-2-2z"/>
          </svg>
        </button>
      )}
    </Link>
  )

  const sidebar = (
    <aside className="w-56 flex-shrink-0 h-[calc(100vh-3.5rem)] overflow-y-auto
                       bg-white/[0.04] backdrop-blur-2xl border-r border-white/10 flex flex-col
                       shadow-[inset_0_1px_0_rgba(255,255,255,0.05)]
                       scrollbar-thin">
      {/* Branding */}
      <div className="hidden lg:block px-4 pt-4 pb-3 border-b border-border mx-3 mb-2">
        <BrandLogo size="sidebar" showTagline />
      </div>

      {/* Search — mobile only */}
      <div className="px-3 pb-2 lg:hidden">
        <SearchDropdown variant="mobile" onNavigate={onClose} />
      </div>

      {/* Sort by */}
      <div className="p-3 pb-2">
        <h2 className="text-[11px] font-semibold text-muted uppercase tracking-widest mb-2 px-1">
          Sort by
        </h2>
        <FilterSelect
          options={sorts}
          current={curSort}
          getHref={v => makeHref({ sort: v as Sort })}
          onOptionClick={() => onClose()}
        />
      </div>

      <div className="mx-3 border-t border-border" />

      {/* Source filter */}
      <div className="p-3 pb-2">
        <h2 className="text-[11px] font-semibold text-muted uppercase tracking-widest mb-2 px-1">
          Source
        </h2>
        <FilterSelect
          options={SOURCES}
          current={curSource}
          getHref={v => makeHref({ source: v })}
          onOptionClick={() => onClose()}
        />
      </div>

      <div className="mx-3 border-t border-border" />

      {/* Pinned categories */}
      {pinnedCats.length > 0 && (
        <div className="p-3 pb-2">
          <h2 className="text-[11px] font-semibold text-orange uppercase tracking-widest mb-2 px-1 flex items-center gap-1">
            <svg viewBox="0 0 24 24" fill="currentColor" className="w-3 h-3">
              <path d="M16 12V4h1V2H7v2h1v8l-2 2v2h5.2v6h1.6v-6H18v-2l-2-2z"/>
            </svg>
            Pinned
          </h2>
          <div className="flex flex-col gap-0.5">
            {pinnedCats.map(c => catLink(c, true))}
          </div>
        </div>
      )}

      {pinnedCats.length > 0 && <div className="mx-3 border-t border-border" />}

      {/* Suggested */}
      {suggestions.length > 0 && (
        <div className="p-3 pb-2">
          <h2 className="text-[11px] font-semibold text-orange uppercase tracking-widest mb-2 px-1">
            Suggested
          </h2>
          <div className="flex flex-col gap-2">
            {suggestions.map(group => {
              const isOpen = expandedGroups.has(group.category)
              return (
                <div key={group.category}>
                  <button
                    onClick={() => toggleGroup(group.category)}
                    className="flex items-center justify-between w-full px-2 py-1 rounded-md text-xs font-medium text-muted hover:text-text hover:bg-white/5 transition-colors capitalize"
                  >
                    <span>{group.category}</span>
                    <svg
                      className={`w-3 h-3 transition-transform ${isOpen ? 'rotate-180' : ''}`}
                      viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"
                    >
                      <polyline points="6 9 12 15 18 9" />
                    </svg>
                  </button>
                  {isOpen && (
                    <div className="mt-1 space-y-1">
                      {group.videos.slice(0, 3).map(v => (
                        <Link viewTransition
                          key={v.id}
                          to={`/play/${v.id}`}
                          className="flex items-center gap-2 px-2 py-1 rounded-md hover:bg-white/5 transition-colors"
                        >
                          <div className="w-10 h-7 rounded bg-bg flex-shrink-0 overflow-hidden">
                            {v.thumb_uuid && (
                              <img
                                src={getVideoThumb(v)}
                                alt={v.title}
                                className="w-full h-full object-cover"
                                loading="lazy"
                              />
                            )}
                          </div>
                          <span className="text-[11px] text-muted leading-tight line-clamp-2 flex-1 min-w-0">
                            {v.title}
                          </span>
                        </Link>
                      ))}
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        </div>
      )}

      {suggestions.length > 0 && <div className="mx-3 border-t border-border" />}

      {/* Categories */}
      <div className="p-3 pt-2">
        <button
          onClick={() => setCatsOpen(o => !o)}
          className="w-full flex items-center justify-between text-[11px] font-semibold text-muted uppercase tracking-widest mb-2 px-1 hover:text-text transition-colors"
          aria-label={catsOpen ? 'Collapse categories' : 'Expand categories'}
        >
          <span>Categories</span>
          <span className="ml-auto mr-2 rounded-full bg-white/5 px-1.5 py-0.5 text-[10px] text-muted/80">
            {unpinnedCats.length}
          </span>
          <svg className={`w-3 h-3 text-muted transition-transform ${catsOpen ? 'rotate-180' : ''}`}
            viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <polyline points="6 9 12 15 18 9" />
          </svg>
        </button>
        {catsOpen && (
          <div className="flex flex-col gap-0.5">
            <Link viewTransition
              to={makeHref({ category: null })}
              onClick={onClose}
              className={`px-3 py-1.5 rounded-md text-sm font-medium transition-colors
                          ${curCats.length === 0
                            ? 'bg-white/8 text-text'
                            : 'text-muted hover:text-text hover:bg-white/5'
                          }`}
            >
              <span className="flex items-center gap-2">
                <CategoryIcon className="h-3.5 w-3.5 text-orange/80" />
                All videos
              </span>
            </Link>
            {pinnedCats.filter(c => cats.includes(c)).length > 0 && unpinnedCats.length === 0 && (
              <span className="px-3 py-1 text-[11px] text-muted">—</span>
            )}
            {visibleCats.map(c => catLink(c, false))}
            {hiddenCatCount > 0 && (
              <button
                onClick={() => setCatsExpanded(true)}
                className="mt-1 px-3 py-1.5 rounded-md text-left text-xs font-medium text-muted hover:text-text hover:bg-white/5 transition-colors"
              >
                Show {hiddenCatCount} more
              </button>
            )}
            {catsExpanded && unpinnedCats.length > INITIAL_CATEGORY_COUNT && (
              <button
                onClick={() => setCatsExpanded(false)}
                className="mt-1 px-3 py-1.5 rounded-md text-left text-xs font-medium text-muted hover:text-text hover:bg-white/5 transition-colors"
              >
                Show fewer categories
              </button>
            )}
          </div>
        )}
      </div>

      {/* Tags */}
      {tags.length > 0 && (
        <div className="p-3 pt-2 border-t border-border mx-3">
          <h2 className="text-[11px] font-semibold text-muted uppercase tracking-widest mb-2 px-1">
            Tags
          </h2>
          <div className="flex flex-wrap gap-1.5" style={{ maxHeight: tagsExpanded ? 'none' : '200px', overflow: 'hidden' }}>
            {tags.map(t => (
              <Link viewTransition
                key={t.name}
                to={`/?q=${encodeURIComponent(t.name)}`}
                onClick={onClose}
                className="px-2 py-1 rounded-full text-[10px] font-medium capitalize
                           bg-white/5 text-muted hover:text-text hover:bg-white/10 transition-colors"
                style={{ fontSize: `${Math.max(9, Math.min(13, 9 + t.count * 0.05))}px` }}
              >
                {t.name}
              </Link>
            ))}
          </div>
          {tags.length > 10 && (
            <button
              onClick={() => setTagsExpanded(!tagsExpanded)}
              className="px-3 py-1.5 rounded-md text-xs text-muted hover:text-text
                         transition-colors text-left mt-1"
            >
              {tagsExpanded ? 'Show less' : `Show all (${tags.length})`}
            </button>
          )}
        </div>
      )}
    </aside>
  )

  return (
    <>
      <div className="hidden lg:block">{sidebar}</div>
      {open && (
        <div className="fixed inset-0 top-14 z-40 lg:hidden">
          <div className="absolute inset-0 bg-bg/60 backdrop-blur-sm" onClick={onClose} />
          <div className="absolute left-0 top-0 h-full animate-[slideRight_200ms_ease-out]">
            {sidebar}
          </div>
        </div>
      )}
    </>
  )
}
