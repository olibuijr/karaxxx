export interface Video {
  id: string
  slug: string
  title: string
  description: string
  categories: string[]
  tags: string[]
  uploader: string
  upload_date: string
  duration: number
  views: number
  added_at: string
  source: string
  thumb_uuid: string
  url_360: string
  url_720: string
  url_1080: string
  preview_url: string
  hls_url: string
  watch_count?: number
  watched_position?: number
}

export interface BrowseParams {
  page?: number
  sort?: 'recent' | 'new' | 'views' | 'duration' | 'trending'
  cat?: string
  q?: string
  uploader?: string
  source?: string
}

export interface TagCount {
  name: string
  count: number
}

export interface BrowseResponse {
  videos: Video[]
  count: number
  page: number
  total_pages: number
}

export interface SearchSuggestResponse {
  categories: { name: string; count: number }[]
  videos: Video[]
}

export type FavoriteSort = 'recent' | 'views' | 'duration' | 'title'

export interface CrawlProgress {
  status: string
  source: string
  scanned: number
  new_videos: number
  cached: number
  detail_done: number
  detail_total: number
  page: number
  total_count: number
  source_counts: Record<string, number>
}

export interface SocialComment {
  id: number
  display_name: string
  body: string
  anonymous: boolean
  created_at: string
}

export interface VideoSocial {
  comments: SocialComment[]
  reactions: Record<string, number>
  user_reactions: string[]
  watch_count: number
}

export interface ProfileSettings {
  username: string
  display_name: string
  anonymous_name: string
  comment_anonymously: boolean
  public_commenter_name: string
}

export interface ChangelogInfo {
  version: string
  updated_at: string
  markdown: string
}

export const SOURCES: { label: string; value: string }[] = [
  { label: 'All', value: '' },
  { label: 'xVideos', value: 'xvideos' },
  { label: 'XNXX', value: 'xnxx' },
  { label: 'xHamster', value: 'xhamster' },
  { label: 'EPorner', value: 'eporner' },
  { label: 'TNAFlix', value: 'tnaflix' },
  { label: 'DrTuber', value: 'drtuber' },
  { label: 'HeavyFetish', value: 'heavyfetish' },
  { label: 'PunishBang', value: 'punishbang' },
  { label: 'SunPorno BDSM', value: 'sunporno' },
]
