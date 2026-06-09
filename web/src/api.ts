import type { BrowseParams, BrowseResponse, CrawlProgress, Video } from './types'

const BASE = '/api'

export async function fetchBrowse(params: BrowseParams): Promise<BrowseResponse> {
  const sp = new URLSearchParams()
  if (params.page) sp.set('page', String(params.page))
  if (params.sort && params.sort !== 'recent') sp.set('sort', params.sort)
  if (params.cat) sp.set('cat', params.cat)
  if (params.q) sp.set('q', params.q)
  if (params.uploader) sp.set('uploader', params.uploader)
  if (params.source) sp.set('source', params.source)

  const qs = sp.toString()
  const res = await fetch(`${BASE}/browse${qs ? '?' + qs : ''}`)
  if (!res.ok) throw new Error(`Browse failed: ${res.status}`)
  return res.json()
}

export async function fetchVideo(id: string): Promise<Video> {
  const res = await fetch(`${BASE}/video/${id}`)
  if (!res.ok) throw new Error(`Video ${id} not found`)
  return res.json()
}

export async function fetchCategories(): Promise<string[]> {
  const res = await fetch(`${BASE}/categories`)
  return res.json()
}

export async function triggerCrawl(): Promise<void> {
  await fetch(`${BASE}/crawl`)
}

export async function refreshVideo(id: string): Promise<Video> {
  const res = await fetch(`${BASE}/refresh?id=${encodeURIComponent(id)}`)
  if (!res.ok) throw new Error(`Refresh failed: ${res.status}`)
  return fetchVideo(id)
}

export function subscribeProgress(onProgress: (p: CrawlProgress) => void): () => void {
  const evtSource = new EventSource(`${BASE}/status`)
  evtSource.onmessage = (e) => {
    try {
      onProgress(JSON.parse(e.data))
    } catch { /* ignore malformed */ }
  }
  return () => evtSource.close()
}

export function formatDuration(secs: number): string {
  const m = Math.floor(secs / 60)
  const s = secs % 60
  return `${m}:${s.toString().padStart(2, '0')}`
}

export function formatViews(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`
  return String(n)
}

export function timeAgo(dateStr: string): string {
  if (!dateStr) return ''
  const d = new Date(dateStr)
  if (isNaN(d.getTime())) return ''
  const now = new Date()
  const diffMs = now.getTime() - d.getTime()
  const diffDays = Math.floor(diffMs / 86400000)
  if (diffDays === 0) return 'Today'
  if (diffDays === 1) return 'Yesterday'
  if (diffDays < 7) return `${diffDays}d ago`
  if (diffDays < 30) return `${Math.floor(diffDays / 7)}w ago`
  if (diffDays < 365) return `${Math.floor(diffDays / 30)}mo ago`
  return `${Math.floor(diffDays / 365)}y ago`
}

export async function fetchRelated(id: string): Promise<Video[]> {
  const res = await fetch(`${BASE}/related/${id}?limit=12`)
  if (!res.ok) return []
  return res.json()
}

export async function fetchRandom(source?: string, cat?: string): Promise<string> {
  const sp = new URLSearchParams()
  if (source) sp.set('source', source)
  if (cat) sp.set('cat', cat)
  const qs = sp.toString()
  const res = await fetch(`${BASE}/random${qs ? '?' + qs : ''}`)
  const data = await res.json()
  return data.id
}

export async function fetchTags(limit: number = 50): Promise<{name: string, count: number}[]> {
  const res = await fetch(`${BASE}/tags?limit=${limit}`)
  if (!res.ok) return []
  return res.json()
}

export async function fetchPlaylists(token: string): Promise<any[]> {
  const res = await fetch(`${BASE}/playlists`, { headers: { Authorization: `Bearer ${token}` } })
  return res.json()
}

export async function createPlaylist(token: string, name: string): Promise<number> {
  const res = await fetch(`${BASE}/playlists`, { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` }, body: JSON.stringify({ name }) })
  const data = await res.json()
  return data.id
}

export async function addToPlaylist(token: string, playlistId: number, videoId: string): Promise<void> {
  await fetch(`${BASE}/playlists/${playlistId}/videos`, { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` }, body: JSON.stringify({ video_id: videoId }) })
}

export async function fetchProfile(token: string): Promise<any> {
  const res = await fetch(`${BASE}/profile`, { headers: { Authorization: `Bearer ${token}` } })
  return res.json()
}

export async function fetchWatchHistory(token: string, limit: number = 8): Promise<Video[]> {
  const res = await fetch(`${BASE}/watch/history?limit=${limit}`, {
    headers: { Authorization: `Bearer ${token}` }
  })
  if (!res.ok) return []
  return res.json()
}

export async function removeWatchHistory(token: string, videoId: string): Promise<void> {
  await fetch(`${BASE}/watch/history/${videoId}`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${token}` }
  })
}
