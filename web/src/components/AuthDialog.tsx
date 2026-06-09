import { useState, type FormEvent } from 'react'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from './ui/dialog'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { Label } from './ui/label'
import { useAuth } from '../lib/auth'

export default function AuthDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { login, register } = useAuth()
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (!username || !password) return
    setError('')
    setBusy(true)
    const ok = mode === 'login'
      ? await login(username, password)
      : await register(username, password)
    setBusy(false)
    if (ok) {
      onClose()
      setUsername('')
      setPassword('')
    } else {
      setError(mode === 'register' ? 'Username taken or password too short' : 'Invalid credentials')
    }
  }

  return (
    <Dialog open={open} onOpenChange={(o: boolean) => { if (!o) onClose() }}>
      <DialogContent className="bg-card border-border text-text sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="text-lg font-bold tracking-tight">
            {mode === 'login' ? 'Sign in' : 'Create account'}
          </DialogTitle>
        </DialogHeader>
        <form onSubmit={submit} className="space-y-4 pt-2">
          <div className="space-y-2">
            <Label htmlFor="username" className="text-muted">Username</Label>
            <Input
              id="username"
              value={username}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => setUsername(e.target.value)}
              className="bg-bg border-border text-text"
              autoComplete="username"
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="password" className="text-muted">Password</Label>
            <Input
              id="password"
              type="password"
              value={password}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => setPassword(e.target.value)}
              className="bg-bg border-border text-text"
              autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
              required
              minLength={4}
            />
          </div>
          {error && <p className="text-red text-xs">{error}</p>}
          <Button
            type="submit"
            disabled={busy}
            className="w-full bg-orange hover:bg-orange/90 text-black font-bold"
          >
            {busy ? '...' : mode === 'login' ? 'Sign in' : 'Register'}
          </Button>
          <p className="text-center text-xs text-muted">
            {mode === 'login' ? "Don't have an account? " : 'Already have an account? '}
            <button
              type="button"
              onClick={() => { setMode(mode === 'login' ? 'register' : 'login'); setError('') }}
              className="text-orange hover:underline font-medium"
            >
              {mode === 'login' ? 'Register' : 'Sign in'}
            </button>
          </p>
        </form>
      </DialogContent>
    </Dialog>
  )
}
