import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { useAuth } from '../lib/auth'
import AuthDialog from './AuthDialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from './ui/dropdown-menu'
import { Avatar, AvatarFallback } from './ui/avatar'
import BrandLogo from './BrandLogo'

interface Props {
  videoCount?: number
  progress?: string
  onMenuToggle?: () => void
}

export default function Header({ videoCount, progress, onMenuToggle }: Props) {
  const [q, setQ] = useState('')
  const [authOpen, setAuthOpen] = useState(false)
  const [randomLoading, setRandomLoading] = useState(false)
  const navigate = useNavigate()
  const [sp] = useSearchParams()
  const { user, logout } = useAuth()

  function submit(e: FormEvent) {
    e.preventDefault()
    if (q.trim()) navigate(`/search?q=${encodeURIComponent(q.trim())}`)
  }

  return (
    <>
      <header className="flex items-center gap-3 py-2.5
                        bg-bg/80 backdrop-blur-xl border-b border-white/5
                        shadow-[0_1px_0_rgba(255,255,255,0.03),0_8px_24px_-12px_rgba(0,0,0,0.6)]
                        sticky top-0 z-50 md:py-3">
        {onMenuToggle && (
          <button onClick={onMenuToggle}
                  className="lg:hidden flex-shrink-0 p-2 ml-3 text-muted hover:text-text transition-colors
                            min-w-[44px] min-h-[44px] flex items-center justify-center"
                  aria-label="Toggle menu">
            <svg width="22" height="22" viewBox="0 0 22 22" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
              <path d="M3 5h16M3 11h16M3 17h16"/>
            </svg>
          </button>
        )}
        <BrandLogo size="nav" className="ml-3 md:ml-6" />

        <form onSubmit={submit} className="flex-1 min-w-0 max-w-xl relative">
          <svg className="absolute left-3.5 top-1/2 -translate-y-1/2 text-muted/60 pointer-events-none"
               width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor"
               strokeWidth="2" strokeLinecap="round">
            <circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/>
          </svg>
          <input
            value={q}
            onChange={e => setQ(e.target.value)}
            placeholder="Search videos..."
            className="w-full pl-9 pr-3 py-2 rounded-full border border-border bg-card/80
                      text-text text-sm outline-none
                      hover:border-border hover:bg-card
                      focus:border-orange/50 focus:ring-2 focus:ring-orange/15 focus:bg-card
                      transition-all duration-200 placeholder:text-muted/50"
          />
        </form>

        <button
          onClick={async () => {
            if (randomLoading) return
            setRandomLoading(true)
            try {
              const params = new URLSearchParams()
              const src = sp.get('source')
              const cat = sp.get('cat')
              if (src) params.set('source', src)
              if (cat) params.set('cat', cat)
              const qs = params.toString()
              const res = await fetch(`/api/random${qs ? '?' + qs : ''}`)
              if (!res.ok) return
              const data = await res.json()
              if (data.id) navigate(`/play/${data.id}`)
            } finally {
              setRandomLoading(false)
            }
          }}
          className="flex-shrink-0 p-2 rounded-full border border-border text-muted hover:text-text hover:border-red/40 transition-colors"
          aria-label="Random video"
          title="Random video"
        >
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <polyline points="16 3 21 3 21 8"/>
            <line x1="4" y1="20" x2="21" y2="3"/>
            <polyline points="21 16 21 21 16 21"/>
            <line x1="15" y1="15" x2="21" y2="21"/>
            <line x1="4" y1="4" x2="9" y2="9"/>
          </svg>
        </button>

        <div className="text-xs text-muted flex-shrink-0 hidden sm:flex items-center gap-2">
          {videoCount != null && (
            <Link to="/status" className="hover:text-orange transition-colors cursor-pointer">
              {videoCount.toLocaleString()} videos
            </Link>
          )}
          {progress && (
            <Link to="/status" className="text-orange font-semibold hover:text-orange/80 transition-colors animate-pulse cursor-pointer">
              {progress}
            </Link>
          )}
        </div>

        {user ? (
          <DropdownMenu>
            <DropdownMenuTrigger className="flex-shrink-0 outline-none mr-3 md:mr-6">
              <Avatar className="h-8 w-8 ring-2 ring-border hover:ring-orange transition-all cursor-pointer">
                <AvatarFallback className="bg-orange/20 text-orange text-xs font-bold">
                  {user.username.slice(0, 2).toUpperCase()}
                </AvatarFallback>
              </Avatar>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="bg-card border-border text-text w-44">
              <div className="px-3 py-2 text-xs text-muted border-b border-border">
                Signed in as <span className="text-text font-semibold">{user.username}</span>
              </div>
              <DropdownMenuItem
                onClick={() => navigate('/profile')}
                className="text-sm cursor-pointer"
              >
                Profile
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() => navigate(`/wall/${encodeURIComponent(user.username)}`)}
                className="text-sm cursor-pointer"
              >
                My Wall
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() => navigate('/playlists')}
                className="text-sm cursor-pointer"
              >
                Playlists
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() => navigate('/favorites')}
                className="text-sm cursor-pointer"
              >
                Favorites
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() => navigate('/changelog')}
                className="text-sm cursor-pointer"
              >
                Changelog
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={logout}
                className="text-sm text-muted cursor-pointer hover:text-red"
              >
                Sign out
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        ) : (
          <button
            onClick={() => setAuthOpen(true)}
            className="flex-shrink-0 text-xs font-semibold px-3 py-1.5 mr-3 md:mr-6 rounded-full
                       bg-orange text-black hover:bg-orange/90 transition-colors"
          >
            Sign in
          </button>
        )}
      </header>

      <AuthDialog open={authOpen} onClose={() => setAuthOpen(false)} />
    </>
  )
}
