import { useState, type FormEvent } from 'react'
import { Button } from '../components/ui/button'
import { Input } from '../components/ui/input'
import { Label } from '../components/ui/label'
import { useAuth } from '../lib/auth'
import BrandLogo from '../components/BrandLogo'

const sourceUrl = 'https://github.com/olibuijr/karaxxx'

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
    <main className="min-h-screen bg-bg text-text flex items-center justify-center px-4 py-8">
      <div className="w-full max-w-[920px] grid gap-6 md:grid-cols-[1fr_420px] md:items-center">
        <section className="space-y-5">
          <div>
            <BrandLogo linked={false} showTagline size="hero" />
            <p className="mt-3 max-w-xl text-sm leading-6 text-muted md:text-base">
              Private, invite-only access with user privacy in mind. No ads, analytics pixels, or third-party tracking scripts are loaded.
            </p>
          </div>

          <div className="max-w-xl rounded-lg border border-white/10 bg-card/70 p-4 text-sm leading-6 text-muted shadow-[0_20px_60px_-40px_rgba(0,0,0,0.9)]">
            <p>
              No third-party tracking is used. Account details and library actions stay in this private server database so sign-in, favorites, playlists, and watch progress work.
            </p>
            <p className="mt-3">
              Aggregate watches, reactions, and quality signals are anonymous and only intended for quality improvements. Comments use a generated anonymous name by default, and you can change that on your profile page.
            </p>
            <p className="mt-3">
              Source code:{' '}
              <a href={sourceUrl} target="_blank" rel="noreferrer" className="font-semibold text-orange hover:underline">
                {sourceUrl}
              </a>
            </p>
            <p className="mt-3">
              Not sure? Drop the GitHub link into an AI chat and ask the AI to explain what the app does and what data stays on the server before creating an account.
            </p>
          </div>
        </section>

        <section className="rounded-lg border border-white/10 bg-card p-5 shadow-[0_24px_80px_-42px_rgba(0,0,0,1)]">
          <div className="mb-5 grid grid-cols-2 rounded-md border border-border bg-bg p-1">
            <button
              type="button"
              onClick={() => { setMode('login'); setError('') }}
              className={`rounded px-3 py-2 text-sm font-semibold transition-colors ${mode === 'login' ? 'bg-orange text-black' : 'text-muted hover:text-text'}`}
            >
              Sign in
            </button>
            <button
              type="button"
              onClick={() => { setMode('register'); setError('') }}
              className={`rounded px-3 py-2 text-sm font-semibold transition-colors ${mode === 'register' ? 'bg-orange text-black' : 'text-muted hover:text-text'}`}
            >
              Use invite
            </button>
          </div>

          <form onSubmit={submit} className="space-y-4">
            {mode === 'register' && (
              <div className="space-y-2">
                <Label htmlFor="setup-invite" className="text-muted">Invite key</Label>
                <Input
                  id="setup-invite"
                  value={inviteKey}
                  onChange={e => setInviteKey(e.target.value)}
                  className="bg-bg border-border text-text"
                  autoComplete="one-time-code"
                  required
                />
              </div>
            )}

            <div className="space-y-2">
              <Label htmlFor="setup-username" className="text-muted">Username</Label>
              <Input
                id="setup-username"
                value={username}
                onChange={e => setUsername(e.target.value)}
                className="bg-bg border-border text-text"
                autoComplete="username"
                required
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="setup-password" className="text-muted">Password</Label>
              <Input
                id="setup-password"
                type="password"
                value={password}
                onChange={e => setPassword(e.target.value)}
                className="bg-bg border-border text-text"
                autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
                minLength={4}
                required
              />
            </div>

            {error && <p className="rounded-md border border-red/25 bg-red/10 px-3 py-2 text-xs text-red">{error}</p>}

            <Button type="submit" disabled={busy} className="w-full bg-orange font-bold text-black hover:bg-orange/90">
              {busy ? 'Please wait...' : mode === 'login' ? 'Sign in' : 'Create account'}
            </Button>
          </form>
        </section>
      </div>
    </main>
  )
}
