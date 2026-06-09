import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { subscribeProgress } from '../api'
import type { CrawlProgress } from '../types'
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from '../components/ui/card'

interface SourceStat {
  name: string
  label: string
  color: string
  count: number
}

export default function Status() {
  const [progress, setProgress] = useState<CrawlProgress | null>(null)
  const [sourceStats, setSourceStats] = useState<SourceStat[]>([])
  const [logs, setLogs] = useState<string[]>([])
  const navigate = useNavigate()

  useEffect(() => {
    const unsub = subscribeProgress(p => {
      setProgress(p)
      if (p.source_counts) {
        setSourceStats([
          { name: 'xnxx', label: 'XNXX', color: '#e50914', count: p.source_counts.xnxx || 0 },
          { name: 'xhamster', label: 'xHamster', color: '#f97316', count: p.source_counts.xhamster || 0 },
          { name: 'eporner', label: 'EPorner', color: '#3b82f6', count: p.source_counts.eporner || 0 },
          { name: 'tnaflix', label: 'TNAFlix', color: '#10b981', count: p.source_counts.tnaflix || 0 },
          { name: 'drtuber', label: 'DrTuber', color: '#8b5cf6', count: p.source_counts.drtuber || 0 },
        ])
      }
    })
    return unsub
  }, [])

  function triggerCrawl(source: string) {
    setLogs(l => [...l.slice(-4), `Starting ${source} crawl...`])
    fetch(`/api/crawl${source === 'xnxx' ? '' : source === 'xhamster' ? '-xh' : source === 'tnaflix' ? '-tf' : source === 'drtuber' ? '-dt' : '-ep'}`)
      .catch(() => setLogs(l => [...l.slice(-4), `${source} crawl trigger failed`]))
  }

  function triggerAll() {
    setLogs(l => [...l.slice(-4), 'Starting ALL crawls in parallel...'])
    fetch('/api/crawl').catch(() => {})
    fetch('/api/crawl-xh').catch(() => {})
    fetch('/api/crawl-ep').catch(() => {})
    fetch('/api/crawl-tf').catch(() => {})
    fetch('/api/crawl-dt').catch(() => {})
  }

  const isActive = !!(progress && progress.status !== 'idle')

  return (
    <div className="max-w-4xl mx-auto p-4 md:p-8 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-text">Scraping Status</h1>
        <button onClick={() => navigate(-1)} className="text-sm text-muted hover:text-text">
          Back
        </button>
      </div>

      {/* Live Status */}
      <Card className="bg-card border-border">
        <CardHeader>
          <CardTitle className="text-base flex items-center gap-2">
            <span className={`inline-block w-2.5 h-2.5 rounded-full ${isActive ? 'bg-orange animate-pulse' : 'bg-green'}`} />
            {isActive ? `${progress?.status || 'working'} — ${progress?.source || 'all'}` : 'Idle'}
            {progress?.total_count != null && (
              <span className="text-muted text-sm font-normal ml-2">{progress.total_count.toLocaleString()} total videos</span>
            )}
          </CardTitle>
        </CardHeader>
        {isActive && progress && (
          <CardContent className="space-y-3">
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 text-sm">
              <div>
                <div className="text-muted">Scanned</div>
                <div className="text-lg font-semibold text-text">{progress.scanned.toLocaleString()}</div>
              </div>
              <div>
                <div className="text-muted">New</div>
                <div className="text-lg font-semibold text-orange">{progress.new_videos.toLocaleString()}</div>
              </div>
              <div>
                <div className="text-muted">Detail Progress</div>
                <div className="text-lg font-semibold text-text">
                  {progress.detail_done}/{progress.detail_total}
                </div>
              </div>
              <div>
                <div className="text-muted">Page</div>
                <div className="text-lg font-semibold text-text">{progress.page}</div>
              </div>
            </div>
            <div className="bg-bg rounded-full h-2 overflow-hidden">
              <div
                className="h-full bg-orange transition-all duration-500"
                style={{ width: `${progress.detail_total > 0 ? (progress.detail_done / progress.detail_total) * 100 : 0}%` }}
              />
            </div>
          </CardContent>
        )}
      </Card>

      {/* Source Overview */}
      <Card className="bg-card border-border">
        <CardHeader>
          <CardTitle className="text-base">Sources</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-5 gap-4">
            {sourceStats.map(s => (
              <div key={s.name} className="text-center">
                <div className="text-2xl font-bold" style={{ color: s.color }}>{s.label}</div>
                <div className="text-sm text-muted mt-1">{s.count.toLocaleString()} recent</div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>

      {/* Crawl Controls */}
      <Card className="bg-card border-border">
        <CardHeader>
          <CardTitle className="text-base">Controls</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex flex-wrap gap-3">
            <button
              onClick={triggerAll}
              disabled={isActive}
              className="px-4 py-2 rounded-full bg-red text-white font-semibold text-sm
                         hover:bg-red/90 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              Crawl All in Parallel
            </button>
            <button
              onClick={() => triggerCrawl('xnxx')}
              disabled={isActive}
              className="px-4 py-2 rounded-full text-sm font-semibold border border-border
                         text-text hover:bg-card-hover disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              XNXX
            </button>
            <button
              onClick={() => triggerCrawl('xhamster')}
              disabled={isActive}
              className="px-4 py-2 rounded-full text-sm font-semibold border border-border
                         text-text hover:bg-card-hover disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              xHamster
            </button>
            <button
              onClick={() => triggerCrawl('eporner')}
              disabled={isActive}
              className="px-4 py-2 rounded-full text-sm font-semibold border border-border
                         text-text hover:bg-card-hover disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              EPorner
            </button>
            <button
              onClick={() => triggerCrawl('tnaflix')}
              disabled={isActive}
              className="px-4 py-2 rounded-full text-sm font-semibold border border-border
                         text-text hover:bg-card-hover disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              TNAFlix
            </button>
            <button
              onClick={() => triggerCrawl('drtuber')}
              disabled={isActive}
              className="px-4 py-2 rounded-full text-sm font-semibold border border-border
                         text-text hover:bg-card-hover disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              DrTuber
            </button>
          </div>
        </CardContent>
      </Card>

      {/* Log */}
      {logs.length > 0 && (
        <Card className="bg-card border-border">
          <CardHeader>
            <CardTitle className="text-base">Activity Log</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-1 font-mono text-xs text-muted">
              {logs.map((l, i) => (
                <div key={i}>{l}</div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
