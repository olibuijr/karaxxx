import { lazy, Suspense, useState, type FormEvent } from 'react'
import { Button } from '../components/ui/button'
import { Input } from '../components/ui/input'
import { Label } from '../components/ui/label'
import { useAuth } from '../lib/auth'
import { prefetchBrowse } from '../api'
import BrandLogo from '../components/BrandLogo'

const LiquidGlassBackground = lazy(() => import('../components/LiquidGlassBackground'))

const sourceUrl = 'https://github.com/olibuijr/karaxxx'

const benefits = [
  { icon: 'M12 2C6.5 2 2 6.5 2 12s4.5 10 10 10 10-4.5 10-10S17.5 2 12 2Zm-1 15v-6h2v6h-2Zm0-8V7h2v2h-2Z', text: 'Zero ads, zero tracking scripts loaded' },
  { icon: 'M4 16V4h16v12H4Zm0 4h16', text: 'Direct MP4 streams from the source' },
  { icon: 'M5 12h14M12 5v14', text: 'Favorites, playlists, and watch history' },
]

const glassInput =
  'h-10 bg-white/[0.07] text-text backdrop-blur-sm transition-all duration-200 ' +
  'border border-white/15 focus:bg-white/[0.10] focus-visible:border-red/50 ' +
  'focus-visible:ring-2 focus-visible:ring-red/20'

