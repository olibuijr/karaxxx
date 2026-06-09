import { createContext, useContext, useState, useEffect, type ReactNode, useCallback } from 'react'

interface User {
  id: number
  username: string
}

interface AuthContextType {
  user: User | null
  token: string | null
  login: (username: string, password: string) => Promise<boolean>
  register: (username: string, password: string) => Promise<boolean>
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
    if (t) {
      fetch('/api/auth/me', { headers: { Authorization: `Bearer ${t}` } })
        .then(r => r.ok ? r.json() : null)
        .then(u => {
          if (u?.username) {
            setUser(u)
            setToken(t)
          } else {
            localStorage.removeItem('kxxx_token')
          }
        })
        .finally(() => setLoading(false))
    } else {
      setLoading(false)
    }
  }, [])

  const authRequest = useCallback(async (path: string, body: object) => {
    const res = await fetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    if (!res.ok) return false
    const data = await res.json()
    localStorage.setItem('kxxx_token', data.token)
    setToken(data.token)
    setUser(data.user)
    return true
  }, [])

  const login = (username: string, password: string) =>
    authRequest('/api/auth/login', { username, password })

  const register = (username: string, password: string) =>
    authRequest('/api/auth/register', { username, password })

  const logout = () => {
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
