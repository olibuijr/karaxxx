import { createContext, useContext, useState, useEffect, type ReactNode, useCallback } from 'react'

interface User {
  id: number
  username: string
}

interface AuthResult {
  ok: boolean
  error?: string
}

interface AuthPayload {
  token?: string
  user?: User
}

interface AuthContextType {
  user: User | null
  token: string | null
  login: (username: string, password: string) => Promise<AuthResult>
  register: (username: string, password: string, inviteKey: string) => Promise<AuthResult>
  logout: () => void
  loading: boolean
}

const AuthContext = createContext<AuthContextType | null>(null)

const USER_CACHE_KEY = 'kxxx_user'

/** Run a state swap inside a View Transition when the browser supports it. */
export function withViewTransition(update: () => void) {
  const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  if (!reduced && typeof document.startViewTransition === 'function') {
    document.startViewTransition(update)
  } else {
    update()
  }
}

function readCachedUser(): User | null {
  try {
    const raw = localStorage.getItem(USER_CACHE_KEY)
    if (!raw) return null
    const u = JSON.parse(raw) as User
    return u && typeof u.username === 'string' ? u : null
  } catch {
    return null
  }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  // Optimistic session restore: trust the cached user immediately, revalidate in background.
  const [user, setUser] = useState<User | null>(readCachedUser)
  const [token, setToken] = useState<string | null>(null)
  const [loading, setLoading] = useState(() => readCachedUser() === null)

  useEffect(() => {
    let cancelled = false

    void (async () => {
      try {
        const headers = token ? { Authorization: `Bearer ${token}` } : undefined
        const response = await fetch('/api/auth/me', { headers })
        if (!response.ok) {
          localStorage.removeItem(USER_CACHE_KEY)
          setToken(null)
          if (!cancelled) withViewTransition(() => setUser(null))
          return
        }

        const data = await response.json() as User
        if (cancelled) return

        if (data.username) {
          setUser(data)
          localStorage.setItem(USER_CACHE_KEY, JSON.stringify(data))
          return
        }

        localStorage.removeItem(USER_CACHE_KEY)
        setToken(null)
        withViewTransition(() => setUser(null))
      } catch {
        // Network error: keep the optimistic session; the API layer will surface real failures.
      } finally {
        if (!cancelled) {
          setLoading(false)
        }
      }
    })()

    return () => {
      cancelled = true
    }
  }, [])

  const authRequest = useCallback(async (path: string, body: object) => {
    const res = await fetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    if (!res.ok) return { ok: false, error: 'Authentication failed' }
    const data = await res.json() as AuthPayload
    const { token: authToken, user: authUser } = data
    if (!authToken || !authUser) return { ok: false, error: 'Authentication failed' }
    localStorage.setItem(USER_CACHE_KEY, JSON.stringify(authUser))
    setToken(authToken)
    // Melt the login screen into the app shell in one continuous transition.
    withViewTransition(() => setUser(authUser))
    return { ok: true }
  }, [])

  const login = (username: string, password: string) =>
    authRequest('/api/auth/login', { username, password })

  const register = (username: string, password: string, inviteKey: string) =>
    authRequest('/api/auth/register', { username, password, invite_key: inviteKey })

  const logout = () => {
    fetch('/api/auth/logout', { method: 'POST' }).catch(() => {})
    localStorage.removeItem(USER_CACHE_KEY)
    setToken(null)
    withViewTransition(() => setUser(null))
  }

  return (
    <AuthContext.Provider value={{ user, token, login, register, logout, loading }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}
