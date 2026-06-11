import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { TooltipProvider } from './components/ui/tooltip'
import { AuthProvider, useAuth } from './lib/auth'
import Header from './components/Header'
import Sidebar from './components/Sidebar'
import Browse from './pages/Browse'
import Play from './pages/Play'
import Favorites from './pages/Favorites'
import Playlists from './pages/Playlists'
import Profile from './pages/Profile'
import Status from './pages/Status'
import Setup from './pages/Setup'
import Wall from './pages/Wall'
import Changelog from './pages/Changelog'
import { useEffect, useState } from 'react'
import { subscribeProgress } from './api'

function AppLayout() {
  const { user, loading } = useAuth()
  const [count, setCount] = useState<number>()
  const [progressMsg, setProgressMsg] = useState('')
  const [sidebarOpen, setSidebarOpen] = useState(false)

  useEffect(() => {
    if (!user) return
    const unsub = subscribeProgress(p => {
      if (p.total_count) setCount(p.total_count)
      if (p.status !== 'idle') {
        const msg = p.status === 'searching'
          ? `scanning page ${p.page} (${p.scanned} found)`
          : `scraping ${p.detail_done}/${p.detail_total}`
        setProgressMsg(msg)
      } else {
        setProgressMsg('')
      }
    })
    return unsub
  }, [user])

  if (loading) {
    return <div className="min-h-screen bg-bg text-muted flex items-center justify-center">Loading...</div>
  }

  if (!user) {
    return <Setup />
  }

  return (
    <div className="min-h-screen bg-bg">
      <Header videoCount={count} progress={progressMsg} onMenuToggle={() => setSidebarOpen(v => !v)} />
      <div className="flex">
        <Sidebar open={sidebarOpen} onClose={() => setSidebarOpen(false)} />
        <div className="flex-1 min-w-0">
          <Routes>
            <Route path="/" element={<Browse />} />
            <Route path="/search" element={<Browse />} />
            <Route path="/play/:id" element={<Play />} />
            <Route path="/favorites" element={<Favorites />} />
            <Route path="/playlists" element={<Playlists />} />
            <Route path="/profile" element={<Profile />} />
            <Route path="/wall/:username" element={<Wall />} />
            <Route path="/status" element={<Status />} />
            <Route path="/changelog" element={<Changelog />} />
          </Routes>
        </div>
      </div>
    </div>
  )
}

export default function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <TooltipProvider>
          <AppLayout />
        </TooltipProvider>
      </AuthProvider>
    </BrowserRouter>
  )
}
