import { createContext, useContext, useState, useEffect, type ReactNode, useCallback } from 'react'

interface User {
  id: number
  username: string
}

interface AuthResult {
  ok: boolean
  error?: string
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

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null)
  const [token, setToken] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const t = localStorage.getItem('kxxx_token')
    const headers = t ? { Authorization: `Bearer ${t}` } : undefined
    fetch('/api/auth/me', { headers })
      .then(r => r.ok ? r.json() : null)
      .then(u => {
        if (u?.username) {
          setUser(u)
          if (t) setToken(t)
        } else {
          localStorage.removeItem('kxxx_token')
        }
      })
      .finally(() => setLoading(false))
  }, [])

  const authRequest = useCallback(async (path: string, body: object) => {
    const res = await fetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    const data = await res.json().catch(() => ({}))
    if (!res.ok) return { ok: false, error: data?.error || 'Authentication failed' }
    localStorage.setItem('kxxx_token', data.token)
    setToken(data.token)
    setUser(data.user)
    return { ok: true }
  }, [])

  const login = (username: string, password: string) =>
    authRequest('/api/auth/login', { username, password })

  const register = (username: string, password: string, inviteKey: string) =>
    authRequest('/api/auth/register', { username, password, invite_key: inviteKey })

  const logout = () => {
    fetch('/api/auth/logout', { method: 'POST' }).catch(() => {})
    localStorage.removeItem('kxxx_token')
    setUser(null)
    setToken(null)
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
