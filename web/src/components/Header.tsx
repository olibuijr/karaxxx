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
      <header className="flex items-center gap-3 px-3 py-2.5
                        bg-bg/95 backdrop-blur-md border-b border-border
                        sticky top-0 z-50 md:px-6 md:py-3">
        {onMenuToggle && (
          <button onClick={onMenuToggle}
                  className="lg:hidden flex-shrink-0 p-2 -ml-1 text-muted hover:text-text transition-colors
                            min-w-[44px] min-h-[44px] flex items-center justify-center"
                  aria-label="Toggle menu">
            <svg width="22" height="22" viewBox="0 0 22 22" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
              <path d="M3 5h16M3 11h16M3 17h16"/>
            </svg>
          </button>
        )}
        <Link to="/" className="font-extrabold text-lg tracking-tight flex-shrink-0 md:text-2xl">
          <span className="text-red">Kara</span>
          <span className="text-orange">XXX</span>
        </Link>

        <form onSubmit={submit} className="flex-1 min-w-0 max-w-full">
          <input
            value={q}
            onChange={e => setQ(e.target.value)}
            placeholder="Search..."
            className="w-full px-3 py-2 rounded-full border border-border bg-card
                      text-text text-sm outline-none
                      focus:border-red focus:ring-2 focus:ring-red/20
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
            <DropdownMenuTrigger className="flex-shrink-0 outline-none">
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
            className="flex-shrink-0 text-xs font-semibold px-3 py-1.5 rounded-full
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
