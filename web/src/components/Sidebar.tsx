import { useEffect, useState, useCallback, type FormEvent } from 'react'
import { Link, useSearchParams, useNavigate } from 'react-router-dom'
import { fetchCategories, fetchTags } from '../api'
import type { TagCount, Video } from '../types'
import { SOURCES } from '../types'
import { useAuth } from '../lib/auth'
import CategoryIcon from './CategoryIcon'
import FilterSelect from './FilterSelect'
import BrandLogo from './BrandLogo'

type Sort = 'recent' | 'new' | 'views' | 'duration'

export default function Sidebar({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [cats, setCats] = useState<string[]>([])
  const [pinnedCats, setPinnedCats] = useState<string[]>([])
  const [catsOpen, setCatsOpen] = useState(() => localStorage.getItem('kxxx_cats_open') !== 'false')
  const [tags, setTags] = useState<TagCount[]>([])
  const [tagsExpanded, setTagsExpanded] = useState(false)
  const [searchQ, setSearchQ] = useState('')
  const navigate = useNavigate()
  const [sp] = useSearchParams()
  const curCat = sp.get('cat') || ''
  const curSort = (sp.get('sort') as Sort) || 'recent'
  const curSource = sp.get('source') || ''
  const { token } = useAuth()

  interface SuggestionGroup {
    category: string
    reason: string
    videos: Video[]
  }

  const [suggestions, setSuggestions] = useState<SuggestionGroup[]>([])
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set())

  useEffect(() => {
    fetchCategories().then(setCats).catch(() => {})
    fetchTags(50).then(setTags).catch(() => {})
  }, [])

  useEffect(() => {
    if (!token) { setPinnedCats([]); return }
    fetch('/api/fav/categories', { headers: { Authorization: `Bearer ${token}` } })
      .then(r => r.json())
      .then(setPinnedCats)
      .catch(() => {})
  }, [token])

  useEffect(() => {
    if (!token) { setSuggestions([]); return }
    fetch('/api/suggestions', { headers: { Authorization: `Bearer ${token}` } })
      .then(r => r.json())
      .then(setSuggestions)
      .catch(() => {})
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

  const makeHref = (cat?: string, s?: Sort, src?: string) => {
    const p = new URLSearchParams()
    if (cat) p.set('cat', cat)
    if (s && s !== 'recent') p.set('sort', s)
    if (src) p.set('source', src)
    if (!cat && curSort !== 'recent' && !s) p.set('sort', curSort)
    const qs = p.toString()
    return qs ? `/?${qs}` : '/'
  }

  const sorts: { label: string; value: Sort }[] = [
    { label: 'Recent', value: 'recent' },
    { label: 'New', value: 'new' },
    { label: 'Popular', value: 'views' },
    { label: 'Longest', value: 'duration' },
  ]

  const pinnedSet = new Set(pinnedCats)
  const unpinnedCats = cats.filter(c => !pinnedSet.has(c))

  const catLink = (c: string, isPinned: boolean) => (
    <Link viewTransition
      key={c}
      to={makeHref(c)}
      onClick={onClose}
      className={`group px-3 py-1.5 rounded-md text-sm font-medium transition-colors capitalize flex items-center justify-between
                  ${c === curCat
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
                       bg-card/50 backdrop-blur-sm border-r border-white/5 flex flex-col
                       scrollbar-thin">
      {/* Branding */}
      <div className="hidden lg:block px-4 pt-4 pb-3 border-b border-border mx-3 mb-2">
        <BrandLogo size="sidebar" showTagline />
      </div>

      {/* Search — mobile only */}
      <div className="px-3 pb-2 lg:hidden">
        <form
          onSubmit={(e: FormEvent) => { e.preventDefault(); if (searchQ.trim()) { navigate(`/search?q=${encodeURIComponent(searchQ.trim())}`); onClose() } }}
        >
          <div className="relative">
            <svg className="absolute left-3 top-1/2 -translate-y-1/2 text-muted/60 pointer-events-none"
                 width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor"
                 strokeWidth="2" strokeLinecap="round">
              <circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/>
            </svg>
            <input
              value={searchQ}
              onChange={e => setSearchQ(e.target.value)}
              placeholder="Search videos..."
              className="w-full pl-8 pr-3 py-1.5 rounded-md border border-border bg-bg/80
                         text-text text-sm outline-none
                         focus:border-orange/50 focus:ring-2 focus:ring-orange/15
                         transition-all duration-200 placeholder:text-muted/50"
            />
          </div>
        </form>
      </div>

      {/* Sort by */}
      <div className="p-3 pb-2">
        <h2 className="text-[11px] font-semibold text-muted uppercase tracking-widest mb-2 px-1">
          Sort by
        </h2>
        <FilterSelect
          options={sorts}
          current={curSort}
          getHref={v => makeHref(curCat || undefined, v as Sort)}
          onOptionClick={onClose}
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
          getHref={v => makeHref(curCat || undefined, undefined, v || undefined)}
          onOptionClick={onClose}
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
                                src={v.source && v.source !== 'xnxx' ? `/media?url=${encodeURIComponent(v.thumb_uuid)}` : `/thumb/${v.thumb_uuid}/0/xn_23_t.jpg`}
                                alt=""
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
        >
          Categories
          <svg className={`w-3 h-3 text-muted transition-transform ${catsOpen ? 'rotate-180' : ''}`}
            viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <polyline points="6 9 12 15 18 9" />
          </svg>
        </button>
        {catsOpen && (
          <div className="flex flex-col gap-0.5">
            <Link viewTransition
              to={makeHref()}
              onClick={onClose}
              className={`px-3 py-1.5 rounded-md text-sm font-medium transition-colors
                          ${!curCat
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
            {unpinnedCats.map(c => catLink(c, false))}
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