export default function Setup() {
  const { login, register } = useAuth()
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [inviteKey, setInviteKey] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const [showPassword, setShowPassword] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setBusy(true)
    prefetchBrowse()
    const result = mode === 'login'
      ? await login(username, password)
      : await register(username, password, inviteKey)
    setBusy(false)
    if (!result.ok) {
      setError(result.error || 'Access failed')
    }
  }

  return (
    <main className="relative isolate min-h-dvh overflow-hidden bg-bg text-text flex items-center justify-center px-4 py-8">
      <Suspense fallback={null}>
        <LiquidGlassBackground />
      </Suspense>

      <div className="relative z-10 w-full max-w-[960px] grid gap-8 md:grid-cols-[1fr_400px] md:items-center md:gap-12">

        {/* Left — Brand + Value Prop */}
        <section className="space-y-6 text-center md:text-left animate-fade-in">
          <div className="flex justify-center md:justify-start">
            <BrandLogo linked={false} showTagline size="hero" />
          </div>

          <p className="text-sm leading-relaxed text-muted max-w-md mx-auto md:mx-0 md:text-base">
            Private, invite-only access to a curated adult video browser.
            No ads, no analytics pixels, no third-party scripts.
          </p>

          <ul className="space-y-3 max-w-sm mx-auto md:mx-0">
            {benefits.map(b => (
              <li key={b.text} className="flex items-start gap-3 text-sm text-muted">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5"
                  className="w-5 h-5 flex-shrink-0 mt-0.5 text-red/70">
                  <path d={b.icon} />
                </svg>
                <span>{b.text}</span>
              </li>
            ))}
          </ul>

          <details className="group max-w-sm mx-auto md:mx-0">
            <summary className="text-xs text-muted/50 cursor-pointer hover:text-muted transition-colors list-none flex items-center gap-1">
              <svg className="w-3 h-3 transition-transform group-open:rotate-90" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <polyline points="9 18 15 12 9 6" />
              </svg>
              Privacy &amp; data
            </summary>
            <div className="mt-2 text-xs leading-relaxed text-muted/60 space-y-2">
              <p>No third-party tracking is used. Account details and library actions stay in this private server database.</p>
              <p>Aggregate watches, reactions, and quality signals are anonymous and only intended for quality improvements.</p>
              <p>Source code: <a href={sourceUrl} target="_blank" rel="noreferrer" className="font-semibold text-red hover:underline">{sourceUrl}</a></p>
            </div>
          </details>
        </section>

        {/* Right — Frosted glass auth card */}
        <section className="relative rounded-2xl border border-white/15 bg-white/[0.06] p-6 backdrop-blur-2xl
                           shadow-[inset_0_1px_0_rgba(255,255,255,0.14),0_24px_80px_-32px_rgba(0,0,0,0.9)]
                           animate-scale-in">
          {/* Specular top edge */}
          <div aria-hidden className="pointer-events-none absolute inset-x-0 top-0 h-px rounded-t-2xl bg-gradient-to-r from-transparent via-white/30 to-transparent" />
          {/* Faint inner sheen */}
          <div aria-hidden className="pointer-events-none absolute inset-0 rounded-2xl bg-gradient-to-b from-white/[0.05] via-transparent to-transparent" />

          <div className="relative">
            {/* Mode toggle */}
            <div className="mb-5 grid grid-cols-2 rounded-lg border border-white/15 bg-white/[0.06] p-1 backdrop-blur-sm">
              <button type="button" onClick={() => { setMode('login'); setError(''); setShowPassword(false) }}
                className={`rounded-md px-3 py-2 text-sm font-semibold transition-all duration-200 ${mode === 'login' ? 'bg-red text-white shadow-sm' : 'text-muted hover:text-text'}`}>
                Sign in
              </button>
              <button type="button" onClick={() => { setMode('register'); setError(''); setShowPassword(false) }}
                className={`rounded-md px-3 py-2 text-sm font-semibold transition-all duration-200 ${mode === 'register' ? 'bg-red text-white shadow-sm' : 'text-muted hover:text-text'}`}>
                Use invite
              </button>
            </div>

            <form onSubmit={submit} className="space-y-4">
              {mode === 'register' && (
                <div className="space-y-2 animate-fade-in">
                  <Label htmlFor="setup-invite" className="text-sm text-muted font-semibold">Invite key</Label>
                  <Input id="setup-invite" value={inviteKey} onChange={e => setInviteKey(e.target.value)}
                    className={glassInput} autoComplete="one-time-code" required placeholder="Enter invite key" />
                </div>
              )}

              <div className="space-y-2">
                <Label htmlFor="setup-username" className="text-sm text-muted font-semibold">Username</Label>
                <Input id="setup-username" value={username} onChange={e => setUsername(e.target.value)}
                  className={glassInput} autoComplete="username" required placeholder="Your username" />
              </div>

              <div className="space-y-2">
                <Label htmlFor="setup-password" className="text-sm text-muted font-semibold">Password</Label>
                <div className="relative">
                  <Input id="setup-password" type={showPassword ? 'text' : 'password'}
                    value={password} onChange={e => setPassword(e.target.value)}
                    className={`${glassInput} pr-10`}
                    autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
                    minLength={4} required placeholder="Your password" />
                  <button type="button" onClick={() => setShowPassword(s => !s)}
                    className="absolute right-2 top-1/2 -translate-y-1/2 text-muted hover:text-text transition-colors p-1"
                    aria-label={showPassword ? 'Hide password' : 'Show password'}
                    tabIndex={-1}>
                    {showPassword ? (
                      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                        <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94" />
                        <path d="M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19" />
                        <line x1="1" y1="1" x2="23" y2="23" /><path d="M14.12 14.12a3 3 0 1 1-4.24-4.24" />
                      </svg>
                    ) : (
                      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                        <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z" />
                        <circle cx="12" cy="12" r="3" />
                      </svg>
                    )}
                  </button>
                </div>
              </div>

              {/* Error state */}
              {error && (
                <div role="alert" className="rounded-md border border-red/25 bg-red/10 px-3 py-2.5 text-xs text-red backdrop-blur-sm flex items-start gap-2 animate-fade-in">
                  <svg className="w-4 h-4 flex-shrink-0 mt-px" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                    <circle cx="12" cy="12" r="10" /><line x1="12" y1="8" x2="12" y2="12" /><line x1="12" y1="16" x2="12.01" y2="16" />
                  </svg>
                  <span>{error}</span>
                </div>
              )}

              <Button type="submit" disabled={busy}
                className="w-full h-10 bg-red font-bold text-white hover:bg-red/90 text-sm
                           shadow-[0_8px_24px_-8px_rgba(225,29,72,0.6)]
                           transition-all duration-200
                           hover:shadow-[0_8px_32px_-6px_rgba(225,29,72,0.75)]
                           disabled:cursor-not-allowed disabled:opacity-60 disabled:shadow-none
                           active:translate-y-px">
                {busy ? (
                  <span className="flex items-center gap-2">
                    <svg className="animate-spin h-4 w-4" viewBox="0 0 24 24" fill="none">
                      <circle cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="3" opacity="0.2" />
                      <path d="M12 2a10 10 0 0 1 10 10" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
                    </svg>
                    {mode === 'login' ? 'Signing in…' : 'Creating account…'}
                  </span>
                ) : (mode === 'login' ? 'Sign in' : 'Create account')}
              </Button>
            </form>
          </div>
        </section>
      </div>
    </main>
  )
}
