import { useState, type FormEvent } from 'react'
import { Button } from '../components/ui/button'
import { Input } from '../components/ui/input'
import { Label } from '../components/ui/label'
import { useAuth } from '../lib/auth'
import BrandLogo from '../components/BrandLogo'

const sourceUrl = 'https://github.com/olibuijr/karaxxx'

const benefits = [
  { icon: 'M12 2C6.5 2 2 6.5 2 12s4.5 10 10 10 10-4.5 10-10S17.5 2 12 2Zm-1 15v-6h2v6h-2Zm0-8V7h2v2h-2Z', text: 'Zero ads, zero tracking scripts loaded' },
  { icon: 'M4 16V4h16v12H4Zm0 4h16', text: 'Direct MP4 streams from the source' },
  { icon: 'M5 12h14M12 5v14', text: 'Favorites, playlists, and watch history' },
]

export default function Setup() {
  const { login, register } = useAuth()
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [inviteKey, setInviteKey] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setBusy(true)
    const result = mode === 'login'
      ? await login(username, password)
      : await register(username, password, inviteKey)
    setBusy(false)
    if (!result.ok) {
      setError(result.error || 'Access failed')
    }
  }

  return (
    <main className="min-h-dvh bg-bg text-text flex items-center justify-center px-4 py-8">
      <div className="w-full max-w-[960px] grid gap-8 md:grid-cols-[1fr_400px] md:items-center md:gap-12">

        {/* Left — Brand + Value Prop */}
        <section className="space-y-6 text-center md:text-left">
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
                  className="w-5 h-5 flex-shrink-0 mt-0.5 text-orange/70">
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
              <p>No third-party tracking is used. Account details and library actions stay in this private server database so sign-in, favorites, playlists, and watch progress work.</p>
              <p>Aggregate watches, reactions, and quality signals are anonymous and only intended for quality improvements. Comments use a generated anonymous name by default, and you can change that on your profile page.</p>
              <p>
                Source code:{' '}
                <a href={sourceUrl} target="_blank" rel="noreferrer" className="font-semibold text-orange hover:underline">
                  {sourceUrl}
                </a>
              </p>
            </div>
          </details>
        </section>

        {/* Right — Auth Card */}
        <section className="rounded-xl border border-white/10 bg-card p-6 shadow-[0_24px_80px_-42px_rgba(0,0,0,1)]">
          <div className="mb-5 grid grid-cols-2 rounded-lg border border-border bg-bg p-1">
            <button
              type="button"
              onClick={() => { setMode('login'); setError('') }}
              className={`rounded-md px-3 py-2 text-sm font-semibold transition-colors ${mode === 'login' ? 'bg-orange text-black shadow-sm' : 'text-muted hover:text-text'}`}
            >
              Sign in
            </button>
            <button
              type="button"
              onClick={() => { setMode('register'); setError('') }}
              className={`rounded-md px-3 py-2 text-sm font-semibold transition-colors ${mode === 'register' ? 'bg-orange text-black shadow-sm' : 'text-muted hover:text-text'}`}
            >
              Use invite
            </button>
          </div>

          <form onSubmit={submit} className="space-y-4">
            {mode === 'register' && (
              <div className="space-y-2">
                <Label htmlFor="setup-invite" className="text-sm text-muted">Invite key</Label>
                <Input
                  id="setup-invite"
                  value={inviteKey}
                  onChange={e => setInviteKey(e.target.value)}
                  className="bg-bg border-border text-text h-10"
                  autoComplete="one-time-code"
                  required
                />
              </div>
            )}

            <div className="space-y-2">
              <Label htmlFor="setup-username" className="text-sm text-muted">Username</Label>
              <Input
                id="setup-username"
                value={username}
                onChange={e => setUsername(e.target.value)}
                className="bg-bg border-border text-text h-10"
                autoComplete="username"
                required
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="setup-password" className="text-sm text-muted">Password</Label>
              <Input
                id="setup-password"
                type="password"
                value={password}
                onChange={e => setPassword(e.target.value)}
                className="bg-bg border-border text-text h-10"
                autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
                minLength={4}
                required
              />
            </div>

            {error && (
              <p className="rounded-md border border-red/25 bg-red/10 px-3 py-2 text-xs text-red">
                {error}
              </p>
            )}

            <Button
              type="submit"
              disabled={busy}
              className="w-full h-10 bg-orange font-bold text-black hover:bg-orange/90 text-sm"
            >
              {busy ? 'Please wait...' : mode === 'login' ? 'Sign in' : 'Create account'}
            </Button>
          </form>
        </section>

      </div>
    </main>
  )
}
